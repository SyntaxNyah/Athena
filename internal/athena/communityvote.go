/* Athena - A server for Attorney Online 2 written in Go
Copyright (C) 2022 MangosArentLiterature <mango@transmenace.dev>

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published
by the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>. */

package athena

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/MangosArentLiterature/Athena/internal/db"
	"github.com/MangosArentLiterature/Athena/internal/logger"
	"github.com/MangosArentLiterature/Athena/internal/packet"
	"github.com/MangosArentLiterature/Athena/internal/permissions"
	"github.com/MangosArentLiterature/Athena/internal/settings"
	"github.com/MangosArentLiterature/Athena/internal/webhook"
	str2duration "github.com/xhit/go-str2duration/v2"
)

// ── Types ────────────────────────────────────────────────────────────────────

// cvoteActionType represents the moderation action requested in a community vote.
type cvoteActionType int

const (
	cvoteActionKick cvoteActionType = iota
	cvoteActionMute
	cvoteActionBan
	cvoteActionWarn
	cvoteActionAreaKick
)

func (a cvoteActionType) String() string {
	switch a {
	case cvoteActionKick:
		return "kick"
	case cvoteActionMute:
		return "mute"
	case cvoteActionBan:
		return "ban"
	case cvoteActionWarn:
		return "warn"
	case cvoteActionAreaKick:
		return "areakick"
	}
	return "unknown"
}

func parseCvoteAction(s string) (cvoteActionType, bool) {
	switch strings.ToLower(s) {
	case "kick":
		return cvoteActionKick, true
	case "mute":
		return cvoteActionMute, true
	case "ban":
		return cvoteActionBan, true
	case "warn":
		return cvoteActionWarn, true
	case "areakick":
		return cvoteActionAreaKick, true
	}
	return 0, false
}

// cvoteEntry holds state for a single active community vote.
type cvoteEntry struct {
	action     cvoteActionType
	targetUID  int
	targetIPID string
	targetName string
	reason     string
	voters     map[string]struct{} // set of voter IPIDs (prevents duplicate votes)
	threshold  int
	pending    bool // true when threshold reached, awaiting mod response
	timer      *time.Timer
	startedBy  string // initiator OOC name
}

// cvoteStore is the global, mutex-protected community vote registry.
type cvoteStore struct {
	mu     sync.Mutex
	active map[int]*cvoteEntry // targetUID → vote entry
}

var communityVotes = &cvoteStore{active: make(map[int]*cvoteEntry)}

// ── Helpers ──────────────────────────────────────────────────────────────────

// cvoteAllowedActions is the set of actions enabled in config, built once at
// server init by initCvote. Lookups are O(1) with no allocations.
var cvoteAllowedActions map[cvoteActionType]struct{}

// initCvote pre-builds the cvoteAllowedActions set from the server config so
// that every /cvote command uses a cheap map lookup instead of a linear scan.
func initCvote(conf *settings.Config) {
	m := make(map[cvoteActionType]struct{}, len(conf.VoteActions))
	for _, s := range conf.VoteActions {
		if a, ok := parseCvoteAction(s); ok {
			m[a] = struct{}{}
		}
	}
	cvoteAllowedActions = m
}

// notifyModsCvoteThreshold sends an alert to every connected moderator when a
// community vote has collected enough votes and is now awaiting mod approval.
// The notification string is formatted once and shared across all recipients.
func notifyModsCvoteThreshold(action cvoteActionType, targetName string, targetUID int) {
	msg := fmt.Sprintf(
		"[MOD ALERT] Community vote to %s %s (UID %d) has PASSED the threshold! "+
			"Run /cvote accept %d to enforce, or /cvote reject %d to deny.",
		action, targetName, targetUID, targetUID, targetUID,
	)
	clients.ForEach(func(c *Client) {
		if c.Authenticated() && permissions.IsModerator(c.Perms()) {
			c.SendServerMessage(msg)
		}
	})
}

// ── Command entry point ──────────────────────────────────────────────────────

// cmdCvote is the entry point for the /cvote command.
//
// Sub-commands:
//
//	/cvote <action> <uid> [reason]   — start or join a vote
//	/cvote accept <uid>              — mod: accept and enforce a passed vote
//	/cvote reject <uid>              — mod: reject a passed vote
//	/cvote cancel <uid>              — mod: cancel any active vote
//	/cvote list                      — list all active votes
func cmdCvote(client *Client, args []string, usage string) {
	if len(args) == 0 {
		cvoteList(client)
		return
	}

	switch strings.ToLower(args[0]) {
	case "accept":
		cvoteAccept(client, args[1:])
	case "reject":
		cvoteReject(client, args[1:])
	case "cancel":
		cvoteCancel(client, args[1:])
	case "list":
		cvoteList(client)
	default:
		cvoteStart(client, args)
	}
}

// ── Starting / joining a vote ─────────────────────────────────────────────────

// cvoteStart creates a new community vote or adds the caller's vote to an existing one.
func cvoteStart(client *Client, args []string) {
	if !config.EnableCommunityVote {
		client.SendServerMessage("Community voting is not enabled on this server.")
		return
	}

	if len(args) < 2 {
		client.SendServerMessage("Usage: /cvote <kick|mute|ban|warn|areakick> <uid> [reason]")
		return
	}

	// Moderators use /cvote accept / reject, not /cvote to cast votes.
	if client.Authenticated() && permissions.IsModerator(client.Perms()) {
		client.SendServerMessage("Moderators cannot participate in community votes. " +
			"Use /cvote accept or /cvote reject instead.")
		return
	}

	action, ok := parseCvoteAction(args[0])
	if !ok {
		client.SendServerMessage(fmt.Sprintf(
			"Unknown vote action '%s'. Valid actions: kick, mute, ban, warn, areakick.", args[0]))
		return
	}

	if _, allowed := cvoteAllowedActions[action]; !allowed {
		client.SendServerMessage(fmt.Sprintf(
			"Voting for '%s' is not enabled on this server.", action))
		return
	}

	targetUID, err := strconv.Atoi(args[1])
	if err != nil || targetUID < 0 {
		client.SendServerMessage("Invalid UID.")
		return
	}

	// Prevent self-votes.
	if targetUID == client.Uid() {
		client.SendServerMessage("You cannot vote against yourself.")
		return
	}

	target, err2 := getClientByUid(targetUID)
	if err2 != nil {
		client.SendServerMessage("Player not found.")
		return
	}

	reason := "No reason given"
	if len(args) > 2 {
		reason = strings.Join(args[2:], " ")
	}

	threshold := config.VoteThreshold
	if threshold <= 0 {
		threshold = 3
	}

	// Compute the vote duration once; reused for both initial and mod-response timers.
	voteDur := time.Duration(config.VoteDuration) * time.Second
	if voteDur <= 0 {
		voteDur = 120 * time.Second
	}

	voterIPID := client.Ipid()
	targetIPID := target.Ipid()
	targetName := clientDisplayName(target)

	communityVotes.mu.Lock()

	existing, exists := communityVotes.active[targetUID]
	if exists {
		// A vote is already in progress for this target — try to join it.

		if existing.pending {
			communityVotes.mu.Unlock()
			client.SendServerMessage(fmt.Sprintf(
				"A community vote against %s (UID %d) has already passed and is awaiting a moderator decision.",
				targetName, targetUID,
			))
			return
		}

		if existing.action != action {
			communityVotes.mu.Unlock()
			client.SendServerMessage(fmt.Sprintf(
				"There is already an active community vote to %s %s (UID %d). "+
					"Type /cvote %s %d to add your vote.",
				existing.action, targetName, targetUID, existing.action, targetUID,
			))
			return
		}

		if _, already := existing.voters[voterIPID]; already {
			communityVotes.mu.Unlock()
			client.SendServerMessage("You have already voted in this community vote.")
			return
		}

		existing.voters[voterIPID] = struct{}{}
		count := len(existing.voters)
		reachedThreshold := count >= existing.threshold

		if reachedThreshold && !existing.pending {
			existing.pending = true
			// Replace the expiry timer with a mod-response timer (same duration).
			if existing.timer != nil {
				existing.timer.Stop()
			}
			captureUID := targetUID
			captureAction := existing.action
			captureName := targetName
			existing.timer = time.AfterFunc(voteDur, func() {
				communityVotes.mu.Lock()
				e, ok := communityVotes.active[captureUID]
				if !ok || !e.pending {
					communityVotes.mu.Unlock()
					return
				}
				delete(communityVotes.active, captureUID)
				communityVotes.mu.Unlock()
				sendGlobalServerMessage(fmt.Sprintf(
					"⚖️ Community vote to %s %s (UID %d) expired with no moderator response.",
					captureAction, captureName, captureUID,
				))
			})
		}

		communityVotes.mu.Unlock()

		sendGlobalServerMessage(fmt.Sprintf(
			"⚖️ %s voted to %s %s (UID %d). [%d/%d]",
			client.OOCName(), action, targetName, targetUID, count, threshold,
		))

		if reachedThreshold {
			sendGlobalServerMessage(fmt.Sprintf(
				"⚖️ Community vote to %s %s (UID %d) has reached the required votes! "+
					"Waiting for a moderator to accept or reject.",
				action, targetName, targetUID,
			))
			notifyModsCvoteThreshold(action, targetName, targetUID)
		}
		return
	}

	// No existing vote — create a fresh one.
	entry := &cvoteEntry{
		action:     action,
		targetUID:  targetUID,
		targetIPID: targetIPID,
		targetName: targetName,
		reason:     reason,
		voters:     map[string]struct{}{voterIPID: {}},
		threshold:  threshold,
		startedBy:  client.OOCName(),
	}

	captureUID := targetUID
	captureAction := action
	captureName := targetName
	captureThreshold := threshold

	entry.timer = time.AfterFunc(voteDur, func() {
		communityVotes.mu.Lock()
		e, ok := communityVotes.active[captureUID]
		if !ok || e.pending {
			// Already handled (reached threshold or accepted/rejected).
			communityVotes.mu.Unlock()
			return
		}
		delete(communityVotes.active, captureUID)
		communityVotes.mu.Unlock()
		sendGlobalServerMessage(fmt.Sprintf(
			"⚖️ Community vote to %s %s (UID %d) expired without reaching the required %d votes.",
			captureAction, captureName, captureUID, captureThreshold,
		))
	})

	communityVotes.active[targetUID] = entry
	communityVotes.mu.Unlock()

	sendGlobalServerMessage(fmt.Sprintf(
		"⚖️ %s started a community vote to %s %s (UID %d)! Reason: %s. "+
			"Type /cvote %s %d to add your vote. [1/%d]",
		client.OOCName(), action, targetName, targetUID, reason,
		action, targetUID, threshold,
	))
	addToBuffer(client, "CMD",
		fmt.Sprintf("Started community vote to %s UID %d (%s): %s", action, targetUID, targetName, reason),
		false)
}

// ── Moderator actions ─────────────────────────────────────────────────────────

// cvoteAccept allows a moderator with the appropriate permission to enforce a
// passed community vote.
func cvoteAccept(client *Client, args []string) {
	if !config.EnableCommunityVote {
		client.SendServerMessage("Community voting is not enabled on this server.")
		return
	}

	if !client.Authenticated() || (!permissions.IsModerator(client.Perms()) && !clientHasCommandGrant(client, "cvote")) {
		client.SendServerMessage("Only moderators can accept community votes.")
		return
	}

	if len(args) < 1 {
		client.SendServerMessage("Usage: /cvote accept <uid>")
		return
	}

	targetUID, err := strconv.Atoi(args[0])
	if err != nil {
		client.SendServerMessage("Invalid UID.")
		return
	}

	communityVotes.mu.Lock()
	entry, ok := communityVotes.active[targetUID]
	if !ok {
		communityVotes.mu.Unlock()
		client.SendServerMessage("No active community vote found for that UID.")
		return
	}

	if !entry.pending {
		count := len(entry.voters)
		communityVotes.mu.Unlock()
		client.SendServerMessage(fmt.Sprintf(
			"Community vote against UID %d has not yet reached the threshold (%d/%d votes). "+
				"It cannot be accepted until it passes.",
			targetUID, count, entry.threshold,
		))
		return
	}

	// Verify the moderator has the required permission for the requested action.
	var requiredPerm uint64
	switch entry.action {
	case cvoteActionKick:
		requiredPerm = permissions.PermissionField["KICK"]
	case cvoteActionMute:
		requiredPerm = permissions.PermissionField["MUTE"]
	case cvoteActionBan:
		requiredPerm = permissions.PermissionField["BAN"]
	case cvoteActionWarn:
		requiredPerm = permissions.PermissionField["KICK"]
	case cvoteActionAreaKick:
		requiredPerm = permissions.PermissionField["KICK"]
	}
	if !permissions.HasPermission(client.Perms(), requiredPerm) && !clientHasCommandGrant(client, "cvote") {
		communityVotes.mu.Unlock()
		client.SendServerMessage(fmt.Sprintf(
			"You do not have permission to %s players.", entry.action))
		return
	}

	// Snapshot needed fields and remove the vote before releasing the lock.
	action := entry.action
	targetIPID := entry.targetIPID
	targetName := entry.targetName
	reason := entry.reason
	if entry.timer != nil {
		entry.timer.Stop()
	}
	delete(communityVotes.active, targetUID)
	communityVotes.mu.Unlock()

	modName := client.DisplayModName()
	storedModName := client.StoredModName()
	communityReason := fmt.Sprintf("[Community Vote] %s (accepted by %s)", reason, modName)

	target, targetErr := getClientByUid(targetUID)

	switch action {
	case cvoteActionKick:
		if targetErr != nil {
			client.SendServerMessage(fmt.Sprintf(
				"Player (UID %d) is no longer connected.", targetUID))
		} else {
			target.SendSync(&packet.KK{Reason: communityReason})
			target.conn.Close()
			sendPlayerArup()
			if err := webhook.PostKick(target.CurrentCharacter(), target.Showname(), target.OOCName(),
				target.Ipid(), communityReason, modName, target.Uid()); err != nil {
				logger.LogErrorf("while posting community vote kick webhook: %v", err)
			}
			sendGlobalServerMessage(fmt.Sprintf(
				"⚖️ Community vote accepted: %s (UID %d) has been kicked. [%s]",
				targetName, targetUID, reason,
			))
			addToBuffer(client, "MOD",
				fmt.Sprintf("Community vote KICK accepted for UID %d (%s): %s",
					targetUID, targetName, reason),
				true)
		}

	case cvoteActionMute:
		if targetErr != nil {
			client.SendServerMessage(fmt.Sprintf(
				"Player (UID %d) is no longer connected.", targetUID))
		} else {
			muteDur := config.VoteMuteDuration
			if muteDur <= 0 {
				muteDur = 300
			}
			muteExpiry := time.Now().UTC().Add(time.Duration(muteDur) * time.Second)
			target.SetMuted(ICOOCMuted)
			target.SetUnmuteTime(muteExpiry)
			if err2 := db.UpsertMute(target.Ipid(), int(ICOOCMuted), muteExpiry.Unix()); err2 != nil {
				logger.LogErrorf("Failed to persist community vote mute for %v: %v", target.Ipid(), err2)
			}
			target.SendServerMessage(fmt.Sprintf(
				"You have been muted by community vote for %d seconds. Reason: %s",
				muteDur, reason,
			))
			sendGlobalServerMessage(fmt.Sprintf(
				"⚖️ Community vote accepted: %s (UID %d) has been muted for %d seconds. [%s]",
				targetName, targetUID, muteDur, reason,
			))
			addToBuffer(client, "MOD",
				fmt.Sprintf("Community vote MUTE accepted for UID %d (%s): %s",
					targetUID, targetName, reason),
				true)
		}

	case cvoteActionBan:
		banTime := time.Now().UTC().Unix()
		dur, parseErr := str2duration.ParseDuration(config.BanLen)
		if parseErr != nil {
			dur = 24 * time.Hour
		}
		until := time.Now().UTC().Add(dur).Unix()
		untilS := time.Unix(until, 0).UTC().Format("02 Jan 2006 15:04 MST")

		if targetErr == nil {
			// Target is online — ban and disconnect.
			id, addErr := db.AddBan(target.Ipid(), target.Hdid(), banTime, until, communityReason, storedModName)
			if addErr != nil {
				logger.LogErrorf("Failed to add community vote ban for %v: %v", target.Ipid(), addErr)
				client.SendServerMessage("Failed to record ban in the database.")
				break
			}
			target.SendSync(&packet.KB{Reason: fmt.Sprintf("%s\nUntil: %s\nID: %d",
				communityReason, untilS, id)})
			target.conn.Close()
			forgetIP(target.Ipid())
			sendPlayerArup()
			if err := webhook.PostBan(target.CurrentCharacter(), target.Showname(), target.OOCName(),
				target.Ipid(), target.Uid(), id, config.BanLen, communityReason, modName); err != nil {
				logger.LogErrorf("while posting community vote ban webhook: %v", err)
			}
		} else if targetIPID != "" {
			// Target disconnected — ban by IPID only.
			id, banErr := db.AddBan(targetIPID, "", banTime, until, communityReason, storedModName)
			if banErr != nil {
				logger.LogErrorf("Failed to record community vote ban for IPID %v: %v", targetIPID, banErr)
			} else {
				forgetIP(targetIPID)
				if err := webhook.PostBan("N/A", "N/A", "N/A",
					targetIPID, -1, id, config.BanLen, communityReason, modName); err != nil {
					logger.LogErrorf("while posting community vote ban webhook: %v", err)
				}
			}
			client.SendServerMessage(fmt.Sprintf(
				"Player (UID %d) is no longer connected, but their IP has been banned until %s.",
				targetUID, untilS))
		} else {
			client.SendServerMessage(fmt.Sprintf(
				"Player (UID %d) is no longer connected and their IP is unknown.", targetUID))
		}
		sendGlobalServerMessage(fmt.Sprintf(
			"⚖️ Community vote accepted: %s (UID %d) has been banned until %s. [%s]",
			targetName, targetUID, untilS, reason,
		))
		addToBuffer(client, "MOD",
			fmt.Sprintf("Community vote BAN accepted for UID %d (%s): %s",
				targetUID, targetName, reason),
			true)

	case cvoteActionWarn:
		if targetErr != nil {
			client.SendServerMessage(fmt.Sprintf(
				"Player (UID %d) is no longer connected.", targetUID))
		} else {
			target.SendServerMessage(fmt.Sprintf(
				"⚠️ You have received a formal warning by community vote. Reason: %s", reason,
			))
			sendGlobalServerMessage(fmt.Sprintf(
				"⚖️ Community vote accepted: %s (UID %d) has been formally warned. [%s]",
				targetName, targetUID, reason,
			))
			addToBuffer(client, "MOD",
				fmt.Sprintf("Community vote WARN accepted for UID %d (%s): %s",
					targetUID, targetName, reason),
				true)
		}

	case cvoteActionAreaKick:
		if targetErr != nil {
			client.SendServerMessage(fmt.Sprintf(
				"Player (UID %d) is no longer connected.", targetUID))
		} else {
			target.SendServerMessage(fmt.Sprintf(
				"You have been moved to the default area by community vote. Reason: %s", reason,
			))
			target.ChangeArea(areas[0])
			sendGlobalServerMessage(fmt.Sprintf(
				"⚖️ Community vote accepted: %s (UID %d) has been moved to the default area. [%s]",
				targetName, targetUID, reason,
			))
			addToBuffer(client, "MOD",
				fmt.Sprintf("Community vote AREAKICK accepted for UID %d (%s): %s",
					targetUID, targetName, reason),
				true)
		}
	}
}

// cvoteReject allows a moderator to reject a passed community vote.
func cvoteReject(client *Client, args []string) {
	if !config.EnableCommunityVote {
		client.SendServerMessage("Community voting is not enabled on this server.")
		return
	}

	if !client.Authenticated() || (!permissions.IsModerator(client.Perms()) && !clientHasCommandGrant(client, "cvote")) {
		client.SendServerMessage("Only moderators can reject community votes.")
		return
	}

	if len(args) < 1 {
		client.SendServerMessage("Usage: /cvote reject <uid>")
		return
	}

	targetUID, err := strconv.Atoi(args[0])
	if err != nil {
		client.SendServerMessage("Invalid UID.")
		return
	}

	communityVotes.mu.Lock()
	entry, ok := communityVotes.active[targetUID]
	if !ok {
		communityVotes.mu.Unlock()
		client.SendServerMessage("No active community vote found for that UID.")
		return
	}

	if !entry.pending {
		communityVotes.mu.Unlock()
		client.SendServerMessage("This vote has not yet reached the threshold. " +
			"Use /cvote cancel to cancel it outright.")
		return
	}

	action := entry.action
	targetName := entry.targetName
	if entry.timer != nil {
		entry.timer.Stop()
	}
	delete(communityVotes.active, targetUID)
	communityVotes.mu.Unlock()

	modName := client.DisplayModName()
	addToBuffer(client, "MOD",
		fmt.Sprintf("Community vote %s REJECTED for UID %d (%s)", action, targetUID, targetName),
		true)
	sendGlobalServerMessage(fmt.Sprintf(
		"⚖️ Community vote to %s %s (UID %d) was rejected by moderator %s.",
		action, targetName, targetUID, modName,
	))
}

// cvoteCancel allows a moderator to cancel any active community vote (regardless
// of whether it has reached the threshold).
func cvoteCancel(client *Client, args []string) {
	if !config.EnableCommunityVote {
		client.SendServerMessage("Community voting is not enabled on this server.")
		return
	}

	if !client.Authenticated() || (!permissions.IsModerator(client.Perms()) && !clientHasCommandGrant(client, "cvote")) {
		client.SendServerMessage("Only moderators can cancel community votes.")
		return
	}

	if len(args) < 1 {
		client.SendServerMessage("Usage: /cvote cancel <uid>")
		return
	}

	targetUID, err := strconv.Atoi(args[0])
	if err != nil {
		client.SendServerMessage("Invalid UID.")
		return
	}

	communityVotes.mu.Lock()
	entry, ok := communityVotes.active[targetUID]
	if !ok {
		communityVotes.mu.Unlock()
		client.SendServerMessage("No active community vote found for that UID.")
		return
	}

	action := entry.action
	targetName := entry.targetName
	if entry.timer != nil {
		entry.timer.Stop()
	}
	delete(communityVotes.active, targetUID)
	communityVotes.mu.Unlock()

	modName := client.DisplayModName()
	addToBuffer(client, "MOD",
		fmt.Sprintf("Community vote %s CANCELLED for UID %d (%s)", action, targetUID, targetName),
		true)
	sendGlobalServerMessage(fmt.Sprintf(
		"⚖️ Community vote to %s %s (UID %d) was cancelled by moderator %s.",
		action, targetName, targetUID, modName,
	))
	client.SendServerMessage(fmt.Sprintf("Community vote against UID %d cancelled.", targetUID))
}

// ── Listing votes ─────────────────────────────────────────────────────────────

// cvoteList shows all currently active community votes.
func cvoteList(client *Client) {
	if !config.EnableCommunityVote {
		client.SendServerMessage("Community voting is not enabled on this server.")
		return
	}

	communityVotes.mu.Lock()
	if len(communityVotes.active) == 0 {
		communityVotes.mu.Unlock()
		client.SendServerMessage("No active community votes.")
		return
	}

	// Snapshot the fields we need under the lock so we can release it before
	// doing any string formatting (which allocates and can be slow).
	type voteSnap struct {
		action     cvoteActionType
		targetUID  int
		targetName string
		reason     string
		votes      int
		threshold  int
		pending    bool
	}
	snaps := make([]voteSnap, 0, len(communityVotes.active))
	for _, e := range communityVotes.active {
		snaps = append(snaps, voteSnap{
			action:     e.action,
			targetUID:  e.targetUID,
			targetName: e.targetName,
			reason:     e.reason,
			votes:      len(e.voters),
			threshold:  e.threshold,
			pending:    e.pending,
		})
	}
	communityVotes.mu.Unlock()

	lines := make([]string, 0, len(snaps))
	for _, s := range snaps {
		status := fmt.Sprintf("%d/%d votes", s.votes, s.threshold)
		if s.pending {
			status = "PASSED – awaiting moderator"
		}
		lines = append(lines, fmt.Sprintf(
			"• /cvote %s %d  —  %s (%s)  [%s]  Reason: %s",
			s.action, s.targetUID, s.targetName, s.action, status, s.reason,
		))
	}
	client.SendServerMessage("⚖️ Active community votes:\n" + strings.Join(lines, "\n"))
}

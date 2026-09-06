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
	"flag"
	"fmt"
	"io"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/MangosArentLiterature/Athena/internal/area"
	"github.com/MangosArentLiterature/Athena/internal/packet"
	str2duration "github.com/xhit/go-str2duration/v2"
)

func cmdPos(client *Client, args []string, _ string) {
	if len(args) == 0 {
		client.SendServerMessage(fmt.Sprintf("Your current position is: %v\nAvailable positions: %v",
			client.Pos(), strings.Join(validPositions, ", ")))
		return
	}
	pos := strings.ToLower(args[0])
	for _, v := range validPositions {
		if pos == v {
			client.SetPos(pos)
			addToBuffer(client, "CMD", fmt.Sprintf("Changed position to %v.", pos), false)
			client.SendServerMessage(fmt.Sprintf("Position changed to: %v", pos))
			return
		}
	}
	client.SendServerMessage(fmt.Sprintf("Invalid position. Available positions: %v", strings.Join(validPositions, ", ")))
}

// oocDisplayName returns the name to show for a player in OOC-channel and
// server messages. It prefers the player's OOC name (matching /global, /pm and
// plain OOC chat), falling back to the showname and then the character name so
// the label is never blank when a player hasn't set an OOC name. Used by pair
// notifications, /rps, /8ball, etc. so every OOC-facing message labels players
// by the same OOC identity rather than mixing in IC shownames.
func oocDisplayName(c *Client) string {
	if name := strings.TrimSpace(c.OOCName()); name != "" {
		return c.OOCName()
	}
	return clientDisplayName(c)
}

// Handles /pair

func cmdPair(client *Client, args []string, _ string) {
	if client.CharID() < 0 {
		client.SendServerMessage("You have not selected a character.")
		return
	}

	uid, err := strconv.Atoi(args[0])
	if err != nil {
		client.SendServerMessage("Invalid UID.")
		return
	}

	target, err := getClientByUid(uid)
	if err != nil {
		client.SendServerMessage("Client does not exist.")
		return
	}

	if target == client {
		client.SendServerMessage("You cannot pair with yourself.")
		return
	}

	if target.Area() != client.Area() {
		client.SendServerMessage("That player is not in your area.")
		return
	}

	if target.CharID() < 0 {
		client.SendServerMessage("That player has not selected a character.")
		return
	}

	client.SetPairWantedID(target.CharID())

	// Check if the target is already requesting to pair with us (mutual pairing).
	if target.PairWantedID() == client.CharID() {
		// Establish UID-tracked pair on both sides so it persists across area changes.
		client.SetForcePairUID(target.Uid())
		target.SetForcePairUID(client.Uid())
		client.SendServerMessage(fmt.Sprintf("Now pairing with %v.", oocDisplayName(target)))
		target.SendServerMessage(fmt.Sprintf("%v accepted your pair request.", oocDisplayName(client)))
	} else {
		client.SendServerMessage(fmt.Sprintf("Sent pair request to %v.", oocDisplayName(target)))
		target.SendServerMessage(fmt.Sprintf("%v wants to pair with you. Type /pair %v to accept.", oocDisplayName(client), client.Uid()))
	}
}

// Handles /forcepair

func cmdForcePair(client *Client, args []string, _ string) {
	uid1, err := strconv.Atoi(args[0])
	if err != nil {
		client.SendServerMessage("Invalid UID.")
		return
	}

	uid2, err := strconv.Atoi(args[1])
	if err != nil {
		client.SendServerMessage("Invalid UID.")
		return
	}

	target1, err := getClientByUid(uid1)
	if err != nil {
		client.SendServerMessage(fmt.Sprintf("Client with UID %v does not exist.", uid1))
		return
	}

	target2, err := getClientByUid(uid2)
	if err != nil {
		client.SendServerMessage(fmt.Sprintf("Client with UID %v does not exist.", uid2))
		return
	}

	if target1 == target2 {
		client.SendServerMessage("Cannot force a player to pair with themselves.")
		return
	}

	if punishmentsSystemDisabled(client) {
		return
	}

	if target1.CharID() < 0 {
		client.SendServerMessage(fmt.Sprintf("UID %v has not selected a character.", uid1))
		return
	}

	if target2.CharID() < 0 {
		client.SendServerMessage(fmt.Sprintf("UID %v has not selected a character.", uid2))
		return
	}

	target1.SetPairWantedID(target2.CharID())
	target2.SetPairWantedID(target1.CharID())
	target1.SetForcePairUID(target2.Uid())
	target2.SetForcePairUID(target1.Uid())

	target1.SendServerMessage(fmt.Sprintf("You have been force-paired with %v by %v.", target2.OOCName(), client.OOCName()))
	target2.SendServerMessage(fmt.Sprintf("You have been force-paired with %v by %v.", target1.OOCName(), client.OOCName()))
	client.SendServerMessage(fmt.Sprintf("Force-paired %v and %v.", target1.OOCName(), target2.OOCName()))
	addToBuffer(client, "CMD", fmt.Sprintf("Force-paired UID %v and UID %v.", uid1, uid2), false)
}

// Handles /unpair
//
// Full bidirectional reset. Previously /unpair only fully cleared the partner's
// state when ForcePairUID was set, which could leave a stale PairWantedID on a
// peer (especially after the canceller's CharID changed since the request was
// sent) and produce a desync where one side still appeared paired. Now we walk
// the entire client list and clear PairWantedID + ForcePairUID on anyone who
// references us by UID OR by current/historical CharID, then clear our own.
func cmdUnpair(client *Client, _ []string, _ string) {
	clientUID := client.Uid()
	clientCharID := client.CharID()
	cancellerName := oocDisplayName(client)

	hadPair := client.PairWantedID() != -1 || client.ForcePairUID() != -1

	// Clear every reference to this client on every other client.
	notified := 0
	clients.ForEach(func(c *Client) {
		if c == client {
			return
		}
		referenced := false
		if c.ForcePairUID() == clientUID {
			c.SetForcePairUID(-1)
			referenced = true
		}
		if clientCharID >= 0 && c.PairWantedID() == clientCharID {
			c.SetPairWantedID(-1)
			referenced = true
		}
		if referenced {
			c.SendServerMessage(fmt.Sprintf("%v has cancelled the pair.", cancellerName))
			notified++
		}
	})

	// Clear our own state regardless of whether a partner was found.
	client.SetForcePairUID(-1)
	client.SetPairWantedID(-1)

	switch {
	case hadPair && notified > 0:
		client.SendServerMessage("Pair cancelled.")
	case hadPair:
		// We had local state but no peer was referencing us — most likely a
		// stale request whose target already disconnected. Clear silently.
		client.SendServerMessage("Pair cancelled.")
	default:
		client.SendServerMessage("You do not have an active pair request.")
	}
}

// clearPairLinksOnDisconnect dissolves any pairing involving a client that is
// leaving the server. Without this, a partner kept a stale ForcePairUID (or
// PairWantedID) pointing at the departed player's now-recyclable UID/CharID,
// which produced the IC pair desync the disconnect was supposed to clean up.
//
// Mirrors cmdUnpair: walk the client list and clear every reference to the
// leaver by UID OR by current CharID, notifying each affected partner, then
// clear the leaver's own state. Must run while the leaver's UID is still valid
// (before uids.ReleaseUid in clientCleanup).
func clearPairLinksOnDisconnect(client *Client) {
	// Scan unconditionally (like cmdUnpair) rather than early-returning when the
	// leaver's own pair fields are clear: a peer can hold an incoming pending
	// request (peer.PairWantedID == leaver's CharID) while the leaver itself
	// never reciprocated, so its own fields stay -1. Leaving that stale request
	// behind lets it auto-complete against a recycled UID/CharID later — the
	// desync this cleanup exists to prevent. Disconnects are infrequent, so the
	// O(n) walk is cheap.
	clientUID := client.Uid()
	clientCharID := client.CharID()
	leaverName := oocDisplayName(client)

	clients.ForEach(func(c *Client) {
		if c == client {
			return
		}
		referenced := false
		if c.ForcePairUID() == clientUID {
			c.SetForcePairUID(-1)
			referenced = true
		}
		if clientCharID >= 0 && c.PairWantedID() == clientCharID {
			c.SetPairWantedID(-1)
			referenced = true
		}
		if referenced {
			c.SendServerMessage(fmt.Sprintf("%v disconnected — your pair has been cancelled.", leaverName))
		}
	})

	client.SetForcePairUID(-1)
	client.SetPairWantedID(-1)
}

// Handles /forceunpair

func cmdForceUnpair(client *Client, args []string, _ string) {
	uid, err := strconv.Atoi(args[0])
	if err != nil {
		client.SendServerMessage("Invalid UID.")
		return
	}

	target, err := getClientByUid(uid)
	if err != nil {
		client.SendServerMessage(fmt.Sprintf("Client with UID %v does not exist.", uid))
		return
	}

	if target.PairWantedID() == -1 && target.ForcePairUID() == -1 {
		client.SendServerMessage("That player is not paired.")
		return
	}

	// Clear the partner's pair state.
	if target.ForcePairUID() >= 0 {
		if partner, err := getClientByUid(target.ForcePairUID()); err == nil {
			partner.SetForcePairUID(-1)
			partner.SetPairWantedID(-1)
			partner.SendServerMessage(fmt.Sprintf("You have been force-unpaired by %v.", client.OOCName()))
		}
		target.SetForcePairUID(-1)
	}

	// Notify any non-UID-tracked client that was paired with the target.
	targetOOCName := target.OOCName()
	targetCharID := target.CharID()
	moderatorName := client.OOCName()
	clients.ForEach(func(c *Client) {
		if c != target && c.PairWantedID() == targetCharID {
			c.SetPairWantedID(-1)
			c.SendServerMessage(fmt.Sprintf("Your pair with %v was cancelled by %v.", targetOOCName, moderatorName))
		}
	})

	target.SetPairWantedID(-1)
	target.SendServerMessage(fmt.Sprintf("You have been force-unpaired by %v.", client.OOCName()))
	client.SendServerMessage(fmt.Sprintf("Force-unpaired %v.", target.OOCName()))
	addToBuffer(client, "CMD", fmt.Sprintf("Force-unpaired UID %v.", uid), false)
}

// Handles /rmusr

func cmdDance(client *Client, _ []string, _ string) {
	client.ToggleDance()
}

// rollRegex matches "<dice>d<sides>" with an optional trailing "+<mod>"/"-<mod>",
// e.g. "2d6" or "1d20+10".
var rollRegex = regexp.MustCompile(`^([[:digit:]]+)d([[:digit:]]+)([+-][[:digit:]]+)?$`)

func cmdRoll(client *Client, args []string, _ string) {
	flags := flag.NewFlagSet("", 0)
	flags.SetOutput(io.Discard)
	private := flags.Bool("p", false, "")
	flags.Parse(args)
	m := rollRegex.FindStringSubmatch(flags.Arg(0))
	if m == nil {
		client.SendServerMessage("Argument not recognized.")
		return
	}
	num, _ := strconv.Atoi(m[1])
	sides, _ := strconv.Atoi(m[2])
	var modifier int
	if m[3] != "" {
		modifier, _ = strconv.Atoi(m[3])
	}
	if num <= 0 || num > config.MaxDice || sides <= 0 || sides > config.MaxSide {
		client.SendServerMessage("Invalid num/side.")
		return
	}
	var result []string
	sum := 0
	for i := 0; i < num; i++ {
		roll := rand.Intn(sides) + 1
		sum += roll
		result = append(result, fmt.Sprint(roll))
	}
	resultStr := strings.Join(result, ", ")
	if num > 1 || modifier != 0 {
		total := sum + modifier
		if modifier != 0 {
			resultStr = fmt.Sprintf("%v (%+d) (Total: %v)", resultStr, modifier, total)
		} else {
			resultStr = fmt.Sprintf("%v (Total: %v)", resultStr, total)
		}
	}
	if *private {
		client.SendServerMessage(fmt.Sprintf("Results: %v.", resultStr))
	} else {
		sendAreaServerMessage(client.Area(), fmt.Sprintf("%v rolled %v. Results: %v.", oocDisplayName(client), flags.Arg(0), resultStr))
	}
	addToBuffer(client, "CMD", fmt.Sprintf("Rolled %v.", flags.Arg(0)), false)
}

// rpsChallenge records the first player's hidden RPS commitment in an area.
// We don't broadcast their choice — the second player has to commit blind so
// they can't game-theory the result by watching the first move.
type rpsChallenge struct {
	UID       int
	Name      string
	Choice    string
	CreatedAt time.Time
}

// rpsState tracks one pending RPS challenge per area. Mod commands and the
// challenge logic both run on goroutines, so accesses are guarded.
var (
	rpsState   = map[*area.Area]*rpsChallenge{}
	rpsStateMu = struct{ sync.Mutex }{}
)

// rpsBeats answers "does a beat b?".
func rpsBeats(a, b string) bool {
	switch {
	case a == "rock" && b == "scissors",
		a == "scissors" && b == "paper",
		a == "paper" && b == "rock":
		return true
	}
	return false
}

// Handles /rps <rock|paper|scissors>
//
// Player-vs-player: the first call starts a hidden challenge in the area,
// the second call (from any other player) commits a choice and resolves it.
// Replaces the prior server-vs-player coin-flip-style version, which felt
// pointless when there are real opponents in the room.
//
// 30-second window per player. Challenges auto-expire after 30s.
func cmdRps(client *Client, args []string, _ string) {
	choice := strings.ToLower(args[0])
	if choice != "rock" && choice != "paper" && choice != "scissors" {
		client.SendServerMessage("Invalid choice. Use: rock, paper, or scissors.")
		return
	}

	if !client.LastRpsTime().IsZero() && time.Since(client.LastRpsTime()) < 30*time.Second {
		remaining := int((30*time.Second - time.Since(client.LastRpsTime())).Seconds()) + 1
		client.SendServerMessage(fmt.Sprintf("Please wait %d seconds before playing RPS again.", remaining))
		return
	}

	rpsStateMu.Lock()
	defer rpsStateMu.Unlock()

	a := client.Area()
	pending, ok := rpsState[a]
	// Stale-challenge cleanup: an old challenge from a player who left or
	// gave up shouldn't block a new game.
	if ok && time.Since(pending.CreatedAt) > 30*time.Second {
		delete(rpsState, a)
		pending, ok = nil, false
	}

	if !ok {
		// First mover: stash the hidden challenge and announce it.
		rpsState[a] = &rpsChallenge{
			UID:       client.Uid(),
			Name:      oocDisplayName(client),
			Choice:    choice,
			CreatedAt: time.Now().UTC(),
		}
		client.SetLastRpsTime(time.Now().UTC())
		sendAreaServerMessage(a, fmt.Sprintf(
			"✊✋✌️ %v has thrown an RPS challenge! Anyone can answer with /rps <rock|paper|scissors> within 30 seconds.",
			oocDisplayName(client)))
		client.SendServerMessage(fmt.Sprintf("Your hidden choice: %s. Waiting for an opponent...", choice))
		return
	}

	if pending.UID == client.Uid() {
		client.SendServerMessage("You're already the challenger — wait for someone else to answer your /rps.")
		return
	}

	// Second mover: resolve.
	delete(rpsState, a)
	client.SetLastRpsTime(time.Now().UTC())

	var result string
	switch {
	case pending.Choice == choice:
		result = "It's a tie!"
	case rpsBeats(pending.Choice, choice):
		result = fmt.Sprintf("%v wins!", pending.Name)
	default:
		result = fmt.Sprintf("%v wins!", oocDisplayName(client))
	}
	sendAreaServerMessage(a, fmt.Sprintf(
		"%v (%s) vs %v (%s) — %s",
		pending.Name, pending.Choice, oocDisplayName(client), choice, result))
	addToBuffer(client, "GAME", fmt.Sprintf("RPS: %v vs %v -> %v", pending.Choice, choice, result), false)
}

// Handles /coinflip

func cmdCoinflip(client *Client, args []string, _ string) {
	if isPunishmentBlocked(client.Ipid()) {
		client.SendServerMessage("A moderator has disabled self-applied punishment commands for you.")
		return
	}
	choice := strings.ToLower(args[0])
	if choice != "heads" && choice != "tails" {
		client.SendServerMessage("Invalid choice. Use: heads or tails.")
		return
	}

	// Check if there's an active coinflip challenge in the area
	activeChallenge := client.Area().ActiveCoinflip()

	if activeChallenge == nil {
		// No active challenge - create a new one
		challenge := &area.CoinflipChallenge{
			PlayerName: client.OOCName(),
			Choice:     choice,
			CreatedAt:  time.Now().UTC(),
		}
		client.Area().SetActiveCoinflip(challenge)
		client.Area().SetLastCoinflipTime(time.Now().UTC())

		// Announce the challenge
		message := fmt.Sprintf("%v has chosen %v and is ready to coinflip! Type /coinflip %v to battle them!",
			client.OOCName(), choice, oppositeChoice(choice))
		sendAreaServerMessage(client.Area(), message)
		addToBuffer(client, "GAME", fmt.Sprintf("Started coinflip challenge with %v", choice), false)

	} else {
		// There's an active challenge

		// Check if challenge has expired (30 seconds)
		if time.Now().UTC().After(activeChallenge.CreatedAt.Add(30 * time.Second)) {
			// Challenge expired, create new one
			challenge := &area.CoinflipChallenge{
				PlayerName: client.OOCName(),
				Choice:     choice,
				CreatedAt:  time.Now().UTC(),
			}
			client.Area().SetActiveCoinflip(challenge)
			client.Area().SetLastCoinflipTime(time.Now().UTC())

			message := fmt.Sprintf("Previous coinflip expired. %v has chosen %v and is ready to coinflip! Type /coinflip %v to battle them!",
				client.OOCName(), choice, oppositeChoice(choice))
			sendAreaServerMessage(client.Area(), message)
			addToBuffer(client, "GAME", fmt.Sprintf("Started coinflip challenge with %v", choice), false)
			return
		}

		// Check if same player is trying to accept their own challenge
		if activeChallenge.PlayerName == client.OOCName() {
			client.SendServerMessage("You cannot accept your own coinflip challenge!")
			return
		}

		// Check if the choice is different from the challenger's choice
		if activeChallenge.Choice == choice {
			client.SendServerMessage(fmt.Sprintf("You must pick the opposite choice! The challenger picked %v, so you must pick %v.",
				activeChallenge.Choice, oppositeChoice(activeChallenge.Choice)))
			return
		}

		// Battle time! Flip the coin
		coinResult := "heads"
		if rand.Intn(2) == 1 {
			coinResult = "tails"
		}

		// Determine winner
		var winner string
		if coinResult == activeChallenge.Choice {
			winner = activeChallenge.PlayerName
		} else {
			winner = client.OOCName()
		}

		// Announce result
		message := fmt.Sprintf("⚔️ COINFLIP BATTLE! %v (%v) vs %v (%v) - The coin landed on %v! 🎉 %v WINS! 🎉",
			activeChallenge.PlayerName, activeChallenge.Choice,
			client.OOCName(), choice,
			coinResult, winner)
		sendAreaServerMessage(client.Area(), message)

		// Log for both players
		addToBuffer(client, "GAME", fmt.Sprintf("Coinflip battle: %v vs %v - Result: %v - Winner: %v",
			activeChallenge.Choice, choice, coinResult, winner), false)

		// Clear the challenge
		client.Area().SetActiveCoinflip(nil)
	}
}

// oppositeChoice returns the opposite coinflip choice
func oppositeChoice(choice string) string {
	if choice == "heads" {
		return "tails"
	}
	return "heads"
}

// Handles /poll

func cmdPoll(client *Client, args []string, usage string) {
	// Check if there's already an active poll
	if client.Area().ActivePoll() != nil {
		client.SendServerMessage("There is already an active poll in this area.")
		return
	}

	// Check cooldown (5 minutes)
	if time.Now().UTC().Before(client.Area().LastPollTime().Add(5*time.Minute)) && !client.Area().LastPollTime().IsZero() {
		remaining := time.Until(client.Area().LastPollTime().Add(5 * time.Minute))
		client.SendServerMessage(fmt.Sprintf("Please wait %v before creating another poll in this area.", remaining.Round(time.Second)))
		return
	}

	// Parse poll format: question|option1|option2|...
	fullArg := strings.Join(args, " ")
	parts := strings.Split(fullArg, "|")

	if len(parts) < 3 {
		client.SendServerMessage("Not enough poll options. Format: " + usage)
		return
	}

	question := strings.TrimSpace(parts[0])
	options := make([]string, 0)
	for i := 1; i < len(parts); i++ {
		opt := strings.TrimSpace(parts[i])
		if opt != "" {
			options = append(options, opt)
		}
	}

	if len(options) < 2 {
		client.SendServerMessage("Poll must have at least 2 options.")
		return
	}

	// Broadcast poll to area
	pollMsg := fmt.Sprintf("=== POLL ===\n%v\n", question)
	for i, opt := range options {
		pollMsg += fmt.Sprintf("%v. %v\n", i+1, opt)
	}
	pollMsg += fmt.Sprintf("\nUse /vote <number> to vote. Poll closes in 2 minutes.")

	// /poll is reqPerms:CM -- grantable per-area, not staff-only on most
	// servers -- and broadcasts free player text (question + options)
	// area-wide, so it must pass the same content/punishment gate /global
	// does -- see oocCommandAllowed. Checked, and the poll only actually
	// created, before any area state is touched, so a blocked poll never
	// leaves a dangling "active poll" that nobody was ever shown.
	checkText := question + "\n" + strings.Join(options, "\n")
	echo := &packet.CTToClient{Name: encodedServerName, Message: encode(pollMsg), IsFromServer: "1"}
	if !oocCommandAllowed(client, checkText, "poll", echo) {
		return
	}
	raidGuardOnOOC(client, client.OOCName(), checkText)
	if oocGuardVerdictSuppresses(client, checkText, echo) {
		return
	}

	// Create poll
	poll := &area.Poll{
		ID:        time.Now().UnixNano(),
		Question:  question,
		Options:   options,
		CreatedAt: time.Now().UTC(),
		ClosesAt:  time.Now().UTC().Add(2 * time.Minute),
		CreatedBy: client.OOCName(),
	}

	client.Area().SetActivePoll(poll)
	client.Area().SetLastPollTime(time.Now().UTC())
	client.Area().SetPollVotes(make(map[int]int))
	client.Area().SetPlayerVotes(make(map[int]int))

	broadcastToArea(client.Area(), echo)
	addToBuffer(client, "CMD", fmt.Sprintf("Created poll: %v", question), false)

	// Schedule auto-close after 2 minutes
	go func(a *area.Area, pollID int64) {
		time.Sleep(2 * time.Minute)
		currentPoll := a.ActivePoll()
		if currentPoll != nil && currentPoll.ID == pollID {
			// Close poll
			resultMsg := fmt.Sprintf("=== POLL CLOSED ===\n%v\nResults:\n", currentPoll.Question)
			votes := a.PollVotes()
			for i, opt := range currentPoll.Options {
				count := 0
				if votes != nil {
					count = votes[i+1]
				}
				resultMsg += fmt.Sprintf("%v. %v - %v votes\n", i+1, opt, count)
			}
			sendAreaServerMessage(a, resultMsg)
			a.ClearPoll()
		}
	}(client.Area(), poll.ID)
}

// Handles /vote

func cmdVote(client *Client, args []string, usage string) {
	// Check if there's an active poll
	poll := client.Area().ActivePoll()
	if poll == nil {
		client.SendServerMessage("There is no active poll in this area.")
		return
	}

	// Check if poll has expired
	if time.Now().UTC().After(poll.ClosesAt) {
		client.SendServerMessage("This poll has expired.")
		client.Area().ClearPoll()
		return
	}

	// Check if player has already voted
	if client.Area().HasPlayerVoted(client.Uid()) {
		client.SendServerMessage("You have already voted in this poll.")
		return
	}

	// Parse vote option
	option, err := strconv.Atoi(args[0])
	if err != nil || option < 1 || option > len(poll.Options) {
		client.SendServerMessage(fmt.Sprintf("Invalid option. Choose a number between 1 and %v.", len(poll.Options)))
		return
	}

	// Record vote
	client.Area().AddPlayerVote(client.Uid(), option)
	client.SendServerMessage(fmt.Sprintf("You voted for: %v", poll.Options[option-1]))

	// Broadcast updated results to area
	resultMsg := fmt.Sprintf("=== POLL UPDATE ===\n%v\nCurrent Results:\n", poll.Question)
	votes := client.Area().PollVotes()
	for i, opt := range poll.Options {
		count := 0
		if votes != nil {
			count = votes[i+1]
		}
		resultMsg += fmt.Sprintf("%v. %v - %v votes\n", i+1, opt, count)
	}
	sendAreaServerMessage(client.Area(), resultMsg)
	addToBuffer(client, "VOTE", fmt.Sprintf("Voted for option %v in poll", option), false)
}

// cmdPunishment is a generic handler for punishment commands

func cmdWhisper(client *Client, args []string, usage string) {
	cmdPunishment(client, args, usage, PunishmentWhisper)
}

// Handles /erp

var erpMessages = []string{
	"Get out of here, you dirty little ERPer. Nobody wants you.",
	"Ewww, an ERPer?! Get off this server immediately!",
	"Pathetic. Absolutely pathetic. Go ERP somewhere else.",
	"The AO ERP police have arrived. You're being escorted OUT.",
	"Did you really just try to ERP here? Disgusting. Goodbye.",
	"This server is an ERP-free zone. Enforcing policy NOW.",
	"Oh sweetie, no. Just... no. Take your cringe and leave.",
	"Your ERP license has been revoked. Please see yourself out.",
	"ERROR 404: Dignity not found. Kicking user for gross misconduct.",
	"The council has spoken. ERPers are NOT welcome. Bye-bye!",
}

func cmdErp(client *Client, _ []string, _ string) {
	msg := erpMessages[rand.Intn(len(erpMessages))]
	client.SendSync(&packet.KK{Reason: msg})
	client.conn.Close()
}

// cmdMaso lets any user self-apply a random punishment for a configurable
// duration (default 10 m, max 24 h). Calling it again while a maso punishment
// is active removes the old one and applies a new (different) random
// punishment, resetting the timer.
func cmdMaso(client *Client, args []string, _ string) {
	if isPunishmentBlocked(client.Ipid()) {
		client.SendServerMessage("A moderator has disabled self-applied punishment commands for you.")
		return
	}
	flags := flag.NewFlagSet("", 0)
	flags.SetOutput(io.Discard)
	durationStr := flags.String("d", "10m", "")
	flags.Parse(args)

	duration, err := str2duration.ParseDuration(*durationStr)
	if err != nil || duration <= 0 {
		client.SendServerMessage("Invalid duration format. Use formats like: 10m, 1h, 30m, 2h30m")
		return
	}
	if duration > 24*time.Hour {
		duration = 24 * time.Hour
	}

	client.mu.Lock()
	prev := client.masoPunishment
	client.mu.Unlock()

	// Remove the previously maso-applied punishment (no-op if it already expired).
	if prev != PunishmentNone {
		client.RemovePunishment(prev)
	}

	// Pick a random punishment; if rerolling, ensure it differs from the previous one.
	newType := areaRandomPunishments[rand.Intn(len(areaRandomPunishments))]
	if prev != PunishmentNone && len(areaRandomPunishments) > 1 {
		for newType == prev {
			newType = areaRandomPunishments[rand.Intn(len(areaRandomPunishments))]
		}
	}

	client.AddPunishment(newType, duration, "maso")

	client.mu.Lock()
	client.masoPunishment = newType
	client.mu.Unlock()

	if prev != PunishmentNone {
		client.SendServerMessage(fmt.Sprintf(
			"Your maso punishment has been rerolled! New effect: '%v' (%v).", newType.String(), duration))
	} else {
		client.SendServerMessage(fmt.Sprintf(
			"You've applied a random maso punishment to yourself: '%v' (%v). Type /maso again to reroll.", newType.String(), duration))
	}
}

// defaultEightBallAnswers is the fallback list when 8ball.txt is missing or
// empty. Mirrors the 20 classic Magic 8 Ball responses.
var defaultEightBallAnswers = []string{
	"It is certain.",
	"It is decidedly so.",
	"Without a doubt.",
	"Yes - definitely.",
	"You may rely on it.",
	"As I see it, yes.",
	"Most likely.",
	"Outlook good.",
	"Yes.",
	"Signs point to yes.",
	"Reply hazy, try again.",
	"Ask again later.",
	"Better not tell you now.",
	"Cannot predict now.",
	"Concentrate and ask again.",
	"Don't count on it.",
	"My reply is no.",
	"My sources say no.",
	"Outlook not so good.",
	"Very doubtful.",
}

// Handles /getmusic
//
// Two functions in one:
//  1. Prints the URL of the song currently playing in the area (some clients
//     have flaky audio handling and players want to copy/paste the URL).
//  2. Replays the song to ONLY the requesting client by sending an MC packet
//     directly to them — useful when their client bugged out and the song
//     never started, without disturbing anyone else's playback position.
func cmdGetMusic(client *Client, _ []string, _ string) {
	song := client.Area().CurrentSong()
	if song == "" {
		client.SendServerMessage("No track is currently playing in this area.")
		return
	}
	decoded := decode(song)
	client.SendServerMessage(fmt.Sprintf("🎵 Now playing: %s", decoded))
	// Re-send the music change to just this client so a stuck audio player
	// gets nudged into starting the track. Other clients are unaffected.
	cidStr := client.CharIDStr()
	if cidStr == "" {
		cidStr = "0"
	}
	cid, _ := strconv.Atoi(cidStr)
	client.Send(&packet.MCToClient{Name: song, CharID: cid, Showname: "Server", Looping: "1", Channel: "0", Effects: "0"})
}

// Handles /8ball
func cmd8Ball(client *Client, args []string, _ string) {
	question := strings.TrimSpace(strings.Join(args, " "))
	if question == "" {
		client.SendServerMessage("Usage: /8ball <question>")
		return
	}
	pool := getEightBall()
	if len(pool) == 0 {
		pool = defaultEightBallAnswers
	}
	answer := pool[rand.Intn(len(pool))]
	fullMsg := fmt.Sprintf("%v asked: %s\n🎱 The Magic 8-Ball says: %s",
		oocDisplayName(client), question, answer)
	echo := &packet.CTToClient{Name: encodedServerName, Message: encode(fullMsg), IsFromServer: "1"}
	// /8ball is reqPerms:NONE and broadcasts free player text area-wide, so it
	// must pass the same content/punishment gate /global does -- see
	// oocCommandAllowed. Gated on the player-supplied question only; the
	// canned answer is server-controlled.
	if !oocCommandAllowed(client, question, "8ball", echo) {
		return
	}
	raidGuardOnOOC(client, client.OOCName(), question)
	if oocGuardVerdictSuppresses(client, question, echo) {
		return
	}
	broadcastToArea(client.Area(), echo)
}

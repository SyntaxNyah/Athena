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
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/MangosArentLiterature/Athena/internal/db"
	"github.com/MangosArentLiterature/Athena/internal/logger"
	"github.com/MangosArentLiterature/Athena/internal/permissions"
	str2duration "github.com/xhit/go-str2duration/v2"
	"golang.org/x/crypto/bcrypt"
)

var validUsernameRe = regexp.MustCompile(`^[A-Za-z0-9_]{3,20}$`)

// generateCaptcha returns a random 16-character hex string used as a registration captcha token.
// Uses a stack-allocated [8]byte to avoid a heap allocation.
func generateCaptcha() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// onRegistered completes account creation after the DB row is already written:
// auto-logs the client in, seeds chip balance (casino only), and sends the
// success message. The message is tailored to whichever feature set is live —
// full casino vs. accounts-only wardrobe / tag / playtime mode.
func onRegistered(client *Client, username string) {
	client.SetAuthenticated(true)
	client.SetModName(username)
	// Backfill IGNORER_USERNAME on any ignore rows added before registration.
	db.BackfillIgnoreUsername(client.Ipid(), username) //nolint:errcheck
	if config.EnableCasino {
		if err := db.EnsureChipBalance(client.Ipid()); err != nil {
			logger.LogErrorf("Failed to seed chip balance on register for %v: %v", username, err)
		}
		client.SendServerMessage(fmt.Sprintf(
			"✅ Account '%v' created and logged in!\n\n"+
				"📋 What your account tracks:\n"+
				"  • 💰 Nyathena Chips (casino balance)\n"+
				"  • ⏱ Playtime on this server\n"+
				"  • 🏆 Casino leaderboard standings\n\n"+
				"Use /account to view your profile.\n"+
				"Use /chips to check your balance.\n"+
				"Your account is linked to your connection — use /login <username> <password> to sign in on reconnect.",
			username))
	} else {
		client.SendServerMessage(fmt.Sprintf(
			"✅ Account '%v' created and logged in!\n\n"+
				"📋 What your account tracks (gambling is off on this server):\n"+
				"  • 👗 Wardrobe — save favourite characters with /favourite <char>\n"+
				"  • 🏷️ Default tag — any tag in /shop is free to equip with /settag <id>\n"+
				"  • ⏱ Playtime — accumulates across sessions, see /playtime top\n\n"+
				"Use /account to view your profile.\n"+
				"Your account is linked to your connection — use /login <username> <password> to sign in on reconnect.",
			username))
	}
	addToBuffer(client, "CMD", fmt.Sprintf("Registered player account %v.", username), false)
}

// Handles /register
//
// Any player can create a free account. Accounts do not grant any extra
// permissions — they exist purely to track in-game features such as
// Nyathena Chip balance, playtime, and future leaderboard standings.
//
// When register_captcha is true (default) this is a two-step flow:
//  1. /register <username> <password> — validates inputs and issues a captcha.
//  2. /captcha <token>               — confirms the token and creates the account.
//
// When register_captcha is false the account is created immediately.
func cmdRegister(client *Client, args []string, _ string) {
	if client.Authenticated() {
		client.SendServerMessage("You already have an account linked to this session. Use /logout first if you want to register a different account.")
		return
	}

	// One account per IPID.
	if existing, err := db.GetUsernameByIPID(client.Ipid()); err == nil && existing != "" {
		client.SendServerMessage(fmt.Sprintf(
			"An account ('%v') is already registered on your connection.\nUse /login %v <password> to sign in.",
			existing, existing))
		return
	}

	username, password := args[0], args[1]

	if !validUsernameRe.MatchString(username) {
		client.SendServerMessage(fmt.Sprintf(
			"❌ '%v' isn't a valid username.\n"+
				"Usernames must be 3–20 characters and may only contain letters (A–Z, a–z), numbers (0–9), and underscores (_).\n"+
				"Examples that work: ena_m00ny, alice42, my_alt",
			username))
		return
	}
	if len(password) < 6 {
		client.SendServerMessage("Password must be at least 6 characters.")
		return
	}
	if db.UserExists(username) {
		// Distinguish "name claimed by a moderator account" from "name claimed
		// by another player" so users picking a name like "ena_m00ny" aren't
		// left wondering why a fresh-looking name is unavailable.
		if db.IsModUser(username) {
			client.SendServerMessage(fmt.Sprintf(
				"That username ('%v') is reserved by a staff account on this server. Please pick a different name for your player account.",
				username))
		} else {
			client.SendServerMessage(fmt.Sprintf(
				"That username ('%v') is already registered by another player. Please choose a different one.",
				username))
		}
		return
	}

	// When captcha is disabled, register immediately — no token generation,
	// no pending state, no extra allocations.
	if !config.RegisterCaptcha {
		if err := db.RegisterPlayer(username, []byte(password), client.Ipid()); err != nil {
			logger.LogErrorf("Register failed for %v (IPID %v): %v", username, client.Ipid(), err)
			client.SendServerMessage("Registration failed. Please try again.")
			return
		}
		onRegistered(client, username)
		return
	}

	// Captcha enabled: hash the password now so no plaintext is kept in pending state.
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		logger.LogErrorf("Failed to hash password for %v (IPID %v): %v", username, client.Ipid(), err)
		client.SendServerMessage("Registration failed. Please try again.")
		return
	}
	token, err := generateCaptcha()
	if err != nil {
		logger.LogErrorf("Failed to generate captcha for %v (IPID %v): %v", username, client.Ipid(), err)
		client.SendServerMessage("Registration failed. Please try again.")
		return
	}
	client.SetPendingReg(username, token, hashed)

	client.SendServerMessage(fmt.Sprintf(
		"🔐 One last step to create account '%v'!\n\n"+
			"To confirm you are human, please type the following command exactly:\n\n"+
			"  /captcha %v\n\n"+
			"This token expires when you disconnect.",
		username, token))
}

// Handles /captcha <token>
//
// Completes a pending registration that was started with /register.
// The player must supply the exact token that was issued during /register.
func cmdCaptcha(client *Client, args []string, _ string) {
	if !config.RegisterCaptcha {
		client.SendServerMessage("Registration captcha is not enabled on this server.")
		return
	}

	pendingUser, expectedToken, pendingHashedPass := client.PendingReg()
	if pendingUser == "" {
		client.SendServerMessage("You don't have a pending registration. Use /register <username> <password> to start one.")
		return
	}

	// Hex tokens are case-insensitive by definition; trimming surrounding
	// whitespace handles AO2 clients that auto-add a trailing space when copying.
	supplied := strings.ToLower(strings.TrimSpace(args[0]))
	// Constant-time comparison prevents timing-based token guessing.
	if subtle.ConstantTimeCompare([]byte(supplied), []byte(expectedToken)) != 1 {
		client.SetPendingReg("", "", nil)
		client.SendServerMessage("❌ Incorrect captcha token. Please use /register <username> <password> again to get a new token.")
		return
	}

	// Token correct — clear pending state before touching the DB.
	client.SetPendingReg("", "", nil)

	// Re-check conditions that may have changed while the captcha was pending.
	if client.Authenticated() {
		client.SendServerMessage("You are already logged in.")
		return
	}
	if existing, err := db.GetUsernameByIPID(client.Ipid()); err == nil && existing != "" {
		client.SendServerMessage(fmt.Sprintf(
			"An account ('%v') was registered on your connection in the meantime. Use /login %v <password> to sign in.",
			existing, existing))
		return
	}
	if db.UserExists(pendingUser) {
		client.SendServerMessage("That username was taken while you were completing the captcha. Please use /register <username> <password> with a different name.")
		return
	}

	// Password was already bcrypt-hashed at /register time.
	if err := db.RegisterPlayerHashed(pendingUser, pendingHashedPass, client.Ipid()); err != nil {
		logger.LogErrorf("Register failed for %v (IPID %v): %v", pendingUser, client.Ipid(), err)
		client.SendServerMessage("Registration failed. Please try again.")
		return
	}
	onRegistered(client, pendingUser)
}

// Handles /account
//
// Displays the current player's account information: username, chip balance,
// and playtime. Prompts unregistered players to create an account.
func cmdAccount(client *Client, _ []string, _ string) {
	var username string
	if client.Authenticated() {
		username = client.ModName()
	} else {
		// Try to find a linked account for this IPID even if not currently logged in.
		u, err := db.GetUsernameByIPID(client.Ipid())
		if err == nil && u != "" {
			client.SendServerMessage(fmt.Sprintf(
				"Your account '%v' is not currently active. Use /login %v <password> to sign in.", u, u))
			return
		}
		if config.EnableCasino {
			client.SendServerMessage(
				"You don't have an account yet.\n\n" +
					"💡 Accounts are free and let you track:\n" +
					"  • 💰 Nyathena Chips (casino currency)\n" +
					"  • ⏱ Playtime on this server\n" +
					"  • 🏆 Casino leaderboard standings\n\n" +
					"Create one now with: /register <username> <password>\n" +
					"(Username: 3–20 chars, letters/numbers/underscore; Password: 6+ chars)")
		} else {
			client.SendServerMessage(
				"You don't have an account yet.\n\n" +
					"💡 Accounts are free and let you track (gambling is off here):\n" +
					"  • 👗 Wardrobe favourites — /favourite <char>\n" +
					"  • 🏷️ Default cosmetic tag — /settag <tag_id> (every tag in /shop is free)\n" +
					"  • ⏱ Playtime on this server — /playtime top\n\n" +
					"Create one now with: /register <username> <password>\n" +
					"(Username: 3–20 chars, letters/numbers/underscore; Password: 6+ chars)")
		}
		return
	}

	chips, playtimeSec, err := db.GetAccountStats(client.Ipid())
	if err != nil {
		chips = 0
		playtimeSec = 0
	}
	// Add the current session's elapsed time so the display reflects live playtime.
	if connAt := client.ConnectedAt(); !connAt.IsZero() {
		playtimeSec += int64(time.Since(connAt).Seconds())
	}

	if config.EnableCasino {
		client.SendServerMessage(fmt.Sprintf(
			"\n👤 Account: %v\n"+
				"💰 Chips: %d\n"+
				"⏱ Playtime: %v",
			username, chips, formatPlaytime(playtimeSec)))
	} else {
		activeTag := db.GetActiveTag(client.Ipid())
		tagDisplay := "(none)"
		if t := formatTagDisplay(activeTag); t != "" {
			tagDisplay = t
		}
		client.SendServerMessage(fmt.Sprintf(
			"\n👤 Account: %v\n"+
				"🏷️ Active tag: %v\n"+
				"⏱ Playtime: %v",
			username, tagDisplay, formatPlaytime(playtimeSec)))
	}
}

// formatPlaytime converts seconds into a human-readable "Xh Ym" / "Ym" string.
func formatPlaytime(seconds int64) string {
	if seconds <= 0 {
		return "less than a minute"
	}
	h := seconds / 3600
	m := (seconds % 3600) / 60
	if h > 0 {
		return fmt.Sprintf("%dh %dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}

// playtimePageSize is the number of entries shown per /playtime top page.
const playtimePageSize = 25

// Handles /playtime [top [page]]
//
// Displays the global playtime leaderboard, paginated 25 per page.
// `/playtime top` shows page 1 (positions 1–25); `/playtime top 2` shows
// 26–50; and so on. Both player accounts and moderator accounts (including
// shadow mods) are eligible — the leaderboard does not filter by permission.
func cmdPlaytimeTop(client *Client, args []string, usage string) {
	// Admin subcommand: /playtime add <username> <duration>
	if len(args) > 0 && strings.EqualFold(args[0], "add") {
		cmdPlaytimeAdd(client, args[1:])
		return
	}
	// Admin subcommand: /playtime remove <username> <duration>
	if len(args) > 0 && strings.EqualFold(args[0], "remove") {
		cmdPlaytimeRemove(client, args[1:])
		return
	}
	// Admin subcommand: /playtime set <username> <duration>
	if len(args) > 0 && strings.EqualFold(args[0], "set") {
		cmdPlaytimeSet(client, args[1:])
		return
	}

	page := 1
	remaining := args

	// Accept an optional leading "top" subcommand keyword.
	if len(remaining) > 0 && strings.EqualFold(remaining[0], "top") {
		remaining = remaining[1:]
	}

	// Accept an optional page number. Anything else is a usage error.
	if len(remaining) > 0 {
		if v, err := strconv.Atoi(remaining[0]); err == nil && v > 0 {
			page = v
		} else {
			client.SendServerMessage(usage)
			return
		}
	}

	offset := (page - 1) * playtimePageSize
	entries, err := db.GetTopPlaytimesPaged(playtimePageSize, offset)
	if err != nil || len(entries) == 0 {
		if page == 1 {
			client.SendServerMessage("No playtime data available yet.")
		} else {
			client.SendServerMessage(fmt.Sprintf("No entries on page %d.", page))
		}
		return
	}

	total, _ := db.CountPlaytimeEntries()
	totalPages := (total + playtimePageSize - 1) / playtimePageSize
	if totalPages < 1 {
		totalPages = 1
	}

	// Build a map of IPID → current-session seconds for all connected clients
	// so the leaderboard reflects live playtime, not just the last-flushed DB value.
	// Multiple clients may share the same IPID (multiclient), so use += to sum
	// all active sessions for the same IPID, matching clientCleanup behaviour.
	liveSecs := make(map[string]int64, players.GetPlayerCount())
	clients.ForEach(func(c *Client) {
		if connAt := c.ConnectedAt(); !connAt.IsZero() {
			if secs := int64(time.Since(connAt).Seconds()); secs > 0 {
				liveSecs[c.Ipid()] += secs
			}
		}
	})
	if len(liveSecs) > 0 {
		for i := range entries {
			if s, ok := liveSecs[entries[i].Ipid]; ok {
				entries[i].Playtime += s
			}
		}
		// Note: re-sorting only re-orders entries within this page slice. Cross-page
		// reordering on tight contests is acceptable; the DB query already provides
		// the canonical ordering.
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].Playtime > entries[j].Playtime
		})
	}

	var sb strings.Builder
	sb.Grow(60 + len(entries)*35)
	sb.WriteString(fmt.Sprintf("\n⏱ Playtime Leaderboard — Page %d/%d (%d total)\n",
		page, totalPages, total))
	for i, e := range entries {
		rank := offset + i + 1
		sb.WriteString(fmt.Sprintf("  %3d. %-20v  %v\n", rank, e.Username, formatPlaytime(e.Playtime)))
	}
	if page < totalPages {
		sb.WriteString(fmt.Sprintf("\nUse /playtime top %d for the next page.\n", page+1))
	}
	client.SendServerMessage(sb.String())
}

// cmdReloadPlaytime is an admin tool that walks every registered account and
// re-runs LinkIPIDToUser to merge any orphaned playtime that may have been
// stranded on a previous IPID. Useful after the bug where an account created
// from a long-running anonymous IPID didn't immediately reflect the
// pre-existing playtime on the leaderboard until a server restart.
func cmdReloadPlaytime(client *Client, _ []string, _ string) {
	rows, err := db.AllRegisteredIPIDs()
	if err != nil {
		client.SendServerMessage(fmt.Sprintf("Reload failed: %v", err))
		return
	}
	if len(rows) == 0 {
		client.SendServerMessage("No registered accounts found.")
		return
	}
	relinked := 0
	for _, r := range rows {
		if err := db.LinkIPIDToUser(r.Username, r.IPID); err != nil {
			logger.LogErrorf("reloadplaytime: relink %v failed: %v", r.Username, err)
			continue
		}
		relinked++
	}
	client.SendServerMessage(fmt.Sprintf(
		"Playtime reload complete. Re-linked %d/%d account(s). The leaderboard now reflects all merged playtime; new entries appear on the next /playtime top.",
		relinked, len(rows)))
}

// cmdPlaytimeAdd handles the admin-only /playtime add <username> <duration>
// subcommand. It increments the target account's stored playtime by the parsed
// duration (e.g. "60h", "30m", "1h30m"). The duration is added to KNOWN_IPS.PLAYTIME
// for the IPID linked to the account, so the leaderboard reflects it immediately.
func cmdPlaytimeAdd(client *Client, args []string) {
	const usage = "Usage: /playtime add <username> <duration>  (e.g. /playtime add alice 60h)"

	if !permissions.HasPermission(client.Perms(), permissions.PermissionField["ADMIN"]) && !clientHasCommandGrant(client, "playtime") {
		client.SendServerMessage("You do not have permission to use that command.")
		return
	}
	if len(args) < 2 {
		client.SendServerMessage(usage)
		return
	}

	targetUser := args[0]
	duration, err := str2duration.ParseDuration(args[1])
	if err != nil {
		client.SendServerMessage("Invalid duration format. Use values like 60h, 30m, or 1h30m.")
		return
	}
	if duration <= 0 {
		client.SendServerMessage("Duration must be greater than zero.")
		return
	}
	addSeconds := int64(duration.Seconds())
	if addSeconds <= 0 {
		client.SendServerMessage("Duration must be at least one second.")
		return
	}

	ipid, err := db.GetIPIDByUsername(targetUser)
	if err != nil {
		client.SendServerMessage("Failed to look up account: " + err.Error())
		return
	}
	if ipid == "" {
		client.SendServerMessage(fmt.Sprintf(
			"Account '%v' was not found, or has never logged in (no IPID linked yet).\n"+
				"Ask the player to /login at least once before granting playtime.",
			targetUser))
		return
	}

	// Ensure a KNOWN_IPS row exists so AddPlaytimeReturning's UPDATE can hit it.
	// Registered, previously-connected accounts already have one, but be defensive.
	if err := db.MarkIPKnown(ipid); err != nil {
		client.SendServerMessage("Failed to update playtime: " + err.Error())
		return
	}

	newTotal, err := db.AddPlaytimeReturning(ipid, addSeconds)
	if err != nil {
		client.SendServerMessage("Failed to update playtime: " + err.Error())
		return
	}

	client.SendServerMessage(fmt.Sprintf(
		"✅ Added %v of playtime to '%v'. New total: %v.",
		formatPlaytime(addSeconds), targetUser, formatPlaytime(newTotal)))

	addToBuffer(client, "CMD",
		fmt.Sprintf("Added %vs of playtime to '%v' (new total %vs).", addSeconds, targetUser, newTotal),
		true)
}

// cmdPlaytimeRemove handles the admin-only /playtime remove <username> <duration>
// subcommand. It decrements the target account's stored playtime by the parsed
// duration (e.g. "60h", "30m", "1h30m"), clamped at 0.
func cmdPlaytimeRemove(client *Client, args []string) {
	const usage = "Usage: /playtime remove <username> <duration>  (e.g. /playtime remove alice 60h)"

	if !permissions.HasPermission(client.Perms(), permissions.PermissionField["ADMIN"]) && !clientHasCommandGrant(client, "playtime") {
		client.SendServerMessage("You do not have permission to use that command.")
		return
	}
	if len(args) < 2 {
		client.SendServerMessage(usage)
		return
	}

	targetUser := args[0]
	duration, err := str2duration.ParseDuration(args[1])
	if err != nil {
		client.SendServerMessage("Invalid duration format. Use values like 60h, 30m, or 1h30m.")
		return
	}
	if duration <= 0 {
		client.SendServerMessage("Duration must be greater than zero.")
		return
	}
	removeSeconds := int64(duration.Seconds())
	if removeSeconds <= 0 {
		client.SendServerMessage("Duration must be at least one second.")
		return
	}

	ipid, err := db.GetIPIDByUsername(targetUser)
	if err != nil {
		client.SendServerMessage("Failed to look up account: " + err.Error())
		return
	}
	if ipid == "" {
		client.SendServerMessage(fmt.Sprintf(
			"Account '%v' was not found, or has never logged in (no IPID linked yet).",
			targetUser))
		return
	}

	if err := db.MarkIPKnown(ipid); err != nil {
		client.SendServerMessage("Failed to update playtime: " + err.Error())
		return
	}

	newTotal, err := db.SubtractPlaytimeReturning(ipid, removeSeconds)
	if err != nil {
		client.SendServerMessage("Failed to update playtime: " + err.Error())
		return
	}

	client.SendServerMessage(fmt.Sprintf(
		"✅ Removed %v of playtime from '%v'. New total: %v.",
		formatPlaytime(removeSeconds), targetUser, formatPlaytime(newTotal)))

	addToBuffer(client, "CMD",
		fmt.Sprintf("Removed %vs of playtime from '%v' (new total %vs).", removeSeconds, targetUser, newTotal),
		true)
}

// cmdPlaytimeSet handles the admin-only /playtime set <username> <duration>
// subcommand. It sets the target account's stored playtime to exactly the
// parsed duration (e.g. "1000h", "3d12h", "90m"), overwriting the previous
// total. Accepts any combination of units str2duration supports: ns, us/µs,
// ms, s, m, h, d, w — e.g. "1d2h30m15s".
func cmdPlaytimeSet(client *Client, args []string) {
	const usage = "Usage: /playtime set <username> <duration>  (e.g. /playtime set alice 1000h, /playtime set alice 3d12h30m)"

	if !permissions.HasPermission(client.Perms(), permissions.PermissionField["ADMIN"]) && !clientHasCommandGrant(client, "playtime") {
		client.SendServerMessage("You do not have permission to use that command.")
		return
	}
	if len(args) < 2 {
		client.SendServerMessage(usage)
		return
	}

	targetUser := args[0]
	duration, err := str2duration.ParseDuration(args[1])
	if err != nil {
		client.SendServerMessage("Invalid duration format. Use values like 1000h, 3d12h30m, or 90m.")
		return
	}
	if duration < 0 {
		client.SendServerMessage("Duration cannot be negative.")
		return
	}
	setSeconds := int64(duration.Seconds())

	ipid, err := db.GetIPIDByUsername(targetUser)
	if err != nil {
		client.SendServerMessage("Failed to look up account: " + err.Error())
		return
	}
	if ipid == "" {
		client.SendServerMessage(fmt.Sprintf(
			"Account '%v' was not found, or has never logged in (no IPID linked yet).",
			targetUser))
		return
	}

	if err := db.MarkIPKnown(ipid); err != nil {
		client.SendServerMessage("Failed to update playtime: " + err.Error())
		return
	}

	newTotal, err := db.SetPlaytimeReturning(ipid, setSeconds)
	if err != nil {
		client.SendServerMessage("Failed to update playtime: " + err.Error())
		return
	}

	client.SendServerMessage(fmt.Sprintf(
		"✅ Set playtime for '%v' to %v.", targetUser, formatPlaytime(newTotal)))

	addToBuffer(client, "CMD",
		fmt.Sprintf("Set playtime for '%v' to %vs.", targetUser, newTotal),
		true)
}

// cmdProfile implements /profile [uid]. With no argument, shows the caller's
// own profile card; with a UID, shows the profile card of the target online
// player. Visible to everyone — the card aggregates data that is already
// readable via /account, /playtime, /chips, /wardrobe.
func cmdProfile(client *Client, args []string, _ string) {
	target := client
	if len(args) > 0 {
		uid, err := strconv.Atoi(args[0])
		if err != nil {
			client.SendServerMessage("Invalid UID.")
			return
		}
		t, err := getClientByUid(uid)
		if err != nil {
			client.SendServerMessage("Client does not exist.")
			return
		}
		target = t
	}

	// Resolve the display name: showname -> character -> "(no character)".
	displayName := clientDisplayName(target)
	if strings.TrimSpace(displayName) == "" {
		displayName = "(no character selected)"
	}

	// Look up any linked account username for this IPID. Viewing your own
	// profile always resolves it (it's your own information, and it's a
	// handy way to check whether this connection is tied to an account at
	// all). Viewing someone else's profile only resolves it if THEY are
	// currently authenticated (ran /login or /register this connection) —
	// an IPID can stay linked to an account from a past session long after
	// that session ends, and showing the account name from that residual
	// link would out an anonymous-looking player as a specific known
	// account without their say-so. Logging in is an explicit statement
	// "yes, this connection is me"; a stale IPID link is not.
	isSelfProfile := target == client
	username := "(guest)"
	if isSelfProfile || target.Authenticated() {
		if u, err := db.GetUsernameByIPID(target.Ipid()); err == nil && u != "" {
			username = u
		}
	}

	// Chips + playtime: pulled from the account-stats table; session time is
	// added on top for a live total, matching /account's behaviour.
	chips, playtimeSec, err := db.GetAccountStats(target.Ipid())
	if err != nil {
		chips = 0
		playtimeSec = 0
	}
	if connAt := target.ConnectedAt(); !connAt.IsZero() {
		playtimeSec += int64(time.Since(connAt).Seconds())
	}

	// The exact playtime figure is itself identifying: /playtime top lists
	// every registered account's hours by username, so even with the account
	// line above hidden, showing the raw number here would let anyone match
	// "this anonymous connection has 47h32m" against that public,
	// named leaderboard and deanonymize the target just the same. Gated by
	// the same isSelfProfile/Authenticated rule as the account line, for the
	// same reason.
	playtimeDisplay := formatPlaytime(playtimeSec)
	if !isSelfProfile && !target.Authenticated() {
		playtimeDisplay = "(hidden)"
	}

	// Favourite characters (wardrobe): keyed by account username, so this
	// already cascades from the same gated `username` value above — a
	// logged-out target's wardrobe stays hidden alongside their account for
	// the identical reason, with no separate check needed.
	var favs []string
	if username != "(guest)" {
		if f, err := db.GetFavourites(username); err == nil {
			favs = f
		}
	}
	favsDisplay := "(none)"
	if len(favs) > 0 {
		if len(favs) > 8 {
			favsDisplay = strings.Join(favs[:8], ", ") + fmt.Sprintf(", … (+%d more)", len(favs)-8)
		} else {
			favsDisplay = strings.Join(favs, ", ")
		}
	}

	// Active cosmetic tag (casino or non-casino modes both expose this).
	// Gated the same way as the account/playtime lines above: an equipped
	// tag is persisted by IPID like everything else here, so a logged-out
	// target's past tag choice (sometimes a one-of-a-kind custom tag naming
	// its owner outright) is just as identifying as their account name or
	// hours would be.
	tagDisplay := "(none)"
	if isSelfProfile || target.Authenticated() {
		if t := formatTagDisplay(db.GetActiveTag(target.Ipid())); t != "" {
			tagDisplay = t
		}
	} else {
		tagDisplay = "(hidden)"
	}

	// Active punishments count — don't leak details, just a count so players
	// can tell at a glance if the target is currently cursed with something.
	activePunishments := len(target.Punishments())

	areaName := "(unknown)"
	if a := target.Area(); a != nil {
		areaName = a.Name()
	}

	var sb strings.Builder
	sb.WriteString("\n👤 Profile\n")
	sb.WriteString(fmt.Sprintf("  Name:        %v\n", displayName))
	sb.WriteString(fmt.Sprintf("  UID:         %d\n", target.Uid()))
	sb.WriteString(fmt.Sprintf("  Account:     %v\n", username))
	sb.WriteString(fmt.Sprintf("  Area:        %v\n", areaName))
	sb.WriteString(fmt.Sprintf("  Playtime:    %v\n", playtimeDisplay))
	if config != nil && config.EnableCasino {
		sb.WriteString(fmt.Sprintf("  Chips:       %d\n", chips))
	}
	sb.WriteString(fmt.Sprintf("  Active tag:  %v\n", tagDisplay))
	sb.WriteString(fmt.Sprintf("  Favourites:  %v\n", favsDisplay))
	// DJ insignia: vinyl record next to the music line so it's obvious at a
	// glance whether the player has DJ privileges. Mods see no badge here —
	// they have their own staff lines and shouldn't double up.
	if permissions.HasPermission(target.Perms(), permissions.PermissionField["DJ"]) &&
		!permissions.IsModerator(target.Perms()) {
		sb.WriteString("  Music:       💿 DJ\n")
	}
	if activePunishments > 0 {
		sb.WriteString(fmt.Sprintf("  Punishments: %d active\n", activePunishments))
	}
	client.SendServerMessage(sb.String())
}

// Handles /resetusername <new-username>
//
// Lets a logged-in player rename their account without losing their
// playtime, chips, wardrobe, tags, or anything else tied to their account.
// Capped at db.MaxUsernameResets renames per account so the system can't be
// abused for impersonation churn.
func cmdResetUsername(client *Client, args []string, _ string) {
	if !client.Authenticated() {
		client.SendServerMessage("You must be logged in to rename your account. Use /login <username> <password> first.")
		return
	}
	oldName := client.ModName()
	if oldName == "" {
		client.SendServerMessage("Could not determine your account name.")
		return
	}
	newName := args[0]
	if !validUsernameRe.MatchString(newName) {
		client.SendServerMessage(
			"❌ That username isn't valid.\n" +
				"Usernames must be 3–20 characters and may only contain letters (A–Z, a–z), digits (0–9), and underscores (_).")
		return
	}
	if newName == oldName {
		client.SendServerMessage("Your new username must be different from your current one.")
		return
	}
	if db.IsModUser(newName) {
		client.SendServerMessage(fmt.Sprintf("'%v' is reserved by a staff account. Please pick a different name.", newName))
		return
	}
	resets, err := db.GetUsernameResets(oldName)
	if err != nil {
		logger.LogErrorf("resetusername: read counter for %v: %v", oldName, err)
		client.SendServerMessage("Rename failed. Please try again later.")
		return
	}
	if resets >= db.MaxUsernameResets {
		client.SendServerMessage(fmt.Sprintf(
			"You've already used all %d username changes on this account. The cap exists to keep impersonation in check — staff can override it if needed.",
			db.MaxUsernameResets))
		return
	}
	switch err := db.RenameAccount(oldName, newName); {
	case err == db.ErrUsernameTaken:
		client.SendServerMessage(fmt.Sprintf("'%v' is already in use by another account. Pick a different name.", newName))
		return
	case err == db.ErrUsernameLimit:
		client.SendServerMessage(fmt.Sprintf("You've already used all %d username changes on this account.", db.MaxUsernameResets))
		return
	case err != nil:
		logger.LogErrorf("resetusername: rename %v -> %v: %v", oldName, newName, err)
		client.SendServerMessage("Rename failed. Please try again later.")
		return
	}
	client.SetModName(newName)
	remaining := db.MaxUsernameResets - (resets + 1)
	client.SendServerMessage(fmt.Sprintf(
		"✅ Renamed '%v' → '%v'. (%d rename(s) remaining on this account.)",
		oldName, newName, remaining))
	addToBuffer(client, "CMD", fmt.Sprintf("Renamed account %v -> %v.", oldName, newName), false)
}

// Copyright (C) 2026 SyntaxNyah
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published
// by the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

// Account-based per-command permission grants.
//
// The server's role system (roles.toml -> permission bitfield) is coarse:
// every command behind a given bit (MUTE, BAN, ADMIN, ...) opens up together.
// That is the right shape for a moderator, but it has no way to say "this one
// account may run /ban and nothing else" without also handing over BAN's
// other ~15 commands, or inventing a new role and a new bit for every such
// request.
//
// ACCOUNT_COMMAND_GRANTS is a second, independent axis alongside that bitfield:
// a console operator (shell/VPS access to the box, not anything reachable
// in-game) can grant one specific command to one specific account by username
// -- the "hundreds of account usernames" already created via /register or
// /mkusr -- without changing that account's role at all. A grant is checked
// purely by command name and only ever adds access on top of whatever the
// account's role already allows; it can never take access away, and it does
// not touch client.perms (so IsModerator/IsAdmin/IsShadow, shadow-mod
// anonymization, DisplayModName, and every other role-based behavior stay
// exactly as they were for a granted account -- it is still a plain player
// or a plain moderator in every other respect, just one that can also run
// this one extra command).
//
// This deliberately mirrors the account-based (not role-based) shape the
// rest of the account system already uses: /createtag, /grantcustomtag,
// /musicban and friends are all keyed by account, not by a bit added to
// roles.toml. It reuses the exact same USERS table both moderator accounts
// (/mkusr) and player accounts (/register) share, so the same mechanism
// covers both "give a plain registered player the ability to /ban someone"
// and "give a moderator who doesn't have BAN a couple of admin commands
// without promoting them to admin" -- both are just a username with a grant.
//
// Granting is console-only, matching the precedent set by the removed
// `grant` console gate (see "Removed: Console-Gated Commands" in CLAUDE.md):
// a capability this broad -- literally any command in the registry, aimed at
// any of potentially hundreds of accounts -- belongs behind shell access to
// the box, not an in-game command any moderator could reach.
package athena

import (
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/MangosArentLiterature/Athena/internal/db"
	"github.com/MangosArentLiterature/Athena/internal/logger"
)

// commandGrantsPtr holds username -> set of lowercase command names granted
// to that username. Published atomically so the hot dispatch path
// (clientCanUseCommand, called on every slash command) never takes a lock or
// hits the database -- it is a single atomic load followed by two map reads.
// Rebuilt wholesale and republished on every console grant/revoke and at
// startup; readers never mutate the returned map.
var commandGrantsPtr atomic.Pointer[map[string]map[string]struct{}]

// getCommandGrants returns the current grant snapshot. Never nil.
func getCommandGrants() map[string]map[string]struct{} {
	if v := commandGrantsPtr.Load(); v != nil {
		return *v
	}
	return nil
}

func setCommandGrants(m map[string]map[string]struct{}) { commandGrantsPtr.Store(&m) }

// buildCommandGrants turns a flat DB row list into the nested username ->
// command-set map the hot path reads.
func buildCommandGrants(rows []db.CommandGrantInfo) map[string]map[string]struct{} {
	m := make(map[string]map[string]struct{}, len(rows))
	for _, r := range rows {
		set, ok := m[r.Username]
		if !ok {
			set = make(map[string]struct{})
			m[r.Username] = set
		}
		set[strings.ToLower(r.Command)] = struct{}{}
	}
	return m
}

// loadCommandGrants reads every grant from the database and publishes it as
// the live snapshot. Called once at startup and again after every console
// grantcmd/revokecmd so the in-memory cache never drifts from the DB.
func loadCommandGrants() error {
	rows, err := db.ListAllCommandGrants()
	if err != nil {
		return err
	}
	setCommandGrants(buildCommandGrants(rows))
	return nil
}

// accountHasCommandGrant reports whether username has been explicitly
// granted command (case-insensitive on both). username == "" (never
// authenticated) always returns false.
func accountHasCommandGrant(username, command string) bool {
	if username == "" {
		return false
	}
	set, ok := getCommandGrants()[username]
	if !ok {
		return false
	}
	_, ok = set[strings.ToLower(command)]
	return ok
}

// clientHasCommandGrant is the call site helper: true when client is logged
// into an account (moderator or plain player, via /login or /register) that
// has been console-granted command. A disconnect never invalidates this --
// it is re-checked from the live cache on every call, keyed by account
// username rather than by connection.
func clientHasCommandGrant(client *Client, command string) bool {
	return client.Authenticated() && accountHasCommandGrant(client.ModName(), command)
}

// grantCommand records a console-issued grant of command to username and
// refreshes the live cache. command must name a real, currently-registered
// command (checked against the Commands registry so a typo fails loudly at
// grant time instead of silently granting nothing); it is normalized to
// lowercase before storage. Returns a human-readable description of what was
// granted for the console to echo back.
func grantCommand(username, command, grantedBy string) (string, error) {
	command = strings.ToLower(strings.TrimSpace(command))
	if command == "" {
		return "", fmt.Errorf("empty command name")
	}
	if _, ok := Commands[command]; !ok {
		return "", fmt.Errorf("no such command %q (see /help in-game, or the Commands registry, for valid names)", command)
	}
	if err := db.AddCommandGrant(username, command, grantedBy, time.Now().Unix()); err != nil {
		return "", err
	}
	if err := loadCommandGrants(); err != nil {
		// The DB write already succeeded; only the in-memory cache refresh
		// failed, so the grant is real but won't take effect until the next
		// successful reload (a server restart also reloads it at startup).
		logger.LogErrorf("grantcmd: DB write for %v/%v succeeded but cache reload failed: %v", username, command, err)
		return "", fmt.Errorf("grant saved but the live cache failed to refresh: %w (it will still apply after a restart)", err)
	}
	return fmt.Sprintf("Granted %q to account %q.", command, username), nil
}

// revokeCommand removes one console-issued grant and refreshes the cache.
func revokeCommand(username, command string) error {
	command = strings.ToLower(strings.TrimSpace(command))
	if err := db.RemoveCommandGrant(username, command); err != nil {
		return err
	}
	return loadCommandGrants()
}

// revokeAllCommandGrants removes every grant held by username, returning how
// many were removed, and refreshes the cache.
func revokeAllCommandGrants(username string) (int64, error) {
	n, err := db.RemoveAllCommandGrants(username)
	if err != nil {
		return 0, err
	}
	if n > 0 {
		if err := loadCommandGrants(); err != nil {
			return n, err
		}
	}
	return n, nil
}

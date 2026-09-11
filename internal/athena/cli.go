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
	"bufio"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/MangosArentLiterature/Athena/internal/db"
	"github.com/MangosArentLiterature/Athena/internal/logger"
)

// ListenInput listens for input on stdin, parsing any commands.
func ListenInput() {
	input := bufio.NewScanner(os.Stdin)
	for input.Scan() {
		cmd := strings.Split(input.Text(), " ")
		cmd[0] = strings.TrimPrefix(cmd[0], "/")
		switch cmd[0] {
		case "help":
			logger.LogInfo("Recognized commands: help, mkusr, rmusr, players, getlog, say, reload, punishment, torment, untorment, grantcmd, revokecmd, grants.")
			logger.LogInfo("  torment <ipid>           Manually add an IPID to the torment list (no in-game equivalent).")
			logger.LogInfo("  untorment <ipid|all>     Remove one or every IPID from the torment list.")
			logger.LogInfo("  grantcmd <username> <command1>[,<command2>...]")
			logger.LogInfo("                           Let one account use one or more specific in-game commands, regardless")
			logger.LogInfo("                           of its role -- e.g. hand a plain player /ban without making them a mod,")
			logger.LogInfo("                           or hand a moderator a couple of admin commands without promoting them.")
			logger.LogInfo("                           Console-only by design; there is no in-game equivalent. Username is")
			logger.LogInfo("                           case-sensitive (must match the account's USERS row exactly).")
			logger.LogInfo("  revokecmd <username> <command1>[,<command2>...]|all")
			logger.LogInfo("                           Revoke one, several, or (with \"all\") every command grant on an account.")
			logger.LogInfo("  grants [username]        List every command grant on the server, or just one account's.")
		case "reload":
			// Full hot-reload: characters.txt (append-only), music.txt, cdns.txt,
			// backgrounds.txt, parrot.txt, 8ball.txt, banned_words.txt and the
			// config.toml motd/desc fields. See ReloadConfig in livereload.go.
			summary, err := ReloadConfig()
			if err != nil {
				logger.LogErrorf("reload failed: %v", err)
				break
			}
			logger.LogInfof("reload ok: %s", summary)
		case "mkusr":
			if len(cmd) < 4 {
				logger.LogInfo("Not enough arguments for command mkusr. Usage: mkusr <username> <password> <role>.")
				break
			}
			if db.UserExists(cmd[1]) {
				logger.LogInfo("User already exists.")
				return
			}
			user := cmd[1]
			pass := cmd[2]
			role, err := getRole(cmd[3])
			if err != nil {
				logger.LogInfo("Invalid role.")
				break
			}

			err = db.CreateUser(user, []byte(pass), role.GetPermissions())
			if err != nil {
				logger.LogInfof("Failed to create user: %v.", err.Error())
				break
			}
			logger.LogInfof("Sucessfully created user %v.", user)
		case "rmusr":
			if len(cmd) < 2 {
				logger.LogInfo("Not enough arguments for command rmusr. Usage: rmusr <username>.")
				break
			}
			if !db.UserExists(cmd[1]) {
				logger.LogInfo("User does not exist.")
			}
			err := db.RemoveUser(cmd[1])
			if err != nil {
				logger.LogInfof("Failed to remove user: %v.", err.Error())
				break
			}
			logger.LogInfof("Sucessfully removed user %v.", cmd[1])
		case "players":
			logger.LogInfof("There are currently %v/%v players online.", players.GetPlayerCount(), config.MaxPlayers)
		case "getlog":
			if len(cmd) < 2 {
				logger.LogInfo("Not enough arguments for command getlog. Usage: getlog <area>.")
				break
			}
			for _, a := range areas {
				if a.Name() == cmd[1] {
					logger.LogInfo(strings.Join(a.Buffer(), "\n"))
				}
			}
		case "punishment":
			// Console-only global kill switch for the punishment system.
			// Outranks every in-game authority tier — there is deliberately
			// no in-game or Discord command that can reach this.
			if len(cmd) < 2 {
				logger.LogInfo("Usage: punishment <enable|disable|status>")
				break
			}
			switch strings.ToLower(cmd[1]) {
			case "disable":
				SetPunishmentsGloballyDisabled(true)
				logger.LogInfo("Punishment system globally DISABLED. Only /ban, /kick, and /mute remain active for moderators.")
			case "enable":
				SetPunishmentsGloballyDisabled(false)
				logger.LogInfo("Punishment system globally ENABLED.")
			case "status":
				if PunishmentsGloballyDisabled() {
					logger.LogInfo("Punishment system is currently DISABLED (console override).")
				} else {
					logger.LogInfo("Punishment system is currently ENABLED.")
				}
			default:
				logger.LogInfo("Usage: punishment <enable|disable|status>")
			}
		case "torment":
			// The manual half of the torment list. /lag used to do this in
			// game and has been removed; the automatic AutoMod path that arms
			// it on a censor trip is untouched.
			if len(cmd) < 2 {
				logger.LogInfo("Usage: torment <ipid>")
				break
			}
			added, note := tormentIPIDFromConsole(cmd[1])
			logger.LogInfo(note)
			if added {
				logger.LogInfo("Remove it in game with /untorment <ipid>, or here with: untorment <ipid>")
			}
		case "untorment":
			if len(cmd) < 2 {
				logger.LogInfo("Usage: untorment <ipid|all>")
				break
			}
			logger.LogInfo(untormentFromConsole(cmd[1]))
		case "grantcmd":
			// Console-only: hand one account (moderator or plain player,
			// whatever the "hundreds of account usernames" from /register or
			// /mkusr already are) the ability to use one or more specific
			// in-game commands, without touching its role/permission
			// bitfield at all. See command_grants.go for the full mechanism
			// and why this deliberately has no in-game equivalent.
			if len(cmd) < 3 {
				logger.LogInfo("Usage: grantcmd <username> <command1>[,<command2>,...]")
				break
			}
			username := cmd[1]
			if !db.UserExists(username) {
				logger.LogInfof("No account named %q exists (checked case-sensitively). Nothing granted.", username)
				break
			}
			var granted, failed []string
			for _, c := range strings.Split(cmd[2], ",") {
				c = strings.TrimSpace(strings.TrimPrefix(c, "/"))
				if c == "" {
					continue
				}
				msg, err := grantCommand(username, c, "console")
				if err != nil {
					failed = append(failed, c+": "+err.Error())
					continue
				}
				granted = append(granted, msg)
			}
			for _, m := range granted {
				logger.LogInfo(m)
			}
			for _, f := range failed {
				logger.LogInfof("Failed to grant %v", f)
			}
			if len(granted) > 0 {
				logger.LogInfof("Revoke with: revokecmd %v <command>", username)
			}
		case "revokecmd":
			if len(cmd) < 3 {
				logger.LogInfo("Usage: revokecmd <username> <command1>[,<command2>,...]|all")
				break
			}
			username := cmd[1]
			if strings.EqualFold(cmd[2], "all") {
				n, err := revokeAllCommandGrants(username)
				if err != nil {
					logger.LogInfof("Failed to revoke grants for %q: %v", username, err)
					break
				}
				logger.LogInfof("Revoked %d command grant(s) from %q.", n, username)
				break
			}
			for _, c := range strings.Split(cmd[2], ",") {
				c = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(c, "/")))
				if c == "" {
					continue
				}
				if err := revokeCommand(username, c); err != nil {
					logger.LogInfof("%v does not currently hold a grant for %q (or revoke failed: %v).", username, c, err)
					continue
				}
				logger.LogInfof("Revoked %q from %q.", c, username)
			}
		case "grants":
			if len(cmd) >= 2 && cmd[1] != "" {
				username := cmd[1]
				rows, err := db.ListCommandGrants(username)
				if err != nil {
					logger.LogInfof("Failed to list grants for %q: %v", username, err)
					break
				}
				if len(rows) == 0 {
					logger.LogInfof("%q has no command grants.", username)
					break
				}
				logger.LogInfof("Command grants for %q:", username)
				for _, g := range rows {
					logger.LogInfof("  %v (granted by %v at %v)", g.Command, g.GrantedBy, time.Unix(g.GrantedAt, 0).Format("2006-01-02 15:04:05"))
				}
				break
			}
			rows, err := db.ListAllCommandGrants()
			if err != nil {
				logger.LogInfof("Failed to list command grants: %v", err)
				break
			}
			if len(rows) == 0 {
				logger.LogInfo("No account currently holds a command grant.")
				break
			}
			byUser := make(map[string][]string)
			var users []string
			for _, g := range rows {
				if _, ok := byUser[g.Username]; !ok {
					users = append(users, g.Username)
				}
				byUser[g.Username] = append(byUser[g.Username], g.Command)
			}
			sort.Strings(users)
			logger.LogInfof("%d account(s) hold command grants:", len(users))
			for _, u := range users {
				logger.LogInfof("  %v: %v", u, strings.Join(byUser[u], ", "))
			}
		case "say":
			if len(cmd) < 2 {
				logger.LogInfo("Not enough arguments for command say. Usage: say <message>.")
				break
			}
			msg := cmd[1]
			clients.ForEach(func(c *Client) {
				c.SendServerMessage(msg)
			})
		default:
			logger.LogInfo("Unrecognized command")
		}
	}
}

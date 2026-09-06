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

// ============================================================
// /mafia command — Social Deduction Minigame
// ============================================================
//
// Subcommands (args[0]):
//   create          – create a lobby in current area
//   join            – join the lobby
//   leave           – leave the lobby or resign from game
//   players / status – list players / game state
//   start           – start the game (assign roles) [host/CM/mod]
//   vote <name>     – vote to lynch during Day phase
//   skip            – skip (no-lynch) vote
//   act <...>       – submit night action
//   shoot <name>    – Sheriff day shoot
//   day             – manually advance to Day [CM/mod]
//   night           – manually advance to Night [CM/mod]
//   resolve         – force-resolve current phase [CM/mod]
//   stop            – abort the game [CM/mod]
//   kick <name>     – kick a player from lobby/game [CM/mod]
//   reveal          – force-reveal all roles [CM/mod]
//   timer <secs>    – set phase auto-timer (0 = off) [CM/mod]
//   roles           – list all roles and descriptions
//   help            – usage text

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/MangosArentLiterature/Athena/internal/packet"
	"github.com/MangosArentLiterature/Athena/internal/permissions"
)

const mafiaUsage = `Usage: /mafia <subcommand> [args]
Subcommands:
  create                 — Open a new Mafia lobby in this area.
  join                   — Join the current lobby.
  leave                  — Leave the lobby or resign from an active game.
  players / status       — Show player list and game status.
  start                  — Start the game and assign roles. (host/CM/mod)
  vote <name>            — Vote to lynch a player during Day.
  skip                   — Clear your vote (no-lynch).
  tally                  — Show the current vote standings.
  act <target>           — Submit your night action (or Mayor reveal during Day).
    Arsonist:  act douse <target>  /  act ignite
    Witch:     act <player> <newtarget>
    Mayor:     act reveal  (Day only — announces you; doubles your vote)
  shoot <name>           — Sheriff: spend your one-time shot on a player.
  will <text>            — Set your last will (revealed publicly when you die).
  whisper <name> <msg>   — Send a private in-game message to another alive player.
  graveyard / dead       — Show the list of eliminated players and cause of death.
  day                    — Force advance to Day phase. (CM/mod)
  night                  — Force advance to Night phase. (CM/mod)
  resolve                — Force-resolve current phase. (CM/mod)
  stop                   — Abort the game. (CM/mod)
  kick <name>            — Kick a player from lobby or game. (CM/mod)
  reveal                 — Force-reveal all roles. (CM/mod)
  timer <secs>           — Set phase auto-timer; 0 = disabled. (CM/mod)
  roles                  — List all roles with descriptions.
  help                   — Show this help.`

const mafiaLobbyRules = `🎭 How to play Mafia:
Players are secretly assigned roles from three teams — Town, Mafia, or Neutral.
GOAL: Town must eliminate all Mafia/threats. Mafia must equal/outnumber Town.
PHASES:
  Day — Discuss and vote to lynch a suspect with /mafia vote <name>. Use /mafia skip to abstain. A player is lynched when they reach a simple majority of votes.
  Night — Special roles use /mafia act <target> to submit their action privately.
ROLES (examples): Villager (no ability), Detective (investigate alignment), Doctor (protect someone), Mafia (team kill each night), Jester (get lynched to win), and more.
Type /mafia roles to see all roles, or /mafia help for the full command list.`

// isMafiaCM returns true if the client is a CM in the area, mod, or the
// game host (first player to create the game).
func isMafiaCM(client *Client, g *MafiaGame) bool {
	if permissions.HasPermission(client.Perms(), permissions.PermissionField["CM"]) {
		return true
	}
	if client.Area().HasCM(client.Uid()) {
		return true
	}
	// First player in the list is the host
	if len(g.Players) > 0 && g.Players[0].Client == client {
		return true
	}
	return false
}

// cmdMafia is the entry-point handler for /mafia.
func cmdMafia(client *Client, args []string, _ string) {
	if len(args) == 0 {
		client.SendServerMessage(mafiaUsage)
		return
	}
	sub := strings.ToLower(args[0])
	rest := args[1:]

	switch sub {
	case "help":
		client.SendServerMessage(mafiaUsage)
	case "roles":
		mafiaSubRoles(client)
	case "create":
		mafiaSubCreate(client)
	case "join":
		mafiaSubJoin(client)
	case "leave":
		mafiaSubLeave(client)
	case "players", "status":
		mafiaSubStatus(client)
	case "start":
		mafiaSubStart(client)
	case "vote":
		mafiaSubVote(client, rest)
	case "skip":
		mafiaSubSkip(client)
	case "tally":
		mafiaSubTally(client)
	case "act":
		mafiaSubAct(client, rest)
	case "shoot":
		mafiaSubShoot(client, rest)
	case "will":
		mafiaSubWill(client, rest)
	case "whisper":
		mafiaSubWhisper(client, rest)
	case "graveyard", "dead":
		mafiaSubGraveyard(client)
	case "day":
		mafiaSubDay(client)
	case "night":
		mafiaSubNight(client)
	case "resolve":
		mafiaSubResolve(client)
	case "stop":
		mafiaSubStop(client)
	case "kick":
		mafiaSubKick(client, rest)
	case "reveal":
		mafiaSubReveal(client)
	case "timer":
		mafiaSubTimer(client, rest)
	default:
		client.SendServerMessage("Unknown subcommand. Type /mafia help for usage.")
	}
}

// ── create ─────────────────────────────────────────────────────────────────

func mafiaSubCreate(client *Client) {
	a := client.Area()
	existing := getMafiaGame(a)
	if existing != nil {
		client.SendServerMessage("A Mafia game already exists in this area. Use /mafia join to join, or /mafia stop to abort it.")
		return
	}
	g := newMafiaGame(a)
	g.mu.Lock()
	g.Players = append(g.Players, &MafiaPlayer{Client: client, Alive: false})
	g.mu.Unlock()
	sendAreaServerMessage(a, fmt.Sprintf("🎭 %v created a Mafia lobby! Type /mafia join to join.", client.OOCName()))
	sendAreaServerMessage(a, mafiaLobbyRules)
	client.SendServerMessage("You are the host. Use /mafia start when ready (min 3 players).")
}

// ── join ───────────────────────────────────────────────────────────────────

func mafiaSubJoin(client *Client) {
	a := client.Area()
	g := getMafiaGame(a)
	if g == nil {
		client.SendServerMessage("No Mafia lobby in this area. Use /mafia create to start one.")
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.Phase != MafiaPhaseLobby {
		client.SendServerMessage("The game is already in progress. You cannot join mid-game.")
		return
	}
	for _, p := range g.Players {
		if p.Client == client {
			client.SendServerMessage("You are already in the lobby.")
			return
		}
	}
	g.Players = append(g.Players, &MafiaPlayer{Client: client, Alive: false})
	sendAreaServerMessage(a, fmt.Sprintf("🎭 %v joined the Mafia lobby! (%d players)", client.OOCName(), len(g.Players)))
}

// ── leave ──────────────────────────────────────────────────────────────────

func mafiaSubLeave(client *Client) {
	a := client.Area()
	g := getMafiaGame(a)
	if g == nil {
		client.SendServerMessage("No active Mafia game in this area.")
		return
	}
	g.mu.Lock()
	p := g.findPlayerByClient(client)
	if p == nil {
		g.mu.Unlock()
		client.SendServerMessage("You are not in the Mafia game.")
		return
	}
	name := p.Name()
	phase := g.Phase
	wasAlive := p.Alive

	if phase == MafiaPhaseLobby {
		newList := make([]*MafiaPlayer, 0, len(g.Players)-1)
		for _, pp := range g.Players {
			if pp != p {
				newList = append(newList, pp)
			}
		}
		g.Players = newList
		g.mu.Unlock()
		sendAreaServerMessage(a, fmt.Sprintf("🎭 %v left the Mafia lobby. (%d players)", name, len(g.Players)))
		return
	}

	// Mid-game: mark dead
	if wasAlive {
		p.Alive = false
		g.addToGraveyard(p, "Resigned")
	}
	winner, won := g.checkWin()
	g.mu.Unlock()

	if wasAlive {
		g.broadcastToGame(fmt.Sprintf("🚪 %v has resigned from the game.", name))
		if won {
			g.endGame(winner)
		}
	} else {
		client.SendServerMessage("You are already dead.")
	}
}

// ── status / players ───────────────────────────────────────────────────────

func mafiaSubStatus(client *Client) {
	a := client.Area()
	g := getMafiaGame(a)
	if g == nil {
		client.SendServerMessage("No active Mafia game in this area.")
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()

	var sb strings.Builder
	sb.Grow(64 + len(g.Players)*48)
	fmt.Fprintf(&sb, "\n🎭 Mafia Status — %v\n", a.Name())
	sb.WriteString("Phase: ")
	sb.WriteString(g.Phase.String())
	fmt.Fprintf(&sb, "  |  Day: %d  |  Timer: ", g.Day)
	if g.PhaseSecs > 0 {
		fmt.Fprintf(&sb, "%ds\n", g.PhaseSecs)
	} else {
		sb.WriteString("manual\n")
	}
	fmt.Fprintf(&sb, "Players (%d):\n", len(g.Players))
	for _, p := range g.Players {
		status := "✅"
		if !p.Alive && g.Phase != MafiaPhaseLobby {
			status = "💀"
		}
		// Show own role to the caller
		roleName := ""
		if p.Client == client || g.Phase == MafiaPhaseEnded {
			roleName = " [" + roleInfoMap[p.Role].Name + "]"
		}
		fmt.Fprintf(&sb, "  %v %v%v\n", status, p.Name(), roleName)
	}
	client.SendServerMessage(sb.String())
}

// ── start ──────────────────────────────────────────────────────────────────

func mafiaSubStart(client *Client) {
	a := client.Area()
	g := getMafiaGame(a)
	if g == nil {
		client.SendServerMessage("No Mafia lobby in this area. Use /mafia create first.")
		return
	}
	g.mu.Lock()
	if g.Phase != MafiaPhaseLobby {
		g.mu.Unlock()
		client.SendServerMessage("The game has already started.")
		return
	}
	if !isMafiaCM(client, g) {
		g.mu.Unlock()
		client.SendServerMessage("Only the host, a CM, or a moderator can start the game.")
		return
	}
	if len(g.Players) < 3 {
		g.mu.Unlock()
		client.SendServerMessage("Need at least 3 players to start.")
		return
	}
	g.assignRoles()
	g.mu.Unlock()

	sendAreaServerMessage(a, fmt.Sprintf("🎭 The Mafia game starts with %d players! Check your private role message.", len(g.Players)))

	// Send each player their role privately
	g.mu.Lock()
	for _, p := range g.Players {
		info := roleInfoMap[p.Role]
		g.privateMsg(p, fmt.Sprintf("Your role: %v (%v team)\n%v\nWin condition: %v\nAbility: %v",
			info.Name, info.Team, info.Desc, info.WinCond, info.Ability))
	}
	g.mu.Unlock()

	g.startDay(1)
}

// ── vote ───────────────────────────────────────────────────────────────────

func mafiaSubVote(client *Client, args []string) {
	if len(args) == 0 {
		client.SendServerMessage("Usage: /mafia vote <player name>")
		return
	}
	targetName := strings.Join(args, " ")
	a := client.Area()
	g := getMafiaGame(a)
	if g == nil {
		client.SendServerMessage("No active Mafia game in this area.")
		return
	}
	g.mu.Lock()
	if g.Phase != MafiaPhaseDayDiscussion {
		g.mu.Unlock()
		client.SendServerMessage("Voting is only available during the Day phase.")
		return
	}
	voter := g.findPlayerByClient(client)
	if voter == nil || !voter.Alive {
		g.mu.Unlock()
		client.SendServerMessage("You are not an alive player in this game.")
		return
	}
	// Validate target exists and is alive
	target := g.findPlayer(targetName)
	if target == nil || !target.Alive {
		g.mu.Unlock()
		client.SendServerMessage(fmt.Sprintf("No alive player named %q found.", targetName))
		return
	}
	voter.VoteTarget = target.Name()

	// Tally (Mayor counts double when revealed)
	tally := make(map[string]int)
	for _, p := range g.Players {
		if p.Alive && p.VoteTarget != "" {
			weight := 1
			if p.Role == RoleMayor && p.MayorRevealed {
				weight = 2
			}
			tally[strings.ToLower(p.VoteTarget)] += weight
		}
	}
	aliveCount := g.aliveCount()
	majority := aliveCount/2 + 1
	voteWeight := 1
	if voter.Role == RoleMayor && voter.MayorRevealed {
		voteWeight = 2
	}
	targetLower := strings.ToLower(target.Name())
	g.mu.Unlock()

	voteLabel := ""
	if voteWeight == 2 {
		voteLabel = " (×2 Mayor vote)"
	}
	sendAreaServerMessage(a, fmt.Sprintf("🗳️  %v votes to lynch %v%v. (%d/%d needed)", voter.Name(), target.Name(), voteLabel, tally[targetLower], majority))

	// Auto-resolve if majority reached
	if tally[targetLower] >= majority {
		g.resolveDay()
	}
}

// ── skip ───────────────────────────────────────────────────────────────────

func mafiaSubSkip(client *Client) {
	a := client.Area()
	g := getMafiaGame(a)
	if g == nil {
		client.SendServerMessage("No active Mafia game in this area.")
		return
	}
	g.mu.Lock()
	if g.Phase != MafiaPhaseDayDiscussion {
		g.mu.Unlock()
		client.SendServerMessage("Skip is only available during the Day phase.")
		return
	}
	voter := g.findPlayerByClient(client)
	if voter == nil || !voter.Alive {
		g.mu.Unlock()
		client.SendServerMessage("You are not an alive player in this game.")
		return
	}
	voter.VoteTarget = ""
	g.mu.Unlock()
	client.SendServerMessage("You have cleared your vote (no-lynch).")
}

// ── tally ──────────────────────────────────────────────────────────────────

func mafiaSubTally(client *Client) {
	a := client.Area()
	g := getMafiaGame(a)
	if g == nil {
		client.SendServerMessage("No active Mafia game in this area.")
		return
	}
	g.mu.Lock()
	if g.Phase != MafiaPhaseDayDiscussion {
		g.mu.Unlock()
		client.SendServerMessage("Vote tally is only available during the Day phase.")
		return
	}
	tally := make(map[string]int)
	for _, p := range g.Players {
		if p.Alive && p.VoteTarget != "" {
			weight := 1
			if p.Role == RoleMayor && p.MayorRevealed {
				weight = 2
			}
			tally[strings.ToLower(p.VoteTarget)] += weight
		}
	}
	aliveCount := g.aliveCount()
	majority := aliveCount/2 + 1
	g.mu.Unlock()

	if len(tally) == 0 {
		client.SendServerMessage("🗳️  No votes have been cast yet.")
		return
	}
	var sb strings.Builder
	sb.Grow(48 + len(tally)*32)
	fmt.Fprintf(&sb, "🗳️  Vote Tally (need %d for majority):\n", majority)
	for name, votes := range tally {
		fmt.Fprintf(&sb, "  %v — %d vote(s)\n", name, votes)
	}
	client.SendServerMessage(sb.String())
}

// ── will ───────────────────────────────────────────────────────────────────

func mafiaSubWill(client *Client, args []string) {
	if len(args) == 0 {
		client.SendServerMessage("Usage: /mafia will <your last will text>")
		return
	}
	a := client.Area()
	g := getMafiaGame(a)
	if g == nil {
		client.SendServerMessage("No active Mafia game in this area.")
		return
	}

	will := strings.Join(args, " ")
	// A last will is stored now and broadcast area-wide, unattended, whenever
	// its owner dies or is lynched (see broadcastToGame in mafia.go) -- there
	// is no delivery happening "now" for oocCommandAllowed's torment/
	// stealthmute/captcha suppression branches to hold back. Mirrors
	// cmdAreaRename's store-now/broadcast-later content check instead:
	// reject the content before it is ever stored, so it can never surface
	// later at a moment nothing is gating.
	m, result, kickAfter := autoModCheckTiered(client, will, "mafia will")
	if m.Matched && m.Entry.Severity == SeverityNuke {
		applyAutoModNuke(client, m, "mafia will")
		return
	}
	raidGuardOnWordHit(client, m)
	switch result {
	case autoModBlocked:
		if kickAfter {
			client.KickForCensorTrip()
		}
		return
	case autoModShadow:
		// Shadow semantics: the sender is told it worked, but nothing is
		// stored, so nobody will ever see it.
		client.SendServerMessage("📜 Last will set. It will be revealed when you die.")
		if kickAfter {
			client.KickForCensorTrip()
		}
		return
	}

	g.mu.Lock()
	p := g.findPlayerByClient(client)
	if p == nil {
		g.mu.Unlock()
		client.SendServerMessage("You are not in the current Mafia game.")
		return
	}
	p.LastWill = will
	g.mu.Unlock()
	client.SendServerMessage("📜 Last will set. It will be revealed when you die.")
}

// ── whisper ────────────────────────────────────────────────────────────────

func mafiaSubWhisper(client *Client, args []string) {
	if len(args) < 2 {
		client.SendServerMessage("Usage: /mafia whisper <player name> <message>")
		return
	}
	a := client.Area()
	g := getMafiaGame(a)
	if g == nil {
		client.SendServerMessage("No active Mafia game in this area.")
		return
	}
	g.mu.Lock()
	if g.Phase == MafiaPhaseEnded || g.Phase == MafiaPhaseLobby {
		g.mu.Unlock()
		client.SendServerMessage("Whispers are only available during an active game.")
		return
	}
	sender := g.findPlayerByClient(client)
	if sender == nil || !sender.Alive {
		g.mu.Unlock()
		client.SendServerMessage("You are not an alive player in this game.")
		return
	}
	// Try longest-prefix matching for the target name
	var target *MafiaPlayer
	var msgStart int
	for split := len(args) - 1; split >= 1; split-- {
		candidate := strings.Join(args[:split], " ")
		if tp := g.findPlayer(candidate); tp != nil && tp.Alive && tp != sender {
			target = tp
			msgStart = split
			break
		}
	}
	if target == nil {
		g.mu.Unlock()
		client.SendServerMessage("Could not find an alive player with that name. Usage: /mafia whisper <player> <message>")
		return
	}
	message := strings.Join(args[msgStart:], " ")
	area := g.Area
	g.mu.Unlock()

	// A whisper delivers free player text directly to another player -- the
	// same shape as /pm -- and took the same unexamined path. echo mirrors
	// what the target is about to receive, so a suppressed whisper still
	// leaves the sender believing it went through (shadow semantics).  Not
	// raid-guard scored, matching /pm's own precedent: a whisper is not a
	// broadcast and there's no correlation data to calibrate against.
	echo := &packet.CTToClient{
		Name:         encodedServerName,
		Message:      encode(fmt.Sprintf("🎭 [Mafia/Private] 💬 [Whisper from %v]: %v", sender.Name(), message)),
		IsFromServer: "1",
	}
	if !oocCommandAllowed(client, message, "mafia whisper", echo) {
		return
	}

	// Target receives the actual message
	g.privateMsg(target, fmt.Sprintf("💬 [Whisper from %v]: %v", sender.Name(), message))
	// Area sees a notice (no content)
	sendAreaServerMessage(area, fmt.Sprintf("💬 [Whisper] %v → %v (message hidden)", sender.Name(), target.Name()))
}

// ── graveyard ──────────────────────────────────────────────────────────────

func mafiaSubGraveyard(client *Client) {
	a := client.Area()
	g := getMafiaGame(a)
	if g == nil {
		client.SendServerMessage("No active Mafia game in this area.")
		return
	}
	g.mu.Lock()
	if len(g.Graveyard) == 0 {
		g.mu.Unlock()
		client.SendServerMessage("⚰️  The graveyard is empty.")
		return
	}
	var sb strings.Builder
	sb.Grow(32 + len(g.Graveyard)*80)
	sb.WriteString("⚰️  Graveyard:\n")
	for _, e := range g.Graveyard {
		info := roleInfoMap[e.Role]
		fmt.Fprintf(&sb, "  Day %d — %v (%v, %v team) — %v\n", e.Day, e.Name, info.Name, info.Team, e.Cause)
		if e.LastWill != "" {
			fmt.Fprintf(&sb, "    📜 Will: %v\n", e.LastWill)
		}
	}
	g.mu.Unlock()
	client.SendServerMessage(sb.String())
}

// ── act ────────────────────────────────────────────────────────────────────

func mafiaSubAct(client *Client, args []string) {
	if len(args) == 0 {
		client.SendServerMessage("Usage: /mafia act <target>\n  Arsonist: /mafia act douse <target>  OR  /mafia act ignite\n  Witch: /mafia act <player> <newtarget>\n  Mayor: /mafia act reveal  (Day only)")
		return
	}
	a := client.Area()
	g := getMafiaGame(a)
	if g == nil {
		client.SendServerMessage("No active Mafia game in this area.")
		return
	}
	g.mu.Lock()

	p := g.findPlayerByClient(client)
	if p == nil || !p.Alive {
		g.mu.Unlock()
		client.SendServerMessage("You are not an alive player in this game.")
		return
	}

	// Mayor reveal is a Day action
	if strings.ToLower(args[0]) == "reveal" && p.Role == RoleMayor {
		if g.Phase != MafiaPhaseDayDiscussion {
			g.mu.Unlock()
			client.SendServerMessage("You can only reveal as Mayor during the Day phase.")
			return
		}
		if p.MayorRevealed {
			g.mu.Unlock()
			client.SendServerMessage("You have already revealed yourself as Mayor.")
			return
		}
		p.MayorRevealed = true
		area := g.Area
		g.mu.Unlock()
		sendAreaServerMessage(area, fmt.Sprintf("📣 %v reveals themselves as the MAYOR! Their votes now count double!", p.Name()))
		return
	}

	if g.Phase != MafiaPhaseNight {
		g.mu.Unlock()
		client.SendServerMessage("Night actions can only be submitted during the Night phase.")
		return
	}

	info := roleInfoMap[p.Role]
	switch p.Role {
	case RoleMafia, RoleShapeshifter:
		targetName := strings.Join(args, " ")
		t := g.findPlayer(targetName)
		if t == nil || !t.Alive {
			g.mu.Unlock()
			client.SendServerMessage(fmt.Sprintf("No alive player named %q found.", targetName))
			return
		}
		if roleInfoMap[t.Role].Team == "Mafia" {
			g.mu.Unlock()
			client.SendServerMessage("You cannot target a fellow Mafia member.")
			return
		}
		// Mafia consensus: last submitted wins (all Mafia members vote)
		g.MafiaKillTarget = t.Name()
		g.mu.Unlock()
		// Notify all Mafia of the selection
		for _, mp := range g.Players {
			if mp.Alive && roleInfoMap[mp.Role].Team == "Mafia" {
				g.privateMsg(mp, fmt.Sprintf("Mafia kill target set to: %v (by %v)", t.Name(), p.Name()))
			}
		}
		client.SendServerMessage(fmt.Sprintf("Kill target set to %v.", t.Name()))

	case RoleDetective:
		targetName := strings.Join(args, " ")
		t := g.findPlayer(targetName)
		if t == nil || !t.Alive {
			g.mu.Unlock()
			client.SendServerMessage(fmt.Sprintf("No alive player named %q found.", targetName))
			return
		}
		p.NightAction = t.Name()
		g.mu.Unlock()
		client.SendServerMessage(fmt.Sprintf("You will investigate %v tonight.", t.Name()))

	case RoleDoctor:
		targetName := strings.Join(args, " ")
		t := g.findPlayer(targetName)
		if t == nil || !t.Alive {
			g.mu.Unlock()
			client.SendServerMessage(fmt.Sprintf("No alive player named %q found.", targetName))
			return
		}
		p.NightAction = t.Name()
		g.mu.Unlock()
		client.SendServerMessage(fmt.Sprintf("You will protect %v tonight.", t.Name()))

	case RoleBodyguard:
		targetName := strings.Join(args, " ")
		t := g.findPlayer(targetName)
		if t == nil || !t.Alive {
			g.mu.Unlock()
			client.SendServerMessage(fmt.Sprintf("No alive player named %q found.", targetName))
			return
		}
		p.NightAction = t.Name()
		g.mu.Unlock()
		client.SendServerMessage(fmt.Sprintf("You will guard %v tonight.", t.Name()))

	case RoleArsonist:
		if len(args) == 0 {
			g.mu.Unlock()
			client.SendServerMessage("Usage: /mafia act douse <target>  OR  /mafia act ignite")
			return
		}
		subAct := strings.ToLower(args[0])
		if subAct == "ignite" {
			p.NightAction = "ignite"
			g.mu.Unlock()
			client.SendServerMessage("You will ignite all doused players tonight!")
		} else if subAct == "douse" && len(args) >= 2 {
			targetName := strings.Join(args[1:], " ")
			t := g.findPlayer(targetName)
			if t == nil || !t.Alive {
				g.mu.Unlock()
				client.SendServerMessage(fmt.Sprintf("No alive player named %q found.", targetName))
				return
			}
			p.NightAction = "douse " + t.Name()
			g.mu.Unlock()
			client.SendServerMessage(fmt.Sprintf("You will douse %v tonight.", t.Name()))
		} else {
			g.mu.Unlock()
			client.SendServerMessage("Usage: /mafia act douse <target>  OR  /mafia act ignite")
		}

	case RoleSerialKiller:
		targetName := strings.Join(args, " ")
		t := g.findPlayer(targetName)
		if t == nil || !t.Alive {
			g.mu.Unlock()
			client.SendServerMessage(fmt.Sprintf("No alive player named %q found.", targetName))
			return
		}
		p.NightAction = t.Name()
		g.mu.Unlock()
		client.SendServerMessage(fmt.Sprintf("Target set to %v.", t.Name()))

	case RoleVigilante:
		if p.VigilanteUsed {
			g.mu.Unlock()
			client.SendServerMessage("You have already used your Vigilante shot.")
			return
		}
		targetName := strings.Join(args, " ")
		t := g.findPlayer(targetName)
		if t == nil || !t.Alive {
			g.mu.Unlock()
			client.SendServerMessage(fmt.Sprintf("No alive player named %q found.", targetName))
			return
		}
		p.NightAction = t.Name()
		g.mu.Unlock()
		client.SendServerMessage(fmt.Sprintf("⚠️  You will execute %v tonight. If they are Town-aligned, you die of guilt tomorrow morning!", t.Name()))

	case RoleEscort:
		targetName := strings.Join(args, " ")
		t := g.findPlayer(targetName)
		if t == nil || !t.Alive {
			g.mu.Unlock()
			client.SendServerMessage(fmt.Sprintf("No alive player named %q found.", targetName))
			return
		}
		p.NightAction = t.Name()
		g.mu.Unlock()
		client.SendServerMessage(fmt.Sprintf("You will keep %v busy tonight — their action will be blocked.", t.Name()))

	case RoleWitch:
		if len(args) < 2 {
			g.mu.Unlock()
			client.SendServerMessage("Usage: /mafia act <player-to-redirect> <new-target>")
			return
		}
		// Try to match longest prefix as player name
		var redirectPlayer *MafiaPlayer
		var newTargetName string
		for split := len(args) - 1; split >= 1; split-- {
			candidate := strings.Join(args[:split], " ")
			if rp := g.findPlayer(candidate); rp != nil && rp.Alive {
				redirectPlayer = rp
				newTargetName = strings.Join(args[split:], " ")
				break
			}
		}
		if redirectPlayer == nil {
			g.mu.Unlock()
			client.SendServerMessage("Could not find the player to redirect. Usage: /mafia act <player> <newtarget>")
			return
		}
		newTarget := g.findPlayer(newTargetName)
		if newTarget == nil || !newTarget.Alive {
			g.mu.Unlock()
			client.SendServerMessage(fmt.Sprintf("No alive player named %q found.", newTargetName))
			return
		}
		p.NightAction = redirectPlayer.Name()
		p.NightAction2 = newTarget.Name()
		g.mu.Unlock()
		client.SendServerMessage(fmt.Sprintf("You will redirect %v's action to %v.", redirectPlayer.Name(), newTarget.Name()))

	default:
		g.mu.Unlock()
		client.SendServerMessage(fmt.Sprintf("The %v role has no night action.", info.Name))
	}
}

// ── shoot (Sheriff) ────────────────────────────────────────────────────────

func mafiaSubShoot(client *Client, args []string) {
	if len(args) == 0 {
		client.SendServerMessage("Usage: /mafia shoot <player name>")
		return
	}
	a := client.Area()
	g := getMafiaGame(a)
	if g == nil {
		client.SendServerMessage("No active Mafia game in this area.")
		return
	}
	g.mu.Lock()
	if g.Phase != MafiaPhaseDayDiscussion {
		g.mu.Unlock()
		client.SendServerMessage("You can only shoot during the Day phase.")
		return
	}
	shooter := g.findPlayerByClient(client)
	if shooter == nil || !shooter.Alive {
		g.mu.Unlock()
		client.SendServerMessage("You are not an alive player in this game.")
		return
	}
	if shooter.Role != RoleSheriff {
		g.mu.Unlock()
		client.SendServerMessage("Only the Sheriff can shoot.")
		return
	}
	if g.SheriffUsed {
		g.mu.Unlock()
		client.SendServerMessage("You have already used your shot.")
		return
	}

	targetName := strings.Join(args, " ")
	target := g.findPlayer(targetName)
	if target == nil || !target.Alive {
		g.mu.Unlock()
		client.SendServerMessage(fmt.Sprintf("No alive player named %q found.", targetName))
		return
	}
	g.SheriffUsed = true

	targetRole := target.Role
	info := roleInfoMap[targetRole]
	if info.Team == "Mafia" && targetRole != RoleGodfather {
		target.Alive = false
		targetWill := target.LastWill
		g.addToGraveyard(target, "Shot (Sheriff)")
		g.mu.Unlock()
		msg := fmt.Sprintf("🔫 The Sheriff shoots %v — and hits a Mafia member! %v is eliminated!", target.Name(), target.Name())
		if targetWill != "" {
			msg += "\n📜 Last Will of " + target.Name() + ": " + targetWill
		}
		g.broadcastToGame(msg)
	} else if targetRole == RoleGodfather {
		// Godfather is immune — bullet bounces, Sheriff dies
		shooter.Alive = false
		shooterWill := shooter.LastWill
		g.addToGraveyard(shooter, "Shot bounced (Godfather immunity)")
		g.mu.Unlock()
		msg := fmt.Sprintf("🔫 The Sheriff shoots %v — but the bullet bounces off the Godfather! %v (Sheriff) falls dead!", target.Name(), shooter.Name())
		if shooterWill != "" {
			msg += "\n📜 Last Will of " + shooter.Name() + ": " + shooterWill
		}
		g.broadcastToGame(msg)
	} else {
		// Innocent target — backfire
		shooter.Alive = false
		shooterWill := shooter.LastWill
		g.addToGraveyard(shooter, "Backfire (Sheriff)")
		g.mu.Unlock()
		msg := fmt.Sprintf("🔫 The Sheriff shoots %v — an innocent! The gun backfires, killing the Sheriff (%v)!", target.Name(), shooter.Name())
		if shooterWill != "" {
			msg += "\n📜 Last Will of " + shooter.Name() + ": " + shooterWill
		}
		g.broadcastToGame(msg)
	}

	g.mu.Lock()
	winner, won := g.checkWin()
	g.mu.Unlock()
	if won {
		g.endGame(winner)
	}
}

// ── day (admin) ────────────────────────────────────────────────────────────

func mafiaSubDay(client *Client) {
	a := client.Area()
	g := getMafiaGame(a)
	if g == nil {
		client.SendServerMessage("No active Mafia game in this area.")
		return
	}
	g.mu.Lock()
	if !isMafiaCM(client, g) {
		g.mu.Unlock()
		client.SendServerMessage("Only the host, a CM, or a moderator can do that.")
		return
	}
	if g.Phase == MafiaPhaseLobby || g.Phase == MafiaPhaseEnded {
		g.mu.Unlock()
		client.SendServerMessage("Cannot advance to Day from the current phase.")
		return
	}
	dayNum := g.Day + 1
	if g.Phase == MafiaPhaseDayDiscussion {
		dayNum = g.Day
	}
	g.mu.Unlock()
	g.cancelTimer()
	g.startDay(dayNum)
}

// ── night (admin) ──────────────────────────────────────────────────────────

func mafiaSubNight(client *Client) {
	a := client.Area()
	g := getMafiaGame(a)
	if g == nil {
		client.SendServerMessage("No active Mafia game in this area.")
		return
	}
	g.mu.Lock()
	if !isMafiaCM(client, g) {
		g.mu.Unlock()
		client.SendServerMessage("Only the host, a CM, or a moderator can do that.")
		return
	}
	if g.Phase == MafiaPhaseLobby || g.Phase == MafiaPhaseEnded {
		g.mu.Unlock()
		client.SendServerMessage("Cannot advance to Night from the current phase.")
		return
	}
	g.mu.Unlock()
	g.cancelTimer()
	g.startNight()
}

// ── resolve (admin) ────────────────────────────────────────────────────────

func mafiaSubResolve(client *Client) {
	a := client.Area()
	g := getMafiaGame(a)
	if g == nil {
		client.SendServerMessage("No active Mafia game in this area.")
		return
	}
	g.mu.Lock()
	if !isMafiaCM(client, g) {
		g.mu.Unlock()
		client.SendServerMessage("Only the host, a CM, or a moderator can do that.")
		return
	}
	phase := g.Phase
	g.mu.Unlock()
	g.cancelTimer()

	switch phase {
	case MafiaPhaseDayDiscussion:
		g.resolveDay()
	case MafiaPhaseNight:
		g.resolveNight()
	default:
		client.SendServerMessage("Nothing to resolve in the current phase.")
	}
}

// ── stop (admin) ───────────────────────────────────────────────────────────

func mafiaSubStop(client *Client) {
	a := client.Area()
	g := getMafiaGame(a)
	if g == nil {
		client.SendServerMessage("No active Mafia game in this area.")
		return
	}
	g.mu.Lock()
	if !isMafiaCM(client, g) {
		g.mu.Unlock()
		client.SendServerMessage("Only the host, a CM, or a moderator can abort the game.")
		return
	}
	g.mu.Unlock()
	g.cancelTimer()
	g.broadcastToGame(fmt.Sprintf("🛑 %v aborted the game.", client.OOCName()))
	deleteMafiaGame(a)
}

// ── kick (admin) ───────────────────────────────────────────────────────────

func mafiaSubKick(client *Client, args []string) {
	if len(args) == 0 {
		client.SendServerMessage("Usage: /mafia kick <player name>")
		return
	}
	a := client.Area()
	g := getMafiaGame(a)
	if g == nil {
		client.SendServerMessage("No active Mafia game in this area.")
		return
	}
	g.mu.Lock()
	if !isMafiaCM(client, g) {
		g.mu.Unlock()
		client.SendServerMessage("Only the host, a CM, or a moderator can kick players.")
		return
	}
	targetName := strings.Join(args, " ")
	p := g.findPlayer(targetName)
	if p == nil {
		g.mu.Unlock()
		client.SendServerMessage(fmt.Sprintf("No player named %q found.", targetName))
		return
	}
	name := p.Name()
	phase := g.Phase
	wasAlive := p.Alive

	if phase == MafiaPhaseLobby {
		newList := make([]*MafiaPlayer, 0, len(g.Players)-1)
		for _, pp := range g.Players {
			if pp != p {
				newList = append(newList, pp)
			}
		}
		g.Players = newList
		g.mu.Unlock()
		sendAreaServerMessage(a, fmt.Sprintf("🎭 %v was kicked from the Mafia lobby by %v.", name, client.OOCName()))
		return
	}

	if wasAlive {
		p.Alive = false
		g.addToGraveyard(p, "Kicked")
	}
	winner, won := g.checkWin()
	g.mu.Unlock()

	if wasAlive {
		g.broadcastToGame(fmt.Sprintf("🥾 %v was removed from the game by %v.", name, client.OOCName()))
		if won {
			g.endGame(winner)
		}
	} else {
		client.SendServerMessage(fmt.Sprintf("%v is already dead.", name))
	}
}

// ── reveal (admin) ─────────────────────────────────────────────────────────

func mafiaSubReveal(client *Client) {
	a := client.Area()
	g := getMafiaGame(a)
	if g == nil {
		client.SendServerMessage("No active Mafia game in this area.")
		return
	}
	g.mu.Lock()
	if !isMafiaCM(client, g) {
		g.mu.Unlock()
		client.SendServerMessage("Only the host, a CM, or a moderator can force-reveal roles.")
		return
	}
	var sb strings.Builder
	sb.WriteString("🎭 Force role reveal:\n")
	for _, p := range g.Players {
		info := roleInfoMap[p.Role]
		status := "✅ alive"
		if !p.Alive {
			status = "💀 dead"
		}
		fmt.Fprintf(&sb, "  %v — %v (%v) [%v]\n", p.Name(), info.Name, info.Team, status)
	}
	a2 := g.Area
	msg := sb.String()
	g.mu.Unlock()
	sendAreaServerMessage(a2, msg)
}

// ── timer (admin) ──────────────────────────────────────────────────────────

func mafiaSubTimer(client *Client, args []string) {
	if len(args) == 0 {
		client.SendServerMessage("Usage: /mafia timer <seconds>  (0 = disabled)")
		return
	}
	a := client.Area()
	g := getMafiaGame(a)
	if g == nil {
		client.SendServerMessage("No active Mafia game in this area.")
		return
	}
	g.mu.Lock()
	if !isMafiaCM(client, g) {
		g.mu.Unlock()
		client.SendServerMessage("Only the host, a CM, or a moderator can change the timer.")
		return
	}
	secs, err := strconv.Atoi(args[0])
	if err != nil || secs < 0 {
		g.mu.Unlock()
		client.SendServerMessage("Timer must be a non-negative integer (seconds).")
		return
	}
	g.PhaseSecs = secs
	g.mu.Unlock()
	if secs == 0 {
		sendAreaServerMessage(a, "🎭 Phase auto-timer disabled.")
	} else {
		sendAreaServerMessage(a, fmt.Sprintf("🎭 Phase auto-timer set to %d seconds.", secs))
	}
}

// ── roles (info) ───────────────────────────────────────────────────────────

func mafiaSubRoles(client *Client) {
	var sb strings.Builder
	sb.WriteString("🎭 Mafia Roles:\n\n")
	order := []RoleID{
		// Town
		RoleVillager, RoleDetective, RoleDoctor, RoleSheriff, RoleBodyguard, RoleVigilante, RoleEscort, RoleMayor,
		// Mafia
		RoleMafia, RoleShapeshifter, RoleGodfather,
		// Neutral
		RoleJester, RoleWitch, RoleLawyer, RoleArsonist, RoleSerialKiller, RoleSurvivor,
	}
	for _, id := range order {
		info := roleInfoMap[id]
		fmt.Fprintf(&sb, "── %v (%v / %v) ──\n", info.Name, info.Team, info.Alignment)
		fmt.Fprintf(&sb, "  %v\n", info.Desc)
		fmt.Fprintf(&sb, "  Win: %v\n", info.WinCond)
		fmt.Fprintf(&sb, "  Ability: %v\n\n", info.Ability)
	}
	client.SendServerMessage(sb.String())
}

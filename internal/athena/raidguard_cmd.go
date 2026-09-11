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

// Staff-facing tooling for the raid guard (raidguard.go, raidguard_corr.go).
//
//	/raidguard status       -- settings, live state, who currently has a score.
//	/raidguard clear <uid>  -- the false-positive escape hatch: wipe one
//	                           connection's accumulated score and lift any
//	                           captcha action the guard put on it.
//	/raidguard clear all    -- the same, for every connected client.
//	/raidguard test <text>  -- show what the engine makes of a line of text
//	                           without acting on anyone or recording anything.
//	/raidguard alert <on|off> -- mute the [RAIDGUARD] staff alerts for your own
//	                           session, the way /censoralerts mutes censor trips.
//
// The command as a whole is gated on MOD_CHAT rather than BAN so that everyone
// who *receives* the alerts can also turn them off; status, clear and test
// re-check BAN in the handler, so lowering the registry gate widens nothing
// but the toggle.
//
// Otherwise the same shape as /joincaptcha (joincaptcha.go): a single
// subcommand dispatcher, status/verify-style reporting, and a UID-or-"all"
// clear form mirroring /joincaptcha reset.
package athena

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/MangosArentLiterature/Athena/internal/logger"
	"github.com/MangosArentLiterature/Athena/internal/permissions"
)

// cmdRaidGuard handles the moderator side of the raid guard.
func cmdRaidGuard(client *Client, args []string, usage string) {
	if len(args) == 0 {
		client.SendServerMessage(usage)
		return
	}
	sub := strings.ToLower(args[0])

	// The opt-out is available to every alert recipient (MOD_CHAT); everything
	// else here inspects or changes live raid state and stays on BAN.
	if sub == "alert" || sub == "alerts" {
		raidGuardAlertCmd(client, args[1:])
		return
	}
	if !permissions.HasPermission(client.Perms(), permissions.PermissionField["BAN"]) && !clientHasCommandGrant(client, "raidguard") {
		client.SendServerMessage("You do not have permission to use that command.\nUsage: /raidguard alert <on|off>")
		return
	}

	switch sub {
	case "status":
		raidGuardStatus(client)
	case "clear":
		if len(args) < 2 {
			client.SendServerMessage("Usage: /raidguard clear <uid|all>")
			return
		}
		raidGuardClearCmd(client, args[1])
	case "test":
		if len(args) < 2 {
			client.SendServerMessage("Usage: /raidguard test <text>")
			return
		}
		raidGuardTest(client, strings.Join(args[1:], " "))
	default:
		client.SendServerMessage(usage)
	}
}

// --- /raidguard alert -----------------------------------------------------

// raidGuardAlertCmd handles /raidguard alert <on|off>, the per-session mute
// for the [RAIDGUARD] staff alerts. With no argument it reports the caller's
// current setting. Per-session by design, mirroring /censoralerts: alerts
// default back to on for every fresh connection, so a mute during one raid
// can never quietly outlive it.
func raidGuardAlertCmd(client *Client, args []string) {
	const usage = "Usage: /raidguard alert <on|off>"
	if len(args) == 0 {
		state := "ENABLED"
		if client.RaidAlertsDisabled() {
			state = "DISABLED"
		}
		client.SendServerMessage(fmt.Sprintf("Raid guard alerts are currently %s for you.\n%s", state, usage))
		return
	}
	switch strings.ToLower(strings.TrimSpace(args[0])) {
	case "on":
		client.SetRaidAlertsDisabled(false)
		client.SendServerMessage("Raid guard alerts are now ON for you.")
	case "off":
		client.SetRaidAlertsDisabled(true)
		client.SendServerMessage("Raid guard alerts are now OFF for you (this session only — they reset to on when you reconnect).")
	default:
		client.SendServerMessage("Invalid argument:\n" + usage)
	}
}

// --- /raidguard status ----------------------------------------------------

// raidGuardStatus reports configuration, live layer-2 state, and every
// connected client the engine currently has a nonzero score or verdict for.
func raidGuardStatus(client *Client) {
	var sb strings.Builder

	enabled := raidGuardActive.Load()
	state := "OFF"
	if enabled {
		state = "ON"
	}
	sb.WriteString(fmt.Sprintf("🛡️ Raid guard: %s, max action '%v'.\n", state, raidGuardMaxAction()))
	if !enabled {
		sb.WriteString("(raid_guard_enabled = false — the hot path is skipped entirely; the rest of this report still reflects the configured thresholds.)\n")
	}

	// Score thresholds, at the configured (baseline) tier.
	sb.WriteString(fmt.Sprintf(
		"\nScore thresholds (baseline): watch %d, captcha %d, silence %d, kick %d, ban %d.\n",
		raidGuardInt(config.RaidGuardScoreWatch, 40), raidGuardInt(config.RaidGuardScoreChallenge, 60),
		raidGuardInt(config.RaidGuardScoreSilence, 80), raidGuardInt(config.RaidGuardScoreKick, 100),
		raidGuardInt(config.RaidGuardScoreBan, 140)))

	// The four playtime tiers, same predicate the lockdown purge, the join
	// captcha and the repeat-offender autoban read.
	sb.WriteString("Playtime tiers (from KNOWN_IPS.PLAYTIME, same figure /playtime uses):\n")
	sb.WriteString(fmt.Sprintf("  strict  — under %dm: thresholds x%d%%\n",
		raidGuardInt(config.RaidGuardStrictPlaytime, 15), raidGuardInt(config.RaidGuardStrictScale, 70)))
	sb.WriteString(fmt.Sprintf("  baseline — %dm and up: thresholds as configured\n",
		raidGuardInt(config.RaidGuardStrictPlaytime, 15)))
	sb.WriteString(fmt.Sprintf("  lenient — %dm and up: thresholds x%d%%\n",
		raidGuardInt(config.RaidGuardLenientPlaytime, 120), raidGuardInt(config.RaidGuardLenientScale, 200)))
	sb.WriteString(fmt.Sprintf("  exempt  — %dm and up, and every moderator: never acted on (still alerted on)\n",
		raidGuardInt(config.RaidGuardMinPlaytime, 1200)))

	banDur, _ := raidGuardBanDuration()
	sb.WriteString(fmt.Sprintf("Autoban duration: %v.\n", banDur))

	// Live layer-2 state.
	corrLen := 0
	if w := raidCorr(); w != nil {
		corrLen = w.Len()
	}
	attack := "no"
	if raidGuardUnderAttack() {
		attack = "YES — a raid has been detected in the last raidAttackHold window"
	}
	sb.WriteString(fmt.Sprintf("\nUnder attack right now: %s.\n", attack))
	_, weak, strong := raidCorrThresholds()
	sb.WriteString(fmt.Sprintf("Correlation window: %d fingerprint(s) currently tracked over %v "+
		"(%d distinct IPIDs on one line = echo, %d = corroborated).\n",
		corrLen, raidCorrWindowLen(), weak, strong))
	sb.WriteString(fmt.Sprintf("Echo breadth: %d distinct line(s) echoing across IPIDs right now (%d marks the server under attack).\n",
		raidGuardEchoBreadth(), raidCorrBreadth()))

	alertState := "ON"
	if client.RaidAlertsDisabled() {
		alertState = "OFF"
	}
	sb.WriteString(fmt.Sprintf("[RAIDGUARD] alerts are %s for you (/raidguard alert <on|off>).\n", alertState))

	// Connected clients the engine currently has anything recorded for.
	type row struct {
		uid     int
		name    string
		score   int
		acted   Verdict
		signals []string
	}
	var rows []row
	clients.ForEach(func(c *Client) {
		rs := c.raidGuard()
		if rs == nil {
			return
		}
		score, signals, acted := rs.snapshot()
		if score == 0 && acted == VerdictClean {
			return
		}
		rows = append(rows, row{uid: c.Uid(), name: clientDisplayName(c), score: score, acted: acted, signals: signals})
	})
	sort.Slice(rows, func(i, j int) bool { return rows[i].score > rows[j].score })

	sb.WriteString(fmt.Sprintf("\nConnected clients with a nonzero score (%d):\n", len(rows)))
	if len(rows) == 0 {
		sb.WriteString("  (nobody)")
	} else {
		var lines []string
		for _, r := range rows {
			lines = append(lines, fmt.Sprintf("  UID %d (%s) — score %d, verdict so far: %v\n    signals: %s",
				r.uid, r.name, r.score, r.acted, strings.Join(r.signals, ", ")))
		}
		sb.WriteString(strings.Join(lines, "\n"))
	}

	client.SendServerMessage(sb.String())
}

// --- /raidguard clear ------------------------------------------------------

// raidGuardClearCmd dispatches /raidguard clear <uid|all>.
func raidGuardClearCmd(client *Client, targetArg string) {
	if strings.EqualFold(targetArg, "all") {
		var cleared, releasedCaptcha int
		clients.ForEach(func(c *Client) {
			did, liftedCaptcha := raidGuardClearOne(c)
			if did {
				cleared++
			}
			if liftedCaptcha {
				releasedCaptcha++
			}
		})
		client.SendServerMessage(fmt.Sprintf(
			"Cleared the raid guard score on %d connected client(s); lifted a raid-guard captcha action on %d of them.",
			cleared, releasedCaptcha))
		logger.LogInfof("%v cleared raid guard state for all connected clients (%d cleared, %d captcha releases)",
			client.ModName(), cleared, releasedCaptcha)
		addToBuffer(client, "CMD", "Cleared raid guard state for all connected clients.", true)
		return
	}

	uid, err := strconv.Atoi(strings.TrimSpace(targetArg))
	if err != nil {
		client.SendServerMessage("Invalid UID. Usage: /raidguard clear <uid|all>")
		return
	}
	target, err := getClientByUid(uid)
	if err != nil {
		client.SendServerMessage("No client with that UID.")
		return
	}
	did, liftedCaptcha := raidGuardClearOne(target)
	if !did {
		client.SendServerMessage(fmt.Sprintf("UID %d has no raid guard score or verdict to clear.", uid))
		return
	}
	msg := fmt.Sprintf("Cleared UID %d's (IPID %s) raid guard score and reset its verdict.", uid, target.Ipid())
	if liftedCaptcha {
		msg += " Also lifted the captcha challenge/restriction the raid guard had placed on them — they can chat normally now."
	}
	client.SendServerMessage(msg)
	logger.LogInfof("%v cleared raid guard state for UID %v (IPID %v), captcha released: %v",
		client.ModName(), uid, target.Ipid(), liftedCaptcha)
	logger.WriteAudit(fmt.Sprintf("%v | RAID_GUARD_CLEAR | IPID:%v | UID:%v | by=%v",
		time.Now().UTC().Format("15:04:05"), target.Ipid(), uid, client.ModName()))
	addToBuffer(client, "CMD", fmt.Sprintf("Cleared raid guard state for UID %v.", uid), true)
}

// raidGuardClearOne resets one client's raidState to a clean slate and, if
// the raid guard itself was the reason this connection is currently gated or
// restricted by the join captcha, releases it via the same releaseJoinCaptcha
// path /joincaptcha verify uses. Returns whether there was anything to clear,
// and whether a captcha action was lifted as part of clearing it.
//
// This is the false-positive escape hatch, so it errs toward clearing: any
// verdict beyond VerdictClean (watch included) counts as "something to
// report", even though watch itself never restricted the player.
func raidGuardClearOne(target *Client) (cleared bool, liftedCaptcha bool) {
	if target == nil {
		return false, false
	}
	rs := target.raidGuard()
	if rs == nil {
		return false, false
	}
	score, _, acted := rs.snapshot()
	if score == 0 && acted == VerdictClean {
		return false, false
	}

	guardRestricted := acted == VerdictChallenge || acted == VerdictSilence ||
		acted == VerdictKick || acted == VerdictBan
	rs.clear()

	if guardRestricted && (target.awaitingCaptcha.Load() || target.captchaRestricted.Load()) {
		target.releaseJoinCaptcha("raidguard-clear", true)
		liftedCaptcha = true
	}
	return true, liftedCaptcha
}

// clear resets a raidState to a fresh, unscored connection. Defined here
// rather than in raidguard.go because it exists purely to serve the /clear
// escape hatch -- the scoring engine itself never calls it.
func (rs *raidState) clear() {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.msgCount = 0
	rs.icCount = 0
	rs.objectionCount = 0
	rs.oocNames = make(map[string]struct{}, 4)
	rs.shownames = make(map[string]struct{}, 4)
	rs.charPicks = 0
	rs.lastCharPick = time.Time{}
	// Deliberately true, not false. Clearing means staff have vouched for this
	// connection, and sawAskchaa=false would re-arm the handshake-order signal:
	// a cleared client that later re-requests the music or character list would
	// trip it again, so the escape hatch would silently reset the trap it had
	// just released.
	rs.sawAskchaa = true
	rs.fired = 0
	rs.score = 0
	rs.acted = VerdictClean
}

// --- /raidguard test ---------------------------------------------------------

// raidGuardTest shows what the engine makes of a piece of text without
// acting on anyone. It deliberately calls only the pure, stateless helpers
// (normalizeRaidText, raidFingerprints, isShoutySpam) and never
// raidObserveContent/CorrelationWindow.Observe/raidState.observe, so running
// it cannot itself feed the correlation window, move the under-attack flag,
// or add to any connection's score -- a moderator tuning the guard should
// never be able to accidentally trip it.
func raidGuardTest(client *Client, text string) {
	norm := normalizeRaidText(text)
	minLen := raidGuardInt(config.RaidGuardCorrMinLen, 15)
	fps := raidFingerprints(text, minLen)

	var sb strings.Builder
	sb.WriteString("🧪 Raid guard test (read-only — nothing recorded):\n")
	sb.WriteString(fmt.Sprintf("  input:      %q\n", text))
	sb.WriteString(fmt.Sprintf("  normalized: %q (%d chars)\n", norm, len(norm)))
	if len(fps) == 0 {
		sb.WriteString(fmt.Sprintf("  fingerprints: none — shorter than the %d-char correlation floor, so it can never correlate across IPIDs.\n", minLen))
	} else {
		sb.WriteString(fmt.Sprintf("  fingerprints: %d (shingle size %d) — would correlate against other IPIDs saying near-identical text.\n", len(fps), shingleSize))
	}
	shouty := isShoutySpam(text)
	sb.WriteString(fmt.Sprintf("  isShoutySpam: %v (25+ chars, ≥80%% caps, and a repeated 3+-letter word, all three required)\n", shouty))

	client.SendServerMessage(sb.String())
}

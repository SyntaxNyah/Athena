/* Athena - A server for Attorney Online 2 written in Go
   Nyathena fork additions: quality-of-life commands.

     /punishments [uid]  list active punishments with remaining durations.
                         Players inspect themselves; the uid form needs MUTE.
                         With 100+ stackable punishment types this is the
                         mod team's missing dashboard.
     /clients <uid>      list every connection sharing the target's IPID
                         (multiclient overview). Requires MUTE.
     /lfp                toggle the Looking-For-Pair flag.
     /pairlist           list everyone in the area flagged /lfp.
     /stealthmute <uid>  punishment: the target's IC/OOC messages echo back
                         to them but reach nobody else. Always silent — the
                         target is never notified. Lift with
                         /unpunish -t stealthmute <uid>. */

package athena

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/MangosArentLiterature/Athena/internal/permissions"
)

// cmdPunishments lists a player's active punishments with time remaining.
func cmdPunishments(client *Client, args []string, usage string) {
	target := client
	if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
		if !permissions.HasPermission(client.Perms(), permissions.PermissionField["MUTE"]) && !clientHasCommandGrant(client, "punishments") {
			client.SendServerMessage("Viewing another player's punishments requires moderator permissions. Use /punishments alone to view your own.")
			return
		}
		uid, err := strconv.Atoi(strings.TrimSpace(args[0]))
		if err != nil {
			client.SendServerMessage("Invalid UID.\n" + usage)
			return
		}
		t, err := getClientByUid(uid)
		if err != nil {
			client.SendServerMessage("No client found with that UID.")
			return
		}
		target = t
	}

	viewerIsMod := permissions.HasPermission(client.Perms(), permissions.PermissionField["MUTE"])
	active := target.GetActivePunishments()

	var lines []string
	for i := range active {
		p := &active[i]
		line := "  • " + p.punishmentType.String()
		if p.expiresAt.IsZero() {
			line += " — permanent"
		} else {
			line += fmt.Sprintf(" — %v left", time.Until(p.expiresAt).Round(time.Second))
		}
		if p.customData != "" {
			line += fmt.Sprintf(" (%v)", p.customData)
		}
		if p.reason != "" {
			line += " — reason: " + p.reason
		}
		if viewerIsMod {
			switch p.issuerTier {
			case IssuerMod:
				line += " [by mod]"
			case IssuerShadow:
				line += " [by shadow]"
			case IssuerAdmin:
				line += " [by admin]"
			}
		}
		lines = append(lines, line)
	}

	// Effects living outside the punishment slice.
	if isIPIDTormented(target.Ipid()) {
		lines = append(lines, "  • lag (torment list — lift with /unpunish -t lag)")
	}
	if target.Muted() != Unmuted {
		line := "  • muted"
		if until := target.UnmuteTime(); !until.IsZero() {
			line += fmt.Sprintf(" — %v left", time.Until(until).Round(time.Second))
		}
		lines = append(lines, line)
	}
	if target.IsJailed() {
		lines = append(lines, fmt.Sprintf("  • jailed — %v left", time.Until(target.JailedUntil()).Round(time.Second)))
	}

	who := fmt.Sprintf("[%v] %v", target.Uid(), clientDisplayName(target))
	if target == client {
		who += " (you)"
	}
	if len(lines) == 0 {
		client.SendServerMessage(fmt.Sprintf("⛓️ %v has no active punishments. Enjoy the freedom while it lasts.", who))
		return
	}
	client.SendServerMessage(fmt.Sprintf("⛓️ Active punishments for %v (%d):\n%v", who, len(lines), strings.Join(lines, "\n")))
}

// cmdClients lists every connection sharing the target's IPID.
func cmdClients(client *Client, args []string, usage string) {
	uid, err := strconv.Atoi(strings.TrimSpace(args[0]))
	if err != nil {
		client.SendServerMessage("Invalid UID.\n" + usage)
		return
	}
	target, err := getClientByUid(uid)
	if err != nil {
		client.SendServerMessage("No client found with that UID.")
		return
	}

	list := getClientsByIpid(target.Ipid())
	var lines []string
	for _, c := range list {
		if c.Uid() == -1 {
			continue // still joining
		}
		charName := "Spectator"
		if id := c.CharID(); id >= 0 && id < len(getCharacters()) {
			charName = getCharacters()[id]
		}
		line := fmt.Sprintf("  [%v] %v — area: %v", c.Uid(), charName, c.Area().Name())
		if name := c.OOCName(); name != "" {
			line += ", OOC: " + name
		}
		if sn := c.EffectiveShowname(); sn != "" {
			line += ", showname: " + sn
		}
		lines = append(lines, line)
	}
	client.SendServerMessage(fmt.Sprintf("🖧 Connections for IPID %v (%d):\n%v", target.Ipid(), len(lines), strings.Join(lines, "\n")))
}

// LookingForPair reports the /lfp flag.
func (client *Client) LookingForPair() bool {
	client.mu.Lock()
	defer client.mu.Unlock()
	return client.lookingForPair
}

// SetLookingForPair sets the /lfp flag.
func (client *Client) SetLookingForPair(v bool) {
	client.mu.Lock()
	client.lookingForPair = v
	client.mu.Unlock()
}

// cmdLfp toggles the Looking-For-Pair flag.
func cmdLfp(client *Client, _ []string, _ string) {
	next := !client.LookingForPair()
	client.SetLookingForPair(next)
	if next {
		client.SendServerMessage("💞 You are now flagged Looking For Pair — players in your area will see you in /pairlist. Type /lfp again to unflag.")
	} else {
		client.SendServerMessage("💔 Looking-For-Pair flag removed.")
	}
}

// cmdPairlist lists everyone in the caller's area flagged /lfp.
func cmdPairlist(client *Client, _ []string, _ string) {
	a := client.Area()
	var lines []string
	clients.ForEach(func(c *Client) {
		if c.Area() != a || !c.LookingForPair() || c.Uid() == -1 {
			return
		}
		charName := "Spectator"
		if id := c.CharID(); id >= 0 && id < len(getCharacters()) {
			charName = getCharacters()[id]
		}
		lines = append(lines, fmt.Sprintf("  [%v] %v — %v", c.Uid(), clientDisplayName(c), charName))
	})
	if len(lines) == 0 {
		client.SendServerMessage("💞 No one in this area is flagged Looking-For-Pair. Flag yourself with /lfp.")
		return
	}
	client.SendServerMessage(fmt.Sprintf("💞 Looking For Pair in %v (%d):\n%v\nPair up with /pair <uid>.", a.Name(), len(lines), strings.Join(lines, "\n")))
}

// cmdStealthMute applies the stealthmute punishment. The -h flag is forced so
// the standard punishment plumbing never notifies the target — that's the
// whole point. The issuer's summary still appends "(hidden)".
func cmdStealthMute(client *Client, args []string, usage string) {
	cmdPunishment(client, append(args, "-h"), usage, PunishmentStealthMute)
}

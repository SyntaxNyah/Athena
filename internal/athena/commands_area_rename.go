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

// /area rename — let whoever is running a room name it after what is actually
// happening in it. A CM sets up a Danganronpa case in "Courtroom 3" and the
// area list still says "Courtroom 3"; /area rename DR Killing Game makes the
// list say what the room is.
//
// It is a LOAN, not a transfer. The configured name in areas.toml is the area's
// real identity, and it comes back the moment the room stops belonging to
// anyone -- when the last person leaves, or when the last CM does. That is what
// keeps a rename from being a way to permanently vandalise the server's area
// list: nobody has to remember to undo one, and there is no state to clean up
// after an operator restarts.
//
// The name reaches every connected client, so it goes through the same tiered
// word list that IC and OOC messages do (see automod.go), nuke tier included.
package athena

import (
	"fmt"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/MangosArentLiterature/Athena/internal/area"
	"github.com/MangosArentLiterature/Athena/internal/logger"
	"github.com/MangosArentLiterature/Athena/internal/packet"
	"github.com/MangosArentLiterature/Athena/internal/sliceutil"
)

// maxAreaNameLen bounds a runtime name in runes. The area list is a narrow
// column in every AO2 client, and a name long enough to need scrolling makes
// the list harder to read for everyone in the server, not just the room that
// set it.
const maxAreaNameLen = 32

// areaNameReserved are the four characters AO2's wire format spends: '#'
// separates packet fields and '%' terminates a packet, while '$' and '&' are
// the other two the escape table covers. Area names are the one player-settable
// string this server emits UNencoded (buildSMPacket writes them verbatim, and
// pktAM compares an incoming area change against them after decoding), so a
// name carrying any of these would either split into two list entries on the
// client or come back as something the area lookup can never match.
const areaNameReserved = "#%$&"

// areaRenameMu serializes every rename, reset and republish. Held across the
// uniqueness re-check and the store so two rooms cannot race each other onto
// the same name, and across the republish so two concurrent renames cannot
// publish their area-name lists out of order.
var areaRenameMu sync.Mutex

// cmdAreaRename handles "/area rename <name>". Gated on CM at the registry, so
// both area CMs (via /cm) and moderators reach it.
func cmdAreaRename(client *Client, args []string, usage string) {
	a := client.Area()
	if len(args) == 0 {
		reportAreaName(client, a)
		return
	}

	name := strings.TrimSpace(strings.Join(args, " "))
	if reason := areaNameRejection(a, name); reason != "" {
		client.SendServerMessage(reason)
		return
	}

	// The same tiered word list, the same evasion normalization and the same
	// configured action IC and OOC messages get -- a room name is broadcast to
	// every connected client, so there is no argument for holding it to a
	// weaker standard than a single line of chat. Mirrors checkCensored in
	// pktIC, including feeding the raid guard everything below nuke tier.
	m, result, kickAfter := autoModCheckTiered(client, name, "area rename")
	if m.Matched && m.Entry.Severity == SeverityNuke {
		// Destroys the rename and bans the IPID. Nothing below may run.
		applyAutoModNuke(client, m, "area rename")
		return
	}
	raidGuardOnWordHit(client, m)
	switch result {
	case autoModBlocked:
		// autoModCheckTiered already did the visible thing (kicked, muted or
		// banned them). The area keeps its name.
		if kickAfter {
			client.KickForCensorTrip()
		}
		return
	case autoModShadow:
		// Shadow semantics, kept intact: their client is told exactly what a
		// successful rename would tell it, while the area is not renamed and no
		// other client ever sees the name. Sent before the kick, so the
		// confirmation lands before the connection closes.
		client.SendServerMessage(fmt.Sprintf("This area is now called %q. It reverts to %q when the room empties or its last CM leaves.",
			name, a.DefaultName()))
		if kickAfter {
			client.KickForCensorTrip()
		}
		return
	}

	areaRenameMu.Lock()
	// Re-check under the lock: a concurrent rename of another area could have
	// taken this name between the check above and here.
	if reason := areaNameRejection(a, name); reason != "" {
		areaRenameMu.Unlock()
		client.SendServerMessage(reason)
		return
	}
	old := a.Name()
	if old == name {
		areaRenameMu.Unlock()
		client.SendServerMessage(fmt.Sprintf("This area is already called %q.", name))
		return
	}
	a.SetName(name)
	republishAreaNames()
	areaRenameMu.Unlock()

	// Area logs are written into a per-name directory created at startup, so a
	// name that has never existed before needs one. A failure is logged and not
	// fatal: WriteAreaLog opens with O_CREATE and simply drops the line if the
	// directory is missing, exactly as it would for an area added to the config
	// without a restart.
	if err := logger.CreateAreaLogDirectory(name); err != nil {
		logger.LogErrorf("Failed to create area log directory for renamed area %q: %v", name, err)
	}

	sendAreaServerMessage(a, fmt.Sprintf("📛 %v renamed this area to %q (it was %q).", oocDisplayName(client), name, old))
	client.SendServerMessage(fmt.Sprintf("Renamed the area to %q. It reverts to %q when the room empties or its last CM leaves — or run /area unrename.",
		name, a.DefaultName()))
	addToBuffer(client, "CMD", fmt.Sprintf("Renamed the area from %q to %q.", old, name), false)
	logger.LogInfof("Area renamed from %q to %q by UID:%v IPID:%v", old, name, client.Uid(), client.Ipid())
}

// cmdAreaUnrename handles "/area unrename": drop a rename early rather than
// waiting for the room to empty.
func cmdAreaUnrename(client *Client, _ []string, _ string) {
	a := client.Area()
	if !a.Renamed() {
		client.SendServerMessage(fmt.Sprintf("This area is not renamed; it is called %q.", a.Name()))
		return
	}
	old := a.Name()
	if !resetAreaName(a) {
		client.SendServerMessage(fmt.Sprintf("This area is not renamed; it is called %q.", a.Name()))
		return
	}
	sendAreaServerMessage(a, fmt.Sprintf("📛 %v restored this area's name to %q (it was %q).", oocDisplayName(client), a.Name(), old))
	addToBuffer(client, "CMD", fmt.Sprintf("Restored the area name from %q to %q.", old, a.Name()), false)
}

// reportAreaName answers the bare "/area rename" with what the room is called
// now and what it will fall back to.
func reportAreaName(client *Client, a *area.Area) {
	if a.Renamed() {
		client.SendServerMessage(fmt.Sprintf("This area is called %q (renamed; %q in the config). Use /area rename <name> to change it, or /area unrename to restore it now.",
			a.Name(), a.DefaultName()))
		return
	}
	client.SendServerMessage(fmt.Sprintf("This area is called %q. Use /area rename <name> to rename it.", a.Name()))
}

// areaNameRejection reports why name cannot be given to area a, or "" when it
// can. Every rule here exists because the name is not just a label: it is the
// key clients send back to change area (pktAM), the string the area list is
// built from, and the directory area logs are written to.
func areaNameRejection(a *area.Area, name string) string {
	if name == "" {
		return "Give the area a name: /area rename <name>"
	}
	// The name becomes a log-directory path component (logger.sanitizeAreaName
	// strips "/" and "\" but leaves "." alone), so the literal names "." and
	// ".." would let a rename escape the configured log directory via
	// filepath.Join(LogPath, ".."). Rejected outright, exact match only --
	// this is not about names merely containing a dot.
	if name == "." || name == ".." {
		return "Area names cannot be \".\" or \"..\"."
	}
	if utf8.RuneCountInString(name) > maxAreaNameLen {
		return fmt.Sprintf("That name is %d characters; the limit is %d so the area list stays readable.",
			utf8.RuneCountInString(name), maxAreaNameLen)
	}
	if i := strings.IndexAny(name, areaNameReserved); i >= 0 {
		return fmt.Sprintf("Area names cannot contain %q — AO2 spends %s on its packet format.",
			string(name[i]), strings.Join(strings.Split(areaNameReserved, ""), " "))
	}
	for _, r := range name {
		// Control characters would break the area list on the client and the
		// per-line format of the area log.
		if unicode.IsControl(r) {
			return "Area names cannot contain control characters (including tabs and newlines)."
		}
	}
	// The name is how a client asks to move ("MC" carries it), and pktAM tests
	// the music list first, so a name that is also a track would make the room
	// unreachable by name and play a song instead.
	if isMusicURL(name) || sliceutil.ContainsString(getMusicList(), name) {
		return "That name is also a music-list entry, which would make this area unreachable by name."
	}
	// Collide with neither what another area is called now nor what it is
	// called in the config: a configured name can come back at any moment (the
	// moment that area empties), and a collision then would be silent.
	for _, other := range areas {
		if other == a {
			continue
		}
		if strings.EqualFold(other.Name(), name) {
			return fmt.Sprintf("Another area is already called %q.", other.Name())
		}
		if strings.EqualFold(other.DefaultName(), name) {
			return fmt.Sprintf("%q is another area's configured name, which it reverts to whenever it empties.", other.DefaultName())
		}
	}
	return ""
}

// republishAreaNames rebuilds everything derived from the area names and hands
// the new list to every connected client. Callers must hold areaRenameMu.
//
// Three things are derived from a name, and all three have to move together:
// the "#"-joined string pktAM matches an area change against, the pre-built SM
// blob every joining client is sent, and the area list the clients already
// connected are holding.
//
// The last of those is why this sends FA rather than a fresh SM. SM is a
// handshake packet: a client that receives one mid-session re-runs the join
// ladder and answers with RD. FA carries the area list on its own and is what
// area-list changes are normally delivered with, so a client can take it
// without being dragged back through its own handshake. (pktReqDone ignores a
// second RD anyway, so the SM route would not have broken anything -- it would
// just have shown everyone a loading screen to change one string.)
func republishAreaNames() {
	names := make([]string, 0, len(areas))
	for _, a := range areas {
		names = append(names, a.Name())
	}
	joined := strings.Join(names, "#")
	setAreaNames(joined)
	setSMPacket(buildSMPacket(joined, getMusicList()))
	broadcastToAll(&packet.FA{Areas: names})
}

// resetAreaName restores a's configured name and republishes, reporting whether
// there was a rename to undo. Safe to call unconditionally.
func resetAreaName(a *area.Area) bool {
	if a == nil {
		return false
	}
	areaRenameMu.Lock()
	defer areaRenameMu.Unlock()
	if !a.ResetName() {
		return false
	}
	republishAreaNames()
	return true
}

// releaseAreaNameOnEmpty gives an area its configured name back once the last
// person has left it. Called from the same departure paths that Area.Reset()
// is, which deliberately does not touch the name itself (a rename is visible
// server-wide and undoing it needs a republish the area package cannot do).
func releaseAreaNameOnEmpty(a *area.Area) {
	if a == nil || !a.Renamed() {
		return
	}
	old := a.Name()
	if resetAreaName(a) {
		logger.LogInfof("Area %q reverted to its configured name %q: the room is empty.", old, a.Name())
	}
}

// releaseAreaNameOnLastCMLeaving gives an area its configured name back when the
// CM who was running it is gone.
//
// It applies only to a name a CM actually took out (Area.NameHeldByCM). A
// moderator does not have to be a CM to rename a room, and for a room that
// never had one "no CMs are left" is true from the moment of the rename -- so
// without that condition a moderator's name would snap back on the next
// unrelated departure. Such a room keeps its name until it empties instead.
func releaseAreaNameOnLastCMLeaving(a *area.Area) {
	if a == nil || !a.Renamed() || !a.NameHeldByCM() || len(a.CMs()) != 0 {
		return
	}
	old := a.Name()
	if resetAreaName(a) {
		sendAreaServerMessage(a, fmt.Sprintf("📛 This area's name reverted to %q — %q was its CM's name for it, and it has no CM left.",
			a.Name(), old))
		logger.LogInfof("Area %q reverted to its configured name %q: no CMs left.", old, a.Name())
	}
}

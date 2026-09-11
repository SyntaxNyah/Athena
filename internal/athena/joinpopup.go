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
	"strings"

	"github.com/MangosArentLiterature/Athena/internal/packet"
)

// Join popup: an optional, operator-authored welcome message shown once to
// any IPID connecting to this server for the very first time ever.
//
// It exists for operators who want rules, a Discord invite, or a website link
// in front of a brand-new player before anything else happens -- reusing the
// same client-side AO2 BB dialog the join captcha (joincaptcha.go) uses to
// make its question unmissable, here repurposed for a static message instead
// of a generated one.
//
// Unlike the join captcha this is not a gate: it blocks nothing, has no
// strikes, and never stands between a player and speaking. "First time ever"
// is recordIPFirstSeen's notion of new, the same one every new-IPID cooldown
// and exemption in this server agrees on -- never seen in the database, and
// not seen yet this session -- not merely "unseen recently", so a returning
// player is never shown it again, restart or not.

// joinPopupMessage returns the configured welcome message, trimmed, and
// whether the feature actually has something to show. A blank message is
// treated as off even when join_popup is true, so switching the toggle on
// without writing anything is a silent no-op rather than an empty dialog.
func joinPopupMessage() (string, bool) {
	if config == nil || !config.JoinPopup {
		return "", false
	}
	msg := strings.TrimSpace(config.JoinPopupMessage)
	return msg, msg != ""
}

// issueJoinPopup shows the operator's welcome message to a connection whose
// IPID has never connected to this server before. Called once from
// pktReqDone, ahead of the account/casino welcome and the MOTD, so a
// genuinely new player sees the operator's own message first, before
// anything else in the join sequence.
//
// Sent two ways, mirroring the join captcha's question: an OOC copy that
// stays in the log after it scrolls by, and the same text in a client-side
// AO2 BB dialog (a modal on desktop AO2, a notice on WebAO) so it isn't
// missed by someone not yet watching the OOC tab.
//
// config.JoinPopupMessage is operator-authored config content, not player
// input, so -- like the MOTD -- it goes out exactly as written; there is
// nothing here that needs filtering.
func issueJoinPopup(client *Client) {
	if !client.isNewIPID {
		return
	}
	msg, ok := joinPopupMessage()
	if !ok {
		return
	}
	client.SendServerMessage(msg)
	client.Send(&packet.BB{Message: encode(msg)})
}

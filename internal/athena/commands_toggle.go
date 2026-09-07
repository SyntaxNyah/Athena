/* Athena - A server for Attorney Online 2 written in Go
   Nyathena fork addition: /toggle global -- a self-service OOC-hide toggle. */

package athena

import "strings"

// Self-service OOC-hide toggle (/toggle global). Some players find the
// constant stream of area OOC chat and /global broadcasts distracting and
// would rather just not see it. This mutes it for the toggling client only:
// it never touches what anyone else sees, and it defaults back to visible
// on every fresh connection. Direct messages (/pm) and staff/system
// broadcasts (/mod -g, /modchat, /announce, moderation alerts) are
// deliberately untouched -- see broadcastOOCToArea/broadcastOOCToAll in
// server.go, the only two call sites that consult this flag.

// OOCHidden reports whether this client has muted regular OOC chat (area
// OOC and /global) for their current session via /toggle global.
func (client *Client) OOCHidden() bool {
	return client.oocHidden.Load()
}

// SetOOCHidden mutes or unmutes regular OOC chat for this client's session.
func (client *Client) SetOOCHidden(hidden bool) {
	client.oocHidden.Store(hidden)
}

// cmdToggle handles /toggle <global>. Currently the only supported setting
// is "global", which flips OOC-hiding on/off for the caller; running it
// again switches it back.
func cmdToggle(client *Client, args []string, usage string) {
	switch strings.ToLower(strings.TrimSpace(args[0])) {
	case "global":
		if client.OOCHidden() {
			client.SetOOCHidden(false)
			client.SendServerMessage("🔊 OOC chat is now visible again.")
		} else {
			client.SetOOCHidden(true)
			client.SendServerMessage("🔇 OOC chat is now hidden (area OOC and /global). Direct messages and staff announcements still get through. Run /toggle global again to show it.")
		}
	default:
		client.SendServerMessage("Invalid argument:\n" + usage)
	}
}

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
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/MangosArentLiterature/Athena/internal/db"
)

// validCustomTagIDRe constrains custom tag ids to lowercase letters, digits,
// and underscores. Limiting the alphabet keeps ids easy to type for players
// and avoids any chance of confusion with shell metacharacters in admin scripts.
var validCustomTagIDRe = regexp.MustCompile(`^[a-z0-9_]{2,32}$`)

// maxCustomTagNameLen caps the human-readable display name. Long names look
// bad next to character names in /gas and /players.
const maxCustomTagNameLen = 30

// cmdCreateTag handles /createtag <id> <display name>.
//
// Moderators use this to mint a new cosmetic tag without rebuilding the
// server. The id becomes the handle used by /grantcustomtag and /settag; the
// display name is what shows in [brackets] beside a player's character name.
//
// Example: /createtag founder ⭐ Founder
func cmdCreateTag(client *Client, args []string, usage string) {
	if len(args) < 2 {
		client.SendServerMessage(usage)
		return
	}
	id := strings.ToLower(args[0])
	name := strings.TrimSpace(strings.Join(args[1:], " "))

	if !validCustomTagIDRe.MatchString(id) {
		client.SendServerMessage("Invalid tag id. Ids must be 2–32 characters of lowercase letters, digits, or underscores.")
		return
	}
	if name == "" {
		client.SendServerMessage("Tag name cannot be empty.")
		return
	}
	if len(name) > maxCustomTagNameLen {
		client.SendServerMessage(fmt.Sprintf("Tag name is too long (max %d characters).", maxCustomTagNameLen))
		return
	}
	if strings.ContainsAny(name, "[]") {
		client.SendServerMessage("Tag name cannot contain '[' or ']'.")
		return
	}
	if _, ok := shopItemByID(id); ok {
		client.SendServerMessage(fmt.Sprintf("Id '%v' is already used by a built-in shop item. Pick a different id.", id))
		return
	}
	if _, ok := db.GetCustomTag(id); ok {
		client.SendServerMessage(fmt.Sprintf("A custom tag with id '%v' already exists. Use /deletetag %v first if you want to redefine it.", id, id))
		return
	}

	if err := db.CreateCustomTag(id, name, client.ModName()); err != nil {
		client.SendServerMessage("Failed to create custom tag: " + err.Error())
		return
	}

	client.SendServerMessage(fmt.Sprintf(
		"✅ Custom tag created: [%v]  (id: %v)\n"+
			"Grant it with: /grantcustomtag <username> %v\n"+
			"Players equip with: /settag %v",
		name, id, id, id))
	addToBuffer(client, "CMD", fmt.Sprintf("Created custom tag '%v' (id %v).", name, id), true)
}

// cmdDeleteTag handles /deletetag <id>.
//
// Removes a custom tag definition along with every player's ownership of it
// and unequips it from anyone currently wearing it. Built-in shop tag ids
// cannot be deleted.
func cmdDeleteTag(client *Client, args []string, usage string) {
	if len(args) < 1 {
		client.SendServerMessage(usage)
		return
	}
	id := strings.ToLower(args[0])

	if _, ok := shopItemByID(id); ok {
		client.SendServerMessage("That id belongs to a built-in shop tag and cannot be deleted via /deletetag.")
		return
	}
	name, ok := db.GetCustomTag(id)
	if !ok {
		client.SendServerMessage(fmt.Sprintf("No custom tag with id '%v' exists. See /listcustomtags.", id))
		return
	}

	if err := db.DeleteCustomTag(id); err != nil {
		client.SendServerMessage("Failed to delete custom tag: " + err.Error())
		return
	}

	client.SendServerMessage(fmt.Sprintf("🗑️ Deleted custom tag [%v] (id: %v). Anyone wearing it has been unequipped.", name, id))
	addToBuffer(client, "CMD", fmt.Sprintf("Deleted custom tag '%v' (id %v).", name, id), true)
}

// cmdListCustomTags handles /listcustomtags.
//
// Visible to everyone so players can see what staff-defined tags exist on
// the server before asking to be granted one.
func cmdListCustomTags(client *Client, _ []string, _ string) {
	tags, err := db.ListCustomTags()
	if err != nil {
		client.SendServerMessage("Failed to list custom tags: " + err.Error())
		return
	}
	if len(tags) == 0 {
		client.SendServerMessage("No custom tags have been created yet. Moderators can mint one with /createtag <id> <name>.")
		return
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("🏷️ Custom Tags (%d):\n", len(tags)))
	for _, t := range tags {
		creator := t.CreatedBy
		if creator == "" {
			creator = "?"
		}
		when := time.Unix(t.CreatedAt, 0).UTC().Format("2006-01-02")
		b.WriteString(fmt.Sprintf("  [%v]  id=%v  by %v on %v\n", t.Name, t.ID, creator, when))
	}
	b.WriteString("\nModerators grant with /grantcustomtag <username> <id>. Players equip with /settag <id>.")
	client.SendServerMessage(b.String())
}

// cmdGrantCustomTag handles /grantcustomtag <username> <tag_id>.
//
// Grants ownership of a tag (built-in or custom) to a registered player
// account, identified by username — not UID. Online presence is not required.
// The recipient can then equip the tag with /settag <id>.
//
// Granting an already-owned tag is a no-op (idempotent).
func cmdGrantCustomTag(client *Client, args []string, usage string) {
	if len(args) < 2 {
		client.SendServerMessage(usage)
		return
	}
	targetUser := args[0]
	tagID := strings.ToLower(args[1])

	// Resolve display name + verify the id refers to an actual tag.
	var displayName string
	if it, ok := shopItemByID(tagID); ok {
		if it.kind != shopKindTag {
			client.SendServerMessage(fmt.Sprintf("'%v' is a pass, not a tag — /grantcustomtag only grants tags.", tagID))
			return
		}
		displayName = it.name
	} else if name, ok := db.GetCustomTag(tagID); ok {
		displayName = name
	} else {
		client.SendServerMessage(fmt.Sprintf("Unknown tag id '%v'. See /listcustomtags or /shop.", tagID))
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
				"Ask the player to /login at least once before granting tags.",
			targetUser))
		return
	}

	if db.HasShopItem(ipid, tagID) {
		client.SendServerMessage(fmt.Sprintf("'%v' already owns [%v]. Nothing to do.", targetUser, displayName))
		return
	}

	if err := db.GrantShopItem(ipid, tagID); err != nil {
		client.SendServerMessage("Failed to grant tag: " + err.Error())
		return
	}

	client.SendServerMessage(fmt.Sprintf("✅ Granted [%v] to '%v'. They can equip it with /settag %v.", displayName, targetUser, tagID))

	// Notify the recipient if they are online right now.
	clients.ForEach(func(c *Client) {
		if c.ModName() == targetUser {
			c.SendServerMessage(fmt.Sprintf("🎁 A moderator granted you a new tag: [%v]. Equip it with /settag %v", displayName, tagID))
		}
	})

	addToBuffer(client, "CMD", fmt.Sprintf("Granted tag '%v' (id %v) to '%v'.", displayName, tagID, targetUser), true)
}

// cmdRevokeCustomTag handles /revokecustomtag <username> <tag_id>.
//
// Removes a previously granted tag from a registered player. If they were
// wearing it, their active tag is cleared.
func cmdRevokeCustomTag(client *Client, args []string, usage string) {
	if len(args) < 2 {
		client.SendServerMessage(usage)
		return
	}
	targetUser := args[0]
	tagID := strings.ToLower(args[1])

	ipid, err := db.GetIPIDByUsername(targetUser)
	if err != nil {
		client.SendServerMessage("Failed to look up account: " + err.Error())
		return
	}
	if ipid == "" {
		client.SendServerMessage(fmt.Sprintf("Account '%v' was not found.", targetUser))
		return
	}

	if err := db.RevokeShopItem(ipid, tagID); err != nil {
		client.SendServerMessage(fmt.Sprintf("Failed to revoke tag from '%v': %v", targetUser, err))
		return
	}

	displayName := tagID
	if name, ok := lookupTag(tagID); ok {
		displayName = name
	}

	client.SendServerMessage(fmt.Sprintf("🗑️ Revoked [%v] from '%v'.", displayName, targetUser))
	addToBuffer(client, "CMD", fmt.Sprintf("Revoked tag id '%v' from '%v'.", tagID, targetUser), true)
}

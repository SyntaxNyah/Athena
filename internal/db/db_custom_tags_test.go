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

package db

import (
	"testing"
)

func TestCreateAndLookupCustomTag(t *testing.T) {
	teardown := setupTestDB(t)
	defer teardown()

	if err := CreateCustomTag("founder", "⭐ Founder", "admin"); err != nil {
		t.Fatalf("CreateCustomTag failed: %v", err)
	}

	name, ok := GetCustomTag("founder")
	if !ok {
		t.Fatal("GetCustomTag returned ok=false for an existing tag")
	}
	if name != "⭐ Founder" {
		t.Errorf("expected name='⭐ Founder', got %q", name)
	}

	// Unknown id should return ok=false.
	if _, ok := GetCustomTag("does_not_exist"); ok {
		t.Error("GetCustomTag returned ok=true for an unknown id")
	}
}

func TestCreateCustomTagDuplicateID(t *testing.T) {
	teardown := setupTestDB(t)
	defer teardown()

	if err := CreateCustomTag("dup", "First", "admin"); err != nil {
		t.Fatalf("first CreateCustomTag failed: %v", err)
	}
	if err := CreateCustomTag("dup", "Second", "admin"); err == nil {
		t.Fatal("expected error on duplicate id, got nil")
	}
}

func TestListCustomTags(t *testing.T) {
	teardown := setupTestDB(t)
	defer teardown()

	if err := CreateCustomTag("a", "Alpha", "admin"); err != nil {
		t.Fatalf("CreateCustomTag a failed: %v", err)
	}
	if err := CreateCustomTag("b", "Beta", "admin"); err != nil {
		t.Fatalf("CreateCustomTag b failed: %v", err)
	}

	tags, err := ListCustomTags()
	if err != nil {
		t.Fatalf("ListCustomTags failed: %v", err)
	}
	if len(tags) != 2 {
		t.Fatalf("expected 2 tags, got %d", len(tags))
	}
	if tags[0].ID != "a" || tags[1].ID != "b" {
		t.Errorf("unexpected ordering: %v", tags)
	}
}

func TestDeleteCustomTagCleansUpReferences(t *testing.T) {
	teardown := setupTestDB(t)
	defer teardown()

	const id = "ephemeral"
	if err := CreateCustomTag(id, "Ephemeral", "admin"); err != nil {
		t.Fatalf("CreateCustomTag failed: %v", err)
	}

	// Grant the tag and equip it for one player.
	if err := GrantShopItem("ipid_player", id); err != nil {
		t.Fatalf("GrantShopItem failed: %v", err)
	}
	if !HasShopItem("ipid_player", id) {
		t.Fatal("HasShopItem returned false after grant")
	}
	if err := SetActiveTag("ipid_player", id); err != nil {
		t.Fatalf("SetActiveTag failed: %v", err)
	}
	if got := GetActiveTag("ipid_player"); got != id {
		t.Fatalf("expected active tag %q, got %q", id, got)
	}

	if err := DeleteCustomTag(id); err != nil {
		t.Fatalf("DeleteCustomTag failed: %v", err)
	}

	// Custom tag definition gone.
	if _, ok := GetCustomTag(id); ok {
		t.Error("GetCustomTag still returns ok=true after delete")
	}
	// Ownership row removed.
	if HasShopItem("ipid_player", id) {
		t.Error("HasShopItem still true after delete; SHOP_PURCHASES not cleaned up")
	}
	// Active tag pointer cleared.
	if got := GetActiveTag("ipid_player"); got != "" {
		t.Errorf("expected active tag cleared, got %q", got)
	}
}

func TestDeleteCustomTagUnknown(t *testing.T) {
	teardown := setupTestDB(t)
	defer teardown()

	if err := DeleteCustomTag("nope"); err == nil {
		t.Fatal("expected error for deleting unknown tag, got nil")
	}
}

// TestGetPlayerShopItemsIncludesCustomTags pins the fact /mytags and the
// /shop items fix both depend on: a custom tag grant lands in the exact
// same SHOP_PURCHASES ownership table as a built-in shop tag, so
// GetPlayerShopItems returns both kinds of id side by side rather than
// only the built-in ones.
func TestGetPlayerShopItemsIncludesCustomTags(t *testing.T) {
	teardown := setupTestDB(t)
	defer teardown()

	if err := CreateCustomTag("founder", "⭐ Founder", "admin"); err != nil {
		t.Fatalf("CreateCustomTag failed: %v", err)
	}
	if err := GrantShopItem("ipid_z", "founder"); err != nil {
		t.Fatalf("GrantShopItem (custom) failed: %v", err)
	}
	if err := GrantShopItem("ipid_z", "tag_gambler"); err != nil {
		t.Fatalf("GrantShopItem (built-in) failed: %v", err)
	}

	items, err := GetPlayerShopItems("ipid_z")
	if err != nil {
		t.Fatalf("GetPlayerShopItems failed: %v", err)
	}
	want := map[string]bool{"founder": false, "tag_gambler": false}
	for _, id := range items {
		if _, ok := want[id]; ok {
			want[id] = true
		}
	}
	for id, found := range want {
		if !found {
			t.Errorf("GetPlayerShopItems did not include %q; got %v", id, items)
		}
	}
}

func TestGrantShopItemIdempotent(t *testing.T) {
	teardown := setupTestDB(t)
	defer teardown()

	if err := GrantShopItem("ipid_x", "tag_gambler"); err != nil {
		t.Fatalf("first GrantShopItem failed: %v", err)
	}
	// Second grant must not fail (INSERT OR IGNORE).
	if err := GrantShopItem("ipid_x", "tag_gambler"); err != nil {
		t.Fatalf("second GrantShopItem failed: %v", err)
	}
	if !HasShopItem("ipid_x", "tag_gambler") {
		t.Error("HasShopItem returned false after grant")
	}
}

func TestRevokeShopItem(t *testing.T) {
	teardown := setupTestDB(t)
	defer teardown()

	if err := GrantShopItem("ipid_y", "tag_lucky"); err != nil {
		t.Fatalf("GrantShopItem failed: %v", err)
	}
	if err := SetActiveTag("ipid_y", "tag_lucky"); err != nil {
		t.Fatalf("SetActiveTag failed: %v", err)
	}

	if err := RevokeShopItem("ipid_y", "tag_lucky"); err != nil {
		t.Fatalf("RevokeShopItem failed: %v", err)
	}
	if HasShopItem("ipid_y", "tag_lucky") {
		t.Error("HasShopItem still true after revoke")
	}
	if got := GetActiveTag("ipid_y"); got != "" {
		t.Errorf("expected active tag cleared after revoking the equipped tag, got %q", got)
	}

	// Revoking again is an error (nothing to revoke).
	if err := RevokeShopItem("ipid_y", "tag_lucky"); err == nil {
		t.Error("expected error revoking already-revoked tag")
	}
}

func TestGetIPIDByUsername(t *testing.T) {
	teardown := setupTestDB(t)
	defer teardown()

	if err := RegisterPlayer("alice", []byte("password1"), "ipid_alice"); err != nil {
		t.Fatalf("RegisterPlayer failed: %v", err)
	}

	ipid, err := GetIPIDByUsername("alice")
	if err != nil {
		t.Fatalf("GetIPIDByUsername failed: %v", err)
	}
	if ipid != "ipid_alice" {
		t.Errorf("expected ipid_alice, got %q", ipid)
	}

	// Unknown account → empty string, no error.
	ipid, err = GetIPIDByUsername("bob")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ipid != "" {
		t.Errorf("expected empty ipid for unknown user, got %q", ipid)
	}
}

func TestIsModUser(t *testing.T) {
	teardown := setupTestDB(t)
	defer teardown()

	// Player accounts have permissions=0.
	if err := RegisterPlayer("playerA", []byte("password1"), "ipid_a"); err != nil {
		t.Fatalf("RegisterPlayer failed: %v", err)
	}
	if IsModUser("playerA") {
		t.Error("IsModUser returned true for a player account")
	}

	// Mod accounts have non-zero permissions.
	if err := CreateUser("modB", []byte("password1"), 0xFFFF); err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	if !IsModUser("modB") {
		t.Error("IsModUser returned false for a moderator account")
	}

	// Unknown user → false.
	if IsModUser("does_not_exist") {
		t.Error("IsModUser returned true for an unknown username")
	}
}

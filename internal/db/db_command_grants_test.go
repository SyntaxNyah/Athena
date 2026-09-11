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

package db

import (
	"database/sql"
	"errors"
	"testing"
)

func TestAddAndListCommandGrant(t *testing.T) {
	teardown := setupTestDB(t)
	defer teardown()

	if err := AddCommandGrant("alice", "ban", "console", 1000); err != nil {
		t.Fatalf("AddCommandGrant failed: %v", err)
	}

	rows, err := ListCommandGrants("alice")
	if err != nil {
		t.Fatalf("ListCommandGrants failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 grant, got %d", len(rows))
	}
	if rows[0].Command != "ban" || rows[0].GrantedBy != "console" || rows[0].GrantedAt != 1000 {
		t.Errorf("unexpected grant row: %+v", rows[0])
	}
}

func TestAddCommandGrantIsIdempotentPerCommand(t *testing.T) {
	teardown := setupTestDB(t)
	defer teardown()

	if err := AddCommandGrant("alice", "ban", "console", 1000); err != nil {
		t.Fatalf("AddCommandGrant failed: %v", err)
	}
	// Re-granting the same command overwrites the issuer/timestamp instead of
	// creating a duplicate row -- mirrors AddMusicBan/AddRandomCharCurse.
	if err := AddCommandGrant("alice", "ban", "console2", 2000); err != nil {
		t.Fatalf("re-grant failed: %v", err)
	}

	rows, err := ListCommandGrants("alice")
	if err != nil {
		t.Fatalf("ListCommandGrants failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 grant after re-grant, got %d", len(rows))
	}
	if rows[0].GrantedBy != "console2" || rows[0].GrantedAt != 2000 {
		t.Errorf("re-grant should overwrite issuer/timestamp, got %+v", rows[0])
	}
}

func TestMultipleCommandsPerAccount(t *testing.T) {
	teardown := setupTestDB(t)
	defer teardown()

	for _, c := range []string{"ban", "kick", "mute"} {
		if err := AddCommandGrant("alice", c, "console", 1000); err != nil {
			t.Fatalf("AddCommandGrant(%v) failed: %v", c, err)
		}
	}
	rows, err := ListCommandGrants("alice")
	if err != nil {
		t.Fatalf("ListCommandGrants failed: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 grants, got %d", len(rows))
	}
}

func TestRemoveCommandGrant(t *testing.T) {
	teardown := setupTestDB(t)
	defer teardown()

	if err := AddCommandGrant("alice", "ban", "console", 1000); err != nil {
		t.Fatalf("AddCommandGrant failed: %v", err)
	}
	if err := RemoveCommandGrant("alice", "ban"); err != nil {
		t.Fatalf("RemoveCommandGrant failed: %v", err)
	}
	rows, err := ListCommandGrants("alice")
	if err != nil {
		t.Fatalf("ListCommandGrants failed: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected 0 grants after removal, got %d", len(rows))
	}

	// Removing an already-absent grant reports sql.ErrNoRows so callers can
	// distinguish "revoked something real" from "nothing to revoke".
	if err := RemoveCommandGrant("alice", "ban"); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("expected sql.ErrNoRows removing an absent grant, got %v", err)
	}
}

func TestRemoveAllCommandGrants(t *testing.T) {
	teardown := setupTestDB(t)
	defer teardown()

	for _, c := range []string{"ban", "kick", "mute"} {
		if err := AddCommandGrant("alice", c, "console", 1000); err != nil {
			t.Fatalf("AddCommandGrant(%v) failed: %v", c, err)
		}
	}
	if err := AddCommandGrant("bob", "ban", "console", 1000); err != nil {
		t.Fatalf("AddCommandGrant for bob failed: %v", err)
	}

	n, err := RemoveAllCommandGrants("alice")
	if err != nil {
		t.Fatalf("RemoveAllCommandGrants failed: %v", err)
	}
	if n != 3 {
		t.Errorf("expected 3 removed, got %d", n)
	}

	rows, err := ListCommandGrants("alice")
	if err != nil {
		t.Fatalf("ListCommandGrants failed: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected alice to have 0 grants left, got %d", len(rows))
	}

	// bob's grant must be untouched -- revoking "all" for one account must
	// never reach another account's rows.
	bobRows, err := ListCommandGrants("bob")
	if err != nil {
		t.Fatalf("ListCommandGrants(bob) failed: %v", err)
	}
	if len(bobRows) != 1 {
		t.Fatalf("expected bob to keep 1 grant, got %d", len(bobRows))
	}
}

func TestListAllCommandGrants(t *testing.T) {
	teardown := setupTestDB(t)
	defer teardown()

	if err := AddCommandGrant("alice", "ban", "console", 1000); err != nil {
		t.Fatalf("AddCommandGrant failed: %v", err)
	}
	if err := AddCommandGrant("bob", "kick", "console", 2000); err != nil {
		t.Fatalf("AddCommandGrant failed: %v", err)
	}

	rows, err := ListAllCommandGrants()
	if err != nil {
		t.Fatalf("ListAllCommandGrants failed: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 total grants, got %d", len(rows))
	}
}

// TestRenameAccountCarriesCommandGrants verifies that /resetusername (backed
// by RenameAccount) moves any console-issued command grants to the new
// username instead of silently orphaning them under the old one.
func TestRenameAccountCarriesCommandGrants(t *testing.T) {
	teardown := setupTestDB(t)
	defer teardown()

	if err := RegisterPlayer("oldname", []byte("password1"), "ipid_1"); err != nil {
		t.Fatalf("RegisterPlayer failed: %v", err)
	}
	if err := AddCommandGrant("oldname", "ban", "console", 1000); err != nil {
		t.Fatalf("AddCommandGrant failed: %v", err)
	}

	if err := RenameAccount("oldname", "newname"); err != nil {
		t.Fatalf("RenameAccount failed: %v", err)
	}

	oldRows, err := ListCommandGrants("oldname")
	if err != nil {
		t.Fatalf("ListCommandGrants(oldname) failed: %v", err)
	}
	if len(oldRows) != 0 {
		t.Errorf("expected no grants left under the old username, got %d", len(oldRows))
	}

	newRows, err := ListCommandGrants("newname")
	if err != nil {
		t.Fatalf("ListCommandGrants(newname) failed: %v", err)
	}
	if len(newRows) != 1 || newRows[0].Command != "ban" {
		t.Fatalf("expected the ban grant to have followed the rename, got %+v", newRows)
	}
}

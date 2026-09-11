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

package athena

import (
	"io"
	"net"
	"os"
	"testing"

	"github.com/MangosArentLiterature/Athena/internal/db"
	"github.com/MangosArentLiterature/Athena/internal/permissions"
)

// setupCommandGrantsTestDB opens a fresh temp DB, matching the pattern every
// other DB-backed athena package test uses, and restores the in-memory grant
// cache to empty afterwards so one test can never leak a grant into another.
func setupCommandGrantsTestDB(t *testing.T) func() {
	t.Helper()
	tmp, err := os.CreateTemp("", "athena-cmdgrants-*.db")
	if err != nil {
		t.Fatalf("failed to create temp db: %v", err)
	}
	tmp.Close()
	db.DBPath = tmp.Name()
	if err := db.Open(); err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	emptyGrants := make(map[string]map[string]struct{})
	setCommandGrants(emptyGrants)
	return func() {
		db.Close()
		os.Remove(tmp.Name())
		setCommandGrants(make(map[string]map[string]struct{}))
	}
}

// newGrantTestClient builds a real Client over a net.Pipe whose peer is
// continuously drained, matching newCurseTestClient's shape.
func newGrantTestClient(t *testing.T) *Client {
	t.Helper()
	a, b := net.Pipe()
	c := NewClient(a, "grant-test-ipid")
	go c.runWriter()
	go io.Copy(io.Discard, b)
	t.Cleanup(func() {
		c.markClosed()
		b.Close()
	})
	return c
}

// TestAccountHasCommandGrant exercises the pure in-memory lookup the hot
// dispatch path uses, independent of the database.
func TestAccountHasCommandGrant(t *testing.T) {
	defer setCommandGrants(nil)

	setCommandGrants(map[string]map[string]struct{}{
		"alice": {"ban": {}, "kick": {}},
	})

	if !accountHasCommandGrant("alice", "ban") {
		t.Error("alice should hold a grant for ban")
	}
	if !accountHasCommandGrant("alice", "BAN") {
		t.Error("grant lookup must be case-insensitive on the command name")
	}
	if accountHasCommandGrant("alice", "mute") {
		t.Error("alice was never granted mute")
	}
	if accountHasCommandGrant("bob", "ban") {
		t.Error("a grant on alice must never leak to a different username")
	}
	if accountHasCommandGrant("", "ban") {
		t.Error("an empty (never-authenticated) username must never match")
	}
}

// TestClientCanUseCommandChecksGrant is the end-to-end proof that the single
// dispatch chokepoint (clientCanUseCommand) picks up a grant: a plain
// zero-permission account is refused an ADMIN-tier command, then allowed the
// instant a grant for that exact command name is published, and still
// refused for every other command.
func TestClientCanUseCommandChecksGrant(t *testing.T) {
	defer setCommandGrants(nil)

	c := newGrantTestClient(t)
	c.SetAuthenticated(true)
	c.SetModName("alice")
	c.SetPerms(0) // plain registered player: no role permissions at all

	adminCmd := Command{reqPerms: permissions.PermissionField["ADMIN"]}
	banCmd := Command{reqPerms: permissions.PermissionField["BAN"]}

	if clientCanUseCommand(c, "purgedb", adminCmd) {
		t.Fatal("an ungranted zero-permission account must not reach an ADMIN command")
	}

	setCommandGrants(map[string]map[string]struct{}{
		"alice": {"purgedb": {}},
	})

	if !clientCanUseCommand(c, "purgedb", adminCmd) {
		t.Error("clientCanUseCommand must allow a command the account was explicitly granted, regardless of its reqPerms tier")
	}
	if clientCanUseCommand(c, "ban", banCmd) {
		t.Error("a grant for one command must never leak into a different command")
	}
}

// TestClientCanUseCommandGrantRequiresAuthentication guards that a grant on
// an account name can never be reached by a connection that hasn't actually
// logged into (or registered) that account.
func TestClientCanUseCommandGrantRequiresAuthentication(t *testing.T) {
	defer setCommandGrants(nil)

	c := newGrantTestClient(t)
	c.SetModName("alice") // ModName set, but never authenticated
	c.SetPerms(0)

	setCommandGrants(map[string]map[string]struct{}{
		"alice": {"ban": {}},
	})

	banCmd := Command{reqPerms: permissions.PermissionField["BAN"]}
	if clientCanUseCommand(c, "ban", banCmd) {
		t.Fatal("a grant must never apply to an unauthenticated connection, even if ModName happens to match")
	}
}

// TestGrantCommandRejectsUnknownCommand ensures a console typo fails loudly
// at grant time instead of silently granting nothing.
func TestGrantCommandRejectsUnknownCommand(t *testing.T) {
	defer setupCommandGrantsTestDB(t)()
	initCommands()

	if _, err := grantCommand("alice", "notarealcommand", "console"); err == nil {
		t.Fatal("expected an error granting a command that does not exist in the registry")
	}
	if accountHasCommandGrant("alice", "notarealcommand") {
		t.Error("a rejected grant must not appear in the live cache")
	}
}

// TestGrantAndRevokeCommandRoundTrip exercises the full athena-level flow a
// console grantcmd/revokecmd invocation drives: DB write, cache reload, and
// revocation clearing the cache again.
func TestGrantAndRevokeCommandRoundTrip(t *testing.T) {
	defer setupCommandGrantsTestDB(t)()
	initCommands()

	if _, err := grantCommand("alice", "Ban", "console"); err != nil {
		t.Fatalf("grantCommand failed: %v", err)
	}
	// Command names are normalized to lowercase regardless of how the
	// console operator typed them.
	if !accountHasCommandGrant("alice", "ban") {
		t.Fatal("expected alice to hold a grant for ban after grantCommand")
	}

	rows, err := db.ListCommandGrants("alice")
	if err != nil {
		t.Fatalf("ListCommandGrants failed: %v", err)
	}
	if len(rows) != 1 || rows[0].Command != "ban" {
		t.Fatalf("expected the DB row to record the lowercase command name, got %+v", rows)
	}

	if err := revokeCommand("alice", "ban"); err != nil {
		t.Fatalf("revokeCommand failed: %v", err)
	}
	if accountHasCommandGrant("alice", "ban") {
		t.Error("expected the grant to be gone from the live cache after revokeCommand")
	}
}

// TestRevokeAllCommandGrants exercises the "revokecmd <username> all" path.
func TestRevokeAllCommandGrants(t *testing.T) {
	defer setupCommandGrantsTestDB(t)()
	initCommands()

	for _, cmd := range []string{"ban", "kick", "mute"} {
		if _, err := grantCommand("alice", cmd, "console"); err != nil {
			t.Fatalf("grantCommand(%v) failed: %v", cmd, err)
		}
	}
	if _, err := grantCommand("bob", "ban", "console"); err != nil {
		t.Fatalf("grantCommand for bob failed: %v", err)
	}

	n, err := revokeAllCommandGrants("alice")
	if err != nil {
		t.Fatalf("revokeAllCommandGrants failed: %v", err)
	}
	if n != 3 {
		t.Errorf("expected 3 grants revoked, got %d", n)
	}
	for _, cmd := range []string{"ban", "kick", "mute"} {
		if accountHasCommandGrant("alice", cmd) {
			t.Errorf("expected alice to no longer hold %v", cmd)
		}
	}
	if !accountHasCommandGrant("bob", "ban") {
		t.Error("revoking every grant for alice must never touch bob's own grant")
	}
}

// TestLoadCommandGrantsSeedsCacheFromDB verifies the startup path: grants
// written straight through the db package (as if from a previous server run)
// are picked up by loadCommandGrants without going through grantCommand.
func TestLoadCommandGrantsSeedsCacheFromDB(t *testing.T) {
	defer setupCommandGrantsTestDB(t)()

	if err := db.AddCommandGrant("alice", "ban", "console", 1000); err != nil {
		t.Fatalf("db.AddCommandGrant failed: %v", err)
	}
	// Cache starts empty in this test (setupCommandGrantsTestDB resets it).
	if accountHasCommandGrant("alice", "ban") {
		t.Fatal("cache should not see the grant before loadCommandGrants runs")
	}

	if err := loadCommandGrants(); err != nil {
		t.Fatalf("loadCommandGrants failed: %v", err)
	}
	if !accountHasCommandGrant("alice", "ban") {
		t.Error("loadCommandGrants should have seeded the grant into the live cache")
	}
}

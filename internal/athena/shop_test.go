/* Athena - A server for Attorney Online 2 written in Go
   Nyathena fork additions: tests for tag ownership resolution (/mytags,
   /shop items). */

package athena

import (
	"os"
	"testing"

	"github.com/MangosArentLiterature/Athena/internal/db"
)

// setupShopTestDB opens a throwaway temp DB, mirroring setupShadowDisconnectTestDB.
func setupShopTestDB(t *testing.T) func() {
	t.Helper()
	tmp, err := os.CreateTemp("", "athena-shop-*.db")
	if err != nil {
		t.Fatalf("failed to create temp db: %v", err)
	}
	tmp.Close()
	db.DBPath = tmp.Name()
	if err := db.Open(); err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	return func() {
		db.Close()
		os.Remove(tmp.Name())
	}
}

// TestLookupTagResolvesCustomTags pins the mechanism /mytags and the
// /shop items fix both rely on: a custom tag id (minted with /createtag)
// must resolve to its display name exactly like a built-in catalog tag
// does. /shop items used to call shopItemByID directly and silently drop
// any owned custom tag when that lookup missed; lookupTag is the fixed
// resolution path that falls back to db.GetCustomTag.
func TestLookupTagResolvesCustomTags(t *testing.T) {
	teardown := setupShopTestDB(t)
	defer teardown()

	if err := db.CreateCustomTag("founder", "⭐ Founder", "admin"); err != nil {
		t.Fatalf("CreateCustomTag failed: %v", err)
	}

	if name, ok := lookupTag("founder"); !ok || name != "⭐ Founder" {
		t.Errorf(`lookupTag("founder") = (%q, %v), want ("⭐ Founder", true)`, name, ok)
	}

	// A built-in tag still resolves the same way.
	const builtinID = "tag_gambler"
	if _, ok := shopItemByID(builtinID); !ok {
		t.Fatalf("test assumption broken: %q is not a built-in shop item", builtinID)
	}
	if name, ok := lookupTag(builtinID); !ok || name == "" {
		t.Errorf("lookupTag(%q) = (%q, %v), want a non-empty name and ok=true", builtinID, name, ok)
	}

	// A pass/passive id is not a tag.
	const passiveID = "pass_income_5x"
	if it, ok := shopItemByID(passiveID); !ok || it.kind == shopKindTag {
		t.Fatalf("test assumption broken: %q is not a non-tag built-in item", passiveID)
	}
	if _, ok := lookupTag(passiveID); ok {
		t.Errorf("lookupTag(%q) returned ok=true for a non-tag item", passiveID)
	}

	// Unknown id.
	if _, ok := lookupTag("does_not_exist"); ok {
		t.Error("lookupTag returned ok=true for an unknown id")
	}
}

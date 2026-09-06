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
	"strings"
	"testing"

	"github.com/MangosArentLiterature/Athena/internal/area"
	"github.com/MangosArentLiterature/Athena/internal/settings"
)

// setupRenameTest stands up two areas and a fresh client list, and restores
// every global it touches.
func setupRenameTest(t *testing.T) (*area.Area, *area.Area) {
	t.Helper()
	newTestClients(t)

	origAreas := areas
	origNames := getAreaNames()
	origSM := getSMPacket()
	origMusic := getMusicList()
	t.Cleanup(func() {
		areas = origAreas
		setAreaNames(origNames)
		setSMPacket(origSM)
		setMusicList(origMusic)
	})

	setMusicList([]string{"Category~", "pursuit.opus"})
	courtroom := area.NewArea(area.AreaData{Name: "Courtroom 3", Bg: "default"}, 2, 10, area.EviAny)
	basement := area.NewArea(area.AreaData{Name: "Basement", Bg: "default"}, 2, 10, area.EviAny)
	areas = []*area.Area{courtroom, basement}
	republishAreaNames()
	return courtroom, basement
}

// newRenameClient builds a CM standing in a.
func newRenameClient(a *area.Area, uid int) (*Client, *captureConn) {
	conn := &captureConn{}
	c := &Client{conn: conn, uid: uid, ipid: "ip-cm", area: a, char: -1, pair: ClientPairInfo{wanted_id: -1}}
	a.AddCM(uid)
	clients.AddClient(c)
	clients.RegisterUID(c)
	return c, conn
}

// TestAreaRenameRepublishesEverythingDerivedFromTheName is the point of the
// feature: a rename is not just a label on the Area struct. Three things are
// built from area names and all of them have to move together, or the room
// becomes unreachable by name (pktAM matches the joined string) or joins show
// the old list (the pre-built SM blob).
func TestAreaRenameRepublishesEverythingDerivedFromTheName(t *testing.T) {
	courtroom, _ := setupRenameTest(t)
	cm, conn := newRenameClient(courtroom, 1)

	cmdAreaRename(cm, []string{"DR", "Killing", "Game"}, "usage")

	if got := courtroom.Name(); got != "DR Killing Game" {
		t.Fatalf("area name = %q, want %q — reply was %q", got, "DR Killing Game", conn.String())
	}
	if got := getAreaNames(); got != "DR Killing Game#Basement" {
		t.Errorf("joined area names = %q, want %q", got, "DR Killing Game#Basement")
	}
	if sm := getSMPacket(); !strings.HasPrefix(sm, "SM#DR Killing Game#Basement#") {
		t.Errorf("SM packet was not rebuilt from the new names: %q", sm)
	}
	// Every connected client is handed the new list, so an already-joined
	// player sees the rename without reconnecting.
	if out := conn.String(); !strings.Contains(out, "FA#DR Killing Game#Basement#%") {
		t.Errorf("no FA area-list broadcast reached the client; got %q", out)
	}
	if courtroom.DefaultName() != "Courtroom 3" {
		t.Errorf("DefaultName = %q, want the configured name to be preserved", courtroom.DefaultName())
	}
}

// TestAreaRenameRejections pins each validation rule and, just as importantly,
// that a rejected rename leaves the area alone.
func TestAreaRenameRejections(t *testing.T) {
	courtroom, basement := setupRenameTest(t)
	_ = basement

	cases := []struct {
		name string
		args []string
		want string
	}{
		{"empty", []string{"   "}, "Give the area a name"},
		{"dot", []string{"."}, `cannot be "." or ".."`},
		{"dotdot", []string{".."}, `cannot be "." or ".."`},
		{"too long", []string{strings.Repeat("x", maxAreaNameLen+1)}, "the limit is"},
		{"packet separator", []string{"Court#room"}, "cannot contain"},
		{"control character", []string{"Court\troom"}, "control characters"},
		{"music entry", []string{"pursuit.opus"}, "music-list entry"},
		{"streaming url", []string{"https://cdn.example/song.mp3"}, "music-list entry"},
		{"other area's current name", []string{"Basement"}, "already called"},
		{"case-insensitive collision", []string{"basement"}, "already called"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cm, conn := newRenameClient(courtroom, 1)
			t.Cleanup(func() { clients.RemoveClient(cm) })

			cmdAreaRename(cm, tc.args, "usage")

			if got := courtroom.Name(); got != "Courtroom 3" {
				t.Fatalf("a rejected rename changed the area name to %q", got)
			}
			if out := conn.String(); !strings.Contains(out, tc.want) {
				t.Errorf("reply %q does not explain the rejection (wanted %q)", out, tc.want)
			}
		})
	}
}

// TestAreaRenameCollidesWithAnotherAreasConfiguredName covers the subtle half
// of the uniqueness rule: a configured name is not currently in use while that
// area carries a rename of its own, but it comes back the instant that area
// empties -- so letting a second area take it would produce a silent duplicate
// later rather than an error now.
func TestAreaRenameCollidesWithAnotherAreasConfiguredName(t *testing.T) {
	courtroom, basement := setupRenameTest(t)

	other, _ := newRenameClient(basement, 2)
	cmdAreaRename(other, []string{"The", "Cellar"}, "usage")
	if basement.Name() != "The Cellar" {
		t.Fatalf("setup failed: basement is %q", basement.Name())
	}

	cm, conn := newRenameClient(courtroom, 1)
	cmdAreaRename(cm, []string{"Basement"}, "usage")

	if courtroom.Name() != "Courtroom 3" {
		t.Fatalf("took another area's configured name: %q", courtroom.Name())
	}
	if out := conn.String(); !strings.Contains(out, "configured name") {
		t.Errorf("reply %q does not explain the configured-name collision", out)
	}
}

// TestAreaRenameRevertsWhenTheRoomIsUnattended pins the loan: a rename lasts
// only as long as the room belongs to somebody.
func TestAreaRenameRevertsWhenTheRoomIsUnattended(t *testing.T) {
	t.Run("last CM leaving", func(t *testing.T) {
		courtroom, _ := setupRenameTest(t)
		cm, _ := newRenameClient(courtroom, 1)
		cmdAreaRename(cm, []string{"DR", "Killing", "Game"}, "usage")

		// A second CM leaving is not the last one: the name stays.
		other, _ := newRenameClient(courtroom, 2)
		courtroom.RemoveCM(other.Uid())
		releaseAreaNameOnLastCMLeaving(courtroom)
		if courtroom.Name() != "DR Killing Game" {
			t.Fatalf("name reverted while a CM was still present: %q", courtroom.Name())
		}

		courtroom.RemoveCM(cm.Uid())
		releaseAreaNameOnLastCMLeaving(courtroom)
		if got := courtroom.Name(); got != "Courtroom 3" {
			t.Fatalf("name = %q after the last CM left, want the configured name", got)
		}
		if got := getAreaNames(); got != "Courtroom 3#Basement" {
			t.Errorf("the revert did not republish the area-name list: %q", got)
		}
	})

	t.Run("room empties", func(t *testing.T) {
		courtroom, _ := setupRenameTest(t)
		cm, _ := newRenameClient(courtroom, 1)
		cmdAreaRename(cm, []string{"DR", "Killing", "Game"}, "usage")

		releaseAreaNameOnEmpty(courtroom)
		if got := courtroom.Name(); got != "Courtroom 3" {
			t.Fatalf("name = %q after the room emptied, want the configured name", got)
		}
		if sm := getSMPacket(); !strings.HasPrefix(sm, "SM#Courtroom 3#Basement#") {
			t.Errorf("the revert did not rebuild the SM packet: %q", sm)
		}
	})

	t.Run("a CM-less room keeps its name until it empties", func(t *testing.T) {
		// A moderator does not have to be a CM to rename a room. Keying the
		// revert on the *transition* to zero CMs rather than on "this area has
		// no CMs" is what stops such a rename from snapping back the instant
		// anybody walks out of the room.
		courtroom, _ := setupRenameTest(t)
		mod, _ := newRenameClient(courtroom, 1)
		courtroom.RemoveCM(mod.Uid())
		cmdAreaRename(mod, []string{"Trial", "Grounds"}, "usage")
		if courtroom.Name() != "Trial Grounds" {
			t.Fatalf("setup failed: %q", courtroom.Name())
		}

		releaseAreaNameOnLastCMLeaving(courtroom)
		if got := courtroom.Name(); got != "Trial Grounds" {
			t.Fatalf("a CM-less room lost its name on an unrelated departure: %q", got)
		}
		// Emptying the room still takes it back.
		releaseAreaNameOnEmpty(courtroom)
		if got := courtroom.Name(); got != "Courtroom 3" {
			t.Fatalf("emptying the room did not revert the name: %q", got)
		}
	})
}

// TestAreaUnrenameRestoresImmediately covers the manual inverse.
func TestAreaUnrenameRestoresImmediately(t *testing.T) {
	courtroom, _ := setupRenameTest(t)
	cm, conn := newRenameClient(courtroom, 1)

	cmdAreaUnrename(cm, nil, "usage")
	if !strings.Contains(conn.String(), "not renamed") {
		t.Errorf("unrename on an un-renamed area should say so; got %q", conn.String())
	}

	cmdAreaRename(cm, []string{"DR", "Killing", "Game"}, "usage")
	cmdAreaUnrename(cm, nil, "usage")
	if got := courtroom.Name(); got != "Courtroom 3" {
		t.Fatalf("name = %q after /area unrename, want the configured name", got)
	}
	if got := getAreaNames(); got != "Courtroom 3#Basement" {
		t.Errorf("/area unrename did not republish: %q", got)
	}
}

// TestAreaRenameGoesThroughTheWordFilter pins that a room name -- broadcast to
// every connected client -- is held to the same standard as a line of IC or
// OOC chat, at every tier. Regressing this would let somebody put a slur on
// the area list of every client on the server.
func TestAreaRenameGoesThroughTheWordFilter(t *testing.T) {
	courtroom, _ := setupRenameTest(t)

	origWords := getBannedWords()
	origEntries := getWordEntries()
	origAction := autoModAction
	t.Cleanup(func() {
		setBannedWords(origWords)
		setWordEntries(origEntries)
		autoModAction = origAction
	})
	origConfig := config
	t.Cleanup(func() { config = origConfig })
	// The shadow action kicks the offender (the escalating censor kick), which
	// reads the autoban settings.
	config = &settings.Config{ServerConfig: settings.ServerConfig{AutoModEnabled: true}}

	setBannedWords(nil)
	setWordEntries([]WordEntry{
		{Raw: "slurword", Pattern: "slurword", Severity: SeverityDefault, Mode: MatchSubstring},
		{Raw: "watchword", Pattern: "watchword", Severity: SeverityWatch, Mode: MatchSubstring},
	})
	// "shadow" is the default action and the one with a visible tell: the
	// caller is told the rename worked while the area is untouched.
	autoModAction = autoModActionShadow

	cm, conn := newRenameClient(courtroom, 1)
	cmdAreaRename(cm, []string{"slurword", "room"}, "usage")
	if got := courtroom.Name(); got != "Courtroom 3" {
		t.Fatalf("a banned word became an area name: %q", got)
	}
	if got := getAreaNames(); strings.Contains(got, "slurword") {
		t.Fatalf("a banned word reached the published area list: %q", got)
	}
	if out := conn.String(); !strings.Contains(out, "slurword") {
		t.Errorf("shadow semantics broken: the caller should see the rename confirmed; got %q", out)
	}

	// Evasion normalization is the same as for chat: spacing a word out does
	// not get it through.
	cmdAreaRename(cm, []string{"s", "l", "u", "r", "w", "o", "r", "d"}, "usage")
	if got := courtroom.Name(); got != "Courtroom 3" {
		t.Fatalf("a spaced-out banned word became an area name: %q", got)
	}

	// A watch-tier word alerts staff but is never punished on its own, so the
	// rename goes through -- the same carve-out chat gets.
	cmdAreaRename(cm, []string{"watchword"}, "usage")
	if got := courtroom.Name(); got != "watchword" {
		t.Fatalf("a watch-tier word was treated as punishable: name = %q", got)
	}
}

// TestRemovedInvisibleEffectCommandsAreGone pins the removal of the four
// commands whose effect was invisible to the person they landed on. Their
// implementations are deleted, so this guards against one being reintroduced
// by a merge that resurrects a registry entry.
func TestRemovedInvisibleEffectCommandsAreGone(t *testing.T) {
	initCommands()
	for _, name := range []string{"possess", "fullpossess", "truepossess", "unpossess", "shadowdisconnect"} {
		if _, ok := Commands[name]; ok {
			t.Errorf("/%v is registered again — it was removed deliberately; see commands_area_rename.go's sibling removal and shadowdisconnect.go", name)
		}
	}
	// The enforcement half of the shadow-disconnect list stays, so entries
	// written before the command was removed can still be found and lifted.
	for _, name := range []string{"shadowundisconnect", "shadowdisconnectlist"} {
		if _, ok := Commands[name]; !ok {
			t.Errorf("/%v is gone — without it, a pre-existing shadow-disconnect can never be lifted", name)
		}
	}
}

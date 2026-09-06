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
	"errors"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/MangosArentLiterature/Athena/internal/area"
	"github.com/MangosArentLiterature/Athena/internal/packet"
	"github.com/MangosArentLiterature/Athena/internal/permissions"
	"github.com/MangosArentLiterature/Athena/internal/settings"
)

// ── Fix 1: a panicking packet handler must not crash the server ────────────

// TestHandlerPanicIsRecoveredAndClosesOnlyThatConnection is the strongest
// proof this recover() exists and works: without it, this test doesn't just
// fail -- it crashes the entire `go test` binary, since an unrecovered panic
// in a goroutine takes down the whole process, not just that goroutine.
func TestHandlerPanicIsRecoveredAndClosesOnlyThatConnection(t *testing.T) {
	defer setupShadowDisconnectTestDB(t)()
	newTestClients(t)

	origConfig := config
	t.Cleanup(func() { config = origConfig })
	config = &settings.Config{}

	const testHeader = "ZZ_PANIC_TEST"
	orig, hadOrig := PacketMap[testHeader]
	PacketMap[testHeader] = pktMapValue{Args: 0, MustJoin: false, Func: func(*Client, *packet.Packet) {
		panic("deliberate test panic")
	}}
	t.Cleanup(func() {
		if hadOrig {
			PacketMap[testHeader] = orig
		} else {
			delete(PacketMap, testHeader)
		}
	})

	a, b := net.Pipe()
	defer b.Close()
	c := NewClient(a, "panic-test-ipid")

	go io.Copy(io.Discard, b) // drain everything the server writes (Decryptor, etc.)

	done := make(chan struct{})
	go func() {
		c.HandleClient()
		close(done)
	}()

	// b.Write blocks (net.Pipe is unbuffered) until HandleClient's read loop
	// is ready for it, so this also synchronizes with the setup preamble
	// (CheckBanned, ignore-list load, MCLimit check, ...) completing first.
	if _, err := b.Write([]byte(testHeader + "#%")); err != nil {
		t.Fatalf("write test packet: %v", err)
	}

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("HandleClient did not return after its handler panicked")
	}

	if !c.closed.Load() {
		t.Fatal("connection was not marked closed after its handler panicked")
	}
}

// ── Fix 4: the packet reader must be bounded ────────────────────────────────

func TestBoundedReaderRejectsOversizedRead(t *testing.T) {
	huge := strings.Repeat("A", maxPacketBytes+1)
	br := &boundedReader{r: strings.NewReader(huge), max: maxPacketBytes}

	total := 0
	buf := make([]byte, 4096)
	var readErr error
	for {
		n, err := br.Read(buf)
		total += n
		if err != nil {
			readErr = err
			break
		}
	}
	if !errors.Is(readErr, errPacketTooLarge) {
		t.Fatalf("expected errPacketTooLarge, got %v", readErr)
	}
	if total > maxPacketBytes {
		t.Fatalf("boundedReader let through %d bytes, want at most %d", total, maxPacketBytes)
	}
}

func TestBoundedReaderAllowsExactlyMaxBytes(t *testing.T) {
	exact := strings.Repeat("B", maxPacketBytes)
	br := &boundedReader{r: strings.NewReader(exact), max: maxPacketBytes}

	got, err := io.ReadAll(br)
	if err != nil && !errors.Is(err, errPacketTooLarge) {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != maxPacketBytes {
		t.Fatalf("boundedReader returned %d bytes for an exactly-sized read, want %d", len(got), maxPacketBytes)
	}
}

func TestBoundedReaderResetAllowsFreshBudgetPerPacket(t *testing.T) {
	br := &boundedReader{r: strings.NewReader(strings.Repeat("C", 100)), max: 10}
	buf := make([]byte, 100)
	n, err := br.Read(buf)
	if n != 10 || err != nil {
		t.Fatalf("first read: got n=%d err=%v, want n=10 err=nil", n, err)
	}
	if _, err := br.Read(buf); !errors.Is(err, errPacketTooLarge) {
		t.Fatalf("expected errPacketTooLarge once budget exhausted, got %v", err)
	}
	// Simulates what HandleClient's loop does at the top of every iteration.
	br.n = 0
	br.r = strings.NewReader(strings.Repeat("D", 5))
	n, err = br.Read(buf)
	if n != 5 || err != nil {
		t.Fatalf("after reset: got n=%d err=%v, want n=5 err=nil", n, err)
	}
}

// ── Fix 2: /8ball, /poll and /mafia whisper must consult the OOC gate ──────

// Extends TestBroadcastingOOCCommandsConsultTheGate's own coverage (which
// only checked cmdGlobal/cmdPM) to the commands found bypassing it in the
// same audit: /8ball and /poll broadcast free player text area-wide with
// reqPerms as low as NONE, so a stealthmuted, tormented or captcha-restricted
// player could still reach the whole room through them.
func TestFunCommandsConsultTheOOCGate(t *testing.T) {
	src, err := os.ReadFile("commands_fun.go")
	if err != nil {
		t.Fatalf("read commands_fun.go: %v", err)
	}
	text := string(src)

	for _, fn := range []string{"cmd8Ball", "cmdPoll"} {
		t.Run(fn, func(t *testing.T) {
			start := strings.Index(text, "func "+fn+"(")
			if start < 0 {
				t.Fatalf("%s not found", fn)
			}
			end := strings.Index(text[start+1:], "\nfunc ")
			body := text[start:]
			if end > 0 {
				body = text[start : start+1+end]
			}

			gate := strings.Index(body, "oocCommandAllowed(")
			if gate < 0 {
				t.Fatalf("%s does not consult oocCommandAllowed; free player text "+
					"reaches the area unexamined by the word filter, the torment "+
					"list, stealthmute and the captcha restriction", fn)
			}

			sendRe := "broadcastToArea("
			for i := strings.Index(body, sendRe); i >= 0; {
				if i < gate {
					t.Errorf("%s broadcasts at offset %d, before the gate at %d", fn, i, gate)
				}
				next := strings.Index(body[i+1:], sendRe)
				if next < 0 {
					break
				}
				i = i + 1 + next
			}
		})
	}
}

// /mafia whisper delivers free player text directly to another player -- the
// same shape as /pm -- and took the same unexamined path.
func TestMafiaWhisperConsultsTheOOCGate(t *testing.T) {
	src, err := os.ReadFile("commands_mafia.go")
	if err != nil {
		t.Fatalf("read commands_mafia.go: %v", err)
	}
	text := string(src)

	start := strings.Index(text, "func mafiaSubWhisper(")
	if start < 0 {
		t.Fatal("mafiaSubWhisper not found")
	}
	end := strings.Index(text[start+1:], "\nfunc ")
	body := text[start:]
	if end > 0 {
		body = text[start : start+1+end]
	}

	gate := strings.Index(body, "oocCommandAllowed(")
	if gate < 0 {
		t.Fatal("mafiaSubWhisper does not consult oocCommandAllowed; a whisper reaches " +
			"its target unexamined by the word filter, the torment list, " +
			"stealthmute and the captcha restriction")
	}
	deliver := strings.Index(body, "g.privateMsg(")
	if deliver < 0 {
		t.Fatal("g.privateMsg( landmark not found; mafiaSubWhisper has been restructured")
	}
	if deliver < gate {
		t.Error("mafiaSubWhisper delivers the whisper before consulting the gate")
	}
}

// /mafia will is stored now and broadcast unattended later (on death/lynch),
// so there is no delivery moment for oocCommandAllowed's suppression
// branches to hold back -- content must instead be rejected before it is
// ever stored, mirroring cmdAreaRename's store-now/broadcast-later check.
func TestMafiaWillIsCheckedBeforeItIsStored(t *testing.T) {
	src, err := os.ReadFile("commands_mafia.go")
	if err != nil {
		t.Fatalf("read commands_mafia.go: %v", err)
	}
	text := string(src)

	start := strings.Index(text, "func mafiaSubWill(")
	if start < 0 {
		t.Fatal("mafiaSubWill not found")
	}
	end := strings.Index(text[start+1:], "\nfunc ")
	body := text[start:]
	if end > 0 {
		body = text[start : start+1+end]
	}

	check := strings.Index(body, "autoModCheckTiered(")
	if check < 0 {
		t.Fatal("mafiaSubWill does not consult autoModCheckTiered; unfiltered text " +
			"can be stored and later broadcast area-wide when the player dies")
	}
	store := strings.Index(body, "p.LastWill = will")
	if store < 0 {
		t.Fatal("p.LastWill = will landmark not found; mafiaSubWill has been restructured")
	}
	if store < check {
		t.Error("mafiaSubWill stores the will before the content check runs")
	}
}

// ── Fix 3 (functional, in addition to commands_area_rename_test.go's table
// and logger's TestSanitizeAreaName/TestCreateAreaLogDirectoryRejectsTraversal) ──

func TestAreaNameRejectionBlocksDotAndDotDot(t *testing.T) {
	room := area.NewArea(area.AreaData{Name: "Courtroom"}, 5, 10, area.EviAny)
	for _, name := range []string{".", ".."} {
		if reason := areaNameRejection(room, name); reason == "" {
			t.Errorf("areaNameRejection(%q) = \"\", want a rejection", name)
		}
	}
}

// ── Fix 5: /login must not allow unlimited guesses ──────────────────────────

func TestLoginRateLimit(t *testing.T) {
	const ipid = "login-ratelimit-test-ipid"
	t.Cleanup(func() { clearLoginAttempts(ipid) })
	clearLoginAttempts(ipid)

	for i := 0; i < maxLoginAttempts; i++ {
		if limited, _ := checkLoginRateLimit(ipid); limited {
			t.Fatalf("attempt %d: rate limited before reaching maxLoginAttempts", i)
		}
		registerFailedLogin(ipid)
	}

	limited, remaining := checkLoginRateLimit(ipid)
	if !limited {
		t.Fatal("not rate limited after maxLoginAttempts failed attempts")
	}
	if remaining <= 0 {
		t.Fatalf("remaining = %d, want > 0", remaining)
	}

	clearLoginAttempts(ipid)
	if limited, _ := checkLoginRateLimit(ipid); limited {
		t.Fatal("still rate limited after clearLoginAttempts (e.g. a successful login)")
	}
}

// ── /log removed ────────────────────────────────────────────────────────────

func TestLogCommandRemoved(t *testing.T) {
	initCommands()
	if _, ok := Commands["log"]; ok {
		t.Fatal("\"log\" is still registered; /log was supposed to be removed entirely")
	}
}

// ── /unpunish self-removal: mod and shadow mod are the same tier ───────────

// A regular moderator must now be able to lift a punishment a shadow mod
// issued off their own UID -- mod and shadow sit at the same power tier for
// this purpose, differing elsewhere (e.g. shadow-mod stealth) but not here --
// while an admin-issued punishment must remain protected.
func TestUnpunishSelfRemovalShadowVsAdmin(t *testing.T) {
	defer setupShadowDisconnectTestDB(t)()
	newTestClients(t)
	room := area.NewArea(area.AreaData{Name: "Courtroom"}, 5, 10, area.EviAny)

	newModClient := func(uid int, ipid string) *Client {
		c := &Client{char: -1, conn: &testConn{}, area: room, uid: uid, ipid: ipid,
			perms: permissions.PermissionField["MUTE"]}
		clients.AddClient(c)
		clients.RegisterUID(c)
		return c
	}

	t.Run("shadow-issued is self-removable by a mod", func(t *testing.T) {
		c := newModClient(101, "unpunish-self-shadow")
		c.AddPunishmentBy(PunishmentTsundere, time.Hour, "test", IssuerShadow)

		cmdUnpunish(c, []string{strconv.Itoa(c.Uid())}, "usage")

		if c.HasPunishment(PunishmentTsundere) {
			t.Error("a mod could not self-remove a punishment a shadow mod issued; " +
				"mod and shadow mod are supposed to sit at the same protection tier")
		}
	})

	t.Run("admin-issued remains protected from a mod", func(t *testing.T) {
		c := newModClient(102, "unpunish-self-admin")
		c.AddPunishmentBy(PunishmentTsundere, time.Hour, "test", IssuerAdmin)

		cmdUnpunish(c, []string{strconv.Itoa(c.Uid())}, "usage")

		if !c.HasPunishment(PunishmentTsundere) {
			t.Error("a mod was able to self-remove an admin-issued punishment; " +
				"only ADMIN should bypass this gate")
		}
	})

	t.Run("HasProtectedPunishment only reports admin-issued", func(t *testing.T) {
		c := newModClient(103, "unpunish-hasprotected")
		c.AddPunishmentBy(PunishmentTsundere, time.Hour, "test", IssuerShadow)
		if c.HasProtectedPunishment() {
			t.Error("a shadow-issued punishment was reported as protected")
		}
		c.AddPunishmentBy(PunishmentUppercase, time.Hour, "test", IssuerAdmin)
		if !c.HasProtectedPunishment() {
			t.Error("an admin-issued punishment was not reported as protected")
		}
	})
}

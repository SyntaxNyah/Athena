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
	"testing"
	"time"

	"github.com/MangosArentLiterature/Athena/internal/settings"
)

// resetFirstSeenTracker clears the ipFirstSeenTracker for a clean test.
func resetFirstSeenTracker() {
	ipFirstSeenTracker.mu.Lock()
	ipFirstSeenTracker.times = make(map[string]time.Time)
	ipFirstSeenTracker.mu.Unlock()
}

// TestNewIPIDOOCCooldownDisabled tests that the OOC cooldown is skipped when set to 0.
func TestNewIPIDOOCCooldownDisabled(t *testing.T) {
	oldConfig := config
	defer func() { config = oldConfig }()

	config = &settings.Config{}
	config.NewIPIDOOCCooldown = 0

	resetFirstSeenTracker()

	ipid := "testNewIPIDOOCDisabled"
	recordIPFirstSeen(ipid)

	// With cooldown disabled, should never be limited.
	if limited, _ := checkNewIPIDOOCCooldown(ipid); limited {
		t.Errorf("OOC was blocked when new IPID OOC cooldown is disabled")
	}
}

// TestNewIPIDOOCCooldownEnforced tests that a brand-new IPID is blocked from OOC chat.
func TestNewIPIDOOCCooldownEnforced(t *testing.T) {
	oldConfig := config
	defer func() { config = oldConfig }()

	config = &settings.Config{}
	config.NewIPIDOOCCooldown = 10

	resetFirstSeenTracker()

	ipid := "testNewIPIDOOCEnforced"
	recordIPFirstSeen(ipid)

	// Should be blocked immediately after first-seen.
	if limited, remaining := checkNewIPIDOOCCooldown(ipid); !limited {
		t.Errorf("New IPID was not blocked from OOC chat immediately after joining")
	} else if remaining <= 0 || remaining > 10 {
		t.Errorf("Unexpected remaining seconds: %d (expected 1-10)", remaining)
	}
}

// TestNewIPIDOOCCooldownExpires tests that the OOC cooldown expires correctly.
func TestNewIPIDOOCCooldownExpires(t *testing.T) {
	oldConfig := config
	defer func() { config = oldConfig }()

	config = &settings.Config{}
	config.NewIPIDOOCCooldown = 1 // 1-second cooldown for a fast test

	resetFirstSeenTracker()

	ipid := "testNewIPIDOOCExpires"
	recordIPFirstSeen(ipid)

	// Should be blocked immediately.
	if limited, _ := checkNewIPIDOOCCooldown(ipid); !limited {
		t.Errorf("New IPID was not blocked from OOC chat during cooldown")
		return
	}

	// Wait for cooldown to expire.
	time.Sleep(1100 * time.Millisecond)

	// Should be allowed after cooldown expires.
	if limited, _ := checkNewIPIDOOCCooldown(ipid); limited {
		t.Errorf("New IPID was still blocked from OOC chat after cooldown expired")
	}
}

// TestNewIPIDICCooldownDisabled tests that the IC cooldown is skipped when set to 0.
func TestNewIPIDICCooldownDisabled(t *testing.T) {
	oldConfig := config
	defer func() { config = oldConfig }()

	config = &settings.Config{}
	config.NewIPIDICCooldown = 0

	resetFirstSeenTracker()

	ipid := "testNewIPIDICDisabled"
	recordIPFirstSeen(ipid)

	// With cooldown disabled, should never be limited.
	if limited, _ := checkNewIPIDICCooldown(ipid); limited {
		t.Errorf("IC was blocked when new IPID IC cooldown is disabled")
	}
}

// TestNewIPIDICCooldownEnforced tests that a brand-new IPID is blocked from speaking IC.
func TestNewIPIDICCooldownEnforced(t *testing.T) {
	oldConfig := config
	defer func() { config = oldConfig }()

	config = &settings.Config{}
	config.NewIPIDICCooldown = 10

	resetFirstSeenTracker()

	ipid := "testNewIPIDICEnforced"
	recordIPFirstSeen(ipid)

	// Should be blocked immediately after first-seen.
	if limited, remaining := checkNewIPIDICCooldown(ipid); !limited {
		t.Errorf("New IPID was not blocked from speaking IC immediately after joining")
	} else if remaining <= 0 || remaining > 10 {
		t.Errorf("Unexpected remaining seconds: %d (expected 1-10)", remaining)
	}
}

// TestNewIPIDICCooldownExpires tests that the IC cooldown expires correctly.
func TestNewIPIDICCooldownExpires(t *testing.T) {
	oldConfig := config
	defer func() { config = oldConfig }()

	config = &settings.Config{}
	config.NewIPIDICCooldown = 1 // 1-second cooldown for a fast test

	resetFirstSeenTracker()

	ipid := "testNewIPIDICExpires"
	recordIPFirstSeen(ipid)

	// Should be blocked immediately.
	if limited, _ := checkNewIPIDICCooldown(ipid); !limited {
		t.Errorf("New IPID was not blocked from speaking IC during cooldown")
		return
	}

	// Wait for cooldown to expire.
	time.Sleep(1100 * time.Millisecond)

	// Should be allowed after cooldown expires.
	if limited, _ := checkNewIPIDICCooldown(ipid); limited {
		t.Errorf("New IPID was still blocked from speaking IC after cooldown expired")
	}
}

// TestNewIPIDModcallCooldownDisabled tests that the modcall cooldown is skipped when set to 0.
func TestNewIPIDModcallCooldownDisabled(t *testing.T) {
	oldConfig := config
	defer func() { config = oldConfig }()

	config = &settings.Config{}
	config.NewIPIDModcallCooldown = 0

	resetFirstSeenTracker()

	ipid := "testNewIPIDModcallDisabled"
	recordIPFirstSeen(ipid)

	// With cooldown disabled, should never be limited.
	if limited, _ := checkNewIPIDModcallCooldown(ipid); limited {
		t.Errorf("Modcall was blocked when new IPID modcall cooldown is disabled")
	}
}

// TestNewIPIDModcallCooldownEnforced tests that a brand-new IPID is blocked from modcall.
func TestNewIPIDModcallCooldownEnforced(t *testing.T) {
	oldConfig := config
	defer func() { config = oldConfig }()

	config = &settings.Config{}
	config.NewIPIDModcallCooldown = 60

	resetFirstSeenTracker()

	ipid := "testNewIPIDModcallEnforced"
	recordIPFirstSeen(ipid)

	// Should be blocked immediately after first-seen.
	if limited, remaining := checkNewIPIDModcallCooldown(ipid); !limited {
		t.Errorf("New IPID was not blocked from modcall immediately after joining")
	} else if remaining <= 0 || remaining > 60 {
		t.Errorf("Unexpected remaining seconds: %d (expected 1-60)", remaining)
	}
}

// TestNewIPIDModcallCooldownExpires tests that the modcall cooldown expires correctly.
func TestNewIPIDModcallCooldownExpires(t *testing.T) {
	oldConfig := config
	defer func() { config = oldConfig }()

	config = &settings.Config{}
	config.NewIPIDModcallCooldown = 1 // 1-second cooldown for a fast test

	resetFirstSeenTracker()

	ipid := "testNewIPIDModcallExpires"
	recordIPFirstSeen(ipid)

	// Should be blocked immediately.
	if limited, _ := checkNewIPIDModcallCooldown(ipid); !limited {
		t.Errorf("New IPID was not blocked from modcall during cooldown")
		return
	}

	// Wait for cooldown to expire.
	time.Sleep(1100 * time.Millisecond)

	// Should be allowed after cooldown expires.
	if limited, _ := checkNewIPIDModcallCooldown(ipid); limited {
		t.Errorf("New IPID was still blocked from modcall after cooldown expired")
	}
}

// TestRecordIPFirstSeenIdempotent tests that calling recordIPFirstSeen multiple times
// does not overwrite the first-seen timestamp (i.e., the IPID stays "new" until the cooldown
// has elapsed from the *first* seen time, not from each re-connection).
func TestRecordIPFirstSeenIdempotent(t *testing.T) {
	resetFirstSeenTracker()

	ipid := "testRecordIdempotent"
	recordIPFirstSeen(ipid)

	ipFirstSeenTracker.mu.Lock()
	first := ipFirstSeenTracker.times[ipid]
	ipFirstSeenTracker.mu.Unlock()

	// Re-connect a few times.
	for i := 0; i < 5; i++ {
		recordIPFirstSeen(ipid)
	}

	ipFirstSeenTracker.mu.Lock()
	second := ipFirstSeenTracker.times[ipid]
	ipFirstSeenTracker.mu.Unlock()

	if !first.Equal(second) {
		t.Errorf("recordIPFirstSeen overwrote first-seen timestamp on subsequent call")
	}
}

// TestRecordIPFirstSeenReturnsWhetherGenuinelyNew pins the return value the
// join popup (joinpopup.go) relies on: true exactly once, for the call that
// actually creates the entry, and false for every call after that -- whether
// that's a reconnect this session or the seeded-from-the-database case (an
// IPID the server already knew about before this process started).
func TestRecordIPFirstSeenReturnsWhetherGenuinelyNew(t *testing.T) {
	resetFirstSeenTracker()

	ipid := "testRecordReturnsNew"
	if !recordIPFirstSeen(ipid) {
		t.Error("first call for a never-seen IPID should report true")
	}
	for i := 0; i < 3; i++ {
		if recordIPFirstSeen(ipid) {
			t.Errorf("call %d for an already-seen IPID should report false", i)
		}
	}

	// An IPID seeded at startup from KNOWN_IPS (see InitServer) is pre-loaded
	// into the tracker before any connection arrives, exactly like an
	// already-seen one -- so it must never report new either.
	seeded := "testRecordSeededFromDB"
	ipFirstSeenTracker.mu.Lock()
	ipFirstSeenTracker.times[seeded] = time.Unix(0, 0)
	ipFirstSeenTracker.mu.Unlock()
	if recordIPFirstSeen(seeded) {
		t.Error("an IPID pre-seeded from the database should not report new")
	}
}

// TestNewIPIDUnseenIsNotBlocked tests that an IPID never passed to recordIPFirstSeen
// is not blocked (e.g., if the tracker is empty for some reason).
func TestNewIPIDUnseenIsNotBlocked(t *testing.T) {
	oldConfig := config
	defer func() { config = oldConfig }()

	config = &settings.Config{}
	config.NewIPIDOOCCooldown = 10
	config.NewIPIDICCooldown = 10
	config.NewIPIDModcallCooldown = 60

	resetFirstSeenTracker()

	ipid := "testUnseenIPID"
	// Do NOT call recordIPFirstSeen – simulate a missing entry.

	if limited, _ := checkNewIPIDOOCCooldown(ipid); limited {
		t.Errorf("Unseen IPID was blocked from OOC chat unexpectedly")
	}
	if limited, _ := checkNewIPIDICCooldown(ipid); limited {
		t.Errorf("Unseen IPID was blocked from speaking IC unexpectedly")
	}
	if limited, _ := checkNewIPIDModcallCooldown(ipid); limited {
		t.Errorf("Unseen IPID was blocked from modcall unexpectedly")
	}
}

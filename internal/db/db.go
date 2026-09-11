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
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

// ─── Chip balance cache ────────────────────────────────────────────────────
//
// GetChipBalance is called on every casino command, chip display, and several
// hot-path checks.  A short-lived in-memory cache avoids hammering SQLite for
// the same IPID within a single player session.
//
// Cache entries expire after chipCacheTTL.  Writes (AddChips, SpendChips,
// SetChips) invalidate the relevant entry immediately so stale reads are only
// possible during the TTL window for pure read paths.

const chipCacheTTL = 10 * time.Second

type chipCacheEntry struct {
	balance int64
	expiry  time.Time
}

var chipCache struct {
	sync.RWMutex
	m map[string]chipCacheEntry
}

func init() {
	chipCache.m = make(map[string]chipCacheEntry)
}

// chipCacheGet returns (balance, true) if a valid cache entry exists for ipid.
// Expired entries are removed immediately to prevent unbounded map growth.
func chipCacheGet(ipid string) (int64, bool) {
	chipCache.RLock()
	e, ok := chipCache.m[ipid]
	chipCache.RUnlock()
	if !ok {
		return 0, false
	}
	if time.Now().After(e.expiry) {
		// Entry appears expired.  Re-check under the write lock in case another
		// goroutine refreshed it between our RUnlock and Lock — we must not
		// evict a valid, freshly-written entry.
		chipCache.Lock()
		if e2, still := chipCache.m[ipid]; still && time.Now().After(e2.expiry) {
			delete(chipCache.m, ipid)
		}
		chipCache.Unlock()
		return 0, false
	}
	return e.balance, true
}

// chipCacheSet stores balance for ipid with a fresh TTL.
func chipCacheSet(ipid string, balance int64) {
	chipCache.Lock()
	chipCache.m[ipid] = chipCacheEntry{balance: balance, expiry: time.Now().Add(chipCacheTTL)}
	chipCache.Unlock()
}

// chipCacheInvalidate removes a cache entry so the next read hits the DB.
func chipCacheInvalidate(ipid string) {
	chipCache.Lock()
	delete(chipCache.m, ipid)
	chipCache.Unlock()
}

// PurgeChipCache removes all expired entries from the cache.  It is called
// periodically by PurgeExpired so the map doesn't grow unbounded.
func PurgeChipCache() {
	now := time.Now()
	chipCache.Lock()
	for k, v := range chipCache.m {
		if now.After(v.expiry) {
			delete(chipCache.m, k)
		}
	}
	chipCache.Unlock()
}

type BanInfo struct {
	Id        int
	Ipid      string
	Hdid      string
	Time      int64
	Duration  int64
	Reason    string
	Moderator string
}

type BanLookup int

const (
	IPID BanLookup = iota
	HDID
	BANID
)

var DBPath string
var db *sql.DB

// defaultChipBalance is the starting chip balance assigned to new players.
const defaultChipBalance = 500

// MaxChipBalance is the hard upper limit on any player's chip balance.
// AddChips will silently clamp the result to this value, preventing runaway
// inflation across all casino games.
const MaxChipBalance = 10_000_000

// Database version.
// This should be incremented whenever changes are made to the DB that require existing databases to upgrade.
const ver = 23

// MaxFavourites is the maximum number of favourite characters a player can save.
const MaxFavourites = 100

// ErrFavouriteLimitReached is returned by AddFavourite when the player's wardrobe is full.
var ErrFavouriteLimitReached = fmt.Errorf("wardrobe full: limit is %d favourites", MaxFavourites)

// Persistent punishment kind constants.
const (
	PunishKindMute      = 0 // Mute/parrot; VALUE holds the MuteState integer.
	PunishKindJail      = 1 // Jail; VALUE unused (0).
	PunishKindText      = 2 // Text/behaviour punishment; SUBTYPE holds the PunishmentType integer.
	PunishKindCharStuck = 3 // Char-stuck; VALUE holds the locked character ID.
)

// PersistentPunishment holds one row from the PUNISHMENTS table.
type PersistentPunishment struct {
	Kind       int
	Subtype    int   // 0 for mute/jail; PunishmentType for text punishments.
	Value      int   // MuteState for mutes; 0 for others.
	Expires    int64 // Unix timestamp; 0 = no expiry (permanent).
	Reason     string
	IssuerTier int // 0=system/legacy, 1=mod, 2=shadow, 3=admin
}

// ChipEntry holds one row from the CHIPS leaderboard query.
type ChipEntry struct {
	Username string
	Balance  int64
}

// PlaytimeEntry holds one row from the playtime leaderboard query.
type PlaytimeEntry struct {
	Ipid     string // used for live-session merge; never displayed
	Username string
	Playtime int64 // seconds
}

// Opens the server's database connection.
func Open() error {
	var err error
	db, err = sql.Open("sqlite", DBPath)
	if err != nil {
		return err
	}
	// SQLite performs best with a single writer connection.
	db.SetMaxOpenConns(1)

	// Performance pragmas: WAL journal is faster for mixed read/write workloads;
	// synchronous=NORMAL is safe with WAL and reduces fsync overhead;
	// cache_size=-2000 allocates ~2 MB of page cache.
	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA cache_size=-2000",
	} {
		if _, err := db.Exec(pragma); err != nil {
			return fmt.Errorf("%s: %w", pragma, err)
		}
	}

	var v int
	r := db.QueryRow("PRAGMA user_version")
	r.Scan(&v)
	if v < ver {
		err := upgradeDB(v)
		if err != nil {
			return err
		}
	}
	_, err = db.Exec("CREATE TABLE IF NOT EXISTS BANS(ID INTEGER PRIMARY KEY, IPID TEXT, HDID TEXT, TIME INTEGER, DURATION INTEGER, REASON TEXT, MODERATOR TEXT)")
	if err != nil {
		return err
	}
	_, err = db.Exec("CREATE TABLE IF NOT EXISTS USERS(USERNAME TEXT PRIMARY KEY, PASSWORD TEXT, PERMISSIONS TEXT, IPID TEXT NOT NULL DEFAULT '', GAMBLE_HIDE INTEGER NOT NULL DEFAULT 0, ACTIVE_TAG TEXT NOT NULL DEFAULT '', USERNAME_RESETS INTEGER NOT NULL DEFAULT 0)")
	if err != nil {
		return err
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS PUNISHMENTS(
		IPID        TEXT    NOT NULL,
		KIND        INTEGER NOT NULL,
		SUBTYPE     INTEGER NOT NULL DEFAULT 0,
		VALUE       INTEGER NOT NULL DEFAULT 0,
		EXPIRES     INTEGER NOT NULL DEFAULT 0,
		REASON      TEXT    NOT NULL DEFAULT '',
		ISSUER_TIER INTEGER NOT NULL DEFAULT 0,
		UNIQUE(IPID, KIND, SUBTYPE)
	)`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS KNOWN_IPS(
		IPID       TEXT    PRIMARY KEY,
		FIRST_SEEN INTEGER NOT NULL DEFAULT 0,
		LAST_SEEN  INTEGER NOT NULL DEFAULT 0,
		PLAYTIME   INTEGER NOT NULL DEFAULT 0
	)`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS TORMENTED_IPS(
		IPID TEXT PRIMARY KEY
	)`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS CHIPS(
		IPID    TEXT    PRIMARY KEY,
		BALANCE INTEGER NOT NULL DEFAULT 100
	)`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS IGNORED_IPS(
		IGNORER_IPID     TEXT NOT NULL,
		IGNORED_IPID     TEXT NOT NULL,
		IGNORER_USERNAME TEXT NOT NULL DEFAULT '',
		IGNORED_SHOWNAME TEXT NOT NULL DEFAULT '',
		IGNORED_OOC      TEXT NOT NULL DEFAULT '',
		PRIMARY KEY (IGNORER_IPID, IGNORED_IPID)
	)`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS UNSCRAMBLE_WINS(
		IPID TEXT PRIMARY KEY,
		WINS INTEGER NOT NULL DEFAULT 0
	)`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS JOB_COOLDOWNS(
		IPID    TEXT    NOT NULL,
		JOB     TEXT    NOT NULL,
		LAST_AT INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY (IPID, JOB)
	)`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS JOB_EARNINGS(
		IPID  TEXT    PRIMARY KEY,
		TOTAL INTEGER NOT NULL DEFAULT 0
	)`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS SHOP_PURCHASES(
		IPID    TEXT NOT NULL,
		ITEM_ID TEXT NOT NULL,
		PRIMARY KEY (IPID, ITEM_ID)
	)`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS PLAYER_ACTIVE_TAG(
		IPID   TEXT PRIMARY KEY,
		TAG_ID TEXT NOT NULL DEFAULT ''
	)`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS FAVOURITES(
		USERNAME  TEXT NOT NULL,
		CHAR_NAME TEXT NOT NULL,
		ADDED_AT  INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY (USERNAME, CHAR_NAME)
	)`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS MODNOTES(
		ID        INTEGER PRIMARY KEY AUTOINCREMENT,
		IPID      TEXT    NOT NULL,
		NOTE      TEXT    NOT NULL,
		ADDED_BY  TEXT    NOT NULL DEFAULT '',
		ADDED_AT  INTEGER NOT NULL DEFAULT 0
	)`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS CUSTOM_TAGS(
		ID         TEXT    PRIMARY KEY,
		NAME       TEXT    NOT NULL,
		CREATED_BY TEXT    NOT NULL DEFAULT '',
		CREATED_AT INTEGER NOT NULL DEFAULT 0
	)`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS MUSIC_BANS(
		IPID      TEXT    PRIMARY KEY,
		REASON    TEXT    NOT NULL DEFAULT '',
		BANNED_BY TEXT    NOT NULL DEFAULT '',
		BANNED_AT INTEGER NOT NULL DEFAULT 0
	)`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS RANDOMCHAR_CURSES(
		IPID      TEXT    PRIMARY KEY,
		ISSUED_BY TEXT    NOT NULL DEFAULT '',
		ISSUED_AT INTEGER NOT NULL DEFAULT 0
	)`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS SHADOW_DISCONNECTS(
		IPID      TEXT    PRIMARY KEY,
		REASON    TEXT    NOT NULL DEFAULT '',
		ISSUED_BY TEXT    NOT NULL DEFAULT '',
		ISSUED_AT INTEGER NOT NULL DEFAULT 0
	)`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS LOCKDOWN_EXEMPT(
		IPID      TEXT    PRIMARY KEY,
		ISSUED_BY TEXT    NOT NULL DEFAULT '',
		ISSUED_AT INTEGER NOT NULL DEFAULT 0
	)`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS JOIN_CAPTCHA_VERIFIED(
		IPID        TEXT    PRIMARY KEY,
		VERIFIED_AT INTEGER NOT NULL DEFAULT 0,
		KIND        TEXT    NOT NULL DEFAULT ''
	)`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS ACCOUNT_COMMAND_GRANTS(
		USERNAME   TEXT    NOT NULL,
		COMMAND    TEXT    NOT NULL,
		GRANTED_BY TEXT    NOT NULL DEFAULT '',
		GRANTED_AT INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY (USERNAME, COMMAND)
	)`)
	if err != nil {
		return err
	}
	return nil
}

// upgradeDB upgrades the server's database to the latest version.
func upgradeDB(v int) error {
	switch v {
	case 0:
		_, err := db.Exec("PRAGMA user_version = 1")
		if err != nil {
			return err
		}
		fallthrough
	case 1:
		_, err := db.Exec("PRAGMA user_version = 2")
		if err != nil {
			return err
		}
		fallthrough
	case 2:
		_, err := db.Exec(`CREATE TABLE IF NOT EXISTS KNOWN_IPS(
			IPID       TEXT    PRIMARY KEY,
			FIRST_SEEN INTEGER NOT NULL DEFAULT 0
		)`)
		if err != nil {
			return err
		}
		_, err = db.Exec("PRAGMA user_version = 3")
		if err != nil {
			return err
		}
		fallthrough
	case 3:
		_, err := db.Exec("ALTER TABLE KNOWN_IPS ADD COLUMN LAST_SEEN INTEGER NOT NULL DEFAULT 0")
		if err != nil {
			return err
		}
		// Initialise LAST_SEEN to FIRST_SEEN for existing rows so that currently
		// active players are not immediately pruned after upgrading.
		_, err = db.Exec("UPDATE KNOWN_IPS SET LAST_SEEN = FIRST_SEEN WHERE LAST_SEEN = 0")
		if err != nil {
			return err
		}
		_, err = db.Exec("PRAGMA user_version = 4")
		if err != nil {
			return err
		}
		fallthrough
	case 4:
		_, err := db.Exec("ALTER TABLE KNOWN_IPS ADD COLUMN PLAYTIME INTEGER NOT NULL DEFAULT 0")
		if err != nil {
			return err
		}
		_, err = db.Exec("PRAGMA user_version = 5")
		if err != nil {
			return err
		}
		fallthrough
	case 5:
		_, err := db.Exec(`CREATE TABLE IF NOT EXISTS TORMENTED_IPS(
			IPID TEXT PRIMARY KEY
		)`)
		if err != nil {
			return err
		}
		_, err = db.Exec("PRAGMA user_version = 6")
		if err != nil {
			return err
		}
		fallthrough
	case 6:
		_, err := db.Exec(`CREATE TABLE IF NOT EXISTS CHIPS(
			IPID    TEXT    PRIMARY KEY,
			BALANCE INTEGER NOT NULL DEFAULT 100
		)`)
		if err != nil {
			return err
		}
		_, err = db.Exec("PRAGMA user_version = 7")
		if err != nil {
			return err
		}
		fallthrough
	case 7:
		// Add IPID column to USERS so player accounts can be linked to their connection fingerprint.
		// Only alter the table when it already exists (i.e. we are upgrading an existing database).
		// Brand-new databases get the column from the CREATE TABLE statement in Open().
		var usersExists int
		db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='USERS'").Scan(&usersExists) //nolint:errcheck
		if usersExists > 0 {
			_, err := db.Exec("ALTER TABLE USERS ADD COLUMN IPID TEXT NOT NULL DEFAULT ''")
			if err != nil {
				return err
			}
		}
		_, err := db.Exec("PRAGMA user_version = 8")
		if err != nil {
			return err
		}
		fallthrough
	case 8:
		_, err := db.Exec(`CREATE TABLE IF NOT EXISTS IGNORED_IPS(
			IGNORER_IPID TEXT NOT NULL,
			IGNORED_IPID TEXT NOT NULL,
			PRIMARY KEY (IGNORER_IPID, IGNORED_IPID)
		)`)
		if err != nil {
			return err
		}
		_, err = db.Exec("PRAGMA user_version = 9")
		if err != nil {
			return err
		}
		fallthrough
	case 9:
		_, err := db.Exec(`CREATE TABLE IF NOT EXISTS UNSCRAMBLE_WINS(
			IPID TEXT PRIMARY KEY,
			WINS INTEGER NOT NULL DEFAULT 0
		)`)
		if err != nil {
			return err
		}
		_, err = db.Exec(`CREATE TABLE IF NOT EXISTS JOB_COOLDOWNS(
			IPID    TEXT    NOT NULL,
			JOB     TEXT    NOT NULL,
			LAST_AT INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (IPID, JOB)
		)`)
		if err != nil {
			return err
		}
		_, err = db.Exec("PRAGMA user_version = 10")
		if err != nil {
			return err
		}
		fallthrough
	case 10:
		_, err := db.Exec(`CREATE TABLE IF NOT EXISTS JOB_EARNINGS(
			IPID  TEXT    PRIMARY KEY,
			TOTAL INTEGER NOT NULL DEFAULT 0
		)`)
		if err != nil {
			return err
		}
		_, err = db.Exec("PRAGMA user_version = 11")
		if err != nil {
			return err
		}
		fallthrough
	case 11:
		_, err := db.Exec(`CREATE TABLE IF NOT EXISTS SHOP_PURCHASES(
			IPID    TEXT NOT NULL,
			ITEM_ID TEXT NOT NULL,
			PRIMARY KEY (IPID, ITEM_ID)
		)`)
		if err != nil {
			return err
		}
		_, err = db.Exec(`CREATE TABLE IF NOT EXISTS PLAYER_ACTIVE_TAG(
			IPID   TEXT PRIMARY KEY,
			TAG_ID TEXT NOT NULL DEFAULT ''
		)`)
		if err != nil {
			return err
		}
		_, err = db.Exec("PRAGMA user_version = 12")
		if err != nil {
			return err
		}
		fallthrough
	case 12:
		_, err := db.Exec(`CREATE TABLE IF NOT EXISTS FAVOURITES(
			USERNAME  TEXT NOT NULL,
			CHAR_NAME TEXT NOT NULL,
			ADDED_AT  INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (USERNAME, CHAR_NAME)
		)`)
		if err != nil {
			return err
		}
		_, err = db.Exec("PRAGMA user_version = 13")
		if err != nil {
			return err
		}
		fallthrough
	case 13:
		// Add GAMBLE_HIDE column to USERS so per-account gamble-hide preference persists across sessions.
		// Brand-new databases get the column from the CREATE TABLE statement in Open().
		var usersExists int
		db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='USERS'").Scan(&usersExists) //nolint:errcheck
		if usersExists > 0 {
			if _, err := db.Exec("ALTER TABLE USERS ADD COLUMN GAMBLE_HIDE INTEGER NOT NULL DEFAULT 0"); err != nil {
				return err
			}
		}
		if _, err := db.Exec("PRAGMA user_version = 14"); err != nil {
			return err
		}
		fallthrough
	case 14:
		// Create MODNOTES table for per-IPID freeform moderator notes.
		if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS MODNOTES(
			ID        INTEGER PRIMARY KEY AUTOINCREMENT,
			IPID      TEXT    NOT NULL,
			NOTE      TEXT    NOT NULL,
			ADDED_BY  TEXT    NOT NULL DEFAULT '',
			ADDED_AT  INTEGER NOT NULL DEFAULT 0
		)`); err != nil {
			return err
		}
		if _, err := db.Exec("PRAGMA user_version = 15"); err != nil {
			return err
		}
		fallthrough
	case 15:
		// Create CUSTOM_TAGS table so admins can mint cosmetic tags at runtime
		// without rebuilding the server. Custom tag IDs share the same namespace
		// as built-in shop tag IDs and get granted via SHOP_PURCHASES.
		if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS CUSTOM_TAGS(
			ID         TEXT    PRIMARY KEY,
			NAME       TEXT    NOT NULL,
			CREATED_BY TEXT    NOT NULL DEFAULT '',
			CREATED_AT INTEGER NOT NULL DEFAULT 0
		)`); err != nil {
			return err
		}
		if _, err := db.Exec("PRAGMA user_version = 16"); err != nil {
			return err
		}
		fallthrough
	case 16:
		// Add ACTIVE_TAG column to USERS so the equipped tag persists across sessions
		// and IPID changes.  Brand-new databases get the column from the CREATE TABLE
		// statement in Open(); this migration handles existing databases.
		var usersExists int
		db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='USERS'").Scan(&usersExists) //nolint:errcheck
		if usersExists > 0 {
			if _, err := db.Exec("ALTER TABLE USERS ADD COLUMN ACTIVE_TAG TEXT NOT NULL DEFAULT ''"); err != nil {
				return err
			}
		}
		if _, err := db.Exec("PRAGMA user_version = 17"); err != nil {
			return err
		}
		fallthrough
	case 17:
		// Add ISSUER_TIER column to PUNISHMENTS so /unpunish self-removal can
		// detect punishments that were issued by an admin or shadow mod and
		// refuse to let the target moderator silently lift them. Existing rows
		// default to 0 (system/legacy), which preserves the current behaviour
		// of allowing self-removal for anything pre-migration.
		//
		// Fresh databases get the column from the CREATE TABLE statement in
		// Open(); migrations run before those CREATEs, so guard with an
		// existence check (matches the case-16 pattern for USERS.ACTIVE_TAG).
		var punishmentsExists int
		db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='PUNISHMENTS'").Scan(&punishmentsExists) //nolint:errcheck
		if punishmentsExists > 0 {
			if _, err := db.Exec("ALTER TABLE PUNISHMENTS ADD COLUMN ISSUER_TIER INTEGER NOT NULL DEFAULT 0"); err != nil {
				return err
			}
		}
		if _, err := db.Exec("PRAGMA user_version = 18"); err != nil {
			return err
		}
		fallthrough
	case 18:
		// Add USERNAME_RESETS counter to USERS so /resetusername can enforce
		// a hard cap (3) on how many times a player may rename their account.
		var usersExists int
		db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='USERS'").Scan(&usersExists) //nolint:errcheck
		if usersExists > 0 {
			if _, err := db.Exec("ALTER TABLE USERS ADD COLUMN USERNAME_RESETS INTEGER NOT NULL DEFAULT 0"); err != nil {
				return err
			}
		}
		if _, err := db.Exec("PRAGMA user_version = 19"); err != nil {
			return err
		}
		fallthrough
	case 19:
		// Add IGNORER_USERNAME to IGNORED_IPS so the ignore list is linked to a
		// player account rather than just an IPID. This lets the list follow the
		// player across IP changes and enables the numbered /ignore list and
		// offline /unignore <N> workflow. Existing rows are backfilled from the
		// USERS table (where the account IPID matches). Fresh databases get the
		// column from the CREATE TABLE statement in Open().
		var ignoredIPsExists int
		db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='IGNORED_IPS'").Scan(&ignoredIPsExists) //nolint:errcheck
		if ignoredIPsExists > 0 {
			if _, err := db.Exec("ALTER TABLE IGNORED_IPS ADD COLUMN IGNORER_USERNAME TEXT NOT NULL DEFAULT ''"); err != nil {
				return err
			}
			// Backfill username for every row whose IGNORER_IPID is linked to an
			// account. USERS may not exist yet on brand-new databases (it is created
			// in Open() after migrations run), so guard with an existence check.
			// Any rows that cannot be backfilled here will be updated the next time
			// the account owner logs in via BackfillIgnoreUsername.
			var usersExists int
			db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='USERS'").Scan(&usersExists) //nolint:errcheck
			if usersExists > 0 {
				if _, err := db.Exec(`
					UPDATE IGNORED_IPS
					SET IGNORER_USERNAME = COALESCE(
						(SELECT USERNAME FROM USERS WHERE IPID = IGNORED_IPS.IGNORER_IPID AND IPID != ''),
						''
					)
					WHERE IGNORER_USERNAME = ''`); err != nil {
					return err
				}
			}
		}
		if _, err := db.Exec("PRAGMA user_version = 20"); err != nil {
			return err
		}
		fallthrough
	case 20:
		// Add IGNORED_SHOWNAME and IGNORED_OOC to IGNORED_IPS so the ignore
		// list can display the target's showname and OOC name for easy recall.
		// Fresh databases get the columns from the CREATE TABLE in Open().
		var ignoredIPsExists int
		db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='IGNORED_IPS'").Scan(&ignoredIPsExists) //nolint:errcheck
		if ignoredIPsExists > 0 {
			if _, err := db.Exec("ALTER TABLE IGNORED_IPS ADD COLUMN IGNORED_SHOWNAME TEXT NOT NULL DEFAULT ''"); err != nil {
				return err
			}
			if _, err := db.Exec("ALTER TABLE IGNORED_IPS ADD COLUMN IGNORED_OOC TEXT NOT NULL DEFAULT ''"); err != nil {
				return err
			}
		}
		if _, err := db.Exec("PRAGMA user_version = 21"); err != nil {
			return err
		}
		fallthrough
	case 21:
		// MUSIC_BANS persists /musicban: each row blocks one IPID from playing
		// music across sessions. Fresh databases get the table from the CREATE
		// TABLE in Open(); the migration is a no-op-safe CREATE for upgrades.
		if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS MUSIC_BANS(
			IPID      TEXT    PRIMARY KEY,
			REASON    TEXT    NOT NULL DEFAULT '',
			BANNED_BY TEXT    NOT NULL DEFAULT '',
			BANNED_AT INTEGER NOT NULL DEFAULT 0
		)`); err != nil {
			return err
		}
		if _, err := db.Exec("PRAGMA user_version = 22"); err != nil {
			return err
		}
		fallthrough
	case 22:
		// RANDOMCHAR_CURSES persists /curserandomchar: each row forces one
		// IPID's character to randomly change every 1-5 seconds, across
		// sessions, until an admin lifts it with /uncurserandomchar. Fresh
		// databases get the table from the CREATE TABLE in Open(); this
		// migration is a no-op-safe CREATE for upgrades.
		if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS RANDOMCHAR_CURSES(
			IPID      TEXT    PRIMARY KEY,
			ISSUED_BY TEXT    NOT NULL DEFAULT '',
			ISSUED_AT INTEGER NOT NULL DEFAULT 0
		)`); err != nil {
			return err
		}
		if _, err := db.Exec("PRAGMA user_version = 23"); err != nil {
			return err
		}
		fallthrough
	case 23:
		// SHADOW_DISCONNECTS persists /shadowdisconnect: each row silently and
		// permanently bars one IPID from ever completing a connection again —
		// every future join attempt is dropped with no explanation, until an
		// admin lifts it with /shadowundisconnect. Fresh databases get the
		// table from the CREATE TABLE in Open(); this migration is a
		// no-op-safe CREATE for upgrades.
		if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS SHADOW_DISCONNECTS(
			IPID      TEXT    PRIMARY KEY,
			REASON    TEXT    NOT NULL DEFAULT '',
			ISSUED_BY TEXT    NOT NULL DEFAULT '',
			ISSUED_AT INTEGER NOT NULL DEFAULT 0
		)`); err != nil {
			return err
		}
		if _, err := db.Exec("PRAGMA user_version = 24"); err != nil {
			return err
		}
		fallthrough
	case 24:
		// LOCKDOWN_EXEMPT persists a verified /lockdown whitelist passkey:
		// each row permanently exempts one IPID from the lockdown playtime
		// purge/silence, regardless of actual playtime, on top of the
		// existing "known IP" join-gate bypass. Fresh databases get the
		// table from the CREATE TABLE in Open(); this migration is a
		// no-op-safe CREATE for upgrades.
		if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS LOCKDOWN_EXEMPT(
			IPID      TEXT    PRIMARY KEY,
			ISSUED_BY TEXT    NOT NULL DEFAULT '',
			ISSUED_AT INTEGER NOT NULL DEFAULT 0
		)`); err != nil {
			return err
		}
		if _, err := db.Exec("PRAGMA user_version = 25"); err != nil {
			return err
		}
		fallthrough
	case 25:
		// JOIN_CAPTCHA_VERIFIED records the IPIDs that have solved the
		// join captcha. Verification must outlive the connection: a
		// purely in-memory flag would be cleared by the disconnect it is
		// meant to survive, so a bot could reconnect straight past the
		// gate. Fresh databases get the table from the CREATE TABLE in
		// Open(); this migration is a no-op-safe CREATE for upgrades.
		if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS JOIN_CAPTCHA_VERIFIED(
			IPID        TEXT    PRIMARY KEY,
			VERIFIED_AT INTEGER NOT NULL DEFAULT 0,
			KIND        TEXT    NOT NULL DEFAULT ''
		)`); err != nil {
			return err
		}
		if _, err := db.Exec("PRAGMA user_version = 26"); err != nil {
			return err
		}
		fallthrough
	case 26:
		// ACCOUNT_COMMAND_GRANTS backs the console-only grantcmd/revokecmd
		// commands: each row lets one account use one specific command
		// regardless of its role/permission bitfield, so an operator with
		// shell access can hand a player "/ban" without making them a full
		// moderator. Fresh databases get the table from the CREATE TABLE in
		// Open(); this migration is a no-op-safe CREATE for upgrades.
		if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS ACCOUNT_COMMAND_GRANTS(
			USERNAME   TEXT    NOT NULL,
			COMMAND    TEXT    NOT NULL,
			GRANTED_BY TEXT    NOT NULL DEFAULT '',
			GRANTED_AT INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (USERNAME, COMMAND)
		)`); err != nil {
			return err
		}
		if _, err := db.Exec("PRAGMA user_version = 27"); err != nil {
			return err
		}
	}
	return nil
}

// UserExists returns whether a user exists within the server's database.
func UserExists(username string) bool {
	if db == nil {
		return false
	}
	var count int
	if err := db.QueryRow(
		"SELECT COUNT(*) FROM USERS WHERE USERNAME = ?",
		username,
	).Scan(&count); err != nil {
		return false
	}
	return count > 0
}

// IsModUser returns true when the given username exists in USERS with
// non-zero permissions — i.e. it is a moderator/admin account created via
// /mkusr rather than a player account from /register. Used by /register to
// give a clearer error when a player picks a username already taken by a
// staff account.
func IsModUser(username string) bool {
	if db == nil {
		return false
	}
	var perms string
	err := db.QueryRow("SELECT PERMISSIONS FROM USERS WHERE USERNAME = ?", username).Scan(&perms)
	if err != nil {
		return false
	}
	p, err := strconv.ParseUint(perms, 10, 64)
	if err != nil {
		return false
	}
	return p != 0
}

// CreateUser adds a new user to the server's database.
// This creates a moderator/admin account with the given permissions.
// The IPID field is left empty and must be linked on first login via LinkIPIDToUser.
func CreateUser(username string, password []byte, permissions uint64) error {
	hashed, err := bcrypt.GenerateFromPassword(password, 12)
	if err != nil {
		return err
	}
	_, err = db.Exec("INSERT INTO USERS(USERNAME, PASSWORD, PERMISSIONS) VALUES(?, ?, ?)", username, hashed, strconv.FormatUint(permissions, 10))
	if err != nil {
		return err
	}
	return nil
}

// RemoveUser deletes a user from the server's database.
func RemoveUser(username string) error {
	_, err := db.Exec("DELETE FROM USERS WHERE USERNAME = ?", username)
	if err != nil {
		return err
	}
	return nil
}

// AuthenticateUser returns whether or not the user's credentials match those in the database, and that user's permissions.
// dummyBcryptHash is compared against whenever a username lookup finds no
// row, so AuthenticateUser costs about the same whether it's rejecting an
// unknown username or a wrong password on a real account. Without this, an
// empty PASSWORD value fails bcrypt's malformed-hash check almost instantly
// -- a measurable timing oracle an attacker could use to enumerate valid
// usernames by response time alone.
var dummyBcryptHash []byte

func init() {
	// Cost matches UpdatePassword/CreateUser below (12) so the dummy compare
	// costs the same as a real one. Only fails on a cost/length violation,
	// neither of which applies to this fixed literal -- unreachable in
	// practice.
	h, err := bcrypt.GenerateFromPassword([]byte("nyathena-timing-safety-dummy"), 12)
	if err != nil {
		panic(fmt.Sprintf("db: failed to generate dummy bcrypt hash: %v", err))
	}
	dummyBcryptHash = h
}

func AuthenticateUser(username string, password []byte) (bool, uint64) {
	var rpass, rperms string
	result := db.QueryRow("SELECT PASSWORD, PERMISSIONS FROM USERS WHERE USERNAME = ?", username)
	found := result.Scan(&rpass, &rperms) == nil

	hash := []byte(rpass)
	if !found {
		hash = dummyBcryptHash
	}
	err := bcrypt.CompareHashAndPassword(hash, password)
	if !found || err != nil {
		return false, 0
	}
	p, err := strconv.ParseUint(rperms, 10, 64)
	if err != nil {
		return false, 0
	}
	return true, p
}

// UpdatePassword replaces the stored bcrypt password hash for the given user.
func UpdatePassword(username string, password []byte) error {
	hashed, err := bcrypt.GenerateFromPassword(password, 12)
	if err != nil {
		return err
	}
	_, err = db.Exec("UPDATE USERS SET PASSWORD = ? WHERE USERNAME = ?", hashed, username)
	return err
}

// ChangePermissions updated the permissions of a user in the database.
func ChangePermissions(username string, permissions uint64) error {
	_, err := db.Exec("UPDATE USERS SET PERMISSIONS = ? WHERE USERNAME = ?", strconv.FormatUint(permissions, 10), username)
	if err != nil {
		return err
	}
	return nil
}

// RegisterPlayer creates a player (non-moderator) account with zero permissions
// and records the player's IPID so it can be looked up later.
// Returns an error if the username is already taken.
func RegisterPlayer(username string, password []byte, ipid string) error {
	hashed, err := bcrypt.GenerateFromPassword(password, 12)
	if err != nil {
		return err
	}
	_, err = db.Exec("INSERT INTO USERS(USERNAME, PASSWORD, PERMISSIONS, IPID) VALUES(?, ?, '0', ?)", username, hashed, ipid)
	return err
}

// RegisterPlayerHashed is like RegisterPlayer but accepts an already-hashed
// bcrypt password. Use this when the password was hashed at an earlier step
// (e.g. before being stored in a pending-registration state) to avoid keeping
// a plaintext password in memory longer than necessary.
func RegisterPlayerHashed(username string, hashedPassword []byte, ipid string) error {
	_, err := db.Exec("INSERT INTO USERS(USERNAME, PASSWORD, PERMISSIONS, IPID) VALUES(?, ?, '0', ?)", username, hashedPassword, ipid)
	return err
}

// MaxUsernameResets is the cap on /resetusername invocations per account.
const MaxUsernameResets = 3

// GetUsernameResets returns how many times this account has renamed itself.
// Used by /resetusername to enforce the per-account cap.
func GetUsernameResets(username string) (int, error) {
	if db == nil {
		return 0, nil
	}
	var n int
	switch err := db.QueryRow("SELECT USERNAME_RESETS FROM USERS WHERE USERNAME = ?", username).Scan(&n); {
	case err == sql.ErrNoRows:
		return 0, nil
	case err != nil:
		return 0, err
	}
	return n, nil
}

// RenameAccount renames an account and increments the per-account rename
// counter atomically. Returns ErrUsernameTaken if the new name is already in
// use, or ErrUsernameLimit if the account has already reached its cap.
func RenameAccount(oldName, newName string) error {
	if db == nil {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	var resets int
	switch err := tx.QueryRow("SELECT USERNAME_RESETS FROM USERS WHERE USERNAME = ?", oldName).Scan(&resets); {
	case err == sql.ErrNoRows:
		return sql.ErrNoRows
	case err != nil:
		return err
	}
	if resets >= MaxUsernameResets {
		return ErrUsernameLimit
	}

	// Reject if the new name is already taken (case-sensitive — matches USERS PK).
	var existing int
	if err := tx.QueryRow("SELECT COUNT(*) FROM USERS WHERE USERNAME = ?", newName).Scan(&existing); err != nil {
		return err
	}
	if existing > 0 {
		return ErrUsernameTaken
	}

	if _, err := tx.Exec("UPDATE USERS SET USERNAME = ?, USERNAME_RESETS = USERNAME_RESETS + 1 WHERE USERNAME = ?", newName, oldName); err != nil {
		return err
	}
	// Carry any console-granted command access along with the account —
	// otherwise a /resetusername would silently drop staff-issued command
	// grants with no error to either the player or the operator who granted
	// them. ACCOUNT_COMMAND_GRANTS is keyed (USERNAME, COMMAND); if the new
	// name already holds an identical grant (only possible if it was granted
	// separately under that name before), keep that row and drop the old
	// one rather than violating the primary key.
	if _, err := tx.Exec(`UPDATE OR REPLACE ACCOUNT_COMMAND_GRANTS SET USERNAME = ? WHERE USERNAME = ?`, newName, oldName); err != nil {
		return err
	}
	return tx.Commit()
}

// ErrUsernameTaken indicates a /resetusername target is already registered.
var ErrUsernameTaken = errors.New("username already taken")

// ErrUsernameLimit indicates the account has hit MaxUsernameResets.
var ErrUsernameLimit = errors.New("username reset limit reached")

// LinkIPIDToUser associates an IPID with a user account.
// Called on every successful login so the leaderboard can show account names.
// When the player's IPID has changed (e.g. their IP address changed), any
// playtime accumulated under the old IPID is merged into the new IPID so the
// leaderboard continues to display the correct total under the player's account name.
func LinkIPIDToUser(username, ipid string) error {
	if db == nil {
		return nil
	}

	// Retrieve the IPID currently stored for this account.
	var oldIPID string
	switch err := db.QueryRow("SELECT COALESCE(IPID, '') FROM USERS WHERE USERNAME = ?", username).Scan(&oldIPID); {
	case err == sql.ErrNoRows:
		// User not found — the UPDATE below will affect 0 rows, which is intentional.
	case err != nil:
		return err
	}

	// Update the USERS table with the new IPID.
	if _, err := db.Exec("UPDATE USERS SET IPID = ? WHERE USERNAME = ?", ipid, username); err != nil {
		return err
	}

	// Nothing more to do if the IPID hasn't changed or was previously unset.
	if oldIPID == "" || oldIPID == ipid {
		return nil
	}

	// Fetch the playtime accumulated under the old IPID.
	var oldPlaytime int64
	switch err := db.QueryRow("SELECT COALESCE(PLAYTIME, 0) FROM KNOWN_IPS WHERE IPID = ?", oldIPID).Scan(&oldPlaytime); {
	case err == sql.ErrNoRows:
		oldPlaytime = 0
	case err != nil:
		return err
	}

	if oldPlaytime > 0 {
		// Merge old playtime into the new IPID. UPSERT ensures the row is created
		// if it does not yet exist in KNOWN_IPS (defensive; in practice MarkIPKnown
		// is called on every connection before /login can be issued).
		now := time.Now().Unix()
		if _, err := db.Exec(`
			INSERT INTO KNOWN_IPS (IPID, FIRST_SEEN, LAST_SEEN, PLAYTIME)
			VALUES (?, ?, ?, ?)
			ON CONFLICT(IPID) DO UPDATE SET PLAYTIME = PLAYTIME + excluded.PLAYTIME`,
			ipid, now, now, oldPlaytime); err != nil {
			return err
		}
		if _, err := db.Exec("UPDATE KNOWN_IPS SET PLAYTIME = 0 WHERE IPID = ?", oldIPID); err != nil {
			return err
		}
	}

	// Merge unscramble wins from the old IPID into the new IPID so the
	// leaderboard continues to reflect the player's full win count.
	var oldWins int64
	switch err := db.QueryRow("SELECT COALESCE(WINS, 0) FROM UNSCRAMBLE_WINS WHERE IPID = ?", oldIPID).Scan(&oldWins); {
	case err == sql.ErrNoRows:
		oldWins = 0
	case err != nil:
		return err
	}

	if oldWins > 0 {
		if _, err := db.Exec(`
			INSERT INTO UNSCRAMBLE_WINS(IPID, WINS) VALUES(?, ?)
			ON CONFLICT(IPID) DO UPDATE SET WINS = WINS + excluded.WINS`,
			ipid, oldWins); err != nil {
			return err
		}
		_, err := db.Exec("UPDATE UNSCRAMBLE_WINS SET WINS = 0 WHERE IPID = ?", oldIPID)
		if err != nil {
			return err
		}
	}

	// Merge job earnings from the old IPID into the new IPID so the
	// job-earnings leaderboard continues to reflect the player's full total.
	var oldJobTotal int64
	switch err := db.QueryRow("SELECT COALESCE(TOTAL, 0) FROM JOB_EARNINGS WHERE IPID = ?", oldIPID).Scan(&oldJobTotal); {
	case err == sql.ErrNoRows:
		oldJobTotal = 0
	case err != nil:
		return err
	}

	if oldJobTotal > 0 {
		if _, err := db.Exec(`
			INSERT INTO JOB_EARNINGS(IPID, TOTAL) VALUES(?, ?)
			ON CONFLICT(IPID) DO UPDATE SET TOTAL = TOTAL + excluded.TOTAL`,
			ipid, oldJobTotal); err != nil {
			return err
		}
		if _, err := db.Exec("UPDATE JOB_EARNINGS SET TOTAL = 0 WHERE IPID = ?", oldIPID); err != nil {
			return err
		}
	}

	// Migrate chip balance from the old IPID to the new IPID so that players
	// never lose their earned chips when reconnecting from a different IP address.
	// The new IPID may have a default starting balance seeded by EnsureChipBalance,
	// but the old IPID's earned balance is always the canonical value and must
	// replace any placeholder on the new IPID unconditionally.
	var oldChips int64
	switch err := db.QueryRow("SELECT COALESCE(BALANCE, 0) FROM CHIPS WHERE IPID = ?", oldIPID).Scan(&oldChips); {
	case err == sql.ErrNoRows:
		oldChips = 0
	case err != nil:
		return err
	}

	if oldChips > 0 {
		if _, err := db.Exec(`
			INSERT INTO CHIPS(IPID, BALANCE) VALUES(?, ?)
			ON CONFLICT(IPID) DO UPDATE SET BALANCE = excluded.BALANCE`,
			ipid, oldChips); err != nil {
			return err
		}
		if _, err := db.Exec("UPDATE CHIPS SET BALANCE = 0 WHERE IPID = ?", oldIPID); err != nil {
			return err
		}
		// Invalidate cached values for both IPIDs so subsequent reads hit the DB.
		chipCacheInvalidate(ipid)
		chipCacheInvalidate(oldIPID)
	}

	// Migrate the active cosmetic tag from the old IPID to the new IPID so the
	// player's equipped tag is not lost when their IP address changes.
	var oldTag string
	db.QueryRow("SELECT COALESCE(TAG_ID, '') FROM PLAYER_ACTIVE_TAG WHERE IPID = ?", oldIPID).Scan(&oldTag) //nolint:errcheck
	if oldTag != "" {
		if _, err := db.Exec(`
			INSERT INTO PLAYER_ACTIVE_TAG(IPID, TAG_ID) VALUES(?, ?)
			ON CONFLICT(IPID) DO UPDATE SET TAG_ID = excluded.TAG_ID`, ipid, oldTag); err != nil {
			return err
		}
		if _, err := db.Exec("UPDATE PLAYER_ACTIVE_TAG SET TAG_ID = '' WHERE IPID = ?", oldIPID); err != nil {
			return err
		}
	}

	return nil
}

// GetUsernameByIPID returns the username whose account is linked to the given IPID.
// Returns ("", nil) when no account is associated with that IPID.
func GetUsernameByIPID(ipid string) (string, error) {
	if db == nil {
		return "", nil
	}
	row := db.QueryRow("SELECT USERNAME FROM USERS WHERE IPID = ?", ipid)
	var username string
	if err := row.Scan(&username); err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", err
	}
	return username, nil
}

// AddBan adds a new ban to the database.
func AddBan(ipid string, hdid string, time int64, duration int64, reason string, moderator string) (int, error) {
	result, err := db.Exec("INSERT INTO BANS VALUES(NULL, ?, ?, ?, ?, ?, ?)", ipid, hdid, time, duration, reason, moderator)
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	return int(id), nil
}

// UnBan nullifies a ban in the database.
func UnBan(id int) error {
	_, err := db.Exec("UPDATE BANS SET DURATION = 0 WHERE ID = ?", id)
	if err != nil {
		return err
	}
	return nil
}

// GetBan returns a list of bans matching a given value.
func GetBan(by BanLookup, value any) ([]BanInfo, error) {
	var stmt *sql.Stmt
	var err error
	switch by {
	case BANID:
		stmt, err = db.Prepare("SELECT * FROM BANS WHERE ID = ?")
	case IPID:
		stmt, err = db.Prepare("SELECT * FROM BANS WHERE IPID = ? ORDER BY TIME DESC")
	}
	if err != nil {
		return []BanInfo{}, err
	}
	result, err := stmt.Query(value)
	if err != nil {
		return []BanInfo{}, err
	}
	stmt.Close()
	defer result.Close()
	var bans []BanInfo
	for result.Next() {
		var b BanInfo
		result.Scan(&b.Id, &b.Ipid, &b.Hdid, &b.Time, &b.Duration, &b.Reason, &b.Moderator)
		bans = append(bans, b)
	}
	return bans, nil
}

// GetRecentBans returns the 5 most recent bans.
func GetRecentBans() ([]BanInfo, error) {
	if db == nil {
		return nil, nil
	}
	result, err := db.Query("SELECT * FROM BANS ORDER BY TIME DESC LIMIT 5")
	if err != nil {
		return []BanInfo{}, err
	}
	defer result.Close()
	var bans []BanInfo
	for result.Next() {
		var b BanInfo
		result.Scan(&b.Id, &b.Ipid, &b.Hdid, &b.Time, &b.Duration, &b.Reason, &b.Moderator)
		bans = append(bans, b)
	}
	return bans, nil
}

// IsBanned returns whether the given ipid/hdid is banned, and the info of the ban.
func IsBanned(by BanLookup, value string) (bool, BanInfo, error) {
	var stmt *sql.Stmt
	var err error
	switch by {
	case IPID:
		stmt, err = db.Prepare("SELECT ID, DURATION, REASON FROM BANS WHERE IPID = ?")
	case HDID:
		stmt, err = db.Prepare("SELECT ID, DURATION, REASON FROM BANS WHERE HDID = ?")
	}
	if err != nil {
		return false, BanInfo{}, err
	}
	result, err := stmt.Query(value)
	if err != nil {
		return false, BanInfo{}, err
	}
	stmt.Close()
	defer result.Close()
	for result.Next() {
		var (
			duration int64
			id       int
			reason   string
		)
		result.Scan(&id, &duration, &reason)
		if duration == -1 || time.Unix(duration, 0).UTC().After(time.Now().UTC()) {
			return true, BanInfo{Id: id, Duration: duration, Reason: reason}, nil
		}
	}
	return false, BanInfo{}, nil
}

// UpdateReason updates the reason of a ban.
func UpdateReason(id int, reason string) error {
	_, err := db.Exec("UPDATE BANS SET REASON = ? WHERE ID = ?", reason, id)
	if err != nil {
		return err
	}
	return nil
}

// UpdateDuration updates the duration of a ban.
func UpdateDuration(id int, duration int64) error {
	_, err := db.Exec("UPDATE BANS SET DURATION = ? WHERE ID = ?", duration, id)
	if err != nil {
		return err
	}
	return nil
}

// Closes the server's database connection.
func Close() {
	db.Close()
}

// UpsertMute stores (or replaces) the mute state for an IPID.
// muteType is the MuteState integer value. expires is a Unix timestamp (0 = permanent).
func UpsertMute(ipid string, muteType int, expires int64) error {
	_, err := db.Exec(
		"INSERT OR REPLACE INTO PUNISHMENTS(IPID, KIND, SUBTYPE, VALUE, EXPIRES, REASON) VALUES(?, ?, 0, ?, ?, '')",
		ipid, PunishKindMute, muteType, expires)
	return err
}

// DeleteMute removes any stored mute for an IPID.
func DeleteMute(ipid string) error {
	_, err := db.Exec("DELETE FROM PUNISHMENTS WHERE IPID = ? AND KIND = ?", ipid, PunishKindMute)
	return err
}

// UpsertJail stores (or replaces) the jail state for an IPID.
// expires is a Unix timestamp of when the jail ends.
// areaID is the area index the player is jailed in; use -1 for "current area" (no forced move on reconnect).
func UpsertJail(ipid string, expires int64, reason string, areaID int) error {
	_, err := db.Exec(
		"INSERT OR REPLACE INTO PUNISHMENTS(IPID, KIND, SUBTYPE, VALUE, EXPIRES, REASON) VALUES(?, ?, 0, ?, ?, ?)",
		ipid, PunishKindJail, areaID, expires, reason)
	return err
}

// DeleteJail removes any stored jail for an IPID.
func DeleteJail(ipid string) error {
	_, err := db.Exec("DELETE FROM PUNISHMENTS WHERE IPID = ? AND KIND = ?", ipid, PunishKindJail)
	return err
}

// UpsertTextPunishment stores (or replaces) a text/behaviour punishment for an IPID.
// pType is the PunishmentType integer. expires is a Unix timestamp (0 = permanent).
// Issuer tier defaults to 0 (system/legacy); use UpsertTextPunishmentBy to record
// the issuer's permission tier so /unpunish can enforce self-removal protection.
func UpsertTextPunishment(ipid string, pType int, expires int64, reason string) error {
	return UpsertTextPunishmentBy(ipid, pType, expires, reason, 0)
}

// UpsertTextPunishmentBy is the same as UpsertTextPunishment but records the
// issuer's permission tier (0=system, 1=mod, 2=shadow, 3=admin). The tier is
// later read by /unpunish to block moderators from silently lifting punishments
// that were applied by a shadow mod or admin against them.
func UpsertTextPunishmentBy(ipid string, pType int, expires int64, reason string, issuerTier int) error {
	_, err := db.Exec(
		"INSERT OR REPLACE INTO PUNISHMENTS(IPID, KIND, SUBTYPE, VALUE, EXPIRES, REASON, ISSUER_TIER) VALUES(?, ?, ?, 0, ?, ?, ?)",
		ipid, PunishKindText, pType, expires, reason, issuerTier)
	return err
}

// DeleteTextPunishment removes a specific text punishment for an IPID.
func DeleteTextPunishment(ipid string, pType int) error {
	_, err := db.Exec("DELETE FROM PUNISHMENTS WHERE IPID = ? AND KIND = ? AND SUBTYPE = ?",
		ipid, PunishKindText, pType)
	return err
}

// UpsertCharStuck stores (or replaces) a character-stuck record for an IPID.
// charID is the character the player is locked to. expires is a Unix timestamp (0 = permanent).
func UpsertCharStuck(ipid string, charID int, expires int64, reason string) error {
	_, err := db.Exec(
		"INSERT OR REPLACE INTO PUNISHMENTS(IPID, KIND, SUBTYPE, VALUE, EXPIRES, REASON) VALUES(?, ?, 0, ?, ?, ?)",
		ipid, PunishKindCharStuck, charID, expires, reason)
	return err
}

// DeleteCharStuck removes any stored char-stuck record for an IPID.
func DeleteCharStuck(ipid string) error {
	_, err := db.Exec("DELETE FROM PUNISHMENTS WHERE IPID = ? AND KIND = ?", ipid, PunishKindCharStuck)
	return err
}

// DeleteAllPunishments removes ALL stored punishments (mute, jail, and text) for an IPID.
func DeleteAllPunishments(ipid string) error {
	_, err := db.Exec("DELETE FROM PUNISHMENTS WHERE IPID = ?", ipid)
	return err
}

// GetPunishments returns all currently active persistent punishments for an IPID.
// Expired entries are filtered in the query rather than deleted, keeping this a pure read.
// Call PurgeExpired periodically to reclaim space from old expired rows.
func GetPunishments(ipid string) ([]PersistentPunishment, error) {
	now := time.Now().Unix()
	rows, err := db.Query(
		"SELECT KIND, SUBTYPE, VALUE, EXPIRES, REASON, ISSUER_TIER FROM PUNISHMENTS WHERE IPID = ? AND (EXPIRES = 0 OR EXPIRES > ?)",
		ipid, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var punishments []PersistentPunishment
	for rows.Next() {
		var p PersistentPunishment
		if err := rows.Scan(&p.Kind, &p.Subtype, &p.Value, &p.Expires, &p.Reason, &p.IssuerTier); err != nil {
			continue
		}
		punishments = append(punishments, p)
	}
	return punishments, nil
}

// PurgeExpired deletes all expired punishment rows from the database.
// It is safe to call from a background goroutine.
func PurgeExpired() error {
	_, err := db.Exec("DELETE FROM PUNISHMENTS WHERE EXPIRES != 0 AND EXPIRES <= ?", time.Now().Unix())
	PurgeChipCache()
	return err
}

// GetAllBans returns all bans from the database.
func GetAllBans() ([]BanInfo, error) {
	result, err := db.Query("SELECT * FROM BANS ORDER BY TIME DESC")
	if err != nil {
		return []BanInfo{}, err
	}
	defer result.Close()
	var bans []BanInfo
	for result.Next() {
		var b BanInfo
		if err := result.Scan(&b.Id, &b.Ipid, &b.Hdid, &b.Time, &b.Duration, &b.Reason, &b.Moderator); err != nil {
			continue
		}
		bans = append(bans, b)
	}
	return bans, nil
}

// MarkIPKnown records an IPID as known and updates its last-seen timestamp.
// For new IPIDs both FIRST_SEEN and LAST_SEEN are set to now.
// For IPIDs already in the table FIRST_SEEN is preserved and only LAST_SEEN is
// updated — unless the row was zeroed by RemoveKnownIP (a ban), in which case
// FIRST_SEEN is re-stamped so the row becomes a normal known-IP row again.
func MarkIPKnown(ipid string) error {
	if db == nil {
		return nil
	}
	now := time.Now().Unix()
	_, err := db.Exec(
		`INSERT INTO KNOWN_IPS(IPID, FIRST_SEEN, LAST_SEEN) VALUES(?, ?, ?)
		 ON CONFLICT(IPID) DO UPDATE SET
		 	LAST_SEEN = excluded.LAST_SEEN,
		 	FIRST_SEEN = CASE WHEN KNOWN_IPS.FIRST_SEEN = 0 THEN excluded.FIRST_SEEN ELSE KNOWN_IPS.FIRST_SEEN END`,
		ipid, now, now)
	return err
}

// LoadKnownIPs returns every IPID that has previously been recorded by MarkIPKnown.
// It is called once at server startup to pre-populate the in-memory first-seen tracker.
// Rows zeroed by RemoveKnownIP (banned IPs whose playtime is being preserved) are
// excluded so a banned IP is still treated as new after a server restart.
func LoadKnownIPs() ([]string, error) {
	if db == nil {
		return nil, nil
	}
	rows, err := db.Query("SELECT IPID FROM KNOWN_IPS WHERE FIRST_SEEN != 0 OR LAST_SEEN != 0")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ipids []string
	for rows.Next() {
		var ipid string
		if err := rows.Scan(&ipid); err != nil {
			return ipids, fmt.Errorf("LoadKnownIPs scan: %w", err)
		}
		ipids = append(ipids, ipid)
	}
	return ipids, rows.Err()
}

// RemoveKnownIP forgets an IPID's known-IP status without destroying its
// accumulated playtime. It is called when an IP is banned so that, once the ban
// expires, the IP is treated as new again (subject to new-connection cooldowns
// and rate limits). A row with no playtime is deleted outright; a row with
// playtime is kept with FIRST_SEEN/LAST_SEEN zeroed — LoadKnownIPs skips zeroed
// rows, so the "new again" behaviour is preserved while /playtime survives the ban.
func RemoveKnownIP(ipid string) error {
	if db == nil {
		return nil
	}
	if _, err := db.Exec("DELETE FROM KNOWN_IPS WHERE IPID = ? AND PLAYTIME <= 0", ipid); err != nil {
		return err
	}
	_, err := db.Exec("UPDATE KNOWN_IPS SET FIRST_SEEN = 0, LAST_SEEN = 0 WHERE IPID = ?", ipid)
	return err
}

// PruneInactiveIPs deletes all KNOWN_IPS rows whose LAST_SEEN timestamp is
// older than the provided Unix timestamp. It returns the number of rows removed.
// Call this at startup (after loading known IPs) to keep the table lean.
func PruneInactiveIPs(before int64) (int64, error) {
	if db == nil {
		return 0, nil
	}
	res, err := db.Exec("DELETE FROM KNOWN_IPS WHERE LAST_SEEN < ? AND LAST_SEEN != 0", before)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// AddPlaytime increments the PLAYTIME counter for an IPID by the given number of seconds.
// It is called when a client disconnects to accumulate their session duration.
func AddPlaytime(ipid string, seconds int64) error {
	if db == nil {
		return nil
	}
	_, err := db.Exec("UPDATE KNOWN_IPS SET PLAYTIME = PLAYTIME + ? WHERE IPID = ?", seconds, ipid)
	return err
}

// AddPlaytimeReturning atomically increments the PLAYTIME counter for an IPID by
// the given number of seconds and returns the new accumulated total.
// Using a single RETURNING statement avoids the read-then-write race that arises
// when two sessions for the same IPID disconnect simultaneously.
// Returns (0, nil) when the database is not initialised.
func AddPlaytimeReturning(ipid string, seconds int64) (int64, error) {
	if db == nil {
		return 0, nil
	}
	row := db.QueryRow(
		"UPDATE KNOWN_IPS SET PLAYTIME = PLAYTIME + ? WHERE IPID = ? RETURNING PLAYTIME",
		seconds, ipid)
	var newTotal int64
	if err := row.Scan(&newTotal); err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, err
	}
	return newTotal, nil
}

// SubtractPlaytimeReturning atomically decrements the PLAYTIME counter for an
// IPID by the given number of seconds, clamped at 0 (never goes negative),
// and returns the new accumulated total.
// Returns (0, nil) when the database is not initialised.
func SubtractPlaytimeReturning(ipid string, seconds int64) (int64, error) {
	if db == nil {
		return 0, nil
	}
	row := db.QueryRow(
		"UPDATE KNOWN_IPS SET PLAYTIME = MAX(0, PLAYTIME - ?) WHERE IPID = ? RETURNING PLAYTIME",
		seconds, ipid)
	var newTotal int64
	if err := row.Scan(&newTotal); err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, err
	}
	return newTotal, nil
}

// SetPlaytimeReturning atomically sets the PLAYTIME counter for an IPID to the
// given number of seconds (clamped at 0) and returns the new total.
// Returns (0, nil) when the database is not initialised.
func SetPlaytimeReturning(ipid string, seconds int64) (int64, error) {
	if db == nil {
		return 0, nil
	}
	if seconds < 0 {
		seconds = 0
	}
	row := db.QueryRow(
		"UPDATE KNOWN_IPS SET PLAYTIME = ? WHERE IPID = ? RETURNING PLAYTIME",
		seconds, ipid)
	var newTotal int64
	if err := row.Scan(&newTotal); err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, err
	}
	return newTotal, nil
}

// PruneShortPlaytimeIPs deletes all KNOWN_IPS rows whose accumulated PLAYTIME is
// less than minSeconds. IPs that have played for at least minSeconds are retained.
// It returns the number of rows removed.
func PruneShortPlaytimeIPs(minSeconds int64) (int64, error) {
	if db == nil {
		return 0, nil
	}
	res, err := db.Exec("DELETE FROM KNOWN_IPS WHERE PLAYTIME < ?", minSeconds)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// GetPlaytime returns the accumulated playtime in seconds for an IPID from the KNOWN_IPS table.
// Returns 0, nil if the IPID is not found.
func GetPlaytime(ipid string) (int64, error) {
	if db == nil {
		return 0, nil
	}
	row := db.QueryRow("SELECT PLAYTIME FROM KNOWN_IPS WHERE IPID = ?", ipid)
	var playtime int64
	if err := row.Scan(&playtime); err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, err
	}
	return playtime, nil
}

// GetAccountStats returns the chip balance and accumulated playtime for an IPID
// in a single query, avoiding two separate database round-trips.
// Returns (0, 0, nil) when the database is not initialised.
func GetAccountStats(ipid string) (chips, playtime int64, err error) {
	if db == nil {
		return 0, 0, nil
	}
	err = db.QueryRow(
		`SELECT COALESCE((SELECT BALANCE FROM CHIPS WHERE IPID = ?), 0),
		        COALESCE((SELECT PLAYTIME FROM KNOWN_IPS WHERE IPID = ?), 0)`,
		ipid, ipid).Scan(&chips, &playtime)
	return
}

// SyncChipsForExistingPlaytime creates CHIPS rows for all KNOWN_IPS entries
// that have accumulated PLAYTIME but do not yet have a CHIPS row. This is a
// one-time migration for players who were active before the casino update.
// The starting balance is the default (100) plus 1 chip per completed hour of
// pre-existing playtime. Existing CHIPS rows are never modified.
// Returns the number of new rows inserted.
func SyncChipsForExistingPlaytime() (int64, error) {
	if db == nil {
		return 0, nil
	}
	res, err := db.Exec(`
		INSERT OR IGNORE INTO CHIPS(IPID, BALANCE)
		SELECT IPID, ? + (PLAYTIME / 3600)
		FROM KNOWN_IPS
		WHERE PLAYTIME > 0`, defaultChipBalance)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// PurgeKnownIPs deletes all rows from the KNOWN_IPS table.
// It returns the number of rows removed.
func PurgeKnownIPs() (int64, error) {
	if db == nil {
		return 0, nil
	}
	res, err := db.Exec("DELETE FROM KNOWN_IPS")
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// AddTormentedIP adds an IPID to the TORMENTED_IPS table.
// Tormented IPIDs experience random disconnects every 30–60 seconds instead of being banned.
func AddTormentedIP(ipid string) error {
	if db == nil {
		return nil
	}
	_, err := db.Exec("INSERT OR IGNORE INTO TORMENTED_IPS(IPID) VALUES(?)", ipid)
	return err
}

// RemoveTormentedIP deletes an IPID from the TORMENTED_IPS table.
func RemoveTormentedIP(ipid string) error {
	if db == nil {
		return nil
	}
	_, err := db.Exec("DELETE FROM TORMENTED_IPS WHERE IPID = ?", ipid)
	return err
}

// ClearTormentedIPs deletes every IPID from the TORMENTED_IPS table.
// Used by /untorment all to purge the whole torment list at once.
func ClearTormentedIPs() error {
	if db == nil {
		return nil
	}
	_, err := db.Exec("DELETE FROM TORMENTED_IPS")
	return err
}

// LoadTormentedIPs returns every IPID currently in the TORMENTED_IPS table.
// Called once at server startup to pre-populate the in-memory torment set.
func LoadTormentedIPs() ([]string, error) {
	if db == nil {
		return nil, nil
	}
	rows, err := db.Query("SELECT IPID FROM TORMENTED_IPS")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ipids []string
	for rows.Next() {
		var ipid string
		if err := rows.Scan(&ipid); err != nil {
			return ipids, fmt.Errorf("LoadTormentedIPs scan: %w", err)
		}
		ipids = append(ipids, ipid)
	}
	return ipids, rows.Err()
}

// IgnoreEntry is one row from the ignore list, as shown to the player.
type IgnoreEntry struct {
	// Position is the 1-based display index in the ordered list.
	Position int
	// IPID is the ignored connection fingerprint.
	IPID string
	// Showname is the ignored user's effective showname at the time they were ignored.
	Showname string
	// OOC is the ignored user's OOC name at the time they were ignored.
	OOC string
}

// AddIgnoredIP records that ignorerIPID has permanently ignored ignoredIPID.
// ignorerUsername may be empty for unauthenticated clients; when set it is
// stored so the ignore survives IP changes. showname and oocName capture the
// target's display names at ignore time for later recall via /ignore list.
func AddIgnoredIP(ignorerIPID, ignorerUsername, ignoredIPID, showname, oocName string) error {
	if db == nil {
		return nil
	}
	_, err := db.Exec(
		"INSERT OR IGNORE INTO IGNORED_IPS(IGNORER_IPID, IGNORER_USERNAME, IGNORED_IPID, IGNORED_SHOWNAME, IGNORED_OOC) VALUES(?, ?, ?, ?, ?)",
		ignorerIPID, ignorerUsername, ignoredIPID, showname, oocName,
	)
	return err
}

// UpdateIgnoredLabel refreshes the stored showname and OOC name for an existing
// ignore row. Called when the ignored user reconnects and their names may have
// changed since they were first ignored.
func UpdateIgnoredLabel(ignorerIPID, ignorerUsername, ignoredIPID, showname, oocName string) error {
	if db == nil {
		return nil
	}
	if ignorerUsername != "" {
		_, err := db.Exec(
			"UPDATE IGNORED_IPS SET IGNORED_SHOWNAME = ?, IGNORED_OOC = ? WHERE (IGNORER_IPID = ? OR IGNORER_USERNAME = ?) AND IGNORED_IPID = ?",
			showname, oocName, ignorerIPID, ignorerUsername, ignoredIPID,
		)
		return err
	}
	_, err := db.Exec(
		"UPDATE IGNORED_IPS SET IGNORED_SHOWNAME = ?, IGNORED_OOC = ? WHERE IGNORER_IPID = ? AND IGNORED_IPID = ?",
		showname, oocName, ignorerIPID, ignoredIPID,
	)
	return err
}

// RemoveIgnoredIPByKnownIPID removes every ignore row that matches the given
// ignored IPID for this ignorer (matched by IPID or username so account-linked
// rows are cleaned up regardless of which connection added them).
func RemoveIgnoredIPByKnownIPID(ignorerIPID, ignorerUsername, ignoredIPID string) error {
	if db == nil {
		return nil
	}
	if ignorerUsername != "" {
		_, err := db.Exec(
			"DELETE FROM IGNORED_IPS WHERE (IGNORER_IPID = ? OR IGNORER_USERNAME = ?) AND IGNORED_IPID = ?",
			ignorerIPID, ignorerUsername, ignoredIPID,
		)
		return err
	}
	_, err := db.Exec("DELETE FROM IGNORED_IPS WHERE IGNORER_IPID = ? AND IGNORED_IPID = ?", ignorerIPID, ignoredIPID)
	return err
}

// LoadIgnoredIPIDs returns all IPIDs that ignorerIPID (or the linked account)
// has permanently ignored. Called when a client connects to pre-populate their
// in-memory ignore set. Results are deduplicated.
func LoadIgnoredIPIDs(ignorerIPID, ignorerUsername string) ([]string, error) {
	if db == nil {
		return nil, nil
	}
	var rows *sql.Rows
	var err error
	if ignorerUsername != "" {
		rows, err = db.Query(
			`SELECT DISTINCT IGNORED_IPID FROM IGNORED_IPS
			 WHERE IGNORER_IPID = ? OR IGNORER_USERNAME = ?`,
			ignorerIPID, ignorerUsername,
		)
	} else {
		rows, err = db.Query("SELECT IGNORED_IPID FROM IGNORED_IPS WHERE IGNORER_IPID = ?", ignorerIPID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ipids []string
	for rows.Next() {
		var ipid string
		if err := rows.Scan(&ipid); err != nil {
			return ipids, fmt.Errorf("LoadIgnoredIPIDs scan: %w", err)
		}
		ipids = append(ipids, ipid)
	}
	return ipids, rows.Err()
}

// LoadIgnoredList returns the full ordered ignore list for display purposes.
// Entries are ordered by rowid (insertion order) and deduplicated. The
// returned Position values are 1-based sequential display indices.
func LoadIgnoredList(ignorerIPID, ignorerUsername string) ([]IgnoreEntry, error) {
	if db == nil {
		return nil, nil
	}
	var rows *sql.Rows
	var err error
	if ignorerUsername != "" {
		rows, err = db.Query(
			`SELECT DISTINCT IGNORED_IPID, IGNORED_SHOWNAME, IGNORED_OOC FROM IGNORED_IPS
			 WHERE IGNORER_IPID = ? OR IGNORER_USERNAME = ?
			 ORDER BY rowid ASC`,
			ignorerIPID, ignorerUsername,
		)
	} else {
		rows, err = db.Query(
			"SELECT IGNORED_IPID, IGNORED_SHOWNAME, IGNORED_OOC FROM IGNORED_IPS WHERE IGNORER_IPID = ? ORDER BY rowid ASC",
			ignorerIPID,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []IgnoreEntry
	pos := 1
	for rows.Next() {
		var ipid, showname, ooc string
		if err := rows.Scan(&ipid, &showname, &ooc); err != nil {
			return list, fmt.Errorf("LoadIgnoredList scan: %w", err)
		}
		list = append(list, IgnoreEntry{Position: pos, IPID: ipid, Showname: showname, OOC: ooc})
		pos++
	}
	return list, rows.Err()
}

// BackfillIgnoreUsername updates IGNORER_USERNAME on existing ignore rows for
// ignorerIPID that were added before the account was linked. Called on login.
func BackfillIgnoreUsername(ignorerIPID, username string) error {
	if db == nil {
		return nil
	}
	_, err := db.Exec(
		"UPDATE IGNORED_IPS SET IGNORER_USERNAME = ? WHERE IGNORER_IPID = ? AND IGNORER_USERNAME = ''",
		username, ignorerIPID,
	)
	return err
}

// EnsureChipBalance ensures an IPID exists in the CHIPS table, creating it with 100 chips if absent.
// It is safe to call on every connect; it is a no-op for known IPIDs.
func EnsureChipBalance(ipid string) error {
	if db == nil {
		return nil
	}
	_, err := db.Exec("INSERT OR IGNORE INTO CHIPS(IPID, BALANCE) VALUES(?, ?)", ipid, defaultChipBalance)
	return err
}

// GetChipBalance returns the current chip balance for an IPID.
// Returns 0, nil if the IPID is not found (EnsureChipBalance has not been called yet).
func GetChipBalance(ipid string) (int64, error) {
	if db == nil {
		return 0, nil
	}
	// Check the cache first.
	if bal, ok := chipCacheGet(ipid); ok {
		return bal, nil
	}
	row := db.QueryRow("SELECT BALANCE FROM CHIPS WHERE IPID = ?", ipid)
	var balance int64
	if err := row.Scan(&balance); err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, err
	}
	chipCacheSet(ipid, balance)
	return balance, nil
}

// AddChips adds the given amount to an IPID's chip balance and returns the new balance.
// The amount must be positive. EnsureChipBalance must be called first.
// The resulting balance is capped at MaxChipBalance to prevent runaway inflation.
func AddChips(ipid string, amount int64) (int64, error) {
	if db == nil {
		return 0, nil
	}
	if amount <= 0 {
		return 0, fmt.Errorf("amount must be positive")
	}
	var newBalance int64
	err := db.QueryRow(
		"UPDATE CHIPS SET BALANCE = MIN(BALANCE + ?, ?) WHERE IPID = ? RETURNING BALANCE",
		amount, MaxChipBalance, ipid).Scan(&newBalance)
	if err != nil {
		return 0, err
	}
	chipCacheSet(ipid, newBalance)
	return newBalance, nil
}

// SpendChips deducts the given amount from an IPID's chip balance, returning the new balance.
// Returns an error if the balance would go below zero. On insufficient-funds errors the first
// return value is the player's current (unmodified) balance, so callers can display it without
// an extra round-trip.
func SpendChips(ipid string, amount int64) (int64, error) {
	if db == nil {
		return 0, nil
	}
	if amount <= 0 {
		return 0, fmt.Errorf("amount must be positive")
	}
	var newBalance int64
	err := db.QueryRow(
		"UPDATE CHIPS SET BALANCE = BALANCE - ? WHERE IPID = ? AND BALANCE >= ? RETURNING BALANCE",
		amount, ipid, amount).Scan(&newBalance)
	if err == sql.ErrNoRows {
		// Fetch the actual balance so callers can show it without an extra query.
		var cur int64
		_ = db.QueryRow("SELECT BALANCE FROM CHIPS WHERE IPID = ?", ipid).Scan(&cur)
		chipCacheInvalidate(ipid) // ensure stale cached value doesn't linger
		return cur, fmt.Errorf("insufficient chips")
	}
	if err != nil {
		return 0, err
	}
	chipCacheSet(ipid, newBalance)
	return newBalance, nil
}

// GetChipBalancesByIPIDs returns a map of IPID → chip balance for all given IPIDs
// in a single query.  IPIDs with no CHIPS row are absent from the result map.
func GetChipBalancesByIPIDs(ipids []string) (map[string]int64, error) {
	if db == nil || len(ipids) == 0 {
		return map[string]int64{}, nil
	}
	placeholders := make([]string, len(ipids))
	args := make([]any, len(ipids))
	for i, id := range ipids {
		placeholders[i] = "?"
		args[i] = id
	}
	rows, err := db.Query(
		"SELECT IPID, BALANCE FROM CHIPS WHERE IPID IN ("+strings.Join(placeholders, ",")+")",
		args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := make(map[string]int64, len(ipids))
	for rows.Next() {
		var ipid string
		var balance int64
		if err := rows.Scan(&ipid, &balance); err != nil {
			return m, err
		}
		m[ipid] = balance
	}
	return m, rows.Err()
}

// GetUsernamesByIPIDs returns a map of IPID → username for all given IPIDs that
// have a linked account. Unlisted IPIDs are simply absent from the result map.
// This batches N individual GetUsernameByIPID calls into a single query.
func GetUsernamesByIPIDs(ipids []string) (map[string]string, error) {
	if db == nil || len(ipids) == 0 {
		return map[string]string{}, nil
	}
	placeholders := make([]string, len(ipids))
	args := make([]any, len(ipids))
	for i, id := range ipids {
		placeholders[i] = "?"
		args[i] = id
	}
	rows, err := db.Query(
		"SELECT IPID, USERNAME FROM USERS WHERE IPID IN ("+strings.Join(placeholders, ",")+")",
		args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := make(map[string]string, len(ipids))
	for rows.Next() {
		var ipid, username string
		if err := rows.Scan(&ipid, &username); err != nil {
			return m, err
		}
		m[ipid] = username
	}
	return m, rows.Err()
}

// GetTopChipBalances returns the top n registered players by chip balance, ordered descending.
// Only players with a linked account are included; anonymous IPIDs are excluded.
func GetTopChipBalances(n int) ([]ChipEntry, error) {
	if db == nil {
		return nil, nil
	}
	rows, err := db.Query(`
		SELECT u.USERNAME, c.BALANCE
		FROM CHIPS c
		INNER JOIN USERS u ON u.IPID = c.IPID
		ORDER BY c.BALANCE DESC LIMIT ?`, n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := make([]ChipEntry, 0, n)
	for rows.Next() {
		var e ChipEntry
		if err := rows.Scan(&e.Username, &e.Balance); err != nil {
			return entries, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// GetTopPlaytimes returns the top n registered players by accumulated playtime, ordered
// descending. Only players with a linked account are included; anonymous IPIDs are excluded.
// The IPID is returned alongside each entry for live-session merging but is never displayed.
// Includes both player and moderator accounts (shadow mods too) — there is no permission
// filter; any USERS row with a linked IPID and non-zero playtime is eligible.
func GetTopPlaytimes(n int) ([]PlaytimeEntry, error) {
	return GetTopPlaytimesPaged(n, 0)
}

// GetTopPlaytimesPaged returns up to n entries starting at the given offset.
// Used to back the paginated /playtime top <page> command.
func GetTopPlaytimesPaged(n, offset int) ([]PlaytimeEntry, error) {
	if db == nil {
		return nil, nil
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := db.Query(`
		SELECT k.IPID, u.USERNAME, k.PLAYTIME
		FROM KNOWN_IPS k
		INNER JOIN USERS u ON u.IPID = k.IPID
		WHERE k.PLAYTIME > 0
		ORDER BY k.PLAYTIME DESC
		LIMIT ? OFFSET ?`, n, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := make([]PlaytimeEntry, 0, n)
	for rows.Next() {
		var e PlaytimeEntry
		if err := rows.Scan(&e.Ipid, &e.Username, &e.Playtime); err != nil {
			return entries, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// CountPlaytimeEntries returns the total number of accounts that would appear
// in the playtime leaderboard. Used to compute the last page number for
// /playtime top <page>.
func CountPlaytimeEntries() (int, error) {
	if db == nil {
		return 0, nil
	}
	var n int
	err := db.QueryRow(`
		SELECT COUNT(*)
		FROM KNOWN_IPS k
		INNER JOIN USERS u ON u.IPID = k.IPID
		WHERE k.PLAYTIME > 0`).Scan(&n)
	return n, err
}

// AllRegisteredIPIDs returns every (username, ipid, current_playtime) triple
// for registered accounts whose IPID is set. Used by /reloadplaytime to
// re-merge any orphaned KNOWN_IPS playtime into the linked account.
func AllRegisteredIPIDs() ([]struct {
	Username string
	IPID     string
	Playtime int64
}, error) {
	if db == nil {
		return nil, nil
	}
	rows, err := db.Query(`
		SELECT u.USERNAME, u.IPID, COALESCE(k.PLAYTIME, 0)
		FROM USERS u
		LEFT JOIN KNOWN_IPS k ON k.IPID = u.IPID
		WHERE u.IPID != ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []struct {
		Username string
		IPID     string
		Playtime int64
	}
	for rows.Next() {
		var r struct {
			Username string
			IPID     string
			Playtime int64
		}
		if err := rows.Scan(&r.Username, &r.IPID, &r.Playtime); err != nil {
			return out, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// UnscrambleEntry holds one row from the UNSCRAMBLE_WINS leaderboard.
type UnscrambleEntry struct {
	// Username is the registered account name linked to the IPID, if any.
	// Falls back to an empty string when the player has no account.
	Username string
	// IPID is the player's connection fingerprint, used as a display-name
	// fallback when Username is empty.
	IPID string
	// Wins is the total number of unscramble puzzles solved.
	Wins int64
}

// AddUnscrambleWin increments the win counter for the given IPID by 1.
func AddUnscrambleWin(ipid string) error {
if db == nil {
return nil
}
_, err := db.Exec(`
INSERT INTO UNSCRAMBLE_WINS(IPID, WINS) VALUES(?, 1)
ON CONFLICT(IPID) DO UPDATE SET WINS = WINS + 1`, ipid)
return err
}

// GetUnscrambleWins returns the total unscramble wins for the given IPID.
func GetUnscrambleWins(ipid string) (int64, error) {
if db == nil {
return 0, nil
}
var wins int64
err := db.QueryRow("SELECT WINS FROM UNSCRAMBLE_WINS WHERE IPID = ?", ipid).Scan(&wins)
if err == sql.ErrNoRows {
return 0, nil
}
return wins, err
}

// GetTopUnscrambleWins returns the top n players by unscramble wins.
// Players without a linked account fall back to their IPID as the display name.
func GetTopUnscrambleWins(n int) ([]UnscrambleEntry, error) {
if db == nil {
return nil, nil
}
rows, err := db.Query(`
SELECT w.IPID, COALESCE(u.USERNAME, '') AS USERNAME, w.WINS
FROM UNSCRAMBLE_WINS w
LEFT JOIN USERS u ON u.IPID = w.IPID
ORDER BY w.WINS DESC LIMIT ?`, n)
if err != nil {
return nil, err
}
defer rows.Close()
entries := make([]UnscrambleEntry, 0, n)
for rows.Next() {
var e UnscrambleEntry
if err := rows.Scan(&e.IPID, &e.Username, &e.Wins); err != nil {
return entries, err
}
entries = append(entries, e)
}
return entries, rows.Err()
}

// CheckAndSetJobCooldown checks whether the given job is on cooldown for the
// IPID. If it is not on cooldown, the last-use timestamp is updated atomically
// and the function returns (false, 0). If it is on cooldown, it returns
// (true, secondsRemaining) without modifying the database.
func CheckAndSetJobCooldown(ipid, job string, cooldownSeconds int64) (onCooldown bool, remaining int64, err error) {
if db == nil {
return false, 0, nil
}
now := time.Now().UTC().Unix()
var lastAt int64
qErr := db.QueryRow("SELECT LAST_AT FROM JOB_COOLDOWNS WHERE IPID = ? AND JOB = ?", ipid, job).Scan(&lastAt)
if qErr != nil && qErr != sql.ErrNoRows {
return false, 0, qErr
}
if qErr == nil {
rem := cooldownSeconds - (now - lastAt)
if rem > 0 {
return true, rem, nil
}
}
_, err = db.Exec(`
INSERT INTO JOB_COOLDOWNS(IPID, JOB, LAST_AT) VALUES(?, ?, ?)
ON CONFLICT(IPID, JOB) DO UPDATE SET LAST_AT = excluded.LAST_AT`, ipid, job, now)
return false, 0, err
}

// JobEarningsEntry holds one row from the job earnings leaderboard query.
type JobEarningsEntry struct {
	// Username is the registered account name, or empty for anonymous players.
	Username string
	// IPID is the player's connection fingerprint, used as a display-name
	// fallback when Username is empty.
	IPID string
	// Total is the cumulative chips earned from jobs.
	Total int64
}

// AddJobEarnings increments the total job-earnings counter for the given IPID.
func AddJobEarnings(ipid string, amount int64) error {
	if db == nil {
		return nil
	}
	_, err := db.Exec(`
INSERT INTO JOB_EARNINGS(IPID, TOTAL) VALUES(?, ?)
ON CONFLICT(IPID) DO UPDATE SET TOTAL = TOTAL + excluded.TOTAL`, ipid, amount)
	return err
}

// GetTopJobEarnings returns the top n players by cumulative job-earned chips.
// Players without a linked account fall back to their IPID as the display name.
func GetTopJobEarnings(n int) ([]JobEarningsEntry, error) {
	if db == nil {
		return nil, nil
	}
	rows, err := db.Query(`
SELECT j.IPID, COALESCE(u.USERNAME, '') AS USERNAME, j.TOTAL
FROM JOB_EARNINGS j
LEFT JOIN USERS u ON u.IPID = j.IPID
ORDER BY j.TOTAL DESC LIMIT ?`, n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := make([]JobEarningsEntry, 0, n)
	for rows.Next() {
		var e JobEarningsEntry
		if err := rows.Scan(&e.IPID, &e.Username, &e.Total); err != nil {
			return entries, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// ── Shop / Tags ───────────────────────────────────────────────────────────────

// PurchaseShopItem atomically deducts cost chips from ipid and records the
// purchase. Returns an error if the player has insufficient funds, or if they
// already own the item.
func PurchaseShopItem(ipid, itemID string, cost int64) error {
if db == nil {
return fmt.Errorf("database unavailable")
}
tx, err := db.Begin()
if err != nil {
return err
}
defer tx.Rollback() //nolint:errcheck

// Check current balance.
var balance int64
if err := tx.QueryRow("SELECT BALANCE FROM CHIPS WHERE IPID = ?", ipid).Scan(&balance); err != nil {
return fmt.Errorf("could not read balance")
}
if balance < cost {
return fmt.Errorf("insufficient chips (have %d, need %d)", balance, cost)
}

// Deduct cost.
if _, err := tx.Exec("UPDATE CHIPS SET BALANCE = BALANCE - ? WHERE IPID = ?", cost, ipid); err != nil {
return err
}

// Record purchase — IGNORE if already owned (caller should check HasShopItem first).
res, err := tx.Exec("INSERT OR IGNORE INTO SHOP_PURCHASES(IPID, ITEM_ID) VALUES(?, ?)", ipid, itemID)
if err != nil {
return err
}
affected, _ := res.RowsAffected()
if affected == 0 {
// Item was already owned — rollback so chips are not deducted.
_ = tx.Rollback()
return fmt.Errorf("already owned")
}

return tx.Commit()
}

// HasShopItem returns true when ipid has purchased itemID.
func HasShopItem(ipid, itemID string) bool {
if db == nil {
return false
}
var count int
db.QueryRow("SELECT COUNT(*) FROM SHOP_PURCHASES WHERE IPID = ? AND ITEM_ID = ?", ipid, itemID).Scan(&count) //nolint:errcheck
return count > 0
}

// GetPlayerShopItems returns all item IDs purchased by ipid.
func GetPlayerShopItems(ipid string) ([]string, error) {
if db == nil {
return nil, nil
}
rows, err := db.Query("SELECT ITEM_ID FROM SHOP_PURCHASES WHERE IPID = ?", ipid)
if err != nil {
return nil, err
}
defer rows.Close()
var items []string
for rows.Next() {
var id string
if err := rows.Scan(&id); err != nil {
return items, err
}
items = append(items, id)
}
return items, rows.Err()
}

// SetActiveTag stores the player's chosen active tag.  Pass an empty string to
// clear the tag.
func SetActiveTag(ipid, tagID string) error {
if db == nil {
return nil
}
_, err := db.Exec(`
INSERT INTO PLAYER_ACTIVE_TAG(IPID, TAG_ID) VALUES(?, ?)
ON CONFLICT(IPID) DO UPDATE SET TAG_ID = excluded.TAG_ID`, ipid, tagID)
return err
}

// GetActiveTag returns the player's active tag ID, or "" if none is set.
func GetActiveTag(ipid string) string {
if db == nil {
return ""
}
var tagID string
db.QueryRow("SELECT TAG_ID FROM PLAYER_ACTIVE_TAG WHERE IPID = ?", ipid).Scan(&tagID) //nolint:errcheck
return tagID
}

// SetAccountActiveTag stores the active tag on the user's account so it survives
// IPID changes and is restored automatically on every future login.
// Pass an empty string to clear the stored tag.
func SetAccountActiveTag(username, tagID string) error {
	if db == nil {
		return nil
	}
	_, err := db.Exec("UPDATE USERS SET ACTIVE_TAG = ? WHERE USERNAME = ?", tagID, username)
	return err
}

// GetAccountActiveTag returns the active tag stored on the user's account, or ""
// if no tag has been saved.
func GetAccountActiveTag(username string) string {
	if db == nil {
		return ""
	}
	var tagID string
	db.QueryRow("SELECT COALESCE(ACTIVE_TAG, '') FROM USERS WHERE USERNAME = ?", username).Scan(&tagID) //nolint:errcheck
	return tagID
}

// CustomTag describes one row in CUSTOM_TAGS.
type CustomTag struct {
	ID        string
	Name      string
	CreatedBy string
	CreatedAt int64
}

// CreateCustomTag inserts a new staff-defined tag. Returns an error if the ID
// is already present in CUSTOM_TAGS — callers should also reject IDs that
// collide with built-in shop tags before invoking this.
func CreateCustomTag(id, name, createdBy string) error {
	if db == nil {
		return fmt.Errorf("database not open")
	}
	_, err := db.Exec(
		"INSERT INTO CUSTOM_TAGS(ID, NAME, CREATED_BY, CREATED_AT) VALUES(?, ?, ?, ?)",
		id, name, createdBy, time.Now().Unix(),
	)
	return err
}

// DeleteCustomTag removes a custom tag definition and any references to it:
// purchase rows in SHOP_PURCHASES and any active equip pointers that are
// pointing at this id are cleared. Returns an error if the tag does not exist.
func DeleteCustomTag(id string) error {
	if db == nil {
		return fmt.Errorf("database not open")
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	res, err := tx.Exec("DELETE FROM CUSTOM_TAGS WHERE ID = ?", id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("no custom tag with id %q", id)
	}
	if _, err := tx.Exec("DELETE FROM SHOP_PURCHASES WHERE ITEM_ID = ?", id); err != nil {
		return err
	}
	if _, err := tx.Exec("UPDATE PLAYER_ACTIVE_TAG SET TAG_ID = '' WHERE TAG_ID = ?", id); err != nil {
		return err
	}
	return tx.Commit()
}

// GetCustomTag returns the (name, true) for a custom tag id, or ("", false)
// when the id does not refer to a custom tag. Used by the shop layer to
// resolve display names for ids not present in the built-in catalog.
func GetCustomTag(id string) (string, bool) {
	if db == nil {
		return "", false
	}
	var name string
	err := db.QueryRow("SELECT NAME FROM CUSTOM_TAGS WHERE ID = ?", id).Scan(&name)
	if err != nil {
		return "", false
	}
	return name, true
}

// ListCustomTags returns every custom tag ordered by creation time (oldest first).
func ListCustomTags() ([]CustomTag, error) {
	if db == nil {
		return nil, nil
	}
	rows, err := db.Query("SELECT ID, NAME, CREATED_BY, CREATED_AT FROM CUSTOM_TAGS ORDER BY CREATED_AT ASC, ID ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CustomTag
	for rows.Next() {
		var t CustomTag
		if err := rows.Scan(&t.ID, &t.Name, &t.CreatedBy, &t.CreatedAt); err != nil {
			return out, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// GrantShopItem records that ipid owns itemID without spending any chips.
// Used by staff grants (e.g. /grantcustomtag); idempotent — returns nil if the
// player already owns the item.
func GrantShopItem(ipid, itemID string) error {
	if db == nil {
		return fmt.Errorf("database not open")
	}
	_, err := db.Exec(
		"INSERT OR IGNORE INTO SHOP_PURCHASES(IPID, ITEM_ID) VALUES(?, ?)",
		ipid, itemID,
	)
	return err
}

// RevokeShopItem removes ownership of itemID from ipid and clears the item as
// the active tag if it was equipped. Returns an error if the player did not
// own the item.
func RevokeShopItem(ipid, itemID string) error {
	if db == nil {
		return fmt.Errorf("database not open")
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	res, err := tx.Exec("DELETE FROM SHOP_PURCHASES WHERE IPID = ? AND ITEM_ID = ?", ipid, itemID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("player does not own that item")
	}
	if _, err := tx.Exec(
		"UPDATE PLAYER_ACTIVE_TAG SET TAG_ID = '' WHERE IPID = ? AND TAG_ID = ?",
		ipid, itemID,
	); err != nil {
		return err
	}
	return tx.Commit()
}

// GetIPIDByUsername returns the IPID linked to a player account, or "" if the
// account does not exist or has no IPID linked yet. Counterpart to
// GetUsernameByIPID.
func GetIPIDByUsername(username string) (string, error) {
	if db == nil {
		return "", nil
	}
	var ipid string
	err := db.QueryRow("SELECT IPID FROM USERS WHERE USERNAME = ?", username).Scan(&ipid)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return ipid, nil
}

// AddFavourite adds a character to the player's wardrobe favourites.
// Returns ErrFavouriteLimitReached when the cap is hit; a UNIQUE-constraint
// error if the character is already saved; or nil on success.
// The limit check and insert are performed in a single atomic statement,
// eliminating the TOCTOU window present in a separate SELECT + INSERT pair.
//
// Row-affected semantics:
//   - rows=1, err=nil  → inserted successfully.
//   - rows=0, err=nil  → WHERE (count < limit) was false; limit reached.
//   - rows=0, err!=nil → UNIQUE constraint violation (duplicate) or other DB error.
//
// Only the limit-reached path (rows=0, err=nil) is unambiguous: a UNIQUE
// violation always produces a non-nil error, never silently zero rows.
func AddFavourite(username, charName string) error {
	if db == nil {
		return nil
	}
	res, err := db.Exec(`
		INSERT INTO FAVOURITES(USERNAME, CHAR_NAME, ADDED_AT)
		SELECT ?, ?, ?
		WHERE (SELECT COUNT(*) FROM FAVOURITES WHERE USERNAME = ?) < ?`,
		username, charName, time.Now().Unix(), username, MaxFavourites,
	)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrFavouriteLimitReached
	}
	return nil
}

// RemoveFavourite removes a character from the player's wardrobe favourites.
func RemoveFavourite(username, charName string) error {
	if db == nil {
		return nil
	}
	_, err := db.Exec("DELETE FROM FAVOURITES WHERE USERNAME = ? AND CHAR_NAME = ?", username, charName)
	return err
}

// GetFavourites returns all favourite character names for the given username,
// ordered by the time they were added. The returned slice is pre-allocated to
// MaxFavourites capacity to avoid incremental re-allocations.
func GetFavourites(username string) ([]string, error) {
	if db == nil {
		return nil, nil
	}
	rows, err := db.Query(
		"SELECT CHAR_NAME FROM FAVOURITES WHERE USERNAME = ? ORDER BY ADDED_AT ASC",
		username,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	chars := make([]string, 0, MaxFavourites)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return chars, err
		}
		chars = append(chars, name)
	}
	return chars, rows.Err()
}

// IsFavourite returns true if charName is in the player's favourites list.
func IsFavourite(username, charName string) (bool, error) {
	if db == nil {
		return false, nil
	}
	var count int
	err := db.QueryRow(
		"SELECT COUNT(*) FROM FAVOURITES WHERE USERNAME = ? AND CHAR_NAME = ?",
		username, charName,
	).Scan(&count)
	return count > 0, err
}

// GetGambleHide returns whether the user has opted out of gambling broadcast messages.
// Returns false if the user does not exist or the database is unavailable.
func GetGambleHide(username string) (bool, error) {
	if db == nil {
		return false, nil
	}
	var hide int
	err := db.QueryRow("SELECT COALESCE(GAMBLE_HIDE, 0) FROM USERS WHERE USERNAME = ?", username).Scan(&hide)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return hide != 0, nil
}

// SetGambleHide persists the gamble-hide preference for the given user account.
func SetGambleHide(username string, hide bool) error {
	if db == nil {
		return nil
	}
	val := 0
	if hide {
		val = 1
	}
	_, err := db.Exec("UPDATE USERS SET GAMBLE_HIDE = ? WHERE USERNAME = ?", val, username)
	return err
}

// ModnoteEntry holds a single moderator note retrieved from the database.
type ModnoteEntry struct {
	ID      int64
	IPID    string
	Note    string
	AddedBy string
	AddedAt int64
}

// AddModnote inserts a new moderator note for the given IPID.
func AddModnote(ipid, note, addedBy string) error {
	if db == nil {
		return nil
	}
	_, err := db.Exec(
		"INSERT INTO MODNOTES(IPID, NOTE, ADDED_BY, ADDED_AT) VALUES(?, ?, ?, ?)",
		ipid, note, addedBy, time.Now().UTC().Unix(),
	)
	return err
}

// GetModnotes returns all moderator notes for the given IPID, ordered oldest first.
func GetModnotes(ipid string) ([]ModnoteEntry, error) {
	if db == nil {
		return nil, nil
	}
	rows, err := db.Query(
		"SELECT ID, IPID, NOTE, ADDED_BY, ADDED_AT FROM MODNOTES WHERE IPID = ? ORDER BY ADDED_AT ASC",
		ipid,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var entries []ModnoteEntry
	for rows.Next() {
		var e ModnoteEntry
		if err := rows.Scan(&e.ID, &e.IPID, &e.Note, &e.AddedBy, &e.AddedAt); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// DeleteModnote removes a single moderator note by its numeric ID.
// It returns sql.ErrNoRows if no note with that ID exists.
func DeleteModnote(id int64) error {
	if db == nil {
		return nil
	}
	res, err := db.Exec("DELETE FROM MODNOTES WHERE ID = ?", id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// MusicBanInfo is the persistent record of a /musicban entry. Each IPID has at
// most one row (the table is keyed by IPID with INSERT OR REPLACE upsert).
type MusicBanInfo struct {
	Ipid     string
	Reason   string
	BannedBy string
	BannedAt int64
}

// AddMusicBan upserts a /musicban for the given IPID. Re-banning an already-
// banned IPID overwrites the existing reason/issuer/timestamp rather than
// creating a duplicate row.
func AddMusicBan(ipid, reason, bannedBy string, bannedAt int64) error {
	if db == nil {
		return nil
	}
	_, err := db.Exec("INSERT OR REPLACE INTO MUSIC_BANS(IPID, REASON, BANNED_BY, BANNED_AT) VALUES(?, ?, ?, ?)",
		ipid, reason, bannedBy, bannedAt)
	return err
}

// RemoveMusicBan deletes the music-ban row for the given IPID. Returns
// sql.ErrNoRows if no such ban existed so callers can distinguish "unbanned a
// real ban" from "no ban was present".
func RemoveMusicBan(ipid string) error {
	if db == nil {
		return nil
	}
	res, err := db.Exec("DELETE FROM MUSIC_BANS WHERE IPID = ?", ipid)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// IsMusicBanned returns whether the given IPID is currently music-banned.
// The hot path; called from the MC packet handler on every music change.
func IsMusicBanned(ipid string) (bool, error) {
	if db == nil {
		return false, nil
	}
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM MUSIC_BANS WHERE IPID = ?", ipid).Scan(&n); err != nil {
		return false, err
	}
	return n > 0, nil
}

// GetMusicBan returns the full record for one IPID's music-ban, or
// (MusicBanInfo{}, sql.ErrNoRows) if none exists. Used by the moderator-facing
// /musicban inspect path.
func GetMusicBan(ipid string) (MusicBanInfo, error) {
	if db == nil {
		return MusicBanInfo{}, sql.ErrNoRows
	}
	var mb MusicBanInfo
	mb.Ipid = ipid
	err := db.QueryRow("SELECT REASON, BANNED_BY, BANNED_AT FROM MUSIC_BANS WHERE IPID = ?", ipid).
		Scan(&mb.Reason, &mb.BannedBy, &mb.BannedAt)
	return mb, err
}

// ListMusicBans returns every active music-ban, newest-first by BANNED_AT.
// Used by /musicbans (the listing command).
func ListMusicBans() ([]MusicBanInfo, error) {
	if db == nil {
		return nil, nil
	}
	rows, err := db.Query("SELECT IPID, REASON, BANNED_BY, BANNED_AT FROM MUSIC_BANS ORDER BY BANNED_AT DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MusicBanInfo
	for rows.Next() {
		var mb MusicBanInfo
		if err := rows.Scan(&mb.Ipid, &mb.Reason, &mb.BannedBy, &mb.BannedAt); err != nil {
			return nil, err
		}
		out = append(out, mb)
	}
	return out, rows.Err()
}

// AddRandomCharCurse upserts a /curserandomchar curse for the given IPID.
// Re-cursing an already-cursed IPID overwrites the issuer/timestamp rather
// than creating a duplicate row.
func AddRandomCharCurse(ipid, issuedBy string, issuedAt int64) error {
	if db == nil {
		return nil
	}
	_, err := db.Exec("INSERT OR REPLACE INTO RANDOMCHAR_CURSES(IPID, ISSUED_BY, ISSUED_AT) VALUES(?, ?, ?)",
		ipid, issuedBy, issuedAt)
	return err
}

// RemoveRandomCharCurse deletes the /curserandomchar curse row for the given
// IPID. Returns sql.ErrNoRows if no such curse existed so callers can
// distinguish "lifted a real curse" from "no curse was present".
func RemoveRandomCharCurse(ipid string) error {
	if db == nil {
		return nil
	}
	res, err := db.Exec("DELETE FROM RANDOMCHAR_CURSES WHERE IPID = ?", ipid)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// IsRandomCharCursed reports whether the given IPID currently carries an
// active /curserandomchar curse. Checked once per client on join so the
// curse re-arms itself after a reconnect — the whole point of persisting it
// by IPID is that relogging cannot be used to escape it.
func IsRandomCharCursed(ipid string) (bool, error) {
	if db == nil {
		return false, nil
	}
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM RANDOMCHAR_CURSES WHERE IPID = ?", ipid).Scan(&n); err != nil {
		return false, err
	}
	return n > 0, nil
}

// ShadowDisconnectInfo describes one /shadowdisconnect entry.
type ShadowDisconnectInfo struct {
	Ipid     string
	Reason   string
	IssuedBy string
	IssuedAt int64
}

// AddShadowDisconnect upserts a /shadowdisconnect entry for the given IPID.
// Re-issuing against an already-listed IPID overwrites the reason/issuer/
// timestamp rather than creating a duplicate row.
func AddShadowDisconnect(ipid, reason, issuedBy string, issuedAt int64) error {
	if db == nil {
		return nil
	}
	_, err := db.Exec("INSERT OR REPLACE INTO SHADOW_DISCONNECTS(IPID, REASON, ISSUED_BY, ISSUED_AT) VALUES(?, ?, ?, ?)",
		ipid, reason, issuedBy, issuedAt)
	return err
}

// RemoveShadowDisconnect deletes the /shadowdisconnect entry for the given
// IPID. Returns sql.ErrNoRows if no such entry existed so callers can
// distinguish "lifted a real listing" from "no listing was present".
func RemoveShadowDisconnect(ipid string) error {
	if db == nil {
		return nil
	}
	res, err := db.Exec("DELETE FROM SHADOW_DISCONNECTS WHERE IPID = ?", ipid)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// ClearAllShadowDisconnects deletes every /shadowdisconnect entry, for the
// /shadowundisconnect all bulk-lift form. Returns the number of entries
// removed.
func ClearAllShadowDisconnects() (int64, error) {
	if db == nil {
		return 0, nil
	}
	res, err := db.Exec("DELETE FROM SHADOW_DISCONNECTS")
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// IsShadowDisconnected reports whether the given IPID currently carries an
// active /shadowdisconnect listing.
func IsShadowDisconnected(ipid string) (bool, error) {
	if db == nil {
		return false, nil
	}
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM SHADOW_DISCONNECTS WHERE IPID = ?", ipid).Scan(&n); err != nil {
		return false, err
	}
	return n > 0, nil
}

// ListShadowDisconnects returns every active /shadowdisconnect entry,
// newest-first by ISSUED_AT. Used by /shadowdisconnectlist.
func ListShadowDisconnects() ([]ShadowDisconnectInfo, error) {
	if db == nil {
		return nil, nil
	}
	rows, err := db.Query("SELECT IPID, REASON, ISSUED_BY, ISSUED_AT FROM SHADOW_DISCONNECTS ORDER BY ISSUED_AT DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ShadowDisconnectInfo
	for rows.Next() {
		var sd ShadowDisconnectInfo
		if err := rows.Scan(&sd.Ipid, &sd.Reason, &sd.IssuedBy, &sd.IssuedAt); err != nil {
			return nil, err
		}
		out = append(out, sd)
	}
	return out, rows.Err()
}

// LockdownExemptInfo describes one verified /lockdown whitelist passkey entry.
type LockdownExemptInfo struct {
	Ipid     string
	IssuedBy string
	IssuedAt int64
}

// AddLockdownExempt upserts a permanent lockdown-purge exemption for the
// given IPID, granted after /lockdown whitelist verifies a passkey for it.
// Re-verifying the same IPID's passkey overwrites the issuer/timestamp
// rather than creating a duplicate row.
func AddLockdownExempt(ipid, issuedBy string, issuedAt int64) error {
	if db == nil {
		return nil
	}
	_, err := db.Exec("INSERT OR REPLACE INTO LOCKDOWN_EXEMPT(IPID, ISSUED_BY, ISSUED_AT) VALUES(?, ?, ?)",
		ipid, issuedBy, issuedAt)
	return err
}

// RemoveLockdownExempt deletes the lockdown-purge exemption for the given
// IPID. Returns sql.ErrNoRows if no such exemption existed so callers can
// distinguish "removed a real exemption" from "none was present".
func RemoveLockdownExempt(ipid string) error {
	if db == nil {
		return nil
	}
	res, err := db.Exec("DELETE FROM LOCKDOWN_EXEMPT WHERE IPID = ?", ipid)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// ListLockdownExempts returns every permanently-exempted IPID, newest-first
// by ISSUED_AT. Used by /lockdown exemptlist and to seed the in-memory set
// at startup.
func ListLockdownExempts() ([]LockdownExemptInfo, error) {
	if db == nil {
		return nil, nil
	}
	rows, err := db.Query("SELECT IPID, ISSUED_BY, ISSUED_AT FROM LOCKDOWN_EXEMPT ORDER BY ISSUED_AT DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LockdownExemptInfo
	for rows.Next() {
		var le LockdownExemptInfo
		if err := rows.Scan(&le.Ipid, &le.IssuedBy, &le.IssuedAt); err != nil {
			return nil, err
		}
		out = append(out, le)
	}
	return out, rows.Err()
}

// JoinCaptchaInfo describes one IPID that has solved the join captcha.
type JoinCaptchaInfo struct {
	Ipid       string
	VerifiedAt int64
	Kind       string // challenge kind solved, kept for staff diagnostics
}

// AddJoinCaptchaVerified records that an IPID solved the join captcha.
// Idempotent: re-solving (after /joincaptcha reset, say) overwrites the timestamp.
func AddJoinCaptchaVerified(ipid string, verifiedAt int64, kind string) error {
	if db == nil {
		return nil
	}
	_, err := db.Exec("INSERT OR REPLACE INTO JOIN_CAPTCHA_VERIFIED(IPID, VERIFIED_AT, KIND) VALUES(?, ?, ?)",
		ipid, verifiedAt, kind)
	return err
}

// RemoveJoinCaptchaVerified clears an IPID's join-captcha verification, so it
// is challenged again on its next connection. Returns sql.ErrNoRows when the
// IPID was not verified, so callers can distinguish "cleared something" from
// "there was nothing to clear".
func RemoveJoinCaptchaVerified(ipid string) error {
	if db == nil {
		return nil
	}
	res, err := db.Exec("DELETE FROM JOIN_CAPTCHA_VERIFIED WHERE IPID = ?", ipid)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// ClearJoinCaptchaVerified drops every join-captcha verification, forcing all
// returning players to solve a fresh challenge. Returns the number of rows
// removed. Used by /joincaptcha reset all when an operator wants to re-gate the
// whole known population mid-incident.
func ClearJoinCaptchaVerified() (int64, error) {
	if db == nil {
		return 0, nil
	}
	res, err := db.Exec("DELETE FROM JOIN_CAPTCHA_VERIFIED")
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// LoadJoinCaptchaVerified returns every verified IPID, used to seed the
// in-memory set at startup so the hot-path check is a map lookup.
func LoadJoinCaptchaVerified() ([]string, error) {
	if db == nil {
		return nil, nil
	}
	rows, err := db.Query("SELECT IPID FROM JOIN_CAPTCHA_VERIFIED")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var ipid string
		if err := rows.Scan(&ipid); err != nil {
			return nil, err
		}
		out = append(out, ipid)
	}
	return out, rows.Err()
}

// CommandGrantInfo is one row of ACCOUNT_COMMAND_GRANTS: a single command an
// operator with server-console access has explicitly granted to one account,
// independent of that account's role/permission bitfield. Command is always
// stored lowercase (matching the athena.Commands registry key); Username is
// stored exactly as given, matching the case-sensitive USERS primary key.
type CommandGrantInfo struct {
	Username  string
	Command   string
	GrantedBy string
	GrantedAt int64
}

// AddCommandGrant upserts a console-issued command grant for username.
// Re-granting an already-granted command overwrites the issuer/timestamp
// rather than creating a duplicate row.
func AddCommandGrant(username, command, grantedBy string, grantedAt int64) error {
	if db == nil {
		return nil
	}
	_, err := db.Exec("INSERT OR REPLACE INTO ACCOUNT_COMMAND_GRANTS(USERNAME, COMMAND, GRANTED_BY, GRANTED_AT) VALUES(?, ?, ?, ?)",
		username, command, grantedBy, grantedAt)
	return err
}

// RemoveCommandGrant deletes one command grant. Returns sql.ErrNoRows if no
// such grant existed so callers can distinguish "revoked a real grant" from
// "there was nothing to revoke".
func RemoveCommandGrant(username, command string) error {
	if db == nil {
		return nil
	}
	res, err := db.Exec("DELETE FROM ACCOUNT_COMMAND_GRANTS WHERE USERNAME = ? AND COMMAND = ?", username, command)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// RemoveAllCommandGrants deletes every command grant held by username,
// returning how many were removed. Used by "revokecmd <username> all".
func RemoveAllCommandGrants(username string) (int64, error) {
	if db == nil {
		return 0, nil
	}
	res, err := db.Exec("DELETE FROM ACCOUNT_COMMAND_GRANTS WHERE USERNAME = ?", username)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ListCommandGrants returns every command granted to one account, oldest
// first. Used by the console "grants <username>" listing.
func ListCommandGrants(username string) ([]CommandGrantInfo, error) {
	if db == nil {
		return nil, nil
	}
	rows, err := db.Query("SELECT USERNAME, COMMAND, GRANTED_BY, GRANTED_AT FROM ACCOUNT_COMMAND_GRANTS WHERE USERNAME = ? ORDER BY GRANTED_AT ASC", username)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CommandGrantInfo
	for rows.Next() {
		var g CommandGrantInfo
		if err := rows.Scan(&g.Username, &g.Command, &g.GrantedBy, &g.GrantedAt); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// ListAllCommandGrants returns every command grant on the server. Used to
// seed the in-memory grant cache at startup (see athena.loadCommandGrants)
// and by the console "grants" listing with no username.
func ListAllCommandGrants() ([]CommandGrantInfo, error) {
	if db == nil {
		return nil, nil
	}
	rows, err := db.Query("SELECT USERNAME, COMMAND, GRANTED_BY, GRANTED_AT FROM ACCOUNT_COMMAND_GRANTS ORDER BY USERNAME ASC, GRANTED_AT ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CommandGrantInfo
	for rows.Next() {
		var g CommandGrantInfo
		if err := rows.Scan(&g.Username, &g.Command, &g.GrantedBy, &g.GrantedAt); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

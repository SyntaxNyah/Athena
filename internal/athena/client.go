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
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/MangosArentLiterature/Athena/internal/area"
	"github.com/MangosArentLiterature/Athena/internal/db"
	"github.com/MangosArentLiterature/Athena/internal/logger"
	"github.com/MangosArentLiterature/Athena/internal/packet"
	"github.com/MangosArentLiterature/Athena/internal/permissions"
	"github.com/MangosArentLiterature/Athena/internal/webhook"
)

// packetBufPool is a pool of reusable byte buffers used by SendPacket to build
// outgoing AO2 packets without allocating a new buffer on every call.
// Each buffer is reset before use and returned to the pool after the write.
var packetBufPool = sync.Pool{
	New: func() any { return new(bytes.Buffer) },
}

type MuteState int

const (
	Unmuted MuteState = iota
	ICMuted
	OOCMuted
	ICOOCMuted
	MusicMuted
	JudMuted
	ParrotMuted
)

type PunishmentType int

const (
	PunishmentNone PunishmentType = iota
	// Text Modification (13 types)
	PunishmentWhisper
	PunishmentBackward
	PunishmentStutterstep
	PunishmentElongate
	PunishmentUppercase
	PunishmentLowercase
	PunishmentRobotic
	PunishmentAlternating
	PunishmentFancy
	PunishmentUwu
	PunishmentPirate
	PunishmentShakespearean
	PunishmentCaveman
	// Visibility/Cosmetic (2 types)
	PunishmentEmoji
	PunishmentInvisible
	// Timing Effects (4 types)
	PunishmentSlowpoke
	PunishmentFastspammer
	PunishmentPause
	PunishmentLag
	// Social Chaos (3 types)
	PunishmentSubtitles
	PunishmentRoulette
	PunishmentSpotlight
	// Text Processing (7 types)
	PunishmentCensor
	PunishmentConfused
	PunishmentParanoid
	PunishmentDrunk
	PunishmentHiccup
	PunishmentWhistle
	PunishmentMumble
	// Complex Effects (3 types)
	PunishmentSpaghetti
	PunishmentTorment
	PunishmentRng
	PunishmentEssay
	// Advanced (2 types)
	PunishmentHaiku
	PunishmentAutospell
	// Animal Punishments (12 types)
	PunishmentMonkey
	PunishmentSnake
	PunishmentDog
	PunishmentCat
	PunishmentBird
	PunishmentCow
	PunishmentFrog
	PunishmentDuck
	PunishmentHorse
	PunishmentLion
	PunishmentZoo
	PunishmentBunny
	// Dere-type Punishments (10 types)
	PunishmentTsundere
	PunishmentYandere
	PunishmentKuudere
	PunishmentDandere
	PunishmentDeredere
	PunishmentHimedere
	PunishmentKamidere
	PunishmentUndere
	PunishmentBakadere
	PunishmentMayadere
	// Text Emoticon Punishment
	PunishmentEmoticon
	// Social Torment Punishments
	PunishmentLovebomb
	PunishmentDegrade
	// Chaos/Outburst Punishments
	PunishmentTourettes
	// Internet Slang Punishment
	PunishmentSlang
	// New Fun Punishment Commands
	PunishmentThesaurusOverload
	PunishmentValleyGirl
	PunishmentBabytalk
	PunishmentThirdPerson
	PunishmentUnreliableNarrator
	PunishmentUncannyValley
	// 51 Messages Punishment
	Punishment51
	// Philosophical / Literary Punishments
	PunishmentPhilosopher
	PunishmentPoet
	PunishmentUpsidedown
	PunishmentSarcasm
	PunishmentAcademic
	PunishmentRecipe
	// Quote Punishment
	PunishmentQuote
	// Translator Punishment — translates IC messages via an external API.
	PunishmentTranslator
	// Chaos / novelty punishments (additions).
	PunishmentTimewarp          // shuffles word order
	PunishmentMorse             // converts text to Morse code
	PunishmentRickroll          // replaces message with a Rick Astley lyric line
	PunishmentVowelhell         // replaces consonants with random vowels
	PunishmentChef              // Swedish-Chef filter
	PunishmentKaren             // entitled "let me speak to your manager" escalation
	PunishmentPassiveAggressive // passive-aggressive filler tacked onto messages
	PunishmentNervous           // nervous stuttering + filler (um, uh, ah...)
	PunishmentDreamSequence     // progressively surreal / dreamlike rewrites
	// IC backlog punishment — area-specific
	PunishmentICWarp // replaces messages with random past IC messages from the same area
	// Romantic humiliation punishment — replaces every IC line with a cheesy pickup line.
	PunishmentPickup
	// Brainrot punishment — maximum internet / Italian brainrot corruption.
	PunishmentBrainrot
	// Gordon Ramsay punishment — replaces every IC line with an iconic Ramsay quote.
	PunishmentGordonRamsay
	// Cherri punishment — Capitalizes Every Single Word like Cherri.
	PunishmentCherri
	// Al Bhed — transliterates every letter through the FFX Al Bhed cipher.
	PunishmentAlbhed
	// Clown / Jester / Joker / Mime — themed text transforms used as
	// moderator gag commands.
	PunishmentClown
	PunishmentJester
	PunishmentJoker
	PunishmentMime
	// Biblebot — replaces every IC line with a random Bible verse.
	PunishmentBiblebot
	// Additional dere archetypes covering the most distinct mainstream entries
	// from the popular dere taxonomy. Less common entries (sub-1% varieties)
	// are not enumerated separately — they fold into /omnidere's random pool.
	PunishmentSmugdere
	PunishmentDeretsun
	PunishmentBokodere
	PunishmentThugdere
	PunishmentTeasedere
	PunishmentDorodere
	PunishmentHinedere
	PunishmentHajidere
	PunishmentRindere
	PunishmentUtsudere
	PunishmentDarudere
	PunishmentButsudere
	PunishmentSDere
	PunishmentMDere
	PunishmentTsuyodere
	// Omnidere — picks a random dere flavour per message, drawing from the
	// full archetype pool for maximum tonal whiplash.
	PunishmentOmnidere
	// Megamaso — random punishment for 10 minutes, retrigger stacks another
	// random punishment on top instead of replacing the previous one.
	PunishmentMegamaso
	// SfxCurse — forces a specific SFX file to be appended to every IC
	// message the target sends. The SFX URL is stored in the punishment's
	// CustomData field rather than the enum.
	PunishmentSfxCurse
	// Shrink / Grow / Wide — lock the target into a fixed vertical or
	// horizontal sprite offset, applied on every IC message. Stored offset
	// lives in the punishment's CustomData (numeric).
	PunishmentShrink
	PunishmentGrow
	PunishmentWide
	// FromSoftware — replaces words that appear in config/fromsoft.txt with ***.
	PunishmentFromSoftware
	// Voice-chat punishments — applied in pktVSFrame (the documented voice
	// relay hook), never in the IC text path. They manipulate the punished
	// speaker's Opus frame flow — dropping, gating, or stale-repeating frames
	// — so they cannot alter audio content (pitch-shift would need an Opus
	// codec and CGO). Appended last so existing persisted SUBTYPE values keep
	// their meaning. See voice_punishments.go.
	PunishmentVoiceMute
	PunishmentVoiceStatic
	PunishmentVoiceGarble
	PunishmentVoiceCutout
	PunishmentVoiceStutter
	// Grounded — replaces every IC line with a GoAnimate "grounded" tirade.
	PunishmentGrounded
	// Display punishments — manipulate which character sprites render in the
	// viewport. Unlike the text effects above they touch the IC packet's sprite
	// fields, not its message text, and are applied late in pktIC (just before
	// broadcast). Appended last so existing persisted SUBTYPE values keep their
	// meaning. See display_punishments.go.
	//
	// HideDisplay pushes the punished speaker's OWN sprite off-screen (a fixed
	// off-viewport self-offset) so their text still appears but their character
	// does not — looks especially funny on pairs, where only the partner shows.
	//
	// ForceDisplay pins the punished player's character onto every non-moderator
	// IC message in their area: while it's active, everyone in the room renders
	// as that one character, so no other sprite can show in the viewport.
	PunishmentHideDisplay
	PunishmentForceDisplay
	// Wave-2 text transforms (see punishments_v2.go / punishments_weeb.go).
	// Appended after every earlier type so persisted SUBTYPE values keep
	// their meaning.
	PunishmentZalgo        // creeping combining-mark corruption
	PunishmentLeetspeak    // h4x0r 5p34k
	PunishmentSmallcaps    // ᴇᴠᴇʀʏᴛʜɪɴɢ ɪɴ ᴛɪɴʏ ᴄᴀᴘs
	PunishmentPiglatin     // igpay atinlay
	PunishmentVaporwave    // ｆｕｌｌｗｉｄｔｈ aesthetic
	PunishmentLisp         // th replaces s
	PunishmentSpoonerism   // swaps word onsets ("shake a tower")
	PunishmentKeysmash     // asdfjkl;; bursts mid-sentence
	PunishmentWeeb         // romaji sprinkles + honorifics
	PunishmentPolitician   // never answers anything directly
	PunishmentClickbait    // message becomes a clickbait headline
	PunishmentMarkov       // markov babble built from the area's chat history
	PunishmentAlliteration // forces awkward alliteration
	PunishmentCipher       // escalating ROT13 → BINARY → BASE64 layers
	// Protocol-field punishments — manipulate the IC packet's non-text fields
	// (offsets, screenshake, flip, text colour, preanim modifiers). Applied in
	// pktIC via applyProtocolPunishments; every written value passes the
	// packet validators. See punishments_protocol.go.
	PunishmentTeleport     // random self-offset per message
	PunishmentShakecurse   // screenshake on every message
	PunishmentRandomflip   // 50% sprite flip per message
	PunishmentForceColor   // text colour locked to customData (0-9)
	PunishmentNoPreanim    // strips preanimations
	PunishmentForcePreanim // forces preanimations to play
	// Delivery punishment — buffers the speaker's IC messages and releases
	// them in reverse (LIFO) order. See punishments_lifo.go.
	PunishmentLifo
	// Mechanic punishments — plague spread and per-message detonations.
	// Hooked after the IC broadcast in pktIC. See punishments_mechanics.go.
	PunishmentContagious // spreads to whoever speaks within the window; customData = underlying type
	PunishmentMinefield  // each message has a 1-in-6 chance to detonate a random 2-min punishment
	// StealthMute — the target's IC/OOC messages echo back to them but are
	// never broadcast; they don't know they're muted. Gated at the broadcast
	// call sites in pktIC/pktOOC.
	PunishmentStealthMute
	// Themed text punishments (see punishments_medieval_cheese.go). Appended
	// last so existing persisted SUBTYPE values keep their meaning.
	PunishmentMedieval // rewrites IC text into Olde-English / medieval speak
	PunishmentCheese   // replaces every message with a random statement about cheese
	// RandomArea — a "moves on its own" punishment like Contagious/Minefield:
	// a per-connection watcher goroutine periodically force-warps the target
	// to a random open area, independent of anything they say. See
	// randomarea_punishment.go. Appended last so existing persisted SUBTYPE
	// values keep their meaning.
	PunishmentRandomArea
	// Animal filters, wave 2 (see punishments_trex_fish.go). Appended last so
	// existing persisted SUBTYPE values keep their meaning.
	PunishmentTrex // RAAASRFH — dinosaur roars
	PunishmentFish // blublublib — fish noises
)

// IssuerTier records the permission tier of the moderator who applied a
// punishment. Used by /unpunish to block self-removal of admin-issued
// punishments by anyone below ADMIN targeting their own UID. Mod and Shadow
// sit at the same tier for this purpose — a shadow mod carries no more
// formal authority than a regular moderator, only stealth (they differ
// elsewhere, e.g. hidden from non-admin /gas/players) — so a punishment
// either of them issues is removable by either of them, self included.
type IssuerTier byte

const (
	IssuerSystem IssuerTier = iota // automod, /maso, hangman, etc. — no protection
	IssuerMod                      // regular moderator with MUTE
	IssuerShadow                   // SHADOW (no ADMIN) — same protection tier as IssuerMod
	IssuerAdmin                    // ADMIN — protected from self-removal
)

type PunishmentState struct {
	punishmentType PunishmentType
	expiresAt      time.Time
	reason         string
	lastMsgTime    time.Time
	msgDelay       time.Duration
	msgCount       int
	lastEffect     int
	targetUID      int        // For PunishmentLovebomb: UID of the lovebomb target (-1 = random area target)
	customData     string     // For PunishmentTranslator: target language name or "random"
	icWarpArea     *area.Area // For PunishmentICWarp: the area where the warp applies; nil = inert
	issuerTier     IssuerTier // who issued this — protects shadow/admin punishments from self-removal
}

type ClientPairInfo struct {
	name      string
	emote     string
	flip      string
	offset    string
	wanted_id int
}

// emergencyBypassWindow is how long a moderator has to confirm an emergency
// entry into a locked area by attempting to join it again after receiving the
// warning message.
const emergencyBypassWindow = 30 * time.Second

type Client struct {
	pair ClientPairInfo
	mu   sync.Mutex
	conn net.Conn
	// writeMu serializes inline socket writes with each other. Deliberately
	// separate from mu: nothing that blocks on I/O may hold the mutex ordinary
	// field accessors use -- see Client.write.
	writeMu             sync.Mutex
	joining             bool
	hdid                string
	uid                 int
	area                *area.Area
	char                int
	charIDStr           string // cached strconv.Itoa(char); updated on every SetCharID call
	ipid                string
	oocName             string
	lastmsg             string
	lastTextColor       string
	perms               uint64
	authenticated       bool
	mod_name            string
	pos                 string
	case_prefs          [5]bool
	muted               MuteState
	muteuntil           time.Time
	areaMuteOrigin      *area.Area // Area whose /area mute produced the client's current ICOOCMuted state; nil if the mute (if any) isn't area-scoped
	showname            string
	narrator            bool
	jailedUntil         time.Time
	lastRpsTime         time.Time
	punishments         []PunishmentState
	msgTimestamps       []time.Time    // Tracks message timestamps for rate limiting
	oocMsgTimestamps    []time.Time    // Tracks OOC message timestamps for OOC rate limiting
	rawPktCount         int            // Packet count in the current raw-rate-limit window
	rawPktWindowStart   time.Time      // Start time of the current raw-rate-limit window
	lastModcallTime     time.Time      // Tracks last modcall time for cooldown
	lastBarDrinkTime    time.Time      // Tracks last /bar buy time for cooldown
	lastRandomCharTime  time.Time      // Tracks last /randomchar time for cooldown
	lastRandomBgTime    time.Time      // Tracks last /randombg time for cooldown
	lastDJBgTime        time.Time      // Tracks last /bg time for DJ rate limit (1 min)
	lastRandomSongTime  time.Time      // Tracks last /randomsong time for cooldown
	lastTranslateTime   time.Time      // Tracks last /translate time for cooldown
	forcePairUID        int            // UID of the client this client is force-paired with (-1 if none)
	forcedShowname      string         // Showname forced by a moderator ("" if none)
	nameReversed        bool           // gates /reversename so it cannot double-apply
	preReverseShowname  string         // forcedShowname before /reversename; restored by /unreversename
	shuffledOrigCharID  int            // Original char ID before /charshuffle (-2 = not shuffled)
	forcedIniswapChar   string         // Character name forced for iniswap-style IC output ("" = none)
	forcedIniswapIDStr  string         // Pre-computed strconv.Itoa(charID) matching forcedIniswapChar ("" = none)
	connectedAt         time.Time      // Time the client joined the server (uid assigned); zero if not yet joined
	acceptedAt          time.Time      // Raw TCP/WS accept time, set in NewClient -- covers the whole handshake window, unlike connectedAt (only set once RD assigns a UID). See raidguard_wire.go.
	charPickedAt        time.Time      // Time of this connection's FIRST character pick (CC); zero until then. See raidguard_wire.go.
	jailAreaID          int            // Area index where this client is jailed; -1 = no specific jail area
	emergencyBypassArea *area.Area     // Locked area the client most recently tried to enter as a mod; nil = no pending bypass
	emergencyBypassAt   time.Time      // Time of the first locked-area attempt; used with emergencyBypassArea to confirm an emergency override
	hidden              bool           // Whether the client is hidden from the player list and area counts
	charStuckUntil      time.Time      // Time when the character-stuck restriction expires; zero = not stuck
	charStuckCharID     int            // Character ID the client is locked to; -1 = not stuck
	dancing             bool           // Whether the client has dance mode active (flips sprite every message)
	danceFlipped        bool           // Current flip state for dance mode; toggles each IC message
	gambleHide          bool           // Whether the client has opted out of seeing gambling broadcast messages
	pendingRegUser      string         // Username from a pending /register that is awaiting captcha confirmation
	pendingRegPass      []byte         // bcrypt hash from a pending /register that is awaiting captcha confirmation
	pendingRegCaptcha   string         // Expected captcha token for the pending registration
	sessionChipsAwarded int64          // Chips already awarded mid-session (hourly ticker); subtracted at disconnect to avoid double-counting
	ignoredIPIDs        sync.Map       // Set of IPIDs permanently ignored by this client. Key: IPID string, Value: struct{}. Lock-free reads.
	lastPingNano        atomic.Int64   // Unix nanosecond timestamp of the last CH packet; 0 until seeded on join.
	masoPunishment      PunishmentType // Active self-applied maso punishment type; PunishmentNone if inactive.
	lookingForPair      bool           // Whether the client is flagged as Looking For Pair (/lfp); shown by /pairlist.
	lovePotionUntil     time.Time      // While in the future, the next area speaker receives a pair request from this client. Zero = not armed.

	// Self-service idle auto-disconnect (/dc, /dctime). Opt-in and isolated to
	// the client that sets it: the watcher goroutine only ever closes THIS
	// connection and sends no packet to anyone else. dcIdleMinutes is the
	// configured idle window in minutes (0 = off); dcLastActivityNano is the
	// Unix-nanosecond timestamp of the client's last IC/OOC activity, refreshed
	// on every message they send. dcWatcherStarted gates the lazy spawn of a
	// single watcher goroutine for the life of the connection.
	dcIdleMinutes      atomic.Int64
	dcLastActivityNano atomic.Int64
	dcWatcherStarted   atomic.Bool

	// Join captcha (see joincaptcha.go). awaitingCaptcha is the hot-path gate:
	// a single atomic load on every IC/OOC/command packet, true only while this
	// connection still owes an answer, so a server with the feature off -- or a
	// player who has already verified -- pays nothing beyond that load.
	// captchaStrikes counts failed attempts (blocked messages and wrong
	// answers alike) toward the configured kick threshold. The challenge
	// itself lives under the client mutex in pendingJoinChallenge.
	// captchaRestricted marks a connection that ran out of captcha attempts
	// under the 'mute' action: what it sends no longer reaches the room.
	awaitingCaptcha      atomic.Bool
	captchaRestricted    atomic.Bool
	captchaStrikes       atomic.Int32
	pendingJoinChallenge joinChallenge

	// raid is this connection's accumulated raid-guard evidence, created lazily
	// on first observation so a server with the guard disabled never allocates
	// one. Guarded by client.mu; the raidState is itself self-synchronised, so
	// callers only hold client.mu long enough to fetch the pointer.
	raid *raidState

	// censorAlertsOff mutes the staff censor-trip OOC alerts for this session
	// (/censoralerts off). Only consulted for clients holding MOD_CHAT; every
	// fresh connection defaults back to alerts on. See censor_alerts.go.
	censorAlertsOff atomic.Bool

	// raidAlertsOff mutes the staff raid-guard OOC alerts for this session
	// (/raidguard alert off). Only consulted for clients holding MOD_CHAT;
	// every fresh connection defaults back to alerts on. See raidguard.go.
	raidAlertsOff atomic.Bool

	// punishAuditOff mutes the punishment-audit OOC alerts for this session
	// (/punishaudit off). Only consulted for clients holding ADMIN; every
	// fresh connection defaults back to alerts on. See punishment_audit.go.
	punishAuditOff atomic.Bool

	// oocHidden mutes regular OOC chat for this client's session (/toggle
	// global): the area's local OOC and player-initiated /global broadcasts
	// stop reaching them, for players who'd rather not see the spam. Direct
	// messages (/pm) and staff/system broadcasts (/mod -g, /modchat,
	// announcements, moderation alerts) are untouched -- this only ever
	// suppresses delivery to THIS client, never affects what anyone else
	// sees, and every fresh connection defaults back to OOC visible. See
	// commands_toggle.go.
	oocHidden atomic.Bool

	// charProtectOn arms /charprotect for this moderator: if they change into
	// an area where another player already holds their claimed character,
	// that player is bumped to a random free character instead of the
	// moderator being demoted to spectator. Session-only (resets to off on
	// reconnect). See charprotect.go.
	charProtectOn atomic.Bool

	// /curserandomchar admin curse: forces this client to a random free
	// character every 1-5 seconds until an admin lifts it with
	// /uncurserandomchar. curseRandomCharActive is the live on/off flag the
	// watcher goroutine checks each tick; curseRandomCharWatcherStarted gates
	// the lazy spawn of that single per-connection goroutine so re-arming
	// (e.g. on reconnect) never double-spawns it. The curse itself is
	// persisted by IPID in the RANDOMCHAR_CURSES table (see
	// curse_randomchar.go) specifically so relogging cannot be used to
	// escape it — these two fields are just the runtime half tied to this
	// connection.
	curseRandomCharActive         atomic.Bool
	curseRandomCharWatcherStarted atomic.Bool

	// Showname punisher (punishment_names.txt): gates the lazy spawn of the
	// per-connection drip watcher goroutine (showname_punishment.go). The
	// stain itself is keyed by IPID in shownamePunishStains so relogging or
	// renaming can't escape it — this field is just the runtime half tied to
	// this connection.
	shownamePunishWatcherStarted atomic.Bool

	// /randomarea punishment (randomarea_punishment.go): gates the lazy spawn
	// of the per-connection watcher goroutine that performs the periodic
	// forced area-warps. Unlike curseRandomChar/shownamePunish there is no
	// separate "active" flag or external persistence table — the watcher's
	// liveness check is simply HasActivePunishment(PunishmentRandomArea),
	// since this punishment already lives in, and persists via, the normal
	// punishment slice/DB like any other cmdPunishment-backed effect.
	randomAreaWatcherStarted atomic.Bool

	// Outbound packet queue. SendPacket enqueues here non-blockingly; a
	// dedicated writer goroutine (started in HandleClient) drains sendCh and
	// performs the actual TCP write. This decouples broadcasters (whose
	// goroutine iterates every client in the area) from individual slow
	// consumers — a single bot whose recv buffer is full can no longer stall
	// every other player's IC sends, which was the connection-flood freeze.
	//
	// If the queue is full a packet is dropped silently — the queue is sized
	// to absorb the worst legitimate burst (sendPlayerListToClient can issue
	// 4–5 packets per existing player on join, so 1000+ players still fits)
	// and dropping a single non-critical packet is far better than kicking
	// the player. Write errors are tolerated to match the original SendPacket
	// behavior; the read loop in HandleClient remains the authoritative
	// "this connection is dead" signal, and once it triggers cleanup,
	// markClosed closes the conn and the writer's next Write returns
	// net.ErrClosed and exits the goroutine.
	sendCh        chan []byte
	done          chan struct{}
	closeDoneOnce sync.Once
	closed        atomic.Bool

	// jsonMode is set the first time this client sends a JSON-encoded packet
	// (object starting with '{'). Once set, every subsequent inbound packet
	// from this client is parsed as JSON and every outbound packet is encoded
	// as JSON. The flag is one-way — switching back to FantaCode mid-session
	// is not supported because a single client genuinely doesn't need to.
	// Reads from the writer goroutine and the SendPacket fan-out are
	// lock-free via atomic.Bool.
	jsonMode atomic.Bool
}

// sendQueueSize bounds the per-client outbound packet backlog. Sized to
// comfortably absorb the worst legitimate burst: sendPlayerListToClient emits
// up to ~5 packets per existing player on join, so even a 1000-player server
// has headroom. At ~150 bytes/packet this is ~1.2 MiB per client in the worst
// case — only reached transiently by a stuck consumer, after which packets
// drop silently rather than disconnecting the client.
const sendQueueSize = 8192

// NewClient returns a new client. The outbound writer goroutine is NOT
// started here — HandleClient starts it once the connection has cleared the
// early-reject checks, so a Client created solely to deliver a one-shot
// rejection packet (e.g. NewClient(...).SendPacketSync("BD",...) on lockdown)
// never leaks a writer goroutine.
func NewClient(conn net.Conn, ipid string) *Client {
	return &Client{
		conn:               conn,
		uid:                -1,
		char:               -1,
		charIDStr:          "-1",
		pair:               ClientPairInfo{wanted_id: -1},
		ipid:               ipid,
		forcePairUID:       -1,
		jailAreaID:         -1,
		charStuckCharID:    -1,
		shuffledOrigCharID: -2,         // -2 = "not shuffled" sentinel; -1 = shuffled but original was charselect
		acceptedAt:         time.Now(), // raw accept time; see the acceptedAt field comment above
		sendCh:             make(chan []byte, sendQueueSize),
		done:               make(chan struct{}),
	}
}

// writeDeadlineNanos backs writeDeadline() below. An atomic, not a plain var:
// runWriter goroutines from a prior test aren't always fully torn down by
// the time the next test starts (existing t.Cleanup ordering elsewhere in
// this package doesn't wait on every goroutine's exit), so a plain var
// mutated by a test racing against a straggler's read trips -race. Storing
// nanoseconds in an atomic.Int64 makes every access race-free regardless.
var writeDeadlineNanos atomic.Int64

// defaultWriteDeadline is writeDeadlineNanos' value outside of tests.
//
// It bounds a single outbound Write (runWriter and SendPacketSync). For a
// plain TCP net.Conn, hitting this deadline only fails that one Write; the
// socket itself is untouched, matching the "ignore transient errors"
// handling below. For a WebSocket connection (client.conn is a
// websocket.NetConn wrapping *websocket.Conn — every WebAO/WSS client), this
// assumption does NOT hold: nhooyr.io/websocket documents that hitting any
// deadline closes the underlying connection outright (see NetConn's doc
// comment, and Conn.timeoutLoop → Conn.close → rwc.Close in
// conn_notjs.go/netconn.go), rather than merely failing the one call. So for
// WS clients this deadline doubles as a hard idle-write timeout: if the
// client's OS isn't draining its socket receive buffer promptly — which is
// exactly what happens to a minimized/backgrounded app under OS-level
// throttling — the very next broadcast that happens to reach it (anyone
// else's IC/OOC line, an ARUP update, etc., not just its own traffic) can
// silently kill the connection the instant the write stalls past this
// deadline, with no packet sent to explain why. 30s (vs. the original 5s)
// gives real-world OS scheduling/backpressure room to clear on its own
// before that happens, without weakening abuse defenses — stuck/malicious
// connections are already reaped independently by the connection rate
// limiter, raw-packet rate limiter, and ping-timeout paths, all of which
// call conn.Close() directly.
const defaultWriteDeadline = 30 * time.Second

func init() {
	writeDeadlineNanos.Store(int64(defaultWriteDeadline))
}

// writeDeadline returns the current single-Write deadline. A function, not a
// plain var, so it's backed by an atomic (see writeDeadlineNanos).
func writeDeadline() time.Duration {
	return time.Duration(writeDeadlineNanos.Load())
}

// runWriter drains the client's outbound packet queue and performs the TCP
// writes. One goroutine per client, started by HandleClient after the
// early-reject checks. Decoupling the write from SendPacket means a stuck
// consumer (full kernel send buffer, dead connection that hasn't FIN'd, etc.)
// only blocks its own writer — never a broadcaster iterating across every
// client in an area, which was the root cause of the connection-flood freeze
// where legitimate IC sends locked up while raid bots filled their TCP
// buffers.
//
// Write errors are deliberately tolerated to match the original SendPacket
// behavior: a brief network blip (Wi-Fi roam, latency spike, momentary
// rebuffer) can make a single Write hit its writeDeadline without the
// connection actually being dead (true for raw TCP; see writeDeadline's doc
// comment for why WebSocket connections don't get that same benefit). The
// old code ignored Write errors entirely for exactly this reason. We do the
// same — the read loop in HandleClient remains the authoritative signal for
// "this connection is gone", and when it triggers cleanup, markClosed closes
// the conn, which makes our next Write fail with net.ErrClosed and exits the
// goroutine.
//
// Stuck raid bots are still reaped: they're caught by the existing
// connection rate limiter, raw-packet rate limiter, and ping-timeout paths
// — all of which existed before this change and which call conn.Close()
// directly, surfacing as net.ErrClosed here.
func (client *Client) runWriter() {
	for {
		select {
		case buf := <-client.sendCh:
			client.conn.SetWriteDeadline(time.Now().Add(writeDeadline())) //nolint:errcheck
			_, err := client.conn.Write(buf)
			client.conn.SetWriteDeadline(time.Time{}) //nolint:errcheck
			if logger.EnableNetworkLogging {
				logger.WriteNetworkLog(client.ipid, client.Hdid(), "SEND", string(buf))
			}
			if err != nil && errors.Is(err, net.ErrClosed) {
				// The conn has been explicitly closed (cleanup, ping
				// timeout, kick path, etc.). No more sends will succeed.
				return
			}
			// Any other error (timeout, transient I/O, broken pipe) is
			// ignored. If the connection is truly dead the read loop will
			// notice and trigger cleanup, which will close the conn and
			// land us in the net.ErrClosed branch above.
		case <-client.done:
			return
		}
	}
}

// markClosed atomically transitions the client to the closed state, signals
// the writer goroutine to exit, and tears down the underlying connection.
// Idempotent and safe to call from any goroutine.
func (client *Client) markClosed() {
	if client.closed.CompareAndSwap(false, true) {
		client.closeDoneOnce.Do(func() { close(client.done) })
		client.conn.Close()
	}
}

// maxPacketBytes bounds how many bytes may be buffered while reading a single
// packet off the wire before a terminator ('%') or a complete JSON value is
// seen. Without this cap a client that never sends a terminator can make
// bufio.Reader/json.Decoder grow their scratch buffer without limit, which
// across many concurrent connections is a memory-amplification DoS bounded
// only by the ping-timeout watchdog. No legitimate AO2 packet (chat text is
// already capped by max_message_length well below this) needs anywhere near
// this much room.
const maxPacketBytes = 64 * 1024

// errPacketTooLarge is returned by boundedReader once a single packet-read
// attempt exceeds maxPacketBytes.
var errPacketTooLarge = errors.New("packet exceeds maximum size")

// boundedReader wraps a connection so a read cycle can be capped without
// touching the FantaCode/JSON parsing logic downstream. n is reset to 0 by
// the caller at the start of each packet-read attempt, so every packet gets
// a fresh budget rather than the cap applying to the whole connection.
type boundedReader struct {
	r   io.Reader
	max int
	n   int
}

func (b *boundedReader) Read(p []byte) (int, error) {
	if b.n >= b.max {
		return 0, errPacketTooLarge
	}
	if rem := b.max - b.n; len(p) > rem {
		p = p[:rem]
	}
	n, err := b.r.Read(p)
	b.n += n
	return n, err
}

// handleClient handles a client connection to the server.
func (client *Client) HandleClient() {
	defer client.clientCleanup()

	// Tracks the header of the packet currently being dispatched, so a
	// recovered panic (below) can name what triggered it. Declared here,
	// ahead of the recover closure, since a deferred closure can only
	// reference a local variable already in scope at the point it's defined.
	var lastHandledHeader string

	// A panic anywhere in a packet handler (of which there are hundreds,
	// spanning the whole command surface) would otherwise be unrecovered and
	// crash the entire process -- taking every connected player down with
	// it, not just this one connection. Recovering here bounds the blast
	// radius to the single connection that triggered it: the recovered
	// panic is logged loudly (a stack trace, since the handler surface this
	// guards is too broad for a bare panic value to be actionable on its
	// own) and the connection is closed rather than left to keep running
	// handlers against what may now be partially-mutated state.
	defer func() {
		if rec := recover(); rec != nil {
			logger.LogErrorf("panic handling %q packet from IPID:%v UID:%v: %v\n%s",
				lastHandledHeader, client.Ipid(), client.Uid(), rec, debug.Stack())
			logger.WriteAudit(fmt.Sprintf("%v | PANIC | IPID:%v | UID:%v | header:%v | %v",
				time.Now().UTC().Format("15:04:05"), client.Ipid(), client.Uid(), lastHandledHeader, rec))
			client.markClosed()
		}
	}()

	if client.CheckBanned(db.IPID) {
		return
	}

	// Defense in depth: the accept-loop checks in ListenTCP/HandleWS already
	// reject a shadow-disconnected IPID before a *Client ever exists, but a
	// connection already past that point when /shadowdisconnect is issued
	// mid-handshake would otherwise slip through. No packet is sent -- see
	// shadowdisconnect.go for why shadowDisconnectNow forces an abrupt close
	// rather than a clean one.
	if isShadowDisconnected(client.Ipid()) {
		client.shadowDisconnectNow()
		return
	}

	// If this IPID has been tormented by automod, schedule a random disconnect.
	if isIPIDTormented(client.Ipid()) {
		go startTormentDisconnect(client)
	}

	// Load this client's persisted ignore list. We don't yet know the account
	// name at connect time, so pass empty username; any account-linked extras
	// are merged in after a successful /login call.
	if ignoredIPs, err := db.LoadIgnoredIPIDs(client.Ipid(), ""); err != nil {
		logger.LogErrorf("Failed to load ignore list for %v: %v", client.Ipid(), err)
	} else {
		for _, ipid := range ignoredIPs {
			client.ignoredIPIDs.Store(ipid, struct{}{})
		}
	}

	if config.MCLimit != 0 && clients.CountByIPID(client.Ipid()) >= config.MCLimit {
		client.SendSync(&packet.BD{Reason: "Too many connections from your IP. Please disconnect your other clients. If you have no other clients open, wait 1-2 minutes and try again."})
		client.conn.Close()
		return
	}

	clients.AddClient(client)

	// Spawn the outbound writer goroutine now that the connection has cleared
	// the early-reject checks. All async SendPacket calls below this point
	// enqueue onto sendCh; runWriter performs the actual TCP write.
	go client.runWriter()

	go timeout(client)

	// FantaCrypt relic. The payload is now "JSON" — a soft capability signal:
	// JSON-aware clients respond with a '{'-prefixed packet and we switch this
	// client to JSON encoding for the rest of the session; older clients
	// ignore the value and stay in classic FantaCode.
	client.Send(&packet.Decryptor{})

	// Reading both wire formats off the same connection means we can't use a
	// pure json.Decoder loop (which would eat FantaCode bytes thinking they
	// were a malformed JSON value). Instead: peek the next non-whitespace
	// byte each iteration. If it's '{', drive json.Decoder for that packet
	// and latch jsonMode on; otherwise bufio.ReadString to the next '%'.
	// The same *bufio.Reader backs both code paths, and the json.Decoder is
	// reused after first creation so it can carry its internal buffer
	// forward across consecutive JSON packets.
	// bcr caps how many bytes any single packet-read attempt below may pull
	// off the raw socket (see maxPacketBytes) -- a client that never sends a
	// terminator/complete JSON value would otherwise let br/jsonDec grow
	// their scratch buffer without bound.
	bcr := &boundedReader{r: client.conn, max: maxPacketBytes}
	br := bufio.NewReader(bcr)
	var jsonDec *json.Decoder

	for {
		// Fresh budget for this packet-read attempt.
		bcr.n = 0

		// Skip ASCII whitespace between packets (web clients sometimes flush
		// a stray "\n" between messages). EOF here means the peer hung up.
		if err := skipNetWhitespace(br); err != nil {
			if errors.Is(err, errPacketTooLarge) {
				client.closeForOversizedPacket()
			}
			return
		}
		lead, err := br.Peek(1)
		if err != nil {
			if errors.Is(err, errPacketTooLarge) {
				client.closeForOversizedPacket()
			}
			return
		}

		var rawPacket string
		if lead[0] == '{' {
			client.jsonMode.Store(true)
			if jsonDec == nil {
				jsonDec = json.NewDecoder(br)
			}
			var msg json.RawMessage
			if err := jsonDec.Decode(&msg); err != nil {
				if errors.Is(err, errPacketTooLarge) {
					client.closeForOversizedPacket()
				}
				return
			}
			rawPacket = string(msg)
		} else {
			line, err := br.ReadString('%')
			if err != nil {
				if errors.Is(err, errPacketTooLarge) {
					client.closeForOversizedPacket()
				}
				return
			}
			rawPacket = strings.TrimSuffix(line, "%")
		}
		rawPacket = strings.TrimSpace(rawPacket)
		if rawPacket == "" {
			continue
		}

		if logger.EnableNetworkLogging {
			logger.WriteNetworkLog(client.ipid, client.Hdid(), "RECV", rawPacket)
		}

		// Raw packet rate limit: disconnect bots/flooders that send far more packets per
		// second than any legitimate client ever would.
		//
		// packet_flood_autoban decides whether that disconnect is also a ban. It is
		// honoured here rather than assumed: the flag existed and was documented for a
		// long time while nothing read it, so this path banned unconditionally and an
		// operator who set it to false got a ban anyway. Disconnecting without banning
		// still fully protects the server -- the flood is off the socket either way, and
		// the rate limit keeps rejecting it on every reconnect -- so the flag only
		// controls whether a mistake is durable.
		if client.CheckRawPacketRateLimit() {
			banning := config.PacketFloodAutoban
			if banning {
				client.SendServerMessage("You have been banned for packet flooding.")
				logger.LogInfof("Client (IPID:%v UID:%v) banned for raw packet flooding", client.Ipid(), client.Uid())
				logger.WriteAudit(fmt.Sprintf("%v | PACKET_FLOOD | IPID:%v | UID:%v | Auto-banned for packet flooding", time.Now().UTC().Format("15:04:05"), client.Ipid(), client.Uid()))
				autoBanPacketFlooder(client.Ipid())
			} else {
				client.SendServerMessage("You have been disconnected for packet flooding.")
				logger.LogInfof("Client (IPID:%v UID:%v) disconnected for raw packet flooding (packet_flood_autoban is off)", client.Ipid(), client.Uid())
				logger.WriteAudit(fmt.Sprintf("%v | PACKET_FLOOD | IPID:%v | UID:%v | Disconnected for packet flooding (no ban)", time.Now().UTC().Format("15:04:05"), client.Ipid(), client.Uid()))
			}
			if enableDiscord && banning {
				ipid, uid := client.Ipid(), client.Uid()
				go func() {
					if err := webhook.PostPacketFlood(ipid, uid); err != nil {
						logger.LogErrorf("while posting packet flood webhook: %v", err)
					}
				}()
			}
			client.conn.Close()
			return
		}

		var pkt *packet.Packet
		if rawPacket[0] == '{' {
			pkt, err = packet.ParseJSON(rawPacket)
		} else {
			pkt, err = packet.NewPacket(rawPacket)
		}
		if err != nil {
			logger.LogWarningf("dropped packet from IPID:%v UID:%v — parse error: %v; raw=%q", client.Ipid(), client.Uid(), err, rawPacket)
			continue
		}
		v, known := PacketMap[pkt.Header]
		if !known || v.Func == nil {
			logger.LogWarningf("dropped packet from IPID:%v UID:%v — unknown header %q; body=%v", client.Ipid(), client.Uid(), pkt.Header, pkt.Body)
			continue
		}
		if len(pkt.Body) < v.Args {
			logger.LogWarningf("dropped %s packet from IPID:%v UID:%v — %d body fields, need %d; body=%v", pkt.Header, client.Ipid(), client.Uid(), len(pkt.Body), v.Args, pkt.Body)
			continue
		}
		if v.MustJoin && client.Uid() == -1 {
			logger.LogWarningf("dropped %s packet from IPID:%v — client has not completed handshake (UID=-1)", pkt.Header, client.Ipid())
			continue
		}
		lastHandledHeader = pkt.Header
		v.Func(client, pkt)
	}
}

// closeForOversizedPacket logs and closes the connection when a single
// packet-read attempt exceeded maxPacketBytes without producing a complete
// packet. Distinguished from an ordinary disconnect (plain EOF) so staff can
// tell the two apart in the logs.
func (client *Client) closeForOversizedPacket() {
	logger.LogWarningf("closing connection IPID:%v UID:%v — packet exceeded %d bytes without completing", client.Ipid(), client.Uid(), maxPacketBytes)
	client.conn.Close()
}

// skipNetWhitespace discards ASCII whitespace bytes at the head of br so the
// caller can peek the first meaningful byte of the next packet. Returns the
// underlying read error (typically io.EOF) on disconnect.
func skipNetWhitespace(br *bufio.Reader) error {
	for {
		b, err := br.Peek(1)
		if err != nil {
			return err
		}
		switch b[0] {
		case ' ', '\t', '\r', '\n':
			if _, err := br.Discard(1); err != nil {
				return err
			}
		default:
			return nil
		}
	}
}

// write sends the given message to the client's network socket.
// Write errors are intentionally ignored: any underlying connection failure
// will surface on the next read in HandleClient, which closes the connection.
//
// The socket write MUST NOT happen under client.mu. It used to, and that was a
// server-wide freeze: client.mu is the general field mutex, taken by ordinary
// accessors including Client.Area() -- which every broadcast calls on every
// client to decide who the recipients are. A connection whose send buffer was
// full would block here holding that mutex, and every broadcast on the server
// then queued behind it. With no deadline set (runWriter clears the deadline
// after each of its own writes) that block is indefinite, so the server stops
// responding without ever crashing. A raid produces exactly this: many
// connections that join and then never read. See
// TestStuckWriterCannotStallBroadcasts.
//
// mu is therefore held only long enough to read the fields, and a separate
// writeMu preserves the one thing it was actually providing here -- that two
// concurrent write calls cannot interleave their bytes on the wire.
func (client *Client) write(message string) {
	client.mu.Lock()
	conn, ipid, hdid := client.conn, client.ipid, client.hdid
	client.mu.Unlock()
	if conn == nil {
		return
	}

	client.writeMu.Lock()
	// Bounded so a stuck socket cannot pin this path forever either.
	conn.SetWriteDeadline(time.Now().Add(writeDeadline())) //nolint:errcheck
	io.WriteString(conn, message)                          //nolint:errcheck
	conn.SetWriteDeadline(time.Time{})                     //nolint:errcheck
	client.writeMu.Unlock()

	if logger.EnableNetworkLogging {
		logger.WriteNetworkLog(ipid, hdid, "SEND", message)
	}
}

// SendPacket enqueues a packet for asynchronous delivery. The packet is built
// into a freshly allocated buffer and pushed to sendCh non-blockingly;
// runWriter performs the actual socket write. This means a broadcaster (e.g.
// an IC broadcast iterating every client in the area) is never blocked on a
// single slow consumer's full TCP send buffer — the symptom during connection
// floods where legitimate IC messages froze even though connection rate
// limits were rejecting the bots.
//
// If sendCh is full the packet is silently DROPPED (not disconnected). The
// queue is sized to absorb the worst legitimate burst (sendPlayerListToClient
// on a heavily populated server) so overflow only happens for a stuck
// consumer, and dropping a single non-critical AO2 packet (IC/OOC/PU/ARUP)
// is far better than kicking a player whose connection is just slightly
// behind.
func (client *Client) SendPacket(header string, contents ...string) {
	// Tests construct *Client via struct literal, bypassing NewClient and
	// leaving sendCh/done nil. Fall back to the synchronous path so existing
	// tests still observe the write on the underlying conn. Production paths
	// always create clients via NewClient so this branch is never taken.
	if client.sendCh == nil {
		client.SendPacketSync(header, contents...)
		return
	}
	if client.closed.Load() {
		return
	}

	var buf []byte
	if client.jsonMode.Load() {
		buf = packet.BuildJSON(header, contents)
		if buf == nil {
			return
		}
		// Enforce the MS broadcast schema on the JSON-mode wire form. A
		// server-side violation is dropped (not sent) and logged so the bug
		// surfaces rather than reaching a client as malformed type nonsense.
		if header == "MS" {
			if err := packet.ValidateMSBroadcast(buf); err != nil {
				logger.LogWarningf("dropped outbound MS to IPID:%v UID:%v — MSBroadcast schema validation failed: %v; json=%s", client.Ipid(), client.Uid(), err, buf)
				return
			}
		}
	} else {
		contents = escapeOutgoing(header, contents)
		// Pre-size the FantaCode buffer in a single allocation:
		// header + ('#' + content)*N + "#%".
		n := len(header) + 2
		for _, c := range contents {
			n += 1 + len(c)
		}
		buf = make([]byte, 0, n)
		buf = append(buf, header...)
		for _, c := range contents {
			buf = append(buf, '#')
			buf = append(buf, c...)
		}
		buf = append(buf, '#', '%')
	}

	select {
	case client.sendCh <- buf:
	default:
		// Queue full — drop the packet. Most AO2 packets are non-critical;
		// losing one is far better than disconnecting the player. A truly
		// stuck consumer's writer will eventually be torn down via the
		// existing ping timeout / rate limiter paths, surfacing here as
		// net.ErrClosed.
	}
}

// SendPacketSync writes a packet directly to the socket, bypassing the
// outbound queue. Use this only on paths that must guarantee the bytes reach
// the kernel before the caller closes the connection — e.g. lockdown / ban
// rejection messages that are immediately followed by conn.Close(). Blocks
// the caller for up to writeDeadline, so it must NOT be used from a
// broadcast loop.
func (client *Client) SendPacketSync(header string, contents ...string) {
	if client.closed.Load() {
		return
	}
	b := packetBufPool.Get().(*bytes.Buffer)
	b.Reset()
	if client.jsonMode.Load() {
		// JSON path: BuildJSON allocates a fresh []byte already; copy it into
		// the pooled buffer so the same SetWriteDeadline / Write code path is
		// reused. The extra copy is negligible because Sync is only used on
		// rare rejection paths (lockdown / ban / MC limit), not the hot loop.
		jb := packet.BuildJSON(header, contents)
		if jb == nil {
			packetBufPool.Put(b)
			return
		}
		if header == "MS" {
			if err := packet.ValidateMSBroadcast(jb); err != nil {
				logger.LogWarningf("dropped outbound MS to IPID:%v UID:%v — MSBroadcast schema validation failed: %v; json=%s", client.Ipid(), client.Uid(), err, jb)
				packetBufPool.Put(b)
				return
			}
		}
		b.Write(jb)
	} else {
		contents = escapeOutgoing(header, contents)
		b.WriteString(header)
		for _, c := range contents {
			b.WriteByte('#')
			b.WriteString(c)
		}
		b.WriteString("#%")
	}

	// Same rule as Client.write: the socket write must not happen under
	// client.mu. The deadline bounds it at writeDeadline (30s by default)
	// rather than forever, but 30 seconds of holding the mutex every broadcast
	// needs -- Client.Area() takes it -- is still a server-wide stall, and this
	// path is reached in bulk during exactly the wrong moment: the lockdown
	// purge delivers its kick reason through SendPacketSync to every connection
	// it is removing, which during a raid is hundreds of sockets that have
	// stopped reading. writeMu keeps concurrent inline writes from interleaving.
	client.mu.Lock()
	conn, ipid, hdid := client.conn, client.ipid, client.hdid
	client.mu.Unlock()
	if conn == nil {
		packetBufPool.Put(b)
		return
	}

	client.writeMu.Lock()
	conn.SetWriteDeadline(time.Now().Add(writeDeadline())) //nolint:errcheck
	var msg string
	if logger.EnableNetworkLogging {
		msg = b.String()
		conn.Write(b.Bytes()) //nolint:errcheck
	} else {
		b.WriteTo(conn) //nolint:errcheck
	}
	conn.SetWriteDeadline(time.Time{}) //nolint:errcheck
	client.writeMu.Unlock()

	if logger.EnableNetworkLogging {
		logger.WriteNetworkLog(ipid, hdid, "SEND", msg)
	}

	packetBufPool.Put(b)
}

// Send enqueues a typed Outgoing packet for asynchronous delivery. The
// packet's Args() is invoked exactly once per Send call. This is the
// canonical send path — SendPacket(header, args...) is now a low-level
// escape hatch reserved for the FantaCrypt "decryptor" relic and the
// hot-path MS broadcast helper.
func (client *Client) Send(p packet.Outgoing) {
	client.SendPacket(p.Header(), p.Args()...)
}

// SendSync writes a typed Outgoing packet directly to the socket,
// bypassing the outbound queue. See SendPacketSync for when to use this.
func (client *Client) SendSync(p packet.Outgoing) {
	client.SendPacketSync(p.Header(), p.Args()...)
}

// JSONMode reports whether this client has opted into JSON-encoded packets
// (true after the first inbound '{'-prefixed packet has been seen on this
// connection). Callers on the rare raw-write code paths — the pre-built
// FantaCode SM blob and the hand-formatted CASEA broadcast — branch on
// this so JSON-mode peers get the correctly encoded form.
func (client *Client) JSONMode() bool {
	return client.jsonMode.Load()
}

// clientLogSnapshot holds the subset of client fields read by addToBuffer.
// All fields are captured under a single mutex acquisition in logSnapshot,
// eliminating the 6+ separate lock/unlock cycles that calling individual
// getters would require.
type clientLogSnapshot struct {
	charName string
	ipid     string
	hdid     string
	showname string
	oocName  string
	area     *area.Area
}

// logSnapshot captures the client fields needed for area-log and buffer entries
// under a single mutex lock.
func (client *Client) logSnapshot() clientLogSnapshot {
	client.mu.Lock()
	var charName string
	if client.char == -1 {
		charName = "Spectator"
	} else if client.char >= 0 && client.char < len(getCharacters()) {
		charName = getCharacters()[client.char]
	}
	snap := clientLogSnapshot{
		charName: charName,
		ipid:     client.ipid,
		hdid:     client.hdid,
		showname: client.showname,
		oocName:  client.oocName,
		area:     client.area,
	}
	client.mu.Unlock()
	return snap
}

// clientCleanup cleans up a disconnected client.
func (client *Client) clientCleanup() {
	if client.Uid() != -1 {
		logger.LogInfof("Client (IPID:%v UID:%v) left the server", client.ipid, client.Uid())

		// Accumulate session playtime and award 1 chip per newly-completed hour.
		// AddPlaytimeReturning is a single atomic SQL operation, so concurrent
		// disconnects for the same IPID (multiclient) cannot race on the boundary.
		if connAt := client.ConnectedAt(); !connAt.IsZero() {
			sessionSecs := int64(time.Since(connAt).Seconds())
			if sessionSecs > 0 {
				ipid := client.Ipid()
				alreadyAwarded := client.SessionChipsAwarded()
				go func() {
					newPt, err := db.AddPlaytimeReturning(ipid, sessionSecs)
					if err != nil {
						logger.LogErrorf("Failed to add playtime for %v: %v", ipid, err)
						return
					}
					oldPt := newPt - sessionSecs
					chipsEarned := (newPt / secondsPerHour) - (oldPt / secondsPerHour) - alreadyAwarded
					if chipsEarned > 0 && config.EnableCasino {
						if err := db.EnsureChipBalance(ipid); err == nil {
							if _, err := db.AddChips(ipid, chipsEarned); err != nil {
								logger.LogErrorf("Failed to award playtime chips for %v: %v", ipid, err)
							}
						}
					}
				}()
			}
		}

		// Release the captcha hot-path gate if this connection was holding it,
		// so a disconnect can never leak the counter and leave every message
		// paying for a lookup that matches nobody.
		if client.captchaRestricted.CompareAndSwap(true, false) {
			activeCaptchaRestricted.Add(-1)
		}

		// Dissolve any pairing involving this client so a partner is never left
		// with a stale pair pointing at this (soon-to-be-recycled) UID/CharID.
		// Runs while client.Uid() is still valid, well before the UID is
		// released for reuse at the end of this function.
		clearPairLinksOnDisconnect(client)

		leaveVoiceForClient(client)
		if client.Area().PlayerCount() <= 1 {
			client.Area().Reset()
			releaseAreaNameOnEmpty(client.Area())
			sendLockArup()
			sendStatusArup()
			sendCMArup()
		} else if client.Area().HasCM(client.Uid()) {
			a := client.Area()
			a.RemoveCM(client.Uid())
			releaseAreaNameOnLastCMLeaving(a)
			sendCMArup()
			if autoUnlockIfLastCMGone(a) {
				sendLockArup()
				sendAreaServerMessage(a, "The area was automatically unlocked because its last CM disconnected.")
				addToBuffer(client, "AREA", "Area auto-unlocked: last CM disconnected.", false)
			}
		}
		for _, a := range areas {
			if a.Lock() != area.LockFree {
				a.RemoveInvited(client.Uid())
			}
		}
		clearVoiceRateStateForUID(client.Uid())
		players.RemovePlayer()
		if config.Advertise {
			updatePlayers <- players.GetPlayerCount()
		}
		client.Area().RemoveChar(client.CharID())
		if !client.Hidden() {
			client.Area().RemoveVisiblePlayer()
		}
		broadcastToAll(&packet.PR{ID: client.Uid(), Type: 1})
		sendPlayerArup()
	}
	handleCasinoDisconnect(client)
	handleMafiaDisconnect(client)
	// Lower the /forcedisplay gate if this client was a pinned target, so the
	// area stops rendering everyone as their character once they're gone.
	client.releaseForceDisplayGate()
	// Disarm any pending love potion so the armed-count gate can't be left
	// permanently elevated by a disconnect (the per-IC scan only self-corrects
	// counters for clients it can still see).
	stopLovePotion(client)
	client.markClosed()
	clients.RemoveClient(client)

	// Release the UID for reuse only now that every teardown step that
	// broadcasts or indexes by it (PR removal, ARUP, the client registry) has
	// already run. Releasing it earlier let a concurrently-joining client pop
	// this same UID from the heap and broadcast its own PR/PU "add" before
	// this client's PR "remove" went out (or before it was dropped from
	// clients.uidIndex), so observers could see the new occupant's join
	// clobbered by the old occupant's belated leave on the same UID.
	if uid := client.Uid(); uid != -1 {
		uids.ReleaseUid(uid)
	}
}

// SendServerMessage sends a server OOC message to the client.
func (client *Client) SendServerMessage(message string) {
	client.Send(&packet.CTToClient{Name: encodedServerName, Message: encode(message), IsFromServer: "1"})
}

// SendMotd sends the MOTD to the client as a single OOC message. Embedded
// newlines are preserved so the client renders the full block as one cohesive
// entry rather than repeating the server name on every line.
func (client *Client) SendMotd(motd string) {
	client.SendServerMessage(strings.TrimRight(motd, "\r\n"))
}

// escalatingKick is the shared engine behind every "repeat offender" kick in the
// server -- currently KickForRateLimit (message-based rate limit kicks) and
// KickForCensorTrip (AutoMod shadow/torment censor trips). It registers one more kick
// against the client's IPID in the same spree counter (registerRateLimitKick) and,
// once the client's playtime tier's threshold is reached (rateLimitAutobanThresholdFor),
// escalates to the shared short automatic ban (autobanFlooderFor) instead of a plain
// kick. The counter and tiers are shared across both violation kinds on purpose: a
// player who racks up one rate-limit kick and one censor-trip kick is just as much a
// repeat offender as one who trips the same check twice, so they escalate the same way
// -- one counter, one leniency system, regardless of which rule they kept breaking.
//
// violation is a short noun phrase describing the offense (e.g. "spamming",
// "prohibited language"), used to build both the player-facing kick/ban messages and
// the info log lines. banReason is the short description recorded on the ban itself
// (via autobanFlooderFor) and the audit log line.
func (client *Client) escalatingKick(banReason, violation string) {
	ipid := client.Ipid()
	uid := client.Uid()

	if count := registerRateLimitKick(ipid); count > 0 && count >= config.RateLimitKickAutobanThreshold {
		if threshold, bannable := rateLimitAutobanThresholdFor(client); bannable && count >= threshold {
			if dur, ok := rateLimitKickAutobanDuration(); ok {
				// Committed synchronously, like the raw-packet-flood ban, so the ban is on
				// record before the connection closes and a fast reconnect attempt can't
				// slip in ahead of it.
				autobanFlooderFor(ipid, banReason, dur)
				client.SendServerMessage(fmt.Sprintf("You have been kicked and temporarily banned for repeated %v (%v).", violation, dur))
				logger.LogInfof("Client (IPID:%v UID:%v) auto-banned for %v after %v kicks for %v", ipid, uid, dur, count, violation)
				logger.WriteAudit(fmt.Sprintf("%v | RATE_LIMIT_AUTOBAN | IPID:%v | UID:%v | Auto-banned for %v after %v kicks for %v",
					time.Now().UTC().Format("15:04:05"), ipid, uid, dur, count, violation))
				client.conn.Close()
				return
			}
		}
	}

	client.SendServerMessage(fmt.Sprintf("You have been kicked for %v.", violation))
	logger.LogInfof("Client (IPID:%v UID:%v) kicked for %v", ipid, uid, violation)
	client.conn.Close()
}

// KickForRateLimit kicks the client for exceeding a message-based rate limit (IC,
// character-select, music/area-change, or either OOC rate limit -- every call site
// checks the rate limit before doing anything else, so the offending message is never
// broadcast). A single violation always results in just a kick. But if the same IPID
// racks up enough kicks within rate_limit_kick_autoban_window -- e.g. a bot that
// reconnects and immediately spams again right after being kicked -- it is additionally
// handed a short automatic ban (rate_limit_kick_autoban_duration), so it can't cycle
// through kick-reconnect-spam forever.
//
// How many kicks is "enough" depends on rateLimitAutobanThresholdFor's playtime tier:
// aggressive (the base rate_limit_kick_autoban_threshold, default 2) for a brand-new
// connection, lenient (default 5) for a connection with a couple hours on it, and fully
// exempt -- kick only, never banned -- for moderators and established regulars. That
// keeps this aggressive against the intended target (a fresh connection that's purely
// there to spam) while staying lenient toward a real, if not yet well-established,
// player who trips the limit from a bad connection rather than actually spamming.
//
// Raw packet flooding is unrelated and always bans on its own, much stricter counter,
// with no such leniency, since no legitimate client ever sends at that rate (see
// CheckRawPacketRateLimit).
func (client *Client) KickForRateLimit() {
	client.escalatingKick("repeated rate-limit violations", "spamming")
}

// KickForCensorTrip kicks the client after an AutoMod censor trip under the shadow or
// torment action (see autoModCheck in automod.go). Those two actions already drop the
// offending message and torment-list the IPID, but previously left the connection open
// with no immediate consequence -- nothing told the sender their message had failed, so
// a determined troll could keep hammering slurs indefinitely (this is what
// KickForCensorTrip fixes). Uses the exact same spree counter and playtime-tiered
// leniency as KickForRateLimit, so repeated censor trips escalate to the same short
// automatic ban that repeated rate-limit kicks do.
//
// This is only ever called from the automatic, server-triggered censor path
// (autoModCheck's shadow/torment cases) -- never from a moderator's manual /lag, which
// calls addTormentedIP directly and is a deliberate staff action, not something that
// should also count toward an automatic ban.
func (client *Client) KickForCensorTrip() {
	client.escalatingKick("repeated censor violations", "prohibited language")
}

// CurrentCharacter returns the client's current character name.
func (client *Client) CurrentCharacter() string {
	if client.CharID() == -1 {
		return "Spectator"
	} else {
		return getCharacters()[client.CharID()]
	}
}

// timeout closes an unjoined client's connection after 1 minute.
// Once the client has joined, if ping_timeout is configured, it also disconnects
// the client whenever the time since its last CH packet exceeds that threshold.
func timeout(client *Client) {
	time.Sleep(1 * time.Minute)
	if client.Uid() == -1 {
		client.conn.Close()
		return
	}
	deadline := config.PingTimeout
	if deadline <= 0 {
		return
	}
	interval := time.Duration(deadline) * time.Second
	intervalNanos := interval.Nanoseconds()
	for {
		time.Sleep(interval)
		if client.Uid() == -1 {
			return
		}
		if nanos := client.lastPingNano.Load(); nanos != 0 && time.Now().UnixNano()-nanos > intervalNanos {
			logger.LogInfof("Client (IPID:%v UID:%v) timed out: no CH packet in %v", client.Ipid(), client.Uid(), interval)
			client.conn.Close()
			return
		}
	}
}

// Hdid returns the client's hdid.
func (client *Client) Hdid() string {
	client.mu.Lock()
	defer client.mu.Unlock()
	return client.hdid
}

// SetHdid sets the client's hdid.
func (client *Client) SetHdid(hdid string) {
	client.mu.Lock()
	client.hdid = hdid
	client.mu.Unlock()
}

// Uid returns the client's user ID.
func (client *Client) Uid() int {
	client.mu.Lock()
	defer client.mu.Unlock()
	return client.uid
}

// SetUid sets the client's user ID.
func (client *Client) SetUid(id int) {
	client.mu.Lock()
	client.uid = id
	client.mu.Unlock()
}

// Area returns the client's current area.
func (client *Client) Area() *area.Area {
	client.mu.Lock()
	defer client.mu.Unlock()
	return client.area
}

// SetArea sets the client's current area.
func (client *Client) SetArea(area *area.Area) {
	client.mu.Lock()
	client.area = area
	client.mu.Unlock()
}

// CharID returns the client's character ID.
func (client *Client) CharID() int {
	client.mu.Lock()
	defer client.mu.Unlock()
	return client.char
}

// SetCharID sets the client's character ID.
func (client *Client) SetCharID(id int) {
	client.mu.Lock()
	client.char = id
	client.charIDStr = strconv.Itoa(id)
	client.mu.Unlock()
}

// CharIDStr returns the client's character ID pre-converted to a string.
// This avoids a strconv.Itoa allocation on every IC/MC packet validation.
func (client *Client) CharIDStr() string {
	client.mu.Lock()
	s := client.charIDStr
	client.mu.Unlock()
	return s
}

// Ipid returns the client's ipid.
func (client *Client) Ipid() string {
	client.mu.Lock()
	defer client.mu.Unlock()
	return client.ipid
}

// OOCName returns the client's current OOC username.
func (client *Client) OOCName() string {
	client.mu.Lock()
	defer client.mu.Unlock()
	return client.oocName
}

// SetOocName sets the client's OOC username.
func (client *Client) SetOocName(name string) {
	client.mu.Lock()
	client.oocName = name
	client.mu.Unlock()
}

// LastMsg returns the client's last sent IC message.
func (client *Client) LastMsg() string {
	client.mu.Lock()
	defer client.mu.Unlock()
	return client.lastmsg
}

// SetLastMsg sets the client's last sent IC message.
func (client *Client) SetLastMsg(msg string) {
	client.mu.Lock()
	client.lastmsg = msg
	client.mu.Unlock()
}

// LastTextColor returns the client's last used text color.
func (client *Client) LastTextColor() string {
	client.mu.Lock()
	defer client.mu.Unlock()
	return client.lastTextColor
}

// SetLastTextColor sets the client's last used text color.
func (client *Client) SetLastTextColor(color string) {
	client.mu.Lock()
	client.lastTextColor = color
	client.mu.Unlock()
}

// Perms returns the client's current permissions.
func (client *Client) Perms() uint64 {
	client.mu.Lock()
	defer client.mu.Unlock()
	return client.perms
}

// SetPerms sets the client's permissionss.
func (client *Client) SetPerms(perms uint64) {
	client.mu.Lock()
	client.perms = perms
	client.mu.Unlock()
}

// Authenticated returns whether the client is logged in to any account
// (moderator, DJ, or plain player). Use permissions.IsModerator(client.Perms())
// to check specifically for server-moderator access.
func (client *Client) Authenticated() bool {
	client.mu.Lock()
	defer client.mu.Unlock()
	return client.authenticated
}

// SetAuthenticated marks the client as logged in to an account; this does not
// imply moderator status. See Authenticated for details.
func (client *Client) SetAuthenticated(auth bool) {
	client.mu.Lock()
	client.authenticated = auth
	client.mu.Unlock()
}

// ModName returns the client's moderator username.
func (client *Client) ModName() string {
	client.mu.Lock()
	defer client.mu.Unlock()
	return client.mod_name
}

// shadowMarker is the byte-prefix (Unit Separator) used to tag a stored
// moderator name as belonging to a shadow mod.  The stored form is
// "\x1f" + real-name so /baninfo and /modnote can reveal the real name to
// admins and render "Moderator" to everyone else.
const shadowMarker = "\x1f"

// DisplayModName is the name this client should appear as in any in-game or
// outbound surface (webhook, community-vote broadcast, /modchat, etc.):
// "Moderator" for shadow mods, their real mod username otherwise.
func (client *Client) DisplayModName() string {
	if permissions.IsShadow(client.Perms()) {
		return "Moderator"
	}
	return client.ModName()
}

// StoredModName is the name to persist into the bans/modnotes tables for an
// action performed by this client.  For shadow mods the real name is kept
// but tagged with shadowMarker so admin lookups can still see who acted.
func (client *Client) StoredModName() string {
	if permissions.IsShadow(client.Perms()) {
		return shadowMarker + client.ModName()
	}
	return client.ModName()
}

// RenderStoredModName decodes a value previously written by StoredModName and
// returns the name to display to a viewer with the given permissions.  Admins
// see the real name of shadow mods; everyone else sees "Moderator".  Values
// without the shadow marker are returned unchanged regardless of viewer.
func RenderStoredModName(stored string, viewerPerms uint64) string {
	if !strings.HasPrefix(stored, shadowMarker) {
		return stored
	}
	real := strings.TrimPrefix(stored, shadowMarker)
	if permissions.IsAdmin(viewerPerms) {
		return real
	}
	return "Moderator"
}

// SetModName sets the client's moderator username.
func (client *Client) SetModName(name string) {
	client.mu.Lock()
	client.mod_name = name
	client.mu.Unlock()
}

// PendingReg returns the client's pending registration data.
// username and captcha are empty strings, hashedPass is nil when no registration is pending.
func (client *Client) PendingReg() (username, captcha string, hashedPass []byte) {
	client.mu.Lock()
	defer client.mu.Unlock()
	return client.pendingRegUser, client.pendingRegCaptcha, client.pendingRegPass
}

// SetPendingReg stores a pending registration awaiting captcha confirmation.
// Pass empty strings and nil to clear any pending registration.
func (client *Client) SetPendingReg(username, captcha string, hashedPass []byte) {
	client.mu.Lock()
	client.pendingRegUser = username
	client.pendingRegCaptcha = captcha
	client.pendingRegPass = hashedPass
	client.mu.Unlock()
}

// Pos returns the client's current position.
func (client *Client) Pos() string {
	client.mu.Lock()
	defer client.mu.Unlock()
	return client.pos
}

// SetPos sets the client's position.
func (client *Client) SetPos(pos string) {
	client.mu.Lock()
	client.pos = pos
	client.mu.Unlock()
}

// CasePrefs returns all client's case preferences.
func (client *Client) CasePrefs() [5]bool {
	client.mu.Lock()
	defer client.mu.Unlock()
	return client.case_prefs
}

// CasePref returns a client's role alert preference.
func (client *Client) AlertRole(index int) bool {
	client.mu.Lock()
	defer client.mu.Unlock()
	return client.case_prefs[index]
}

// SetCasePref sets a client's role alert preference.
func (client *Client) SetRoleAlert(index int, b bool) {
	client.mu.Lock()
	client.case_prefs[index] = b
	client.mu.Unlock()
}

// PairInfo returns a client's pairing info.
func (client *Client) PairInfo() ClientPairInfo {
	client.mu.Lock()
	defer client.mu.Unlock()
	return client.pair
}

// SetPairInfo updates a client's pairing info.
func (client *Client) SetPairInfo(name string, emote string, flip string, offset string) {
	client.mu.Lock()
	client.pair.name, client.pair.emote, client.pair.flip, client.pair.offset = name, emote, flip, offset
	client.mu.Unlock()
}

// PairWantedID returns the character the client wishes to pair with.
func (client *Client) PairWantedID() int {
	client.mu.Lock()
	defer client.mu.Unlock()
	return client.pair.wanted_id
}

// SetPairWantedID sets the character the client wishes to pair with.
func (client *Client) SetPairWantedID(id int) {
	client.mu.Lock()
	defer client.mu.Unlock()
	client.pair.wanted_id = id
}

// ForcePairUID returns the UID of the client this client is force-paired with, or -1 if none.
func (client *Client) ForcePairUID() int {
	client.mu.Lock()
	defer client.mu.Unlock()
	return client.forcePairUID
}

// SetForcePairUID sets the UID of the client this client is force-paired with.
func (client *Client) SetForcePairUID(uid int) {
	client.mu.Lock()
	defer client.mu.Unlock()
	client.forcePairUID = uid
}

// RemoveAuth logs a client out as moderator.
func (client *Client) RemoveAuth() {
	client.mu.Lock()
	client.authenticated, client.perms, client.mod_name = false, 0, ""
	client.mu.Unlock()
	client.SendServerMessage("Logged out as moderator.")
	client.Send(&packet.AUTH{State: -1})
}

// RemoveAccountAuth logs a client out of a player account (no moderator badge change needed).
func (client *Client) RemoveAccountAuth() {
	client.mu.Lock()
	username := client.mod_name
	client.authenticated, client.perms, client.mod_name = false, 0, ""
	client.mu.Unlock()
	client.SendServerMessage(fmt.Sprintf("Logged out of account '%v'.", username))
}

// restorePunishments loads any persistent punishments for this client from the database
// and applies them. Called once after the client successfully joins the server.
func (client *Client) restorePunishments() {
	punishments, err := db.GetPunishments(client.Ipid())
	if err != nil {
		logger.LogErrorf("Error loading punishments for %v: %v", client.Ipid(), err)
		return
	}
	if len(punishments) == 0 {
		return
	}
	for _, p := range punishments {
		var expiresAt time.Time
		if p.Expires != 0 {
			expiresAt = time.Unix(p.Expires, 0).UTC()
		}
		switch p.Kind {
		case db.PunishKindMute:
			m := MuteState(p.Value)
			client.SetMuted(m)
			client.SetUnmuteTime(expiresAt)
		case db.PunishKindJail:
			client.SetJailedUntil(expiresAt)
			client.SetJailAreaID(p.Value)
		case db.PunishKindCharStuck:
			client.SetCharStuck(p.Value, expiresAt)
		case db.PunishKindText:
			pType := PunishmentType(p.Subtype)
			var remaining time.Duration
			if p.Expires != 0 {
				remaining = time.Until(expiresAt)
			}
			tier := IssuerTier(p.IssuerTier)
			// Text-punishment reasons may embed per-instance metadata (e.g. the
			// translator target language) separated from the user-visible reason
			// by an ASCII Unit Separator (0x1F). Decode that here so the custom
			// data survives restarts.
			if idx := strings.IndexByte(p.Reason, 0x1F); idx >= 0 {
				customData := p.Reason[:idx]
				realReason := p.Reason[idx+1:]
				client.AddPunishmentWithData(pType, remaining, realReason, customData)
				// AddPunishmentWithData doesn't take a tier; stamp it after.
				client.setPunishmentTier(pType, tier)
			} else {
				client.AddPunishmentBy(pType, remaining, p.Reason, tier)
			}
		}
	}
	client.SendServerMessage("Your active punishments have been restored.")
	// If the client is jailed to a specific area, force-move them there on reconnect.
	jailArea := client.JailAreaID()
	if jailArea >= 0 {
		if jailArea < len(areas) && areas[jailArea] != client.Area() {
			client.forceChangeArea(areas[jailArea])
			client.SendServerMessage(fmt.Sprintf("You have been returned to your jail area: %v.", areas[jailArea].Name()))
		} else if jailArea >= len(areas) {
			logger.LogErrorf("Jailed IPID %v has out-of-range jail area ID %d; skipping force-move.", client.Ipid(), jailArea)
		}
	}
}

// CheckBanned checks whether the client is currently banned.
// If banned, it sends the BD packet, closes the connection, and returns true.
// Callers must return immediately when CheckBanned returns true.
func (client *Client) CheckBanned(by db.BanLookup) bool {
	var banned bool
	var baninfo db.BanInfo
	var err error
	switch by {
	case db.IPID:
		banned, baninfo, err = db.IsBanned(by, client.Ipid())
		if err != nil {
			logger.LogErrorf("Error reading IP ban for %v: %v", client.Ipid(), err)
		}
	case db.HDID:
		banned, baninfo, err = db.IsBanned(by, client.Hdid())
		if err != nil {
			logger.LogErrorf("Error reading HDID ban for %v: %v", client.Ipid(), err)
		}
	}

	if banned {
		var duration string
		if baninfo.Duration == -1 {
			duration = "∞"
		} else {
			duration = time.Unix(baninfo.Duration, 0).UTC().Format("02 Jan 2006 15:04 MST")
		}
		client.SendSync(&packet.BD{Reason: fmt.Sprintf("%v\nUntil: %v\nID: %v", baninfo.Reason, duration, baninfo.Id)})
		client.conn.Close()
		return true
	}
	return false
}

// JoinArea adds a client to an area.
func (client *Client) JoinArea(area *area.Area) {
	client.SetArea(area)
	area.AddChar(client.CharID())
	if !client.Hidden() {
		area.AddVisiblePlayer()
	}
	def, pro := area.HP()
	client.Send(&packet.LE{Items: areas[0].Evidence()})
	client.Send(&packet.CharsCheck{Entries: area.Taken()})
	client.Send(&packet.HPPacket{Bar: 1, Value: def})
	client.Send(&packet.HPPacket{Bar: 2, Value: pro})
	if desc := area.Description(); desc != "" {
		client.SendServerMessage("📍 " + desc)
	}
	// Sync the joining client to the area's music state.
	//
	// If a track is playing, send it so a client that connects or walks into
	// the area mid-track hears it instead of sitting in silence until someone
	// changes the music again (or the player thinks to run /getmusic by hand).
	//
	// If nothing is playing — the area has never played a track (CurrentSong
	// == "") or its music was explicitly stopped ("~stop.mp3") — send an
	// explicit ~stop.mp3 so the joiner's client stops whatever track it was
	// still playing from its previous area. Without this stop, walking out of a
	// room with music into a silent room left the old track playing forever:
	// the server never told the client to stop, so it had no way to know it
	// should. (The explicit-stop case already worked because "~stop.mp3" is a
	// non-empty CurrentSong; the gap was only the never-played "" case.)
	song := area.CurrentSong()
	if song == "" {
		song = "~stop.mp3"
	}
	client.Send(&packet.MCToClient{Name: song, CharID: client.CharID(), Showname: "Server", Looping: "1", Channel: "0", Effects: "0"})
	sendPlayerArup()
}

// ChangeArea changes the client's current area.
func (client *Client) ChangeArea(a *area.Area) bool {
	// Check if client is jailed
	if time.Now().UTC().Before(client.JailedUntil()) && !client.JailedUntil().IsZero() {
		client.SendServerMessage("You are jailed in this area")
		return false
	}
	// Admin-lock: a /adminlock'd area refuses everyone but administrators —
	// even moderators and shadow mods with BYPASS_LOCK, and even invited
	// players. Checked before the normal lock so there is no emergency-bypass or
	// invite escape hatch.
	if a.AdminLocked() && !permissions.IsAdmin(client.Perms()) {
		client.SendServerMessage("This area is admin-locked. Only an administrator can enter.")
		return false
	}
	if a.Lock() == area.LockLocked &&
		!a.HasInvited(client.Uid()) &&
		!permissions.HasPermission(client.Perms(), permissions.PermissionField["BYPASS_LOCK"]) {
		// Moderators without BYPASS_LOCK can force entry for emergencies on a
		// second attempt within the window — the first attempt warns them.
		if permissions.IsModerator(client.Perms()) {
			client.mu.Lock()
			pendingArea := client.emergencyBypassArea
			pendingAt := client.emergencyBypassAt
			client.mu.Unlock()
			if pendingArea == a && time.Since(pendingAt) <= emergencyBypassWindow {
				client.mu.Lock()
				client.emergencyBypassArea = nil
				client.emergencyBypassAt = time.Time{}
				client.mu.Unlock()
				logger.LogInfof("Moderator %v (UID:%v IPID:%v) used emergency bypass to enter locked area %v",
					client.ModName(), client.Uid(), client.Ipid(), a.Name())
				addToBuffer(client, "MOD", fmt.Sprintf("Used emergency bypass to enter locked area %v.", a.Name()), true)
			} else {
				client.mu.Lock()
				client.emergencyBypassArea = a
				client.emergencyBypassAt = time.Now()
				client.mu.Unlock()
				client.SendServerMessage(fmt.Sprintf(
					"🔒 %v is locked. If this is an emergency, try again within %d seconds to force entry.",
					a.Name(), int(emergencyBypassWindow.Seconds())))
				return false
			}
		} else {
			return false
		}
	}
	if client.Area() != nil {
		client.liftAreaMuteOnDeparture(a)
		addToBuffer(client, "AREA", "Left area.", false)
		leaveVoiceForClient(client)
		if client.Area().PlayerCount() <= 1 {
			client.Area().Reset()
			releaseAreaNameOnEmpty(client.Area())
			sendLockArup()
			sendStatusArup()
			sendCMArup()
		} else if client.Area().HasCM(client.Uid()) {
			client.Area().RemoveCM(client.Uid())
			releaseAreaNameOnLastCMLeaving(client.Area())
			sendCMArup()
		}
		client.Area().RemoveChar(client.CharID())
		if !client.Hidden() {
			client.Area().RemoveVisiblePlayer()
		}
	}
	if a.IsTaken(client.CharID()) {
		if !resolveCharProtectOnJoin(client, a) {
			client.SetCharID(-1)
		}
	}
	client.JoinArea(a)
	broadcastToAll(&packet.PU{ID: client.Uid(), Type: 3, Data: strconv.Itoa(getAreaIndex(a))})
	if client.CharID() == -1 {
		// Send DONE before BN so WebAO's character-select viewport is
		// initialized before the bench-overlay image loads fire.  This
		// mirrors pktReqDone's ordering and matches Akashi's behaviour.
		client.Send(&packet.DONE{})
	} else {
		broadcastToAreaOnce(a, &packet.CharsCheck{Entries: a.Taken()})
	}
	// BN always last — after any DONE — so desk-overlay images never load
	// against an unrendered viewport on WebAO (same fix as initial join).
	client.Send(&packet.BN{Background: a.Background()})
	addToBuffer(client, "AREA", "Joined area.", false)
	return true
}

// liftAreaMuteOnDeparture auto-lifts an /area mute the moment the client
// leaves the area that applied it. /area mute is meant to silence someone only
// while they're in that specific room, and it lives purely in memory (it is
// never persisted). Without this, the in-memory ICOOCMuted state would follow
// the player to every other area for the rest of the session with no way back,
// since /area unmute only reaches players currently in the CM's area. A full
// disconnect ends the mute too, since the *Client (and its area-mute state) is
// gone and nothing restores it — which is what fixes the "forever muted by a
// CM" bug that persisting the mute used to cause.
func (client *Client) liftAreaMuteOnDeparture(dest *area.Area) {
	origin := client.AreaMuteOrigin()
	if origin == nil || origin != client.Area() || origin == dest || client.Muted() != ICOOCMuted {
		return
	}
	client.SetMuted(Unmuted)
	client.SetUnmuteTime(time.Time{})
	client.SendServerMessage("You've left the area that muted you; you can speak again.")
}

// HasCMPermission returns whether the client has CM permissions in it's area.
func (client *Client) HasCMPermission() bool {
	return client.Area().HasCM(client.Uid()) || permissions.HasPermission(client.Perms(), permissions.PermissionField["CM"])
}

// CanSpeakIC returns whether the client can send IC messages.
func (client *Client) CanSpeakIC() bool {
	switch {
	case client.CharID() == -1:
		return false
	case client.Area().Lock() == area.LockSpectatable && !client.area.HasInvited(client.Uid()) &&
		!permissions.HasPermission(client.Perms(), permissions.PermissionField["BYPASS_LOCK"]):
		return false
	case client.Area().SpectateMode() && !client.Area().HasCM(client.Uid()) && !client.Area().HasSpectateInvited(client.Uid()) &&
		!permissions.HasPermission(client.Perms(), permissions.PermissionField["BYPASS_LOCK"]):
		return false
	case client.Muted() == ICMuted || client.Muted() == ICOOCMuted:
		return client.CheckUnmute()
	}
	return true
}

// CanSpeakOOC returns whether the client can send OOC messages.
func (client *Client) CanSpeakOOC() bool {
	if client.IsJailed() {
		return false
	}
	m := client.Muted()
	if m == OOCMuted || m == ICOOCMuted {
		return client.CheckUnmute()
	}
	return true
}

// IsJailed returns true if the client is currently serving an active jail sentence.
func (client *Client) IsJailed() bool {
	t := client.JailedUntil()
	return !t.IsZero() && time.Now().UTC().Before(t)
}

// CanChangeMusic returns whether the client can change the music.
func (client *Client) CanChangeMusic() bool {
	switch {
	case client.CharID() == -1:
		return false
	case client.Area().MusicFrozen() && !permissions.IsModerator(client.Perms()) && !client.HasCMPermission():
		return false
	case client.Area().LockMusic() && !client.HasCMPermission():
		return false
	case client.Area().Lock() == area.LockSpectatable && !client.area.HasInvited(client.Uid()) &&
		!permissions.HasPermission(client.Perms(), permissions.PermissionField["BYPASS_LOCK"]):
		return false
	case client.Muted() == MusicMuted || client.Muted() == ICMuted || client.Muted() == ICOOCMuted:
		return client.CheckUnmute()
	}
	return true
}

// CanJud returns whether the client can use judge actions.
func (client *Client) CanJud() bool {
	switch {
	case client.CharID() == -1:
		return false
	case client.Area().Lock() == area.LockSpectatable && !client.area.HasInvited(client.Uid()) &&
		!permissions.HasPermission(client.Perms(), permissions.PermissionField["BYPASS_LOCK"]):
		return false
	case client.Muted() == JudMuted || client.Muted() == ICMuted || client.Muted() == ICOOCMuted:
		return client.CheckUnmute()
	}
	return true
}

// CheckUnmute checks the client's mute duration, unmuting them if nessecary, and returning whether the client is still muted.
func (client *Client) CheckUnmute() bool {
	if time.Now().UTC().After(client.UnmuteTime()) && !client.UnmuteTime().IsZero() {
		client.SendServerMessage("You have been unmuted.")
		client.SetMuted(Unmuted)
		go func(ipid string) {
			if err := db.DeleteMute(ipid); err != nil {
				logger.LogErrorf("Failed to remove expired mute from DB for %v: %v", ipid, err)
			}
		}(client.ipid)
		return true
	}
	return false
}

// IsParrot returns if the client has been parroted.
func (client *Client) IsParrot() bool {
	if client.Muted() == ParrotMuted {
		return !client.CheckUnmute()
	}
	return false
}

// IsNarrator returns whether the client is a narrator.
func (client *Client) IsNarrator() bool {
	return client.narrator
}

// ToggleNarrator sets whether a client is a narrator.
func (client *Client) ToggleNarrator() {
	client.mu.Lock()
	client.narrator = !client.narrator
	client.mu.Unlock()
	if client.narrator {
		client.SendServerMessage("You are now in narrator mode.")
	} else {
		client.SendServerMessage("You are no longer in narrator mode.")
	}
}

// ToggleDance toggles dance mode on or off and notifies the client.
func (client *Client) ToggleDance() {
	client.mu.Lock()
	client.dancing = !client.dancing
	if !client.dancing {
		client.danceFlipped = false
	}
	client.mu.Unlock()
	if client.dancing {
		client.SendServerMessage("Dance mode enabled. Your sprite will flip and unflip with every message.")
	} else {
		client.SendServerMessage("Dance mode disabled.")
	}
}

// CheckAndToggleDanceFlip atomically checks if dance mode is active and, if so,
// toggles the flip state in one lock acquisition. Returns the new flip value
// ("0" or "1") when dancing, or "" when dance mode is off.
func (client *Client) CheckAndToggleDanceFlip() string {
	client.mu.Lock()
	if !client.dancing {
		client.mu.Unlock()
		return ""
	}
	client.danceFlipped = !client.danceFlipped
	flipped := client.danceFlipped
	client.mu.Unlock()
	if flipped {
		return "1"
	}
	return "0"
}

// canAlterEvidence is a helper function that returns if a client can alter evidence in their current area.
func (client *Client) CanAlterEvidence() bool {
	if client.CharID() == -1 || !client.CanSpeakIC() {
		return false
	}
	switch client.Area().EvidenceMode() {
	case area.EviMods:
		if !permissions.HasPermission(client.Perms(), permissions.PermissionField["MOD_EVI"]) {
			return false
		}
	case area.EviCMs:
		if !client.HasCMPermission() {
			return false
		}
	}
	return true
}

// ChangeCharacter changes the client's character to the given character.
func (client *Client) ChangeCharacter(id int) {
	if client.Area().SwitchChar(client.CharID(), id) {
		client.SetCharID(id)
		// Do not reset showname here; it is set from IC messages so the
		// player's display name (e.g. "Adachi") persists across character
		// changes.
		client.Send(&packet.PV{PlayerID: 0, CharID: id})
		broadcastToAreaOnce(client.Area(), &packet.CharsCheck{Entries: client.Area().Taken()})
		if client.Uid() != -1 {
			broadcastToAll(&packet.PU{ID: client.Uid(), Type: 1, Data: client.CurrentCharacter()})
			broadcastToAll(&packet.PU{ID: client.Uid(), Type: 2, Data: decode(client.Showname())})
		}
	}
}

// Muted returns the client's mute state.
func (client *Client) Muted() MuteState {
	client.mu.Lock()
	defer client.mu.Unlock()
	return client.muted
}

// SetMuted sets the client's mute state. This always clears any recorded
// /area mute origin — SetMuted is used for ordinary, non-area-scoped mutes
// (including lifting a mute entirely), while an /area mute is applied via
// SetAreaMuted so the two can be told apart later.
func (client *Client) SetMuted(m MuteState) {
	client.mu.Lock()
	client.muted = m
	client.areaMuteOrigin = nil
	client.mu.Unlock()
}

// SetAreaMuted marks the client as muted (IC and OOC) by /area mute in a, so
// the mute can be told apart from an ordinary /mute and auto-lifted once the
// client leaves a. This state is in-memory only — /area mute is deliberately
// never persisted, so a reconnect ends it (see liftAreaMuteOnDeparture).
func (client *Client) SetAreaMuted(a *area.Area) {
	client.mu.Lock()
	client.muted = ICOOCMuted
	client.areaMuteOrigin = a
	client.mu.Unlock()
}

// AreaMuteOrigin returns the area whose /area mute produced the client's
// current mute state, or nil if the current mute (if any) isn't area-scoped.
func (client *Client) AreaMuteOrigin() *area.Area {
	client.mu.Lock()
	defer client.mu.Unlock()
	return client.areaMuteOrigin
}

// UnmuteTime returns the time when the client should be unmuted.
// If this the time is zero, the mute does not expire.
func (client *Client) UnmuteTime() time.Time {
	client.mu.Lock()
	defer client.mu.Unlock()
	return client.muteuntil
}

// SetUnmuteTime sets the time when the client should be unmuted.
func (client *Client) SetUnmuteTime(t time.Time) {
	client.mu.Lock()
	client.muteuntil = t
	client.mu.Unlock()
}

// Showname returns the client's showname.
func (client *Client) Showname() string {
	client.mu.Lock()
	defer client.mu.Unlock()
	return client.showname
}

// SetShowname sets the client's showname
func (client *Client) SetShowname(s string) {
	client.mu.Lock()
	client.showname = s
	client.mu.Unlock()
}

// ForcedShowname returns the showname forced by a moderator, or "" if none is set.
func (client *Client) ForcedShowname() string {
	client.mu.Lock()
	defer client.mu.Unlock()
	return client.forcedShowname
}

// SetForcedShowname sets the moderator-forced showname for the client.
func (client *Client) SetForcedShowname(s string) {
	client.mu.Lock()
	client.forcedShowname = s
	client.mu.Unlock()
}

// ForcedIniswapInfo returns the moderator-forced iniswap character name and its
// pre-computed char-ID string. Both values are read under a single mutex
// acquisition. Returns two empty strings when no forced iniswap is active.
func (client *Client) ForcedIniswapInfo() (charName, charIDStr string) {
	client.mu.Lock()
	charName, charIDStr = client.forcedIniswapChar, client.forcedIniswapIDStr
	client.mu.Unlock()
	return
}

// IsTunged returns true when the client has an active moderator-forced iniswap
// (i.e. a /tung effect). While true the client must not be allowed to change
// characters on their own — the lock is lifted by calling
// SetForcedIniswapChar("", "").
func (client *Client) IsTunged() bool {
	client.mu.Lock()
	active := client.forcedIniswapChar != ""
	client.mu.Unlock()
	return active
}

// SetForcedIniswapChar sets (or clears) the moderator-forced iniswap character.
// Pass charName="" and charIDStr="" to clear the effect.
// Both fields are written under a single mutex acquisition.
// Note: these are distinct from client.char / client.charIDStr (the real
// character slot). CharIDStr() always returns the real slot regardless of
// whether a forced iniswap is active.
func (client *Client) SetForcedIniswapChar(charName, charIDStr string) {
	client.mu.Lock()
	client.forcedIniswapChar, client.forcedIniswapIDStr = charName, charIDStr
	client.mu.Unlock()
}

// EffectiveShowname returns the moderator-forced showname if one is set,
// otherwise the client's own showname. Both fields are read under a single
// mutex acquisition to avoid two separate lock/unlock cycles in callers.
func (client *Client) EffectiveShowname() string {
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.forcedShowname != "" {
		return client.forcedShowname
	}
	return client.showname
}

// UpdateShowname atomically updates the client's showname and returns true if
// the stored value actually changed. Callers use the boolean to decide whether
// to broadcast a PU packet, eliminating two extra lock/unlock pairs that a
// read-then-write-then-read pattern would require.
func (client *Client) UpdateShowname(s string) bool {
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.showname == s {
		return false
	}
	client.showname = s
	return true
}

// JailedUntil returns the time when the client's jail expires.
func (client *Client) JailedUntil() time.Time {
	client.mu.Lock()
	defer client.mu.Unlock()
	return client.jailedUntil
}

// SetJailedUntil sets the time when the client's jail expires.
func (client *Client) SetJailedUntil(t time.Time) {
	client.mu.Lock()
	client.jailedUntil = t
	client.mu.Unlock()
}

// JailAreaID returns the area index where this client is jailed (-1 = no specific area).
func (client *Client) JailAreaID() int {
	client.mu.Lock()
	defer client.mu.Unlock()
	return client.jailAreaID
}

// SetJailAreaID sets the area index where this client is jailed.
func (client *Client) SetJailAreaID(id int) {
	client.mu.Lock()
	client.jailAreaID = id
	client.mu.Unlock()
}

// IsCharStuck returns true if the client is currently under a character-stuck restriction.
// Both fields are read under a single mutex lock to avoid double-locking.
func (client *Client) IsCharStuck() bool {
	client.mu.Lock()
	id := client.charStuckCharID
	t := client.charStuckUntil
	client.mu.Unlock()
	return id >= 0 && !t.IsZero() && time.Now().UTC().Before(t)
}

// charStuckID returns the locked character ID if the client is currently stuck, or -1 if not.
// Both fields are read under a single lock; this is the preferred hot-path check that avoids
// the need to call IsCharStuck and CharStuckCharID separately.
func (client *Client) charStuckID() int {
	client.mu.Lock()
	id := client.charStuckCharID
	t := client.charStuckUntil
	client.mu.Unlock()
	if id >= 0 && !t.IsZero() && time.Now().UTC().Before(t) {
		return id
	}
	return -1
}

// SetCharStuck atomically sets both the locked character ID and the expiry time in one lock.
func (client *Client) SetCharStuck(id int, until time.Time) {
	client.mu.Lock()
	client.charStuckCharID = id
	client.charStuckUntil = until
	client.mu.Unlock()
}

// ClearCharStuck atomically clears both char-stuck fields in one lock.
func (client *Client) ClearCharStuck() {
	client.mu.Lock()
	client.charStuckCharID = -1
	client.charStuckUntil = time.Time{}
	client.mu.Unlock()
}

// Hidden returns whether the client is hidden from the player list and area counts.
func (client *Client) Hidden() bool {
	client.mu.Lock()
	defer client.mu.Unlock()
	return client.hidden
}

// SetHidden sets whether the client is hidden from the player list and area counts.
func (client *Client) SetHidden(h bool) {
	client.mu.Lock()
	client.hidden = h
	client.mu.Unlock()
}

// GambleHide returns whether the client has opted out of gambling broadcast messages.
func (client *Client) GambleHide() bool {
	client.mu.Lock()
	defer client.mu.Unlock()
	return client.gambleHide
}

// SetGambleHide sets whether the client wants to suppress gambling broadcast messages.
func (client *Client) SetGambleHide(h bool) {
	client.mu.Lock()
	client.gambleHide = h
	client.mu.Unlock()
}

// forceChangeArea moves the client to the given area unconditionally, bypassing
// the jailed-player area-lock and area invitation checks that ChangeArea enforces.
// Used to place a jailed player into their designated cell (both at jail time and on reconnect).
func (client *Client) forceChangeArea(a *area.Area) {
	client.liftAreaMuteOnDeparture(a)
	addToBuffer(client, "AREA", "Left area.", false)
	if client.Area().PlayerCount() <= 1 {
		client.Area().Reset()
		releaseAreaNameOnEmpty(client.Area())
		sendLockArup()
		sendStatusArup()
		sendCMArup()
	} else if client.Area().HasCM(client.Uid()) {
		client.Area().RemoveCM(client.Uid())
		releaseAreaNameOnLastCMLeaving(client.Area())
		sendCMArup()
	}
	client.Area().RemoveChar(client.CharID())
	if !client.Hidden() {
		client.Area().RemoveVisiblePlayer()
	}
	if a.IsTaken(client.CharID()) {
		if !resolveCharProtectOnJoin(client, a) {
			client.SetCharID(-1)
		}
	}
	client.JoinArea(a)
	broadcastToAll(&packet.PU{ID: client.Uid(), Type: 3, Data: strconv.Itoa(getAreaIndex(a))})
	if client.CharID() == -1 {
		// Send DONE before BN for the same reason as ChangeArea: WebAO
		// must initialize the viewport before desk-overlay images load.
		client.Send(&packet.DONE{})
	} else {
		broadcastToAreaOnce(a, &packet.CharsCheck{Entries: a.Taken()})
	}
	// BN always after any DONE so desk overlays load correctly on WebAO.
	client.Send(&packet.BN{Background: a.Background()})
	addToBuffer(client, "AREA", "Joined area.", false)
}

// LastRpsTime returns the last time the client played RPS.
func (client *Client) LastRpsTime() time.Time {
	client.mu.Lock()
	defer client.mu.Unlock()
	return client.lastRpsTime
}

// SetLastRpsTime sets the last time the client played RPS.
func (client *Client) SetLastRpsTime(t time.Time) {
	client.mu.Lock()
	client.lastRpsTime = t
	client.mu.Unlock()
}

// CheckModcallCooldown checks if the client is within the modcall cooldown period.
// Returns true (and the remaining seconds, rounded up) if the client must wait, false otherwise.
// When the cooldown is disabled (0), always returns false.
func (client *Client) CheckModcallCooldown() (bool, int) {
	if config.ModcallCooldown <= 0 {
		return false, 0
	}
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.lastModcallTime.IsZero() {
		return false, 0
	}
	elapsed := time.Since(client.lastModcallTime)
	cooldown := time.Duration(config.ModcallCooldown) * time.Second
	if elapsed < cooldown {
		remaining := int(math.Ceil((cooldown - elapsed).Seconds()))
		return true, remaining
	}
	return false, 0
}

// SetLastModcallTime records the current time as the client's last modcall time.
func (client *Client) SetLastModcallTime() {
	client.mu.Lock()
	client.lastModcallTime = time.Now()
	client.mu.Unlock()
}

const barDrinkCooldown = 20 * time.Second

// CheckBarDrinkCooldown checks if the client is within the bar drink cooldown period.
// Returns true (and the remaining seconds, rounded up) if the client must wait, false otherwise.
func (client *Client) CheckBarDrinkCooldown() (bool, int) {
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.lastBarDrinkTime.IsZero() {
		return false, 0
	}
	elapsed := time.Since(client.lastBarDrinkTime)
	if elapsed < barDrinkCooldown {
		remaining := int(math.Ceil((barDrinkCooldown - elapsed).Seconds()))
		return true, remaining
	}
	return false, 0
}

// SetLastBarDrinkTime records the current time as the client's last /bar buy time.
func (client *Client) SetLastBarDrinkTime() {
	client.mu.Lock()
	client.lastBarDrinkTime = time.Now()
	client.mu.Unlock()
}

// LastRandomCharTime returns the last time the client used /randomchar.
func (client *Client) LastRandomCharTime() time.Time {
	client.mu.Lock()
	defer client.mu.Unlock()
	return client.lastRandomCharTime
}

// SetLastRandomCharTime records the current time as the client's last /randomchar time.
func (client *Client) SetLastRandomCharTime(t time.Time) {
	client.mu.Lock()
	client.lastRandomCharTime = t
	client.mu.Unlock()
}

// CheckAndUpdateRandomBgCooldown atomically checks whether the /randombg cooldown
// has elapsed and, if so, records the current time as the new last-use timestamp.
// It returns (true, 0) when the command is allowed, or (false, remaining) when
// the client is still in cooldown.
func (client *Client) CheckAndUpdateRandomBgCooldown(cooldown time.Duration) (bool, time.Duration) {
	client.mu.Lock()
	defer client.mu.Unlock()
	now := time.Now()
	elapsed := now.Sub(client.lastRandomBgTime)
	if !client.lastRandomBgTime.IsZero() && elapsed < cooldown {
		return false, cooldown - elapsed
	}
	client.lastRandomBgTime = now
	return true, 0
}

// CheckAndUpdateDJBgCooldown atomically checks whether the DJ /bg cooldown has elapsed
// and, if so, records the current time as the new last-use timestamp.
// It returns (true, 0) when the command is allowed, or (false, remaining) when the
// client is still in cooldown.
func (client *Client) CheckAndUpdateDJBgCooldown(cooldown time.Duration) (bool, time.Duration) {
	client.mu.Lock()
	defer client.mu.Unlock()
	now := time.Now()
	elapsed := now.Sub(client.lastDJBgTime)
	if !client.lastDJBgTime.IsZero() && elapsed < cooldown {
		return false, cooldown - elapsed
	}
	client.lastDJBgTime = now
	return true, 0
}

// CheckAndUpdateRandomSongCooldown atomically checks whether the /randomsong cooldown
// has elapsed and, if so, records the current time as the new last-use timestamp.
// It returns (true, 0) when the command is allowed, or (false, remaining) when
// the client is still in cooldown.
func (client *Client) CheckAndUpdateRandomSongCooldown(cooldown time.Duration) (bool, time.Duration) {
	client.mu.Lock()
	defer client.mu.Unlock()
	now := time.Now()
	elapsed := now.Sub(client.lastRandomSongTime)
	if !client.lastRandomSongTime.IsZero() && elapsed < cooldown {
		return false, cooldown - elapsed
	}
	client.lastRandomSongTime = now
	return true, 0
}

// CheckAndUpdateTranslateCooldown atomically checks whether the /translate
// cooldown has elapsed and, if so, records the current time as the new
// last-use timestamp.  It returns (true, 0) when the command is allowed, or
// (false, remaining) when the client is still in cooldown.
func (client *Client) CheckAndUpdateTranslateCooldown(cooldown time.Duration) (bool, time.Duration) {
	client.mu.Lock()
	defer client.mu.Unlock()
	now := time.Now()
	elapsed := now.Sub(client.lastTranslateTime)
	if !client.lastTranslateTime.IsZero() && elapsed < cooldown {
		return false, cooldown - elapsed
	}
	client.lastTranslateTime = now
	return true, 0
}

// String returns the string representation of a mute state.
func (m MuteState) String() string {
	switch m {
	case ICMuted:
		return "IC"
	case OOCMuted:
		return "OOC"
	case ICOOCMuted:
		return "IC/OOC"
	case MusicMuted:
		return "from changing the music"
	case JudMuted:
		return "from judge controls"
	}
	return ""
}

// AddPunishment adds a punishment to the client. The issuer tier defaults to
// IssuerSystem; use AddPunishmentBy to record the issuing moderator's tier.
func (client *Client) AddPunishment(pType PunishmentType, duration time.Duration, reason string) {
	client.AddPunishmentBy(pType, duration, reason, IssuerSystem)
}

// AddPunishmentBy adds a punishment and records the tier of the issuer so that
// /unpunish can block anyone below ADMIN from silently lifting a punishment
// that an admin applied to them.
func (client *Client) AddPunishmentBy(pType PunishmentType, duration time.Duration, reason string, tier IssuerTier) {
	client.mu.Lock()
	defer client.mu.Unlock()

	// Remove existing punishment of the same type (prevent duplicate same-type punishments)
	// Different punishment types can coexist and stack their effects
	existed := false
	for i := len(client.punishments) - 1; i >= 0; i-- {
		if client.punishments[i].punishmentType == pType {
			existed = true
			client.punishments = append(client.punishments[:i], client.punishments[i+1:]...)
			break
		}
	}

	expiresAt := time.Time{}
	if duration > 0 {
		expiresAt = time.Now().UTC().Add(duration)
	}

	client.punishments = append(client.punishments, PunishmentState{
		punishmentType: pType,
		expiresAt:      expiresAt,
		reason:         reason,
		lastMsgTime:    time.Time{},
		msgDelay:       0,
		msgCount:       0,
		lastEffect:     0,
		targetUID:      -1,
		issuerTier:     tier,
	})
	// Maintain the /forcedisplay hot-path gate. A forcedisplay that wasn't
	// already present — a fresh application or a DB restore on reconnect — raises
	// the count by one; a same-type re-apply leaves it unchanged. Only
	// AddPunishmentBy is hooked because forcedisplay carries no customData and so
	// never routes through AddPunishmentWithData.
	if pType == PunishmentForceDisplay && !existed {
		activeForceDisplay.Add(1)
	}
	// /randomarea has no customData, so — like /forcedisplay above — it only
	// ever routes through AddPunishmentBy, both for a fresh apply and for a
	// DB restore on reconnect (restorePunishments). Arming is CAS-gated and
	// idempotent, so a re-apply that refreshes the duration on an existing
	// randomarea punishment is harmless here.
	if pType == PunishmentRandomArea {
		client.armRandomAreaWatch()
	}
}

// PunishmentIssuerTier returns the tier of the moderator who applied the given
// punishment type to this client, or IssuerSystem if no such punishment exists.
func (client *Client) PunishmentIssuerTier(pType PunishmentType) IssuerTier {
	client.mu.Lock()
	defer client.mu.Unlock()
	for _, p := range client.punishments {
		if p.punishmentType == pType {
			return p.issuerTier
		}
	}
	return IssuerSystem
}

// HasProtectedPunishment reports whether the client carries any punishment
// that was applied by an admin (issuer tier IssuerAdmin). Used by /unpunish
// to decide whether to block a self-removal request from anyone below
// ADMIN. A shadow-mod-issued punishment is NOT protected here — Mod and
// Shadow sit at the same tier, so either can remove what the other issued.
func (client *Client) HasProtectedPunishment() bool {
	client.mu.Lock()
	defer client.mu.Unlock()
	for _, p := range client.punishments {
		if p.issuerTier >= IssuerAdmin {
			return true
		}
	}
	return false
}

// setPunishmentTier stamps the issuer tier on the most recent punishment of
// the given type. Used by the DB-restore path where AddPunishmentWithData
// can't carry the tier through its existing signature.
func (client *Client) setPunishmentTier(pType PunishmentType, tier IssuerTier) {
	client.mu.Lock()
	defer client.mu.Unlock()
	for i := len(client.punishments) - 1; i >= 0; i-- {
		if client.punishments[i].punishmentType == pType {
			client.punishments[i].issuerTier = tier
			return
		}
	}
}

// AddPunishmentWithData adds a punishment that carries arbitrary per-instance
// metadata (customData), e.g. the target language for PunishmentTranslator.
// Same same-type-deduplication as AddPunishment.
func (client *Client) AddPunishmentWithData(pType PunishmentType, duration time.Duration, reason, customData string) {
	client.mu.Lock()
	defer client.mu.Unlock()

	for i := len(client.punishments) - 1; i >= 0; i-- {
		if client.punishments[i].punishmentType == pType {
			client.punishments = append(client.punishments[:i], client.punishments[i+1:]...)
			break
		}
	}

	expiresAt := time.Time{}
	if duration > 0 {
		expiresAt = time.Now().UTC().Add(duration)
	}

	client.punishments = append(client.punishments, PunishmentState{
		punishmentType: pType,
		expiresAt:      expiresAt,
		reason:         reason,
		targetUID:      -1,
		customData:     customData,
	})
}

// RemovePunishment removes a punishment type from the client.
func (client *Client) RemovePunishment(pType PunishmentType) {
	client.mu.Lock()
	defer client.mu.Unlock()

	for i := len(client.punishments) - 1; i >= 0; i-- {
		if client.punishments[i].punishmentType == pType {
			client.punishments = append(client.punishments[:i], client.punishments[i+1:]...)
			if pType == PunishmentForceDisplay {
				activeForceDisplay.Add(-1)
			}
			return
		}
	}
}

// AddLovebombPunishment adds a lovebomb punishment with an optional specific target UID.
// Use targetUID=-1 for a random area target on each message.
func (client *Client) AddLovebombPunishment(targetUID int, duration time.Duration, reason string) {
	client.mu.Lock()
	defer client.mu.Unlock()

	// Remove existing lovebomb (same-type deduplication)
	for i := len(client.punishments) - 1; i >= 0; i-- {
		if client.punishments[i].punishmentType == PunishmentLovebomb {
			client.punishments = append(client.punishments[:i], client.punishments[i+1:]...)
			break
		}
	}

	expiresAt := time.Time{}
	if duration > 0 {
		expiresAt = time.Now().UTC().Add(duration)
	}

	client.punishments = append(client.punishments, PunishmentState{
		punishmentType: PunishmentLovebomb,
		expiresAt:      expiresAt,
		reason:         reason,
		targetUID:      targetUID,
	})
}

// AddICWarpPunishment adds an icwarp punishment tied to a specific area.
// The warp only activates while the client is in that particular area.
func (client *Client) AddICWarpPunishment(warpArea *area.Area, duration time.Duration, reason string) {
	client.mu.Lock()
	defer client.mu.Unlock()

	// Same-type deduplication: replace any existing icwarp punishment.
	for i := len(client.punishments) - 1; i >= 0; i-- {
		if client.punishments[i].punishmentType == PunishmentICWarp {
			client.punishments = append(client.punishments[:i], client.punishments[i+1:]...)
			break
		}
	}

	expiresAt := time.Time{}
	if duration > 0 {
		expiresAt = time.Now().UTC().Add(duration)
	}

	client.punishments = append(client.punishments, PunishmentState{
		punishmentType: PunishmentICWarp,
		expiresAt:      expiresAt,
		reason:         reason,
		targetUID:      -1,
		icWarpArea:     warpArea,
	})
}

// RemoveAllPunishments removes all punishments from the client.
func (client *Client) RemoveAllPunishments() {
	client.mu.Lock()
	defer client.mu.Unlock()
	for i := range client.punishments {
		if client.punishments[i].punishmentType == PunishmentForceDisplay {
			activeForceDisplay.Add(-1)
			break // at most one forcedisplay per client (same-type dedup)
		}
	}
	client.punishments = []PunishmentState{}
}

// HasPunishment checks if the client has a specific punishment type.
func (client *Client) HasPunishment(pType PunishmentType) bool {
	client.mu.Lock()
	defer client.mu.Unlock()

	for _, p := range client.punishments {
		if p.punishmentType == pType {
			return true
		}
	}
	return false
}

// GetPunishment returns the punishment state for a specific type, or nil if not found.
// WARNING: The returned pointer should not be modified directly. Use UpdatePunishmentState instead.
// This method is provided for read-only access to punishment state.
func (client *Client) GetPunishment(pType PunishmentType) *PunishmentState {
	client.mu.Lock()
	defer client.mu.Unlock()

	for i := range client.punishments {
		if client.punishments[i].punishmentType == pType {
			return &client.punishments[i]
		}
	}
	return nil
}

// Punishments returns a snapshot copy of the client's active punishment list.
// The returned slice is a copy; modifying it has no effect on the client.
func (client *Client) Punishments() []PunishmentState {
	client.mu.Lock()
	defer client.mu.Unlock()
	snap := make([]PunishmentState, len(client.punishments))
	copy(snap, client.punishments)
	return snap
}

// HasAnyPunishment reports whether the client has at least one active
// punishment.  Unlike Punishments(), it never allocates a copy of the slice.
func (client *Client) HasAnyPunishment() bool {
	client.mu.Lock()
	n := len(client.punishments)
	client.mu.Unlock()
	return n > 0
}

// UpdatePunishmentState updates the state of a punishment (e.g., message counts).
func (client *Client) UpdatePunishmentState(pType PunishmentType, updateFunc func(*PunishmentState)) {
	client.mu.Lock()
	defer client.mu.Unlock()

	for i := range client.punishments {
		if client.punishments[i].punishmentType == pType {
			updateFunc(&client.punishments[i])
			return
		}
	}
}

// CheckExpiredPunishments removes all expired punishments and returns true if any were removed.
// Expired entries are also deleted from the database.
func (client *Client) CheckExpiredPunishments() bool {
	client.mu.Lock()
	defer client.mu.Unlock()

	now := time.Now().UTC()
	removed := false

	for i := len(client.punishments) - 1; i >= 0; i-- {
		p := &client.punishments[i]
		if !p.expiresAt.IsZero() && now.After(p.expiresAt) {
			pType := p.punishmentType
			client.punishments = append(client.punishments[:i], client.punishments[i+1:]...)
			removed = true
			go func(ipid string, t PunishmentType) {
				if err := db.DeleteTextPunishment(ipid, int(t)); err != nil {
					logger.LogErrorf("Failed to remove expired punishment from DB for %v: %v", ipid, err)
				}
			}(client.ipid, pType)
		}
	}

	return removed
}

// GetActivePunishments returns a copy of all active punishments.
func (client *Client) GetActivePunishments() []PunishmentState {
	client.mu.Lock()
	defer client.mu.Unlock()

	// Clean up expired punishments first
	now := time.Now().UTC()
	active := make([]PunishmentState, 0, len(client.punishments))

	for _, p := range client.punishments {
		if p.expiresAt.IsZero() || now.Before(p.expiresAt) {
			active = append(active, p)
		}
	}

	return active
}

// CheckExpiredAndGetPunishments removes expired punishments and returns the remaining
// active ones under a single mutex acquisition.  wasExpired is true if at least one
// punishment was removed.  active is nil when there are no punishments.
//
// Use this instead of calling CheckExpiredPunishments() + GetActivePunishments()
// separately; it halves the number of mutex lock/unlock cycles and eliminates the
// redundant time.Now() call on every IC message.
func (client *Client) CheckExpiredAndGetPunishments() (wasExpired bool, active []PunishmentState) {
	client.mu.Lock()
	defer client.mu.Unlock()

	if len(client.punishments) == 0 {
		return false, nil
	}

	now := time.Now().UTC()
	w := 0
	for i := range client.punishments {
		p := &client.punishments[i]
		if !p.expiresAt.IsZero() && now.After(p.expiresAt) {
			wasExpired = true
			pType := p.punishmentType
			if pType == PunishmentForceDisplay {
				activeForceDisplay.Add(-1)
			}
			ipid := client.ipid
			go func() {
				if err := db.DeleteTextPunishment(ipid, int(pType)); err != nil {
					logger.LogErrorf("Failed to remove expired punishment from DB for %v: %v", ipid, pType)
				}
			}()
		} else {
			client.punishments[w] = client.punishments[i]
			w++
		}
	}
	client.punishments = client.punishments[:w]

	if w == 0 {
		return wasExpired, nil
	}
	active = make([]PunishmentState, w)
	copy(active, client.punishments)
	return wasExpired, active
}

// String returns the string representation of a punishment type.
func (p PunishmentType) String() string {
	switch p {
	case PunishmentWhisper:
		return "whisper"
	case PunishmentBackward:
		return "backward"
	case PunishmentStutterstep:
		return "stutterstep"
	case PunishmentElongate:
		return "elongate"
	case PunishmentUppercase:
		return "uppercase"
	case PunishmentLowercase:
		return "lowercase"
	case PunishmentRobotic:
		return "robotic"
	case PunishmentAlternating:
		return "alternating"
	case PunishmentFancy:
		return "fancy"
	case PunishmentUwu:
		return "uwu"
	case PunishmentPirate:
		return "pirate"
	case PunishmentShakespearean:
		return "shakespearean"
	case PunishmentCaveman:
		return "caveman"
	case PunishmentEmoji:
		return "emoji"
	case PunishmentInvisible:
		return "invisible"
	case PunishmentSlowpoke:
		return "slowpoke"
	case PunishmentFastspammer:
		return "fastspammer"
	case PunishmentPause:
		return "pause"
	case PunishmentLag:
		return "lag"
	case PunishmentSubtitles:
		return "subtitles"
	case PunishmentRoulette:
		return "roulette"
	case PunishmentSpotlight:
		return "spotlight"
	case PunishmentCensor:
		return "censor"
	case PunishmentConfused:
		return "confused"
	case PunishmentParanoid:
		return "paranoid"
	case PunishmentDrunk:
		return "drunk"
	case PunishmentHiccup:
		return "hiccup"
	case PunishmentWhistle:
		return "whistle"
	case PunishmentMumble:
		return "mumble"
	case PunishmentSpaghetti:
		return "spaghetti"
	case PunishmentTorment:
		return "torment"
	case PunishmentRng:
		return "rng"
	case PunishmentEssay:
		return "essay"
	case PunishmentHaiku:
		return "haiku"
	case PunishmentAutospell:
		return "autospell"
	case PunishmentMonkey:
		return "monkey"
	case PunishmentSnake:
		return "snake"
	case PunishmentDog:
		return "dog"
	case PunishmentCat:
		return "cat"
	case PunishmentBird:
		return "bird"
	case PunishmentCow:
		return "cow"
	case PunishmentFrog:
		return "frog"
	case PunishmentDuck:
		return "duck"
	case PunishmentHorse:
		return "horse"
	case PunishmentLion:
		return "lion"
	case PunishmentZoo:
		return "zoo"
	case PunishmentBunny:
		return "bunny"
	case PunishmentTsundere:
		return "tsundere"
	case PunishmentYandere:
		return "yandere"
	case PunishmentKuudere:
		return "kuudere"
	case PunishmentDandere:
		return "dandere"
	case PunishmentDeredere:
		return "deredere"
	case PunishmentHimedere:
		return "himedere"
	case PunishmentKamidere:
		return "kamidere"
	case PunishmentUndere:
		return "undere"
	case PunishmentBakadere:
		return "bakadere"
	case PunishmentMayadere:
		return "mayadere"
	case PunishmentEmoticon:
		return "emoticon"
	case PunishmentLovebomb:
		return "lovebomb"
	case PunishmentDegrade:
		return "degrade"
	case PunishmentTourettes:
		return "tourettes"
	case PunishmentSlang:
		return "slang"
	case PunishmentThesaurusOverload:
		return "thesaurusoverload"
	case PunishmentValleyGirl:
		return "valleygirl"
	case PunishmentBabytalk:
		return "babytalk"
	case PunishmentThirdPerson:
		return "thirdperson"
	case PunishmentUnreliableNarrator:
		return "unreliablenarrator"
	case PunishmentUncannyValley:
		return "uncannyvalley"
	case Punishment51:
		return "51"
	case PunishmentPhilosopher:
		return "philosopher"
	case PunishmentPoet:
		return "poet"
	case PunishmentUpsidedown:
		return "upsidedown"
	case PunishmentSarcasm:
		return "sarcasm"
	case PunishmentAcademic:
		return "academic"
	case PunishmentRecipe:
		return "recipe"
	case PunishmentQuote:
		return "quote"
	case PunishmentTranslator:
		return "translator"
	case PunishmentTimewarp:
		return "timewarp"
	case PunishmentMorse:
		return "morse"
	case PunishmentRickroll:
		return "rickroll"
	case PunishmentVowelhell:
		return "vowelhell"
	case PunishmentChef:
		return "chef"
	case PunishmentKaren:
		return "karen"
	case PunishmentPassiveAggressive:
		return "passiveaggressive"
	case PunishmentNervous:
		return "nervous"
	case PunishmentDreamSequence:
		return "dreamsequence"
	case PunishmentICWarp:
		return "icwarp"
	case PunishmentPickup:
		return "pickup"
	case PunishmentBrainrot:
		return "brainrot"
	case PunishmentGordonRamsay:
		return "gordonramsay"
	case PunishmentCherri:
		return "cherri"
	case PunishmentAlbhed:
		return "albhed"
	case PunishmentClown:
		return "clown"
	case PunishmentJester:
		return "jester"
	case PunishmentJoker:
		return "joker"
	case PunishmentMime:
		return "mime"
	case PunishmentBiblebot:
		return "biblebot"
	case PunishmentSmugdere:
		return "smugdere"
	case PunishmentDeretsun:
		return "deretsun"
	case PunishmentBokodere:
		return "bokodere"
	case PunishmentThugdere:
		return "thugdere"
	case PunishmentTeasedere:
		return "teasedere"
	case PunishmentDorodere:
		return "dorodere"
	case PunishmentHinedere:
		return "hinedere"
	case PunishmentHajidere:
		return "hajidere"
	case PunishmentRindere:
		return "rindere"
	case PunishmentUtsudere:
		return "utsudere"
	case PunishmentDarudere:
		return "darudere"
	case PunishmentButsudere:
		return "butsudere"
	case PunishmentSDere:
		return "sdere"
	case PunishmentMDere:
		return "mdere"
	case PunishmentTsuyodere:
		return "tsuyodere"
	case PunishmentOmnidere:
		return "omnidere"
	case PunishmentMegamaso:
		return "megamaso"
	case PunishmentSfxCurse:
		return "sfxcurse"
	case PunishmentShrink:
		return "shrink"
	case PunishmentGrow:
		return "grow"
	case PunishmentWide:
		return "wide"
	case PunishmentFromSoftware:
		return "fromsoftware"
	case PunishmentVoiceMute:
		return "voicemute"
	case PunishmentVoiceStatic:
		return "voicestatic"
	case PunishmentVoiceGarble:
		return "voicegarble"
	case PunishmentVoiceCutout:
		return "voicecutout"
	case PunishmentVoiceStutter:
		return "voicestutter"
	case PunishmentGrounded:
		return "grounded"
	case PunishmentHideDisplay:
		return "hidedisplay"
	case PunishmentForceDisplay:
		return "forcedisplay"
	case PunishmentZalgo:
		return "zalgo"
	case PunishmentLeetspeak:
		return "leetspeak"
	case PunishmentSmallcaps:
		return "smallcaps"
	case PunishmentPiglatin:
		return "piglatin"
	case PunishmentVaporwave:
		return "vaporwave"
	case PunishmentLisp:
		return "lisp"
	case PunishmentSpoonerism:
		return "spoonerism"
	case PunishmentKeysmash:
		return "keysmash"
	case PunishmentWeeb:
		return "weeb"
	case PunishmentPolitician:
		return "politician"
	case PunishmentClickbait:
		return "clickbait"
	case PunishmentMarkov:
		return "markov"
	case PunishmentAlliteration:
		return "alliteration"
	case PunishmentCipher:
		return "cipher"
	case PunishmentTeleport:
		return "teleport"
	case PunishmentShakecurse:
		return "shakecurse"
	case PunishmentRandomflip:
		return "randomflip"
	case PunishmentForceColor:
		return "forcecolor"
	case PunishmentNoPreanim:
		return "nopreanim"
	case PunishmentForcePreanim:
		return "forcepreanim"
	case PunishmentLifo:
		return "lifo"
	case PunishmentContagious:
		return "contagious"
	case PunishmentMinefield:
		return "minefield"
	case PunishmentStealthMute:
		return "stealthmute"
	case PunishmentMedieval:
		return "medieval"
	case PunishmentCheese:
		return "cheese"
	case PunishmentRandomArea:
		return "randomarea"
	case PunishmentTrex:
		return "trex"
	case PunishmentFish:
		return "fish"
	default:
		return "none"
	}
}

// CheckRateLimit checks if the client has exceeded the message rate limit.
// Returns true if the client should be kicked for spam, false otherwise.
// This is a lightweight implementation using a sliding window approach.
func (client *Client) CheckRateLimit() bool {
	// If rate limiting is disabled, always allow
	if config.RateLimit <= 0 {
		return false
	}

	client.mu.Lock()
	defer client.mu.Unlock()

	now := time.Now()
	window := rateLimitWindowDur
	if window == 0 {
		window = time.Duration(config.RateLimitWindow) * time.Second
	}

	// Remove timestamps outside the current window (sliding window)
	cutoff := now.Add(-window)

	// Find the first timestamp that is still within the window
	validIdx := -1
	for i, ts := range client.msgTimestamps {
		if ts.After(cutoff) {
			validIdx = i
			break
		}
	}

	// Clean up old timestamps
	if validIdx == -1 {
		// All timestamps are expired, release the underlying array for GC
		client.msgTimestamps = nil
	} else if validIdx > 0 {
		// Some timestamps are expired, remove them
		client.msgTimestamps = client.msgTimestamps[validIdx:]
	}

	// Check if rate limit is exceeded
	if len(client.msgTimestamps) >= config.RateLimit {
		return true
	}

	// Add current timestamp
	client.msgTimestamps = append(client.msgTimestamps, now)
	return false
}

// CheckOOCRateLimit checks if the client has exceeded the OOC message rate limit.
// Returns true if rate limit is exceeded and the OOC packet should be dropped, false if the packet is allowed.
// Uses a sliding window approach, mirroring CheckRateLimit.
func (client *Client) CheckOOCRateLimit() bool {
	if config.OOCRateLimit <= 0 {
		return false
	}

	client.mu.Lock()
	defer client.mu.Unlock()

	now := time.Now()
	window := oocRateLimitWindowDur
	if window == 0 {
		window = time.Duration(config.OOCRateLimitWindow) * time.Second
	}

	cutoff := now.Add(-window)

	validIdx := -1
	for i, ts := range client.oocMsgTimestamps {
		if ts.After(cutoff) {
			validIdx = i
			break
		}
	}

	if validIdx == -1 {
		client.oocMsgTimestamps = nil
	} else if validIdx > 0 {
		client.oocMsgTimestamps = client.oocMsgTimestamps[validIdx:]
	}

	if len(client.oocMsgTimestamps) >= config.OOCRateLimit {
		return true
	}

	client.oocMsgTimestamps = append(client.oocMsgTimestamps, now)
	return false
}

// CheckRawPacketRateLimit checks if the client has exceeded the raw packet rate limit.
// This counts every AO2 protocol packet regardless of type (PU, MS, CC, CH, etc.).
// Returns true if the client is flooding (and should be immediately banned), false otherwise.
//
// Uses a fixed-window counter: one int + one time.Time per client, zero heap allocations
// per call. This is called on every single incoming packet, so O(1) cost matters.
//
// Limit semantics: exactly RawPacketRateLimit packets are allowed per window;
// the (RawPacketRateLimit+1)th packet in the same window returns true.
func (client *Client) CheckRawPacketRateLimit() bool {
	if config.RawPacketRateLimit <= 0 {
		return false
	}

	client.mu.Lock()
	defer client.mu.Unlock()

	now := time.Now()
	window := rawPktRateLimitWindowDur
	if window == 0 {
		window = time.Duration(float64(time.Second) * config.RawPacketRateLimitWindow)
	}

	// Reset the counter when the window has expired or on the very first packet
	// (rawPktWindowStart is zero-valued for new clients).
	if client.rawPktWindowStart.IsZero() || now.Sub(client.rawPktWindowStart) >= window {
		client.rawPktWindowStart = now
		client.rawPktCount = 0
	}

	client.rawPktCount++
	return client.rawPktCount > config.RawPacketRateLimit
}

// ConnectedAt returns the time the client joined the server (was assigned a UID).
// Returns a zero Time if the client has not yet joined.
func (client *Client) ConnectedAt() time.Time {
	client.mu.Lock()
	defer client.mu.Unlock()
	return client.connectedAt
}

// SetConnectedAt records the time the client joined the server.
func (client *Client) SetConnectedAt(t time.Time) {
	client.mu.Lock()
	client.connectedAt = t
	client.mu.Unlock()
}

// AcceptedAt returns the raw TCP/WS accept time recorded when this
// connection's *Client was constructed (NewClient), before any handshake
// packet has been read. Unlike ConnectedAt -- only set once RD assigns a UID,
// well after a raid-guard signal like the fast-character-pick one would need
// to measure against -- this covers the whole handshake window.
func (client *Client) AcceptedAt() time.Time {
	client.mu.Lock()
	defer client.mu.Unlock()
	return client.acceptedAt
}

// SetAcceptedAt overrides the recorded accept time. NewClient already sets
// this for every real connection; exposed mainly so tests can backdate it to
// exercise the raid guard's timing signals without a real sleep.
func (client *Client) SetAcceptedAt(t time.Time) {
	client.mu.Lock()
	client.acceptedAt = t
	client.mu.Unlock()
}

// CharPickedAt returns the time of this connection's first character pick, or
// the zero Time if it has not picked one yet.
func (client *Client) CharPickedAt() time.Time {
	client.mu.Lock()
	defer client.mu.Unlock()
	return client.charPickedAt
}

// SetCharPickedAt records a character-pick time. Callers that only want the
// FIRST pick recorded (the raid guard's fast-charpick signal cares about
// "just connected -> already playing", not every later re-pick) guard the
// call with CharPickedAt().IsZero() themselves -- see raidGuardOnCharPick in
// raidguard_wire.go. That check-then-set is race-free because one
// connection's packets are handled strictly sequentially by its own
// HandleClient read loop; nothing else ever writes this field concurrently.
func (client *Client) SetCharPickedAt(t time.Time) {
	client.mu.Lock()
	client.charPickedAt = t
	client.mu.Unlock()
}

// SessionChipsAwarded returns the number of chips already awarded to this client
// by the hourly mid-session ticker during the current connection.
func (client *Client) SessionChipsAwarded() int64 {
	client.mu.Lock()
	defer client.mu.Unlock()
	return client.sessionChipsAwarded
}

// AddSessionChipsAwarded increments the mid-session chip award counter and
// returns the updated total.
func (client *Client) AddSessionChipsAwarded(n int64) int64 {
	client.mu.Lock()
	defer client.mu.Unlock()
	client.sessionChipsAwarded += n
	return client.sessionChipsAwarded
}

// senderBypassesIgnore reports whether a sender's IC/OOC messages should
// override recipients' /ignore lists. Real moderators and administrators
// always reach the whole area so their messages can't be silenced.
//
// Shadow moderators deliberately do NOT bypass ignore: a sender who can't be
// ignored betrays staff status, so to a player who ignores them a shadow mod
// must vanish exactly like any normal player. This is the same anonymity
// contract that hides shadow mods from /gas and /players.
func senderBypassesIgnore(perm uint64) bool {
	return permissions.IsModerator(perm) && !permissions.IsShadow(perm)
}

// IgnoresIPID returns true if this client has permanently ignored the given IPID.
// Uses sync.Map.Load which is essentially lock-free for keys that have been stored
// at least once, keeping the hot per-message path free of mutex contention.
func (client *Client) IgnoresIPID(ipid string) bool {
	_, ok := client.ignoredIPIDs.Load(ipid)
	return ok
}

// AddIgnoredIPID adds an IPID to this client's in-memory permanent ignore set.
func (client *Client) AddIgnoredIPID(ipid string) {
	client.ignoredIPIDs.Store(ipid, struct{}{})
}

// RemoveIgnoredIPID removes an IPID from this client's in-memory permanent ignore set.
func (client *Client) RemoveIgnoredIPID(ipid string) {
	client.ignoredIPIDs.Delete(ipid)
}

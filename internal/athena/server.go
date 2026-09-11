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
	"context"
	"crypto/md5"
	"encoding/base64"
	"fmt"
	"math"
	"math/rand"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/MangosArentLiterature/Athena/internal/area"
	"github.com/MangosArentLiterature/Athena/internal/db"
	discordbot "github.com/MangosArentLiterature/Athena/internal/discord/bot"
	"github.com/MangosArentLiterature/Athena/internal/logger"
	"github.com/MangosArentLiterature/Athena/internal/ms"
	"github.com/MangosArentLiterature/Athena/internal/packet"
	"github.com/MangosArentLiterature/Athena/internal/permissions"
	"github.com/MangosArentLiterature/Athena/internal/playercount"
	"github.com/MangosArentLiterature/Athena/internal/settings"
	"github.com/MangosArentLiterature/Athena/internal/uidmanager"
	"github.com/MangosArentLiterature/Athena/internal/webhook"
	"github.com/ecnepsnai/discord"
	"github.com/xhit/go-str2duration/v2"
	"nhooyr.io/websocket"
)

const (
	version         = "v1.0.2"
	secondsPerHour  = int64(3600) // seconds in one hour; used for playtime-to-chips conversions
	lockdownJoinMsg = "Server is in lockdown. Please try again later."
)

// encodedServerName is the AO2-encoded form of config.Name, pre-computed once
// at startup so that every server message avoids a repeated strings.Replacer call.
// Set in NewServer immediately after config is wired up; never modified afterwards.
var encodedServerName string

// connPool is a semaphore channel that limits the number of concurrently active
// connection goroutines when config.MaxConnectionGoroutines > 0.
// A nil channel means the pool is disabled (unbounded behaviour).
var connPool chan struct{}

// Precomputed rate-limit window durations.  These are derived from config at
// startup and never change; reading a time.Duration is cheaper than multiplying
// an int (or float64) by time.Second on every single incoming packet.
var (
	rateLimitWindowDur       time.Duration
	oocRateLimitWindowDur    time.Duration
	rawPktRateLimitWindowDur time.Duration
	connRateLimitWindowDur   time.Duration
)

var (
	config *settings.Config
	// The character list, music list, background list, parrot list, 8-ball
	// answers, CDN whitelist, the derived name→ID index, the /bglist string and
	// the pre-built SM packet are all hot-reloadable (see /reload) and now live
	// behind atomic.Pointers in livereload.go. Read them via the get* accessors
	// (getCharacters(), getMusicList(), getCDNs(), …) instead of as plain
	// globals, so a runtime swap never races a reader.
	areas          []*area.Area
	areaIndexMap   map[*area.Area]int // pre-computed index lookup for O(1) getAreaIndex
	roles          []permissions.Role
	uids           *uidmanager.UidManager
	players        playercount.PlayerCount
	enableDiscord  bool
	clients        *ClientList = &ClientList{list: make(map[*Client]struct{}), uidIndex: make(map[int]*Client), ipidCounts: make(map[string]int)}
	updatePlayers              = make(chan int)      // Updates the advertiser's player count.
	advertDone                 = make(chan struct{}) // Signals the advertiser to stop.
	FatalError                 = make(chan error)    // Signals that the server should stop after a fatal error.
	RestartRequest             = make(chan struct{}) // Signals that the server should restart.

	// connTracker tracks connection attempts per IP for connection-rate limiting.
	connTracker = struct {
		mu         sync.Mutex
		timestamps map[string][]time.Time // ipid -> connection attempt timestamps
		rejections map[string]int         // ipid -> consecutive rejection count
	}{
		timestamps: make(map[string][]time.Time),
		rejections: make(map[string]int),
	}

	// ipModcallTracker tracks the last modcall time per IP across connections.
	ipModcallTracker = struct {
		mu    sync.Mutex
		times map[string]time.Time // ipid -> last modcall time
	}{
		times: make(map[string]time.Time),
	}

	// ipOOCTracker tracks OOC message timestamps per IP across connections.
	ipOOCTracker = struct {
		mu         sync.Mutex
		timestamps map[string][]time.Time // ipid -> OOC message timestamps
	}{
		timestamps: make(map[string][]time.Time),
	}

	// ipPingTracker tracks ping (CH) packet timestamps per IP across connections.
	ipPingTracker = struct {
		mu         sync.Mutex
		timestamps map[string][]time.Time // ipid -> ping timestamps
	}{
		timestamps: make(map[string][]time.Time),
	}

	// rateLimitKickTracker counts message-based rate-limit kicks (IC/CC/MC/OOC spam --
	// see Client.KickForRateLimit) per IPID, across connections. Used to auto-ban repeat
	// offenders: a bot that just reconnects and immediately spams again right after being
	// kicked racks this up instead of getting an endless kick-reconnect-spam loop.
	rateLimitKickTracker = struct {
		mu     sync.Mutex
		counts map[string]int       // ipid -> kicks within the current spree
		last   map[string]time.Time // ipid -> time of the last counted kick
	}{
		counts: make(map[string]int),
		last:   make(map[string]time.Time),
	}

	// loginAttemptTracker counts failed /login attempts per IPID within a rolling
	// window (see checkLoginRateLimit/registerFailedLogin), so a connection can't
	// just keep guessing passwords against a real account indefinitely. bcrypt's
	// own cost already slows this down, but with no limiter at all a raider running
	// many connections in parallel still has unbounded attempts.
	loginAttemptTracker = struct {
		mu     sync.Mutex
		counts map[string]int       // ipid -> failed attempts within the current window
		last   map[string]time.Time // ipid -> time of the last counted attempt
	}{
		counts: make(map[string]int),
		last:   make(map[string]time.Time),
	}

	// ipFirstSeenTracker records the first time each IPID connected to this server session.
	// Entries are never deleted so that returning IPIDs are never treated as "new" again.
	ipFirstSeenTracker = struct {
		mu    sync.Mutex
		times map[string]time.Time // ipid -> first seen time
	}{
		times: make(map[string]time.Time),
	}

	// globalNewIPTracker records timestamps of new unique IPs connecting to the server.
	// Used for the global new-connection rate limiter: if too many distinct new IPs arrive
	// within the configured window, additional unknown IPs are rejected until the window clears.
	globalNewIPTracker = struct {
		mu         sync.Mutex
		timestamps []time.Time
	}{}

	// serverLockdown, when set to true, prevents all new (previously-unseen) IPIDs from
	// connecting. Known IPIDs (those in ipFirstSeenTracker) are still allowed through.
	serverLockdown atomic.Bool

	// playerLockdownThreshold holds the runtime-adjustable player count cap for the
	// capacity lockdown feature.  When the connected player count reaches this value,
	// new join attempts are rejected.  0 means the threshold is disabled.
	// Mods can change this at runtime with /setplayerlimit.
	playerLockdownThreshold atomic.Int32

	// lockdownMinPlaytime holds the runtime-adjustable minimum total playtime
	// (in seconds) a connected IPID needs to survive the purge that runs when
	// lockdown is switched on. 0 means the purge is disabled.
	// Mods can change this at runtime with /setlockdownplaytime.
	lockdownMinPlaytime atomic.Int64

	// areaLastOOCMsg stores the last OOC message body (raw, as received) sent in each area.
	// Used to prevent consecutive identical OOC messages from different clients in the same area.
	// Key: *area.Area, Value: string. sync.Map is zero-value ready; no initialisation required.
	areaLastOOCMsg sync.Map

	// tormentedIPIDs holds the set of IPIDs that should be periodically disconnected.
	// Populated at startup from the database and updated at runtime by automod.
	// RWMutex is used so that the frequent read path (isIPIDTormented, called per
	// connection) never blocks other concurrent readers.
	tormentedIPIDs = struct {
		mu  sync.RWMutex
		set map[string]struct{}
	}{
		set: make(map[string]struct{}),
	}

	// Tournament mode state
	tournamentActive       bool
	tournamentMutex        sync.Mutex
	tournamentStartTime    time.Time
	tournamentParticipants map[int]*TournamentParticipant // uid -> participant data

	// server is the package-level singleton created by InitServer.
	server *Server
)

// TournamentParticipant tracks a user's tournament performance
type TournamentParticipant struct {
	uid          int
	messageCount int
	joinedAt     time.Time
}

// Server owns the runtime state for an Athena server instance and provides
// a structured API over it. It is created by NewServer and stored as the
// active instance in the package-level server variable. Package-level
// functions (e.g. ListenTCP) delegate to the active server's methods.
//
// Dependency injection: pass a *settings.Config to NewServer; all other
// dependencies (areas, roles, uids, etc.) are wired up inside the constructor.
// The package-level globals are kept in sync so that existing helper functions
// and command handlers continue to operate correctly.
type Server struct {
	config                 *settings.Config
	characters             []string
	music                  []string
	backgrounds            []string
	parrot                 []string
	eightBall              []string
	cdns                   []string
	areas                  []*area.Area
	areaNames              string
	bgListStr              string
	areaIndexMap           map[*area.Area]int
	roles                  []permissions.Role
	uids                   *uidmanager.UidManager
	players                playercount.PlayerCount
	enableDiscord          bool
	clients                *ClientList
	updatePlayers          chan int
	advertDone             chan struct{}
	tournamentActive       bool
	tournamentMutex        sync.Mutex
	tournamentStartTime    time.Time
	tournamentParticipants map[int]*TournamentParticipant
}

// NewServer initializes a new Server from the provided configuration, wiring
// up all dependencies (database, areas, roles, uid heap, advertiser, etc.).
// It also populates the package-level global variables so that existing helper
// functions and command handlers continue to operate correctly.
// Call InitServer for the legacy single-process entry point.
func NewServer(conf *settings.Config) (*Server, error) {
	db.Open()
	// Remove expired punishment rows left over from previous sessions.
	// A failure here is non-fatal: expired rows are harmless (GetPunishments filters
	// them at read-time), so we log and continue rather than aborting startup.
	if err := db.PurgeExpired(); err != nil {
		logger.LogErrorf("Failed to purge expired punishments: %v", err)
	}

	// Prune KNOWN_IPS rows with less than 1 hour of accumulated playtime.
	// IPs that have played for at least an hour are retained permanently.
	// This keeps bots and drive-by connections from polluting the known-IP list.
	const minPlaytimeSeconds int64 = 3600
	if n, err := db.PruneShortPlaytimeIPs(minPlaytimeSeconds); err != nil {
		logger.LogErrorf("Failed to prune low-playtime IPs: %v", err)
	} else if n > 0 {
		logger.LogInfof("Pruned %d IP(s) with less than 1 hour of playtime.", n)
	}

	// Sync chip balances for players who accumulated playtime before the casino
	// was introduced. Uses INSERT OR IGNORE so existing balances are never touched.
	if conf.EnableCasino {
		if n, err := db.SyncChipsForExistingPlaytime(); err != nil {
			logger.LogErrorf("Failed to sync chips for pre-casino playtime: %v", err)
		} else if n > 0 {
			logger.LogInfof("Synced chip balances for %d IP(s) with pre-casino playtime.", n)
		}
	}

	// Pre-populate the in-memory first-seen tracker with IPs that were seen in
	// previous server sessions.  This ensures returning players are never treated
	// as "new" by the global new-IP rate limiter after a restart.
	if knownIPs, err := db.LoadKnownIPs(); err != nil {
		logger.LogErrorf("Failed to load known IPs from database: %v", err)
	} else {
		// Use the zero time so that any per-IP cooldowns (OOC, modcall) are
		// considered long-expired for these returning players.
		epoch := time.Unix(0, 0)
		ipFirstSeenTracker.mu.Lock()
		for _, ipid := range knownIPs {
			if _, exists := ipFirstSeenTracker.times[ipid]; !exists {
				ipFirstSeenTracker.times[ipid] = epoch
			}
		}
		ipFirstSeenTracker.mu.Unlock()
		if len(knownIPs) > 0 {
			logger.LogInfof("Loaded %d known IPs from database.", len(knownIPs))
		}
	}

	// Account command grants: load console-issued grants (grantcmd) so the
	// in-memory cache the dispatch hot path reads is populated before the
	// first connection can send a command. A failure here is non-fatal --
	// it just means grants aren't checked until "reload" or the next
	// grantcmd/revokecmd is run, which itself always reloads the cache.
	if err := loadCommandGrants(); err != nil {
		logger.LogErrorf("Failed to load account command grants from database: %v", err)
	}

	// Join captcha: load stored verifications, the operator question file and
	// the captcha plugin (each is a no-op when the feature is unconfigured).
	seedJoinCaptchaVerified()
	initCustomChallenges()
	initCaptchaPlugin()
	if bad := validateJoinCaptchaKinds(conf.JoinCaptchaKinds); len(bad) > 0 {
		logger.LogWarningf("config: join_captcha_kinds contains unknown challenge kind(s) %v -- valid kinds are %v",
			bad, joinChallengeKindList())
	}

	// Pre-populate the in-memory tormented IPID set from the database.
	if tormentedIPs, err := db.LoadTormentedIPs(); err != nil {
		logger.LogErrorf("Failed to load tormented IPs from database: %v", err)
	} else {
		tormentedIPIDs.mu.Lock()
		for _, ipid := range tormentedIPs {
			tormentedIPIDs.set[ipid] = struct{}{}
		}
		tormentedIPIDs.mu.Unlock()
		if len(tormentedIPs) > 0 {
			logger.LogInfof("Loaded %d tormented IP(s) from database.", len(tormentedIPs))
		}
	}

	s := &Server{
		config:                 conf,
		clients:                &ClientList{list: make(map[*Client]struct{}), uidIndex: make(map[int]*Client), ipidCounts: make(map[string]int)},
		uids:                   &uidmanager.UidManager{},
		updatePlayers:          updatePlayers,
		advertDone:             advertDone,
		tournamentParticipants: make(map[int]*TournamentParticipant),
	}

	s.uids.InitHeap(conf.MaxPlayers)

	// Load server data.
	var err error
	s.music, err = settings.LoadMusic()
	if err != nil {
		return nil, err
	}
	s.characters, err = settings.LoadFile("/characters.txt")
	if err != nil {
		return nil, err
	} else if len(s.characters) == 0 {
		return nil, fmt.Errorf("empty character list")
	}
	areaData, err := settings.LoadAreas()
	if err != nil {
		return nil, err
	}

	s.roles, err = settings.LoadRoles()
	if err != nil {
		return nil, err
	}

	s.backgrounds, err = settings.LoadFile("/backgrounds.txt")
	if err != nil {
		return nil, err
	} else if len(s.backgrounds) == 0 {
		return nil, fmt.Errorf("empty background list")
	}
	var bgBuilder strings.Builder
	bgBuilder.Grow(len("Available backgrounds:\n") + estimateJoinedLen(s.backgrounds))
	bgBuilder.WriteString("Available backgrounds:\n")
	for i, bg := range s.backgrounds {
		if i > 0 {
			bgBuilder.WriteByte('\n')
		}
		bgBuilder.WriteString(bg)
	}
	s.bgListStr = bgBuilder.String()

	// Build O(1) background lookup set for area validation.
	bgSet := make(map[string]struct{}, len(s.backgrounds))
	for _, bg := range s.backgrounds {
		bgSet[bg] = struct{}{}
	}

	s.parrot, err = settings.LoadFile("/parrot.txt")
	if err != nil {
		return nil, err
	} else if len(s.parrot) == 0 {
		return nil, fmt.Errorf("empty parrot list")
	}

	// 8ball.txt is optional. When the file is missing or empty, /8ball falls
	// back to a hard-coded default answer set so the command always works.
	if loaded, eerr := settings.LoadFile("/8ball.txt"); eerr == nil {
		s.eightBall = loaded
	}
	s.cdns = settings.LoadCDNs()
	_, err = str2duration.ParseDuration(conf.BanLen)
	if err != nil {
		return nil, fmt.Errorf("failed to parse default_ban_duration: %v", err.Error())
	}
	_, err = str2duration.ParseDuration(conf.RateLimitKickAutobanDuration)
	if err != nil {
		return nil, fmt.Errorf("failed to parse rate_limit_kick_autoban_duration: %v", err.Error())
	}

	// Webhook setup: set server name once for all webhook functions.
	webhook.ServerName = conf.Name

	// Discord webhook.
	if conf.WebhookURL != "" {
		s.enableDiscord = true
		webhook.PingRoleID = conf.WebhookPingRoleID
		discord.WebhookURL = conf.WebhookURL
	}

	// Punishment webhook (ban/kick logging).
	if conf.PunishmentWebhookURL != "" {
		webhook.PunishmentWebhookURL = conf.PunishmentWebhookURL
	}

	// Load areas.
	s.areas = make([]*area.Area, 0, len(areaData))
	var areaNameBuilder strings.Builder
	for i, a := range areaData {
		if i > 0 {
			areaNameBuilder.WriteByte('#')
		}
		areaNameBuilder.WriteString(a.Name)
		var evi_mode area.EvidenceMode
		switch strings.ToLower(a.Evi_mode) {
		case "any":
			evi_mode = area.EviAny
		case "cms":
			evi_mode = area.EviCMs
		case "mods":
			evi_mode = area.EviMods
		default:
			logger.LogWarningf("Area %v has an invalid or undefined evidence mode, defaulting to 'cms'.", a.Name)
			evi_mode = area.EviCMs
		}
		if _, validBg := bgSet[a.Bg]; a.Bg == "" || !validBg {
			logger.LogWarningf("Area %v has an invalid or undefined background, defaulting to 'default'.", a.Name)
			a.Bg = "default"
		}
		s.areas = append(s.areas, area.NewAreaWithVoiceDefault(a, len(s.characters), conf.BufSize, evi_mode, conf.DefaultAreaVoiceAllowed))
	}
	s.areaNames = areaNameBuilder.String()

	// Build O(1) area-index lookup map.
	s.areaIndexMap = make(map[*area.Area]int, len(s.areas))
	for i, a := range s.areas {
		s.areaIndexMap[a] = i
	}

	// Initialize area logging if enabled.
	//
	// Directories are created from each area's configured (DefaultName), not
	// its current display name, because that is also what every log write
	// keys on -- see the matching WriteAreaLog call below. At startup the two
	// are always identical anyway (no rename has happened yet), but naming it
	// DefaultName here documents that a later /area rename must never cause
	// this to run again for a second, renamed directory.
	logger.EnableAreaLogging = conf.EnableAreaLogging
	if logger.EnableAreaLogging {
		logger.LogInfo("Area logging is enabled. Creating area log directories...")
		for _, a := range s.areas {
			if err := logger.CreateAreaLogDirectory(a.DefaultName()); err != nil {
				logger.LogErrorf("Failed to create area log directory for %v: %v", a.DefaultName(), err)
			}
		}
	}

	// Initialize network logging if enabled.
	logger.EnableNetworkLogging = conf.EnableNetworkLogging
	if logger.EnableNetworkLogging {
		logger.LogInfo("Network logging is enabled. All packets will be logged to network.log.")
	}

	if conf.Advertise {
		advert := ms.Advertisement{
			Port:    conf.Port,
			Players: s.players.GetPlayerCount(),
			Name:    conf.Name,
			Desc:    conf.Desc}
		if conf.AdvertiseHostname != "" {
			advert.IP = conf.AdvertiseHostname
		}
		if conf.EnableWS {
			if conf.ReverseProxyMode {
				advert.WSPort = conf.ReverseProxyHTTPPort
			} else {
				advert.WSPort = conf.WSPort
			}
		}
		if conf.EnableWSS {
			if conf.ReverseProxyMode {
				advert.WSSPort = conf.ReverseProxyHTTPSPort
			} else {
				advert.WSSPort = conf.WSSPort
			}
		}
		go ms.Advertise(conf.MSAddr, advert, updatePlayers, advertDone)
	}

	// Propagate to package-level globals so that existing helper functions
	// and command handlers continue to work without modification.
	config = s.config
	encodedServerName = encode(s.config.Name) // cache once; config.Name never changes at runtime
	// Precompute rate-limit windows once so the hot packet-check paths only
	// perform a load instead of a multiply on every incoming packet.
	rateLimitWindowDur = time.Duration(config.RateLimitWindow) * time.Second
	oocRateLimitWindowDur = time.Duration(config.OOCRateLimitWindow) * time.Second
	rawPktRateLimitWindowDur = time.Duration(float64(time.Second) * config.RawPacketRateLimitWindow)
	connRateLimitWindowDur = time.Duration(config.ConnRateLimitWindow) * time.Second
	// Publish the hot-reloadable data behind their atomic.Pointers. setCharacters
	// derives the name→ID index and setBackgrounds derives the /bglist string, so
	// neither needs to be assigned separately here. These are read at runtime via
	// the get* accessors and can later be swapped atomically by /reload.
	setCharacters(s.characters)
	setMusicList(s.music)
	setBackgrounds(s.backgrounds)
	setParrotList(s.parrot)
	setEightBall(s.eightBall)
	setCDNs(s.cdns)
	areas = s.areas
	setAreaNames(s.areaNames)
	areaIndexMap = s.areaIndexMap
	roles = s.roles
	uids = s.uids
	enableDiscord = s.enableDiscord
	tournamentParticipants = s.tournamentParticipants

	// Pre-build the SM packet (sent to every client on join) once at startup
	// so that pktReqAM performs a single write with no allocations. Rebuilt by
	// /reload when music.txt changes.
	setSMPacket(buildSMPacket(s.areaNames, s.music))

	// Dump the decoded [Voice] config so operators can confirm at a glance
	// whether the section was loaded.  If voice_allowed appears off here even
	// though config.toml has enable_voice = true, the section name or struct
	// tags are mismatched and VS_CAPS will go out as "voice disabled".
	LogVoiceConfig()

	initCommands()
	validateCommands()
	initAutoMod(conf)
	initShownameCensor()
	initShownamePunisher()
	initFromSoftWords()
	initCvote(conf)
	initHotConfig(conf)
	initMusicBans()
	initShadowDisconnects()
	initLockdownExempt()
	// Initialise the goroutine pool if a limit is configured.
	if conf.MaxConnectionGoroutines > 0 {
		connPool = make(chan struct{}, conf.MaxConnectionGoroutines)
	} else {
		connPool = nil
	}
	// Initialize the player-capacity lockdown threshold from config.
	playerLockdownThreshold.Store(int32(conf.PlayerLockdownThreshold))
	// Raid guard's master switch (see raidguard.go). Off by default; every hot
	// IC/OOC/handshake hook gates on this single atomic load first.
	raidGuardActive.Store(conf.RaidGuardEnabled)
	// Initialize the lockdown min-playtime purge threshold from config (minutes -> seconds).
	lockdownMinPlaytime.Store(int64(conf.LockdownMinPlaytime) * 60)
	go startConnTrackerCleanup()
	if conf.EnableCasino {
		go startHourlyChipAward()
		go startUnscrambleLoop()
	}
	if conf.EnableNewspaper {
		go startNewspaperLoop()
	}
	return s, nil
}

// InitServer initializes the server and stores it as the package-level singleton.
// It is the legacy entry point used by the main package; callers that need to
// manage the server lifecycle directly should use NewServer instead.
func InitServer(conf *settings.Config) error {
	var err error
	server, err = NewServer(conf)
	return err
}

// StartDiscordBot starts the Discord bot if a token is configured.
// It should be called after InitServer.
func (s *Server) StartDiscordBot() {
	if s.config.BotToken == "" {
		return
	}
	cfg := discordbot.Config{
		Token:     s.config.BotToken,
		GuildID:   s.config.GuildID,
		ModRoleID: s.config.ModRoleID,
	}
	b, err := discordbot.New(cfg, NewServerAdapter())
	if err != nil {
		logger.LogErrorf("Failed to create Discord bot: %v", err)
		return
	}
	if err := b.Start(); err != nil {
		logger.LogErrorf("Failed to start Discord bot: %v", err)
		return
	}
	logger.LogInfo("Discord bot started.")
}

// StartDiscordBot starts the Discord bot on the active server instance.
// Kept for backward compatibility; delegates to server.StartDiscordBot.
func StartDiscordBot() { server.StartDiscordBot() }

// ListenTCP starts the server's TCP listener.
func (s *Server) ListenTCP() {
	listener, err := net.Listen("tcp", config.Addr+":"+strconv.Itoa(config.Port))
	if err != nil {
		FatalError <- err
		return
	}
	logger.LogInfo("TCP listener started.")
	defer listener.Close()

	for {
		conn, err := listener.Accept()
		if err != nil {
			logger.LogError(err.Error())
		}
		rawAddr := conn.RemoteAddr().String()
		ipid := getIpid(rawAddr)
		if isShadowDisconnected(ipid) {
			// No packet, no reason -- an instant, silent drop that looks
			// exactly like a network failure or a server that isn't there,
			// not a deliberate rejection.
			logger.LogInfof("Connection from %v rejected (shadow-disconnected)", ipid)
			conn.Close()
			continue
		}
		if reject, autoban := checkConnRateLimit(ipid); reject {
			logger.LogInfof("Connection from %v rejected (connection rate limit exceeded)", ipid)
			if autoban {
				go autobanFlooder(ipid, "connection flooding")
			}
			conn.Close()
			continue
		}
		if checkGlobalNewIPRateLimit(ipid) {
			if lockdownReject := serverLockdownRejection(ipid); lockdownReject {
				logger.LogInfof("Connection from new IP %v rejected (server lockdown active)", ipid)
				NewClient(conn, ipid).SendSync(&packet.BD{Reason: lockdownRejectionMessage(lockdownJoinMsg, ipid)})
			} else {
				logger.LogInfof("Connection from new IP %v rejected (global new IP rate limit exceeded)", ipid)
			}
			conn.Close()
			continue
		}
		// The firewall check may block on a network round-trip to IPHub.
		// Dispatch everything after the fast in-memory checks into its own
		// goroutine so the accept loop is never stalled waiting for the API.
		go acceptTCPConnection(conn, extractIP(rawAddr), ipid)
	}
}

// acceptTCPConnection completes the setup for a single accepted TCP connection.
// It runs in its own goroutine so that an IPHub API call (when the firewall is
// active) never stalls the ListenTCP accept loop.
// HandleClient is called without `go` because this goroutine IS the connection
// goroutine — it blocks for the lifetime of the connection, which is the
// standard one-goroutine-per-connection pattern in Go.
func acceptTCPConnection(conn net.Conn, rawIP, ipid string) {
	// Acquire a pool slot when the goroutine pool is enabled.  This blocks
	// until a slot is free, bounding the total number of active connections
	// that are in the "setup + serve" phase at any moment.
	if connPool != nil {
		connPool <- struct{}{}
		defer func() { <-connPool }()
	}
	if checkFirewallForIP(rawIP, ipid) {
		logger.LogInfof("Connection from %v rejected (VPN/proxy detected by IPHub firewall)", ipid)
		conn.Close()
		return
	}
	isNewIPID := recordIPFirstSeen(ipid)
	// Persist the IP and update its last-seen timestamp for all connections
	// (new and returning). The upsert keeps FIRST_SEEN intact for existing rows.
	go func() {
		if err := db.MarkIPKnown(ipid); err != nil {
			logger.LogErrorf("Failed to update known IP %s: %v", ipid, err)
		}
	}()
	client := NewClient(conn, ipid)
	client.isNewIPID = isNewIPID
	client.HandleClient()
}

// ListenTCP starts the TCP listener on the active server instance.
// Kept for backward compatibility; delegates to server.ListenTCP.
func ListenTCP() { server.ListenTCP() }

// ListenWS starts the server's websocket listener.
func (s *Server) ListenWS() {
	listener, err := net.Listen("tcp", config.Addr+":"+strconv.Itoa(config.WSPort))
	if err != nil {
		FatalError <- err
		return
	}
	logger.LogInfo("WS listener started.")
	defer listener.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/", HandleWS)
	srv := &http.Server{
		Handler: mux,
	}
	err = srv.Serve(listener)
	if err != http.ErrServerClosed {
		FatalError <- err
	}
}

// ListenWS starts the WebSocket listener on the active server instance.
// Kept for backward compatibility; delegates to server.ListenWS.
func ListenWS() { server.ListenWS() }

// ListenWSS starts the server's secure websocket listener.
// If TLS certificate and key paths are provided, it serves with TLS (direct HTTPS).
// If not provided, it serves plain HTTP (useful when behind a reverse proxy like Cloudflare).
func (s *Server) ListenWSS() {
	listener, err := net.Listen("tcp", config.Addr+":"+strconv.Itoa(config.WSSPort))
	if err != nil {
		FatalError <- err
		return
	}
	logger.LogInfo("WSS listener started.")
	defer listener.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/", HandleWS)
	srv := &http.Server{
		Handler: mux,
	}

	// Use TLS if certificate and key paths are provided, otherwise serve plain HTTP
	// (useful when behind a reverse proxy that handles TLS termination)
	if config.TLSCertPath != "" && config.TLSKeyPath != "" {
		logger.LogInfof("WSS using TLS with cert: %s", config.TLSCertPath)
		err = srv.ServeTLS(listener, config.TLSCertPath, config.TLSKeyPath)
	} else {
		logger.LogInfo("WSS using plain HTTP (expecting reverse proxy for TLS)")
		err = srv.Serve(listener)
	}

	if err != http.ErrServerClosed {
		FatalError <- err
	}
}

// ListenWSS starts the secure WebSocket listener on the active server instance.
// Kept for backward compatibility; delegates to server.ListenWSS.
func ListenWSS() { server.ListenWSS() }

// webaoAcceptOptions returns the nhooyr AcceptOptions used for every WebAO
// connection. Compression (permessage-deflate, RFC 7692) is explicitly
// disabled: iPadOS / Safari WebKit ship a permessage-deflate decompressor
// that aborts the connection on large compressed payloads (the AO2 SM music
// list and CT MOTD packets are the consistent triggers in production), so
// enabling it — which is nhooyr's default — guarantees a 1006 close partway
// through the handshake for those clients. Disabling it makes the server
// advertise no extensions, the browser skips its decompressor entirely, and
// the wire-format hit (no deflate) is irrelevant because TCP already
// segments large frames across MTU-sized packets.
func webaoAcceptOptions() *websocket.AcceptOptions {
	return &websocket.AcceptOptions{
		OriginPatterns:  []string{config.WebAOAllowedOrigin},
		CompressionMode: websocket.CompressionDisabled,
	}
}

// HandleWS handles a websocket connection.
func HandleWS(w http.ResponseWriter, r *http.Request) {
	rawIP := getRealIP(r)
	ipid := getIpid(rawIP)
	if isShadowDisconnected(ipid) {
		// Refuse the WebSocket upgrade outright, with a generic response and
		// no body -- to a browser this reads as an ordinary failed handshake
		// ("Error during WebSocket handshake: Unexpected response code..."),
		// not a deliberate, explained rejection.
		logger.LogInfof("Connection from %v rejected (shadow-disconnected)", ipid)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if reject, autoban := checkConnRateLimit(ipid); reject {
		logger.LogInfof("Connection from %v rejected (connection rate limit exceeded)", ipid)
		if autoban {
			go autobanFlooder(ipid, "connection flooding")
		}
		http.Error(w, "Too many connections", http.StatusTooManyRequests)
		return
	}
	// Check if the IP is banned before consuming a global new-IP rate limit slot.
	// Banned clients that repeatedly reconnect must not exhaust the limit and
	// block legitimate new users from joining.
	if banned, _, err := db.IsBanned(db.IPID, ipid); err != nil {
		logger.LogErrorf("Failed to check IP ban for %v: %v", ipid, err)
	} else if banned {
		c, wsErr := websocket.Accept(w, r, webaoAcceptOptions())
		if wsErr != nil {
			logger.LogError(wsErr.Error())
			return
		}
		client := NewClient(websocket.NetConn(r.Context(), c, websocket.MessageText), ipid)
		client.CheckBanned(db.IPID)
		return
	}
	if checkGlobalNewIPRateLimit(ipid) {
		if lockdownReject := serverLockdownRejection(ipid); lockdownReject {
			logger.LogInfof("Connection from new IP %v rejected (server lockdown active)", ipid)
			c, wsErr := websocket.Accept(w, r, webaoAcceptOptions())
			if wsErr != nil {
				logger.LogError(wsErr.Error())
				http.Error(w, lockdownRejectionMessage(lockdownJoinMsg, ipid), http.StatusServiceUnavailable)
				return
			}
			client := NewClient(websocket.NetConn(r.Context(), c, websocket.MessageText), ipid)
			client.SendSync(&packet.BD{Reason: lockdownRejectionMessage(lockdownJoinMsg, ipid)})
			client.conn.Close()
		} else {
			logger.LogInfof("Connection from new IP %v rejected (global new IP rate limit exceeded)", ipid)
			http.Error(w, "Too many connections", http.StatusTooManyRequests)
		}
		return
	}
	if checkFirewallForIP(extractIP(rawIP), ipid) {
		logger.LogInfof("Connection from %v rejected (VPN/proxy detected by IPHub firewall)", ipid)
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	isNewIPID := recordIPFirstSeen(ipid)
	// Persist the IP and update its last-seen timestamp for all connections
	// (new and returning). The upsert keeps FIRST_SEEN intact for existing rows.
	go func(id string) {
		if err := db.MarkIPKnown(id); err != nil {
			logger.LogErrorf("Failed to update known IP %s: %v", id, err)
		}
	}(ipid)
	c, err := websocket.Accept(w, r, webaoAcceptOptions())
	if err != nil {
		logger.LogError(err.Error())
		return
	}
	client := NewClient(websocket.NetConn(context.TODO(), c, websocket.MessageText), ipid)
	client.isNewIPID = isNewIPID
	go client.HandleClient()
}

// CleanupServer closes all connections to the server and closes the database.
func (s *Server) CleanupServer() {
	clients.ForEach(func(client *Client) {
		client.conn.Close()
	})
	db.Close()
	logger.CloseLogFiles()
}

// CleanupServer closes all connections on the active server instance.
// Kept for backward compatibility; delegates to server.CleanupServer.
func CleanupServer() { server.CleanupServer() }

// RequestRestart signals the main process to restart the server.
func RequestRestart() {
	RestartRequest <- struct{}{}
}

// writeToAll sends a message to all connected clients.
func writeToAll(header string, contents ...string) {
	clients.ForEach(func(client *Client) {
		if client.Uid() != -1 {
			client.SendPacket(header, contents...)
		}
	})
}

// writeToArea sends a message to all clients in a given area.
func writeToArea(area *area.Area, header string, contents ...string) {
	clients.ForEach(func(client *Client) {
		if client.Area() == area {
			client.SendPacket(header, contents...)
		}
	})
}

// writeToAreaFrom sends a message to all clients in a given area, skipping
// any recipient that has permanently ignored the sender's IPID.
// If senderIsMod is true the ignore list is bypassed so moderator messages
// always reach every client in the area.
func writeToAreaFrom(senderIPID string, senderIsMod bool, area *area.Area, header string, contents ...string) {
	clients.ForEach(func(client *Client) {
		if client.Area() == area && (senderIsMod || !client.IgnoresIPID(senderIPID)) {
			client.SendPacket(header, contents...)
		}
	})
}

// writeToAllClients writes a packet to all connected clients
func writeToAllClients(header string, contents ...string) {
	clients.ForEach(func(client *Client) {
		client.SendPacket(header, contents...)
	})
}

// broadcastToAll fans a typed packet to every UID-registered client.
// Args() is invoked exactly once and the resulting slice is reused for
// every recipient.
func broadcastToAll(p packet.Outgoing) {
	// Name-carrying PU packets are filtered here rather than at each of the
	// dozen call sites that emit them -- see pu_name_filter.go for why the
	// point of exposure is the only place a name filter cannot be bypassed by
	// a code path nobody thought about. Checked once, before the fan-out,
	// rather than per recipient.
	if !puAllowed(p) {
		return
	}
	header, args := p.Header(), p.Args()
	clients.ForEach(func(client *Client) {
		if client.Uid() != -1 {
			client.SendPacket(header, args...)
		}
	})
}

// broadcastToArea fans a typed packet to every client in the given area.
func broadcastToArea(area *area.Area, p packet.Outgoing) {
	header, args := p.Header(), p.Args()
	clients.ForEach(func(client *Client) {
		if client.Area() == area {
			client.SendPacket(header, args...)
		}
	})
}

// broadcastToAreaFrom fans a typed packet to an area, honoring per-recipient
// ignore lists unless the sender is a moderator.
func broadcastToAreaFrom(senderIPID string, senderIsMod bool, area *area.Area, p packet.Outgoing) {
	header, args := p.Header(), p.Args()
	clients.ForEach(func(client *Client) {
		if client.Area() == area && (senderIsMod || !client.IgnoresIPID(senderIPID)) {
			client.SendPacket(header, args...)
		}
	})
}

// broadcastOOCToArea fans a plain area-OOC-chat packet to an area, honoring
// per-recipient ignore lists (unless the sender is a moderator) exactly like
// broadcastToAreaFrom, plus each recipient's /toggle global OOC-hide
// preference. Used only for ordinary local OOC chat -- PMs and staff/system
// broadcasts go through broadcastToAreaFrom/broadcastToArea directly so this
// toggle can never hide something a player was specifically sent or
// something staff need seen.
func broadcastOOCToArea(senderIPID string, senderIsMod bool, area *area.Area, p packet.Outgoing) {
	header, args := p.Header(), p.Args()
	clients.ForEach(func(client *Client) {
		if client.Area() == area && (senderIsMod || !client.IgnoresIPID(senderIPID)) && !client.OOCHidden() {
			client.SendPacket(header, args...)
		}
	})
}

// broadcastOOCToAll fans a player-initiated /global OOC broadcast to every
// UID-registered client, skipping anyone who has muted OOC chat via /toggle
// global. Used only for /global -- moderator broadcasts (/mod -g,
// /modchat) and system announcements go through broadcastToAll directly so
// they are never hidden by this toggle.
func broadcastOOCToAll(p packet.Outgoing) {
	if !puAllowed(p) {
		return
	}
	header, args := p.Header(), p.Args()
	clients.ForEach(func(client *Client) {
		if client.Uid() != -1 && !client.OOCHidden() {
			client.SendPacket(header, args...)
		}
	})
}

// broadcastToAllClients fans a typed packet to every connected client,
// including those that haven't yet been assigned a UID.
func broadcastToAllClients(p packet.Outgoing) {
	if !puAllowed(p) {
		return
	}
	header, args := p.Header(), p.Args()
	clients.ForEach(func(client *Client) {
		client.SendPacket(header, args...)
	})
}

// logBufPool provides reusable *strings.Builder instances for addToBuffer,
// avoiding a builder allocation on every logged message (every IC, OOC, etc.).
var logBufPool = sync.Pool{
	New: func() any { b := new(strings.Builder); b.Grow(256); return b },
}

// addToBuffer writes to an area buffer according to a client's action.
// All client fields are read in one logSnapshot call (one mutex acquisition)
// rather than via individual getters, which would each acquire and release the
// client lock separately.
func addToBuffer(client *Client, action string, message string, audit bool) {
	now := time.Now().UTC().Format("15:04:05")
	snap := client.logSnapshot()

	b := logBufPool.Get().(*strings.Builder)
	b.Reset()
	b.WriteString(now)
	b.WriteString(" | ")
	b.WriteString(action)
	b.WriteString(" | ")
	b.WriteString(snap.charName)
	b.WriteString(" | ")
	b.WriteString(snap.ipid)
	b.WriteString(" | ")
	b.WriteString(snap.oocName)
	b.WriteString(" | ")
	b.WriteString(message)
	s := b.String()
	// Only add to the in-memory buffer when log silencing is not active, so that
	// messages sent during a silence window are never included in modcall reports
	// or webhook posts.
	if !snap.area.LogSilenced() {
		snap.area.UpdateBuffer(s)
	}

	// Write to area-specific log file if area logging is enabled and not silenced.
	if logger.EnableAreaLogging && !snap.area.LogSilenced() {
		b.Reset()
		b.WriteByte('[')
		b.WriteString(now)
		b.WriteString("] | ")
		b.WriteString(action)
		b.WriteString(" | ")
		b.WriteString(snap.charName)
		b.WriteString(" | ")
		b.WriteString(snap.ipid)
		b.WriteString(" | ")
		b.WriteString(snap.hdid)
		b.WriteString(" | ")
		b.WriteString(snap.showname)
		b.WriteString(" | ")
		b.WriteString(snap.oocName)
		b.WriteString(" | ")
		b.WriteString(message)
		// Keyed by the area's configured name (areas.toml), not its current
		// display name -- a live /area rename must never fork the log into a
		// second directory. "Teto Cafe" renamed to "Miku Cafe" keeps writing
		// into Teto Cafe/Teto Cafe-<date>.txt the whole time it's renamed; the
		// rename itself is still visible in-line as a CMD log entry (see
		// cmdAreaRename), so nothing about the change is lost, it just never
		// gets its own folder.
		logger.WriteAreaLog(snap.area.DefaultName(), b.String())
	}

	logBufPool.Put(b)

	if audit {
		logger.WriteAudit(s)
	}
}

// getAreaIndex returns the index of a given area in the areas slice.
// All areas come from the global slice initialised at startup, so the map
// always contains every valid *Area pointer.  A missing key returns 0,
// which matches the historic fallback behaviour.
func getAreaIndex(a *area.Area) int {
	return areaIndexMap[a]
}

// sendPlayerListToClient sends PR and PU packets for all currently joined players to a new client.
func sendPlayerListToClient(newClient *Client) {
	clients.ForEach(func(c *Client) {
		if c.Uid() == -1 || c == newClient || c.Hidden() {
			return
		}
		uid := c.Uid()
		newClient.Send(&packet.PR{ID: uid, Type: 0})
		// Filtered like any other name broadcast. This is the path that made
		// the filter necessary: a name set before a word was added to the list
		// is otherwise re-sent, in full, to every person who joins from then
		// on, long after the input-time check stopped running.
		if n := (&packet.PU{ID: uid, Type: 0, Data: c.OOCName()}); c.OOCName() != "" && puAllowed(n) {
			newClient.Send(n)
		}
		newClient.Send(&packet.PU{ID: uid, Type: 1, Data: c.CurrentCharacter()})
		if n := (&packet.PU{ID: uid, Type: 2, Data: decode(c.Showname())}); puAllowed(n) {
			newClient.Send(n)
		}
		newClient.Send(&packet.PU{ID: uid, Type: 3, Data: strconv.Itoa(getAreaIndex(c.Area()))})
	})
}

// broadcastPlayerJoin sends PR and PU packets to all clients when a new player joins.
// Hidden players are not broadcast.
func broadcastPlayerJoin(client *Client) {
	if client.Hidden() {
		return
	}
	uid := client.Uid()
	broadcastToAll(&packet.PR{ID: uid, Type: 0})
	if client.OOCName() != "" {
		broadcastToAll(&packet.PU{ID: uid, Type: 0, Data: client.OOCName()})
	}
	broadcastToAll(&packet.PU{ID: uid, Type: 1, Data: client.CurrentCharacter()})
	broadcastToAll(&packet.PU{ID: uid, Type: 2, Data: decode(client.Showname())})
	broadcastToAll(&packet.PU{ID: uid, Type: 3, Data: strconv.Itoa(getAreaIndex(client.Area()))})
}

// broadcastIPIDToMods sends a player's IPID (PU type 4) to every currently
// connected client the server has itself verified holds BAN_INFO —
// moderators. This must never go through broadcastToAll: that fans one
// identical packet to every client with no per-recipient check, which would
// leak IPID to non-moderators regardless of client-side display intent.
// Mirrors the same per-viewer permission gate /gas already uses
// (hasBanInfo in cmdPlayers) before including IPID in its own reply.
func broadcastIPIDToMods(uid int, ipid string) {
	clients.ForEach(func(c *Client) {
		if c.Uid() != -1 && permissions.HasPermission(c.Perms(), permissions.PermissionField["BAN_INFO"]) {
			c.Send(&packet.PU{ID: uid, Type: 4, Data: ipid})
		}
	})
}

// sendModIPIDsToClient backfills every currently-joined, non-hidden player's
// IPID to newClient — but only if newClient itself is server-verified to
// hold BAN_INFO; it's a no-op otherwise. Called both when a
// BAN_INFO-permissioned client finishes joining and whenever a connection is
// granted BAN_INFO mid-session (/login, /setrole), so a moderator's live
// roster never needs a /gas poll to fill in IPID.
func sendModIPIDsToClient(newClient *Client) {
	if !permissions.HasPermission(newClient.Perms(), permissions.PermissionField["BAN_INFO"]) {
		return
	}
	clients.ForEach(func(c *Client) {
		if c.Uid() == -1 || c == newClient || c.Hidden() {
			return
		}
		newClient.Send(&packet.PU{ID: c.Uid(), Type: 4, Data: c.Ipid()})
	})
}

// sendPlayerArup sends a player ARUP to all connected clients.
// Visible (non-hidden) player counts are read from each area's pre-maintained counter.
func sendPlayerArup() {
	plCounts := make([]string, 0, len(areas))
	for _, a := range areas {
		plCounts = append(plCounts, strconv.Itoa(a.VisiblePlayerCount()))
	}
	broadcastToAll(&packet.ARUP{Type: packet.ARUPPlayerCounts, Data: plCounts})
}

// sendCMArup sends a CM ARUP to all connected clients.
func sendCMArup() {
	returnL := make([]string, 0, len(areas))
	for _, a := range areas {
		cmUIDs := a.CMs()
		if len(cmUIDs) == 0 {
			returnL = append(returnL, "FREE")
			continue
		}
		cms := make([]string, 0, len(cmUIDs))
		for _, u := range cmUIDs {
			c, err := getClientByUid(u)
			if err != nil {
				continue
			}
			cms = append(cms, fmt.Sprintf("%v (%v)", c.CurrentCharacter(), u))
		}
		returnL = append(returnL, strings.Join(cms, ", "))
	}
	broadcastToAll(&packet.ARUP{Type: packet.ARUPCMs, Data: returnL})
}

// sendStatusArup sends a status ARUP to all connected clients.
func sendStatusArup() {
	statuses := make([]string, 0, len(areas))
	for _, a := range areas {
		statuses = append(statuses, a.Status().String())
	}
	broadcastToAll(&packet.ARUP{Type: packet.ARUPStatuses, Data: statuses})
}

// areaLockDisplay returns the lock state to advertise for an area in the lock
// ARUP, which is what the client's area list renders in its lock column.
//
// /spectate deliberately leaves the area's stored lock alone (it restricts
// speech, not entry), so an area in spectate mode reported its raw LockFree and
// the area list showed it as FREE — nothing anywhere told a player looking at
// the list that the room was watch-only, not even the CM who had just enabled
// it. SPECTATABLE is precisely AO2's name for "you may enter and watch, but not
// take part", which is what spectate mode does, so that is what gets sent.
//
// A locked area keeps reporting LOCKED: that is a claim about entry, which
// spectate mode makes no claim about, and it is the stronger of the two.
//
// Display only — the area's stored lock is never changed. /unlock, the
// auto-unlock when the last CM leaves, and the LockSpectatable branches in
// CanSpeakIC/CanChangeMusic/CanJud (which gate on the /lock invite list, not
// the separate spectate-invite list) all keep reasoning about the lock the area
// actually has, so advertising this can't quietly rewrite what /spectate means.
func areaLockDisplay(a *area.Area) area.Lock {
	if l := a.Lock(); l != area.LockFree {
		return l
	}
	if a.SpectateMode() {
		return area.LockSpectatable
	}
	return area.LockFree
}

// sendLockArup sends a lock ARUP to all connected clients.
func sendLockArup() {
	locks := make([]string, 0, len(areas))
	for _, a := range areas {
		locks = append(locks, areaLockDisplay(a).String())
	}
	broadcastToAll(&packet.ARUP{Type: packet.ARUPLocks, Data: locks})
}

// getRole returns the role with the corresponding name, or an error if the role does not exist.
func getRole(name string) (permissions.Role, error) {
	for _, role := range roles {
		if strings.EqualFold(role.Name, name) {
			return role, nil
		}
	}
	return permissions.Role{}, fmt.Errorf("role does not exist")
}

// getClientByUid returns the client with the given uid.
func getClientByUid(uid int) (*Client, error) {
	if c := clients.GetClientByUID(uid); c != nil {
		return c, nil
	}
	return nil, fmt.Errorf("client does not exist")
}

// getClientsByIpid returns all clients with the given ipid.
func getClientsByIpid(ipid string) []*Client {
	return clients.GetByIPID(ipid)
}

// sendAreaServerMessage sends a server OOC message to all clients in an area.
func sendAreaServerMessage(area *area.Area, message string) {
	broadcastToArea(area, &packet.CTToClient{Name: encodedServerName, Message: encode(message), IsFromServer: "1"})
}

// sendAreaGamblingMessage sends a gambling-result OOC message to all clients
// in an area who have not opted out of gambling broadcasts via /gamble hide.
func sendAreaGamblingMessage(a *area.Area, message string) {
	out := &packet.CTToClient{Name: encodedServerName, Message: encode(message), IsFromServer: "1"}
	header, args := out.Header(), out.Args()
	clients.ForEach(func(client *Client) {
		if client.Area() == a && !client.GambleHide() {
			client.SendPacket(header, args...)
		}
	})
}

// sendGlobalServerMessage broadcasts a server OOC message to every joined client.
func sendGlobalServerMessage(message string) {
	broadcastToAll(&packet.CTToClient{Name: encodedServerName, Message: encode(message), IsFromServer: "1"})
}

// getRealIP extracts the real client IP address from an HTTP request.
// When reverse_proxy_mode is enabled in the config, it checks X-Forwarded-For
// and X-Real-IP headers (for reverse proxy setups like nginx or Cloudflare).
// When reverse_proxy_mode is disabled, it always uses RemoteAddr directly.
//
// Security Note: Proxy headers (X-Forwarded-For, X-Real-IP) are only trusted when
// reverse_proxy_mode is explicitly enabled. This prevents IP spoofing when the server
// is directly exposed to the internet without a reverse proxy.
func getRealIP(r *http.Request) string {
	// Only trust proxy headers if reverse_proxy_mode is enabled in config
	if config.ReverseProxyMode {
		// Check X-Forwarded-For header first (may contain multiple IPs)
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			// X-Forwarded-For can contain multiple IPs: "client, proxy1, proxy2"
			// The first IP is the original client
			ips := strings.Split(xff, ",")
			if len(ips) > 0 {
				return strings.TrimSpace(ips[0])
			}
		}

		// Check X-Real-IP header (single IP from reverse proxy)
		if xri := r.Header.Get("X-Real-IP"); xri != "" {
			return xri
		}
	}

	// Use RemoteAddr if reverse_proxy_mode is disabled or no proxy headers are present
	return r.RemoteAddr
}

// Returns the IPID for a given IP address.
func getIpid(s string) string {
	// For privacy and ease of use, AO servers traditionally use a hashed version of a client's IP address to identify a client.
	// Athena uses the MD5 hash of the IP address, encoded in base64.
	ip := extractIP(s)
	hash := md5.Sum([]byte(ip))
	ipid := base64.StdEncoding.EncodeToString(hash[:])
	return ipid[:len(ipid)-2] // Removes the trailing padding.
}

// extractIP returns the plain IP address from a "host:port" string (or plain IP).
// It mirrors the extraction logic inside getIpid so callers can obtain the raw IP
// without re-parsing the same string.
func extractIP(s string) string {
	ip, _, err := net.SplitHostPort(s)
	if err != nil {
		return s
	}
	return ip
}

// getParrotMsg returns a random string from the server's parrot list.
// parrot is validated to be non-empty in InitServer, so no bounds check is required here.
func getParrotMsg() string {
	return getParrotList()[rand.Intn(len(getParrotList()))]
}

// checkConnRateLimit checks whether the given ipid has exceeded the connection rate limit.
// It records every connection attempt (including rejected ones) in the sliding window.
// Returns rejected=true if the connection should be rejected.
// Returns autoban=true when conn_flood_autoban is enabled and the rejection count hits
// the threshold exactly this call — the caller should ban the IP asynchronously.
// Both values are determined under a single lock acquisition.
func checkConnRateLimit(ipid string) (rejected, autoban bool) {
	if config.ConnRateLimit <= 0 {
		return false, false
	}

	connTracker.mu.Lock()
	defer connTracker.mu.Unlock()

	now := time.Now()
	window := connRateLimitWindowDur
	if window == 0 {
		window = time.Duration(config.ConnRateLimitWindow) * time.Second
	}
	cutoff := now.Add(-window)

	// Prune timestamps outside the current window (zero-allocation pivot trim;
	// timestamps are always appended in order so expired entries are at the front).
	times := connTracker.timestamps[ipid]
	i := 0
	for i < len(times) && !times[i].After(cutoff) {
		i++
	}
	if i == len(times) {
		times = nil
	} else if i > 0 {
		times = times[i:]
	}

	// Record this attempt regardless of outcome, so floods are always counted.
	times = append(times, now)
	connTracker.timestamps[ipid] = times

	if len(times) <= config.ConnRateLimit {
		return false, false
	}

	// Connection is rejected. Check whether the auto-ban threshold has been reached.
	// Use == (not >=) so the autoban goroutine is spawned exactly once per flood.
	if config.ConnFloodAutoban && config.ConnFloodAutobanThreshold > 0 {
		connTracker.rejections[ipid]++
		autoban = connTracker.rejections[ipid] == config.ConnFloodAutobanThreshold
	}
	return true, autoban
}

// registerRateLimitKick records a message-based rate-limit kick (see
// Client.KickForRateLimit, which covers IC, character-select, music/area-change, and
// both OOC rate limits) against ipid, and returns the number of kicks counted within
// the current spree. Returns 0 if rate_limit_kick_autoban is disabled or its threshold
// is 0, so the caller can skip the rest of the escalation path entirely.
//
// A kick that arrives more than rate_limit_kick_autoban_window after the previous one
// restarts the count at 1, so an IPID has to earn a threshold within a single spam
// spree rather than accumulating kicks over its entire lifetime on the server.
func registerRateLimitKick(ipid string) int {
	if !config.RateLimitKickAutoban || config.RateLimitKickAutobanThreshold <= 0 {
		return 0
	}

	rateLimitKickTracker.mu.Lock()
	defer rateLimitKickTracker.mu.Unlock()

	now := time.Now()
	if window := time.Duration(config.RateLimitKickAutobanWindow) * time.Second; window > 0 {
		if last, ok := rateLimitKickTracker.last[ipid]; ok && now.Sub(last) > window {
			rateLimitKickTracker.counts[ipid] = 0
		}
	}
	rateLimitKickTracker.last[ipid] = now
	rateLimitKickTracker.counts[ipid]++

	return rateLimitKickTracker.counts[ipid]
}

// maxLoginAttempts and loginAttemptWindow bound how many failed /login
// attempts an IPID gets before checkLoginRateLimit starts refusing further
// tries. A local constant rather than a config key, matching how other
// small safety caps in this codebase (e.g. maxAreaNameLen) are sized without
// needing an operator-facing knob.
const (
	maxLoginAttempts   = 5
	loginAttemptWindow = 5 * time.Minute
)

// checkLoginRateLimit reports whether ipid has exceeded maxLoginAttempts
// failed /login attempts within loginAttemptWindow, and if so how many
// seconds remain before it may try again. An attempt outside the window
// clears the counter as a side effect, mirroring registerRateLimitKick's own
// "a gap this long means a fresh spree" treatment.
func checkLoginRateLimit(ipid string) (limited bool, remaining int) {
	loginAttemptTracker.mu.Lock()
	defer loginAttemptTracker.mu.Unlock()

	count, ok := loginAttemptTracker.counts[ipid]
	if !ok || count < maxLoginAttempts {
		return false, 0
	}
	elapsed := time.Since(loginAttemptTracker.last[ipid])
	if elapsed >= loginAttemptWindow {
		delete(loginAttemptTracker.counts, ipid)
		delete(loginAttemptTracker.last, ipid)
		return false, 0
	}
	return true, int(math.Ceil((loginAttemptWindow - elapsed).Seconds()))
}

// registerFailedLogin records a failed /login attempt for ipid. A gap longer
// than loginAttemptWindow since the previous attempt restarts the count at 1,
// so this targets a concentrated burst of guesses rather than attempts spread
// thinly over a whole session.
func registerFailedLogin(ipid string) {
	loginAttemptTracker.mu.Lock()
	defer loginAttemptTracker.mu.Unlock()

	now := time.Now()
	if last, ok := loginAttemptTracker.last[ipid]; ok && now.Sub(last) > loginAttemptWindow {
		loginAttemptTracker.counts[ipid] = 0
	}
	loginAttemptTracker.last[ipid] = now
	loginAttemptTracker.counts[ipid]++
}

// clearLoginAttempts resets the failed-attempt counter for ipid, called after
// a successful login so a legitimate sign-in isn't left one attempt closer to
// a limit some earlier typo racked up.
func clearLoginAttempts(ipid string) {
	loginAttemptTracker.mu.Lock()
	delete(loginAttemptTracker.counts, ipid)
	delete(loginAttemptTracker.last, ipid)
	loginAttemptTracker.mu.Unlock()
}

// rateLimitKickAutobanDuration parses the configured cooldown duration for the
// rate-limit-kick autoban. Returns ok=false if the config value is invalid, so the
// caller can skip banning rather than apply a garbage duration; in practice this
// can't happen at runtime since InitServer validates the value at startup.
func rateLimitKickAutobanDuration() (dur time.Duration, ok bool) {
	dur, err := str2duration.ParseDuration(config.RateLimitKickAutobanDuration)
	return dur, err == nil
}

// forgetIP removes an IPID from the in-memory first-seen tracker and clears its
// known-IP status in the database. This is called when an IP is banned so that, once
// the ban expires, the IP will be treated as new again (subject to cooldowns and rate
// limits). Accumulated /playtime is preserved — db.RemoveKnownIP only zeroes the
// seen-timestamps, it does not delete the playtime row.
func forgetIP(ipid string) {
	ipFirstSeenTracker.mu.Lock()
	delete(ipFirstSeenTracker.times, ipid)
	ipFirstSeenTracker.mu.Unlock()

	connTracker.mu.Lock()
	delete(connTracker.rejections, ipid)
	connTracker.mu.Unlock()

	rateLimitKickTracker.mu.Lock()
	delete(rateLimitKickTracker.counts, ipid)
	delete(rateLimitKickTracker.last, ipid)
	rateLimitKickTracker.mu.Unlock()

	go func() {
		if err := db.RemoveKnownIP(ipid); err != nil {
			logger.LogErrorf("Failed to clear known-IP status for banned IP %s: %v", ipid, err)
		}
	}()
}

// resetKnownIPTracker clears the in-memory first-seen tracker entirely.
// Called after a full database purge so that all IPIDs are treated as new on
// their next connection.
func resetKnownIPTracker() {
	ipFirstSeenTracker.mu.Lock()
	ipFirstSeenTracker.times = make(map[string]time.Time)
	ipFirstSeenTracker.mu.Unlock()
}

// addTormentedIP adds an IPID to the in-memory torment set and persists it to the database.
func addTormentedIP(ipid string) {
	tormentedIPIDs.mu.Lock()
	tormentedIPIDs.set[ipid] = struct{}{}
	tormentedIPIDs.mu.Unlock()

	go func() {
		if err := db.AddTormentedIP(ipid); err != nil {
			logger.LogErrorf("Failed to persist tormented IP %s: %v", ipid, err)
		}
	}()
}

// removeTormentedIP removes an IPID from the in-memory torment set and from the database.
func removeTormentedIP(ipid string) {
	tormentedIPIDs.mu.Lock()
	delete(tormentedIPIDs.set, ipid)
	tormentedIPIDs.mu.Unlock()

	go func() {
		if err := db.RemoveTormentedIP(ipid); err != nil {
			logger.LogErrorf("Failed to remove tormented IP %s from database: %v", ipid, err)
		}
	}()
}

// snapshotTormentedIPs returns a copy of every IPID currently on the torment
// list, for /tormentlist.
func snapshotTormentedIPs() []string {
	tormentedIPIDs.mu.RLock()
	ipids := make([]string, 0, len(tormentedIPIDs.set))
	for ipid := range tormentedIPIDs.set {
		ipids = append(ipids, ipid)
	}
	tormentedIPIDs.mu.RUnlock()
	return ipids
}

// clearAllTormentedIPs empties the entire torment list — in memory and in the
// database — and returns how many IPIDs were removed. Emptying the in-memory
// set also cancels every pending torment disconnect timer, since they re-check
// isIPIDTormented before firing.
func clearAllTormentedIPs() int {
	tormentedIPIDs.mu.Lock()
	n := len(tormentedIPIDs.set)
	tormentedIPIDs.set = make(map[string]struct{})
	tormentedIPIDs.mu.Unlock()

	go func() {
		if err := db.ClearTormentedIPs(); err != nil {
			logger.LogErrorf("Failed to clear tormented IPs from database: %v", err)
		}
	}()
	return n
}

// isIPIDTormented returns true if the IPID is in the tormented set.
func isIPIDTormented(ipid string) bool {
	tormentedIPIDs.mu.RLock()
	_, ok := tormentedIPIDs.set[ipid]
	tormentedIPIDs.mu.RUnlock()
	return ok
}

// autobanFlooder bans an IP for the given flood reason using the configured default ban
// duration. reason should be a short description, e.g. "packet flooding".
// No-op if the IP is already banned or the ban cannot be recorded.
func autobanFlooder(ipid, reason string) {
	dur, err := str2duration.ParseDuration(config.BanLen)
	if err != nil {
		return
	}
	autobanFlooderFor(ipid, reason, dur)
}

// autobanFlooderFor bans an IP for the given flood reason using an explicit duration,
// rather than the configured default ban length. Used by autoban paths that
// intentionally apply a shorter cooldown than a moderator-issued ban -- e.g. the
// rate-limit-kick autoban in Client.KickForRateLimit -- so a genuine player who trips
// it by accident isn't shut out nearly as long as a real ban would shut out an abuser.
// No-op if the IP is already banned or the ban cannot be recorded.
//
// Same as a moderator-issued /ban, this alerts online staff (alertBannedAccountLinks)
// if the banned IPID turns out to be linked to a registered player account -- purely
// informational, the account itself is never touched -- so an automatic ban that catches
// the wrong person (a real, if not yet well-established, player rather than a bot) is
// immediately visible to staff and easy to catch and reverse with /unban.
func autobanFlooderFor(ipid, reason string, dur time.Duration) {
	banned, _, err := db.IsBanned(db.IPID, ipid)
	if err != nil || banned {
		return
	}
	now := time.Now().UTC()
	_, err = db.AddBan(ipid, "", now.Unix(), now.Add(dur).Unix(), "Automatic ban: "+reason, "Server")
	if err != nil {
		logger.LogErrorf("Failed to auto-ban %v (%s): %v", ipid, reason, err)
		return
	}
	forgetIP(ipid)
	alertBannedAccountLinks(map[string]struct{}{ipid: {}})
	logger.LogInfof("Auto-banned %v for %s (%v)", ipid, reason, dur)
}

// autoBanPacketFlooder bans an IP that exceeded the raw packet rate limit.
// This is only triggered by raw packet flooding (sending hundreds of packets per second),
// which is characteristic of bots/DDoS tools. If the IP is already banned, no additional
// ban is added. A single IC/OOC/music message rate limit violation still results in just
// a kick -- see KickForRateLimit -- but repeated violations escalate to a ban too, via
// registerRateLimitKick/autobanFlooderFor above.
func autoBanPacketFlooder(ipid string) {
	autobanFlooder(ipid, "packet flooding")
}

// startConnTrackerCleanup periodically removes stale entries from the connection tracker
// to prevent unbounded memory growth from unique IPs that no longer connect.
// This goroutine runs for the lifetime of the server process; a graceful stop is not
// required because the process exits when the server stops.
func startConnTrackerCleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	for range ticker.C {
		if config == nil || config.ConnRateLimitWindow <= 0 {
			continue
		}
		window := time.Duration(config.ConnRateLimitWindow) * time.Second
		cutoff := time.Now().Add(-window)
		connTracker.mu.Lock()
		for ipid, times := range connTracker.timestamps {
			i := 0
			for i < len(times) && !times[i].After(cutoff) {
				i++
			}
			if i == len(times) {
				delete(connTracker.timestamps, ipid)
				// Also clear rejection counts for IPs with no recent connection attempts,
				// so short-lived flooders that stop connecting don't accumulate forever.
				delete(connTracker.rejections, ipid)
			} else if i > 0 {
				connTracker.timestamps[ipid] = times[i:]
			}
		}
		connTracker.mu.Unlock()

		// Clean up stale modcall entries.
		if config.ModcallCooldown > 0 {
			cooldown := time.Duration(config.ModcallCooldown) * time.Second
			modcallCutoff := time.Now().Add(-cooldown)
			ipModcallTracker.mu.Lock()
			for ipid, t := range ipModcallTracker.times {
				if !t.After(modcallCutoff) {
					delete(ipModcallTracker.times, ipid)
				}
			}
			ipModcallTracker.mu.Unlock()
		}

		// Clean up stale OOC rate limit entries.
		if config.OOCRateLimitWindow > 0 {
			oocWindow := time.Duration(config.OOCRateLimitWindow) * time.Second
			oocCutoff := time.Now().Add(-oocWindow)
			ipOOCTracker.mu.Lock()
			for ipid, times := range ipOOCTracker.timestamps {
				i := 0
				for i < len(times) && !times[i].After(oocCutoff) {
					i++
				}
				if i == len(times) {
					delete(ipOOCTracker.timestamps, ipid)
				} else if i > 0 {
					ipOOCTracker.timestamps[ipid] = times[i:]
				}
			}
			ipOOCTracker.mu.Unlock()
		}

		// Clean up stale ping rate limit entries.
		if config.PingRateLimitWindow > 0 {
			pingWindow := time.Duration(config.PingRateLimitWindow) * time.Second
			pingCutoff := time.Now().Add(-pingWindow)
			ipPingTracker.mu.Lock()
			for ipid, times := range ipPingTracker.timestamps {
				i := 0
				for i < len(times) && !times[i].After(pingCutoff) {
					i++
				}
				if i == len(times) {
					delete(ipPingTracker.timestamps, ipid)
				} else if i > 0 {
					ipPingTracker.timestamps[ipid] = times[i:]
				}
			}
			ipPingTracker.mu.Unlock()
		}

		// Clean up stale rate-limit-kick entries, so an IPID that stops tripping the
		// rate limit doesn't hold a slot in the tracker forever.
		if config.RateLimitKickAutobanWindow > 0 {
			kickWindow := time.Duration(config.RateLimitKickAutobanWindow) * time.Second
			kickCutoff := time.Now().Add(-kickWindow)
			rateLimitKickTracker.mu.Lock()
			for ipid, last := range rateLimitKickTracker.last {
				if !last.After(kickCutoff) {
					delete(rateLimitKickTracker.last, ipid)
					delete(rateLimitKickTracker.counts, ipid)
				}
			}
			rateLimitKickTracker.mu.Unlock()
		}

		// Clean up stale global new-IP rate limit entries.
		if config.GlobalNewIPRateLimitWindow > 0 {
			globalWindow := time.Duration(config.GlobalNewIPRateLimitWindow) * time.Second
			globalCutoff := time.Now().Add(-globalWindow)
			globalNewIPTracker.mu.Lock()
			times := globalNewIPTracker.timestamps
			i := 0
			for i < len(times) && !times[i].After(globalCutoff) {
				i++
			}
			if i == len(times) {
				globalNewIPTracker.timestamps = nil
			} else if i > 0 {
				globalNewIPTracker.timestamps = times[i:]
			}
			globalNewIPTracker.mu.Unlock()
		}
	}
}

// checkIPModcallCooldown checks if the given IPID is within the modcall cooldown period.
// Unlike the per-client check, this persists across connections, preventing bypass via reconnection.
// Returns true (and remaining seconds, rounded up) if the IPID must wait, false otherwise.
func checkIPModcallCooldown(ipid string) (bool, int) {
	if config.ModcallCooldown <= 0 {
		return false, 0
	}
	ipModcallTracker.mu.Lock()
	defer ipModcallTracker.mu.Unlock()
	last, exists := ipModcallTracker.times[ipid]
	if !exists {
		return false, 0
	}
	elapsed := time.Since(last)
	cooldown := time.Duration(config.ModcallCooldown) * time.Second
	if elapsed < cooldown {
		remaining := int(math.Ceil((cooldown - elapsed).Seconds()))
		return true, remaining
	}
	return false, 0
}

// setIPModcallTime records the current time as the last modcall time for the given IPID.
func setIPModcallTime(ipid string) {
	ipModcallTracker.mu.Lock()
	ipModcallTracker.times[ipid] = time.Now()
	ipModcallTracker.mu.Unlock()
}

// checkIPOOCRateLimit checks if the given IPID has exceeded the OOC message rate limit.
// Unlike the per-client check, this persists across connections, preventing bypass via reconnection.
// Returns true if the packet should be dropped, false if allowed.
func checkIPOOCRateLimit(ipid string) bool {
	if config.OOCRateLimit <= 0 {
		return false
	}
	ipOOCTracker.mu.Lock()
	defer ipOOCTracker.mu.Unlock()
	now := time.Now()
	window := time.Duration(config.OOCRateLimitWindow) * time.Second
	cutoff := now.Add(-window)
	times := ipOOCTracker.timestamps[ipid]
	i := 0
	for i < len(times) && !times[i].After(cutoff) {
		i++
	}
	if i == len(times) {
		times = nil
	} else if i > 0 {
		times = times[i:]
	}
	if len(times) >= config.OOCRateLimit {
		ipOOCTracker.timestamps[ipid] = times
		return true
	}
	ipOOCTracker.timestamps[ipid] = append(times, now)
	return false
}

// checkIPPingRateLimit checks if the given IPID has exceeded the ping (CH) rate limit.
// This persists across connections, preventing bypass via reconnection.
// Returns true if the ping should be dropped, false if allowed.
func checkIPPingRateLimit(ipid string) bool {
	if config.PingRateLimit <= 0 {
		return false
	}
	ipPingTracker.mu.Lock()
	defer ipPingTracker.mu.Unlock()
	now := time.Now()
	window := time.Duration(config.PingRateLimitWindow) * time.Second
	cutoff := now.Add(-window)
	times := ipPingTracker.timestamps[ipid]
	i := 0
	for i < len(times) && !times[i].After(cutoff) {
		i++
	}
	if i == len(times) {
		times = nil
	} else if i > 0 {
		times = times[i:]
	}
	if len(times) >= config.PingRateLimit {
		ipPingTracker.timestamps[ipid] = times
		return true
	}
	ipPingTracker.timestamps[ipid] = append(times, now)
	return false
}

// recordIPFirstSeen records the first time an IPID connects to this server session.
// If the IPID has already been seen this call is a no-op for the in-memory tracker.
// Entries are normally kept indefinitely so that returning players are never treated
// as new again; however, banned IPs are explicitly removed via forgetIP so that a
// reconnect after a ban expiry is subject to new-connection cooldowns.
// When a genuinely new IPID is recorded, a timestamp is also pushed to globalNewIPTracker
// for global new-connection rate limiting.
// NOTE: the caller is responsible for persisting the IP to the database (db.MarkIPKnown).
//
// Returns true the one time an IPID is genuinely new -- never seen by this
// server before, in the database or this session -- and false on every call
// after that, including every reconnect. The join popup (joinpopup.go) is the
// one caller that reads this return value; every other call site predates it
// and just discards it, which remains correct since a no-op call has nothing
// new to report.
func recordIPFirstSeen(ipid string) bool {
	ipFirstSeenTracker.mu.Lock()
	if _, exists := ipFirstSeenTracker.times[ipid]; exists {
		ipFirstSeenTracker.mu.Unlock()
		return false
	}
	ipFirstSeenTracker.times[ipid] = time.Now()
	ipFirstSeenTracker.mu.Unlock()

	// Inform the global new-IP rate limiter that a new unique IP has arrived.
	// globalNewIPTracker.mu is acquired only after ipFirstSeenTracker.mu is released
	// to keep lock-acquisition order consistent and avoid potential deadlocks.
	globalNewIPTracker.mu.Lock()
	globalNewIPTracker.timestamps = append(globalNewIPTracker.timestamps, time.Now())
	globalNewIPTracker.mu.Unlock()

	// Raid guard layer 2: feed this genuinely-new IPID into the arrival-burst
	// signal (raidguard_corr.go) alongside the global-new-IP tracking above --
	// same event, a different question ("is this a raid" vs. "should we rate
	// limit"). One atomic load when the guard is off.
	if raidGuardActive.Load() {
		raidObserveArrival(time.Now())
	}
	return true
}

// checkNewIPIDOOCCooldown checks whether a newly-seen IPID is still within the OOC chat cooldown.
// Returns true (and remaining seconds, rounded up) if the IPID must wait, false otherwise.
func checkNewIPIDOOCCooldown(ipid string) (bool, int) {
	if config.NewIPIDOOCCooldown <= 0 {
		return false, 0
	}
	ipFirstSeenTracker.mu.Lock()
	defer ipFirstSeenTracker.mu.Unlock()
	t, exists := ipFirstSeenTracker.times[ipid]
	if !exists {
		return false, 0
	}
	cooldown := time.Duration(config.NewIPIDOOCCooldown) * time.Second
	elapsed := time.Since(t)
	if elapsed < cooldown {
		remaining := int(math.Ceil((cooldown - elapsed).Seconds()))
		return true, remaining
	}
	return false, 0
}

// checkNewIPIDICCooldown checks whether a newly-seen IPID is still within the IC
// speaking cooldown (new_ipid_ic_cooldown). Mirrors checkNewIPIDOOCCooldown exactly,
// but callers must place this check AFTER the rate-limit and automod/censor checks in
// pktIC, not before -- see the call site's comment. Returns true (and remaining
// seconds, rounded up) if the IPID must wait, false otherwise.
func checkNewIPIDICCooldown(ipid string) (bool, int) {
	if config.NewIPIDICCooldown <= 0 {
		return false, 0
	}
	ipFirstSeenTracker.mu.Lock()
	defer ipFirstSeenTracker.mu.Unlock()
	t, exists := ipFirstSeenTracker.times[ipid]
	if !exists {
		return false, 0
	}
	cooldown := time.Duration(config.NewIPIDICCooldown) * time.Second
	elapsed := time.Since(t)
	if elapsed < cooldown {
		remaining := int(math.Ceil((cooldown - elapsed).Seconds()))
		return true, remaining
	}
	return false, 0
}

// checkNewIPIDModcallCooldown checks whether a newly-seen IPID is still within the modcall cooldown.
// Returns true (and remaining seconds, rounded up) if the IPID must wait, false otherwise.
func checkNewIPIDModcallCooldown(ipid string) (bool, int) {
	if config.NewIPIDModcallCooldown <= 0 {
		return false, 0
	}
	ipFirstSeenTracker.mu.Lock()
	defer ipFirstSeenTracker.mu.Unlock()
	t, exists := ipFirstSeenTracker.times[ipid]
	if !exists {
		return false, 0
	}
	cooldown := time.Duration(config.NewIPIDModcallCooldown) * time.Second
	elapsed := time.Since(t)
	if elapsed < cooldown {
		remaining := int(math.Ceil((cooldown - elapsed).Seconds()))
		return true, remaining
	}
	return false, 0
}

// checkGlobalNewIPRateLimit checks whether too many new unique IPs have arrived within
// the configured time window. If so, connections from IPs that have never been seen before
// are rejected to protect the server against distributed floods using many unique IPs.
// Already-known IPs (those with an entry in ipFirstSeenTracker) are always permitted, as
// is any IPID holding a verified lockdown whitelist passkey (see lockdown_passkey.go) --
// that exemption must hold even if ipFirstSeenTracker has no entry for it (e.g. right
// after a restart, before the IPID has reconnected).
// Also enforces lockdown mode: while lockdown is active, all new (unseen) IPs are rejected.
// Returns true if the connection should be rejected.
func checkGlobalNewIPRateLimit(ipid string) bool {
	// Known IPs are always allowed through; only new, unseen IPs can trigger this limit.
	ipFirstSeenTracker.mu.Lock()
	_, known := ipFirstSeenTracker.times[ipid]
	ipFirstSeenTracker.mu.Unlock()
	if known || isLockdownExempt(ipid) {
		return false
	}

	// Lockdown mode: block all new IPIDs regardless of the configured rate limit.
	if serverLockdown.Load() {
		return true
	}

	if config.GlobalNewIPRateLimit <= 0 {
		return false
	}

	globalNewIPTracker.mu.Lock()
	defer globalNewIPTracker.mu.Unlock()

	cutoff := time.Now().Add(-time.Duration(config.GlobalNewIPRateLimitWindow) * time.Second)

	// Prune timestamps outside the current window (zero-allocation pivot trim;
	// timestamps are always appended in order so expired entries are at the front).
	times := globalNewIPTracker.timestamps
	i := 0
	for i < len(times) && !times[i].After(cutoff) {
		i++
	}
	if i == len(times) {
		times = nil
	} else if i > 0 {
		times = times[i:]
	}
	globalNewIPTracker.timestamps = times

	return len(times) >= config.GlobalNewIPRateLimit
}

// serverLockdownRejection reports whether this IPID is being rejected specifically
// because server lockdown is active (and not merely due to the global new-IP cap).
func serverLockdownRejection(ipid string) bool {
	if !serverLockdown.Load() {
		return false
	}
	ipFirstSeenTracker.mu.Lock()
	_, known := ipFirstSeenTracker.times[ipid]
	ipFirstSeenTracker.mu.Unlock()
	return !known
}

// lockdownPurgeEligible reports whether a connection with the given total
// (all-time, DB-recorded) playtime in seconds should be kicked by the
// lockdown purge, given the configured minimum-playtime threshold in
// seconds. A threshold of 0 disables the purge entirely; a player/IP whose
// playtime meets or exceeds the threshold is always exempt.
func lockdownPurgeEligible(totalPlaytimeSeconds, thresholdSeconds int64) bool {
	if thresholdSeconds <= 0 {
		return false
	}
	return totalPlaytimeSeconds < thresholdSeconds
}

// rateLimitAutobanThresholdFor returns the number of spree kicks c must reach before
// Client.KickForRateLimit escalates to the rate_limit_kick_autoban ban, and whether c
// is bannable by this mechanism at all. Three tiers, based on c's accumulated all-time
// playtime on its IPID (KNOWN_IPS.PLAYTIME, the same figure /playtime and the Lockdown
// Playtime Purge read):
//
//   - Moderators, and any IPID at or above rate_limit_kick_autoban_min_playtime
//     (default 20h): fully exempt (bannable=false) -- an established regular is never
//     banned by this mechanism, no matter how many times it trips the rate limit.
//   - At or above rate_limit_kick_autoban_lenient_playtime (default 2h): the lenient
//     tier -- needs rate_limit_kick_autoban_lenient_threshold kicks (default 5,
//     clamped up to at least the base threshold so a misconfigured value can't make
//     this tier *more* aggressive than the base one) rather than the base threshold.
//     Not immune, just harder to trip by accident than a brand-new connection.
//   - Below that, including a brand-new connection with no playtime at all: the base
//     rate_limit_kick_autoban_threshold (default 2) -- the aggressive tier aimed at
//     connections that are purely there to spam.
//
// Reuses lockdownPurgeEligible's exact playtime-threshold comparison for the same
// reason the Lockdown Playtime Purge uses it: both features are answering the same
// underlying question -- "is this a real player, or a brand-new connection with
// nothing vouching for it." A DB error while checking playtime fails open
// (bannable=false) -- never bans a real player on a hiccup. An exempt/not-yet-eligible
// IPID still gets the ordinary kick from KickForRateLimit; only the autoban escalation
// is gated by this.
func rateLimitAutobanThresholdFor(c *Client) (threshold int, bannable bool) {
	base := config.RateLimitKickAutobanThreshold
	if base <= 0 {
		return 0, false
	}
	if permissions.IsModerator(c.Perms()) {
		return 0, false
	}

	minPlaytime := int64(config.RateLimitKickAutobanMinPlaytime) * 60
	lenientPlaytime := int64(config.RateLimitKickAutobanLenientPlaytime) * 60
	if minPlaytime <= 0 && lenientPlaytime <= 0 {
		return base, true // no playtime tiers configured -- flat threshold for everyone
	}

	playtime, err := db.GetPlaytime(c.Ipid())
	if err != nil {
		logger.LogErrorf("rate-limit autoban: failed to get playtime for IPID %v: %v", c.Ipid(), err)
		return 0, false // fail open: never auto-ban a real player on a DB hiccup
	}

	if minPlaytime > 0 && playtime >= minPlaytime {
		return 0, false
	}
	if lenientPlaytime > 0 && playtime >= lenientPlaytime {
		lenient := config.RateLimitKickAutobanLenientThreshold
		if lenient < base {
			lenient = base
		}
		return lenient, true
	}
	return base, true
}

// lockdownShouldSilence reports whether, while lockdown is active, c's IC/OOC
// messages should be silently dropped instead of broadcast -- the same
// non-moderator, under-threshold population purgeLockdownFloodClients kicks.
// Callers on the IC/OOC hot path must check serverLockdown.Load() first (this
// function does not) so the cost of a moderator check plus a DB read is only
// ever paid while lockdown is actually active. A connection the one-shot
// purge sweep hasn't reached yet, or one that reconnects mid-lockdown as an
// already-known IPID (which the sweep never sees at all, since it only runs
// once, the instant lockdown switches on), is still caught here on every
// message for as long as lockdown remains active.
func lockdownShouldSilence(c *Client) bool {
	threshold := lockdownMinPlaytime.Load()
	if threshold <= 0 {
		return false
	}
	if permissions.IsModerator(c.Perms()) {
		return false
	}
	if isLockdownExempt(c.Ipid()) {
		return false
	}
	playtime, err := db.GetPlaytime(c.Ipid())
	if err != nil {
		logger.LogErrorf("lockdown: failed to get playtime for IPID %v: %v", c.Ipid(), err)
		return false // fail open: never silence a real message on a DB hiccup
	}
	return lockdownPurgeEligible(playtime, threshold)
}

// purgeLockdownFloodClients runs the instant lockdown is switched on, from
// either /lockdown or the Discord bot's /lockdown on. Server-wide lockdown
// only ever stopped new (previously-unseen) IPIDs from connecting -- it did
// nothing about a flood that's already inside spamming, since by the time a
// mod reacts and flips lockdown on, every flooding connection has already
// gotten past the join gate and is "known" for the rest of the session. This
// sweeps every currently-connected, non-moderator client and instantly
// disconnects anyone whose total (all-time) playtime on their IPID is under
// lockdownMinPlaytime, then forgets their IPID so lockdown's new-connection
// block actually keeps them out on any immediate reconnect attempt.
//
// Each kick runs on its own goroutine. SendPacketSync blocks its caller for
// up to the write deadline (see its doc comment) -- looping it across
// however many flooded connections are in the room on a single goroutine
// would serialize on the slowest socket (exactly the connections a raid
// leaves half-open/unread) instead of finishing in about one write-deadline's
// worth of time no matter how many clients are being cleared. The DB
// playtime lookups are likewise fanned out concurrently so a flood of
// connections costs one round-trip's worth of latency, not one per client.
func purgeLockdownFloodClients() {
	threshold := lockdownMinPlaytime.Load()
	if threshold <= 0 {
		return
	}
	baseReason := fmt.Sprintf("Server lockdown: connections with under %d minute(s) of total playtime are being cleared.", threshold/60)

	var wg sync.WaitGroup
	var kicked int32
	clients.ForEach(func(c *Client) {
		if permissions.IsModerator(c.Perms()) {
			return
		}
		wg.Add(1)
		go func(c *Client) {
			defer wg.Done()
			if !lockdownShouldSilence(c) {
				return
			}
			c.SendSync(&packet.KK{Reason: lockdownRejectionMessage(baseReason, c.Ipid())})
			c.conn.Close()
			forgetIP(c.Ipid())
			atomic.AddInt32(&kicked, 1)
		}(c)
	})
	wg.Wait()

	n := atomic.LoadInt32(&kicked)
	if n == 0 {
		return
	}
	sendPlayerArup()
	msg := fmt.Sprintf("🔒 Lockdown purge: kicked %d connection(s) with under %d minute(s) of total playtime.", n, threshold/60)
	logger.LogInfof("%s", msg)
	clients.ForEach(func(c *Client) {
		if c.Uid() != -1 && permissions.IsModerator(c.Perms()) {
			c.Send(&packet.CTToClient{Name: "OOC", Message: msg, IsFromServer: "1"})
		}
	})
}

// estimateJoinedLen returns the total byte length of all strings in ss joined by newlines.
// Used to pre-size a strings.Builder, avoiding intermediate reallocations.
func estimateJoinedLen(ss []string) int {
	if len(ss) == 0 {
		return 0
	}
	n := len(ss) - 1 // separators
	for _, s := range ss {
		n += len(s)
	}
	return n
}

// buildSMPacket constructs the full SM#<areas>#<music>#% packet string that is
// sent verbatim to every client on join. Building it once at startup avoids a
// strings.Join allocation on every connection.
func buildSMPacket(areaNamesStr string, musicList []string) string {
	// SM# + areaNames + # + music[0] + # + ... + music[n-1] + #%
	// Add 8 bytes per entry as headroom for encoding expansion (worst case: '%' → "<percent>").
	size := 3 + len(areaNamesStr) + 1 + estimateJoinedLen(musicList) + len(musicList)*8 + 2
	var b strings.Builder
	b.Grow(size)
	b.WriteString("SM#")
	b.WriteString(areaNamesStr)
	for _, m := range musicList {
		b.WriteByte('#')
		encoder.WriteString(&b, m) //nolint:errcheck // strings.Builder.Write never returns an error
	}
	b.WriteString("#%")
	return b.String()
}

// hourlyChipMsg is the notification sent when a player earns exactly 1 chip from the
// hourly ticker.  Defined as a constant to avoid a fmt.Sprintf allocation every tick.
const hourlyChipMsg = "💰 You earned 1 chip for being online! Balance updated."

// startHourlyChipAward runs in the background and awards 1 chip to every connected
// player for each hour of online time they have accumulated during the current session.
// This ensures players receive their hourly chip without needing to disconnect first.
// Awards are tracked per-client in sessionChipsAwarded so the disconnect handler does
// not double-count chips that have already been granted.
//
// EnsureChipBalance is intentionally not called here: chip rows are seeded at connect
// time (pktReqDone), so by the time a player has been online for a full hour the row
// is guaranteed to exist.
func startHourlyChipAward() {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for range ticker.C {
		if config == nil || !config.EnableCasino {
			continue
		}
		// Capture now once so all per-client calculations use the same instant,
		// avoiding N repeated time.Now() syscalls inside the loop.
		now := time.Now()
		clients.ForEach(func(client *Client) {
			connAt := client.ConnectedAt()
			if connAt.IsZero() {
				return
			}
			sessionHours := int64(now.Sub(connAt).Seconds()) / secondsPerHour
			toAward := sessionHours - client.SessionChipsAwarded()
			if toAward <= 0 {
				return
			}
			ipid := client.Ipid()
			// Apply any passive income bonus passes the player has purchased.
			// chipsPerHour = 1 (base) + any bonuses from passive income upgrades.
			chipsPerHour := int64(1) + getPlayerHourlyBonus(ipid)
			chipsToAward := toAward * chipsPerHour
			if _, err := db.AddChips(ipid, chipsToAward); err != nil {
				logger.LogErrorf("startHourlyChipAward: AddChips failed for %v: %v", ipid, err)
				return
			}
			// Track hours credited (not chips) so the next tick knows where to resume.
			client.AddSessionChipsAwarded(toAward)
			var msg string
			if chipsToAward == 1 {
				msg = hourlyChipMsg
			} else if chipsPerHour > 1 {
				msg = fmt.Sprintf("💰 You earned %d chips for being online (%d/hr with income passes)! Balance updated.", chipsToAward, chipsPerHour)
			} else {
				msg = fmt.Sprintf("💰 You earned %d chips for being online! Balance updated.", chipsToAward)
			}
			client.SendServerMessage(msg)
		})
	}
}

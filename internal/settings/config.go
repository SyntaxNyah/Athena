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

// Package settings handles reading and writing to Athena's configuration files.
package settings

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/BurntSushi/toml"
	"github.com/MangosArentLiterature/Athena/internal/area"
	"github.com/MangosArentLiterature/Athena/internal/logger"
	"github.com/MangosArentLiterature/Athena/internal/permissions"
)

// Stores the path to the config directory
var ConfigPath string

type Config struct {
	ServerConfig  `toml:"Server"`
	LogConfig     `toml:"Logging"`
	MSConfig      `toml:"MasterServer"`
	DiscordConfig `toml:"Discord"`
	VoiceConfig   `toml:"Voice"`
}

type ServerConfig struct {
	Addr                                 string   `toml:"addr"`
	Port                                 int      `toml:"port"`
	AdvertiseHostname                    string   `toml:"advertise_hostname"`
	Name                                 string   `toml:"name"`
	Desc                                 string   `toml:"description"`
	MaxPlayers                           int      `toml:"max_players"`
	MaxMsg                               int      `toml:"max_message_length"`
	BanLen                               string   `toml:"default_ban_duration"`
	EnableWS                             bool     `toml:"enable_webao"`
	WSPort                               int      `toml:"webao_port"`
	EnableWSS                            bool     `toml:"enable_webao_secure"`
	WSSPort                              int      `toml:"webao_secure_port"`
	TLSCertPath                          string   `toml:"tls_cert_path"`
	TLSKeyPath                           string   `toml:"tls_key_path"`
	ReverseProxyMode                     bool     `toml:"reverse_proxy_mode"`
	ReverseProxyHTTPPort                 int      `toml:"reverse_proxy_http_port"`
	ReverseProxyHTTPSPort                int      `toml:"reverse_proxy_https_port"`
	MCLimit                              int      `toml:"multiclient_limit"`
	AssetURL                             string   `toml:"asset_url"`
	WebhookURL                           string   `toml:"webhook_url"`
	WebhookPingRoleID                    string   `toml:"webhook_ping_role_id"`
	PunishmentWebhookURL                 string   `toml:"punishment_webhook_url"`
	MaxDice                              int      `toml:"max_dice"`
	MaxSide                              int      `toml:"max_side"`
	Motd                                 string   `toml:"motd"`
	MaxStatement                         int      `toml:"max_testimony"`
	RateLimit                            int      `toml:"message_rate_limit"`
	RateLimitWindow                      int      `toml:"message_rate_limit_window"`
	RateLimitKickAutoban                 bool     `toml:"rate_limit_kick_autoban"`
	RateLimitKickAutobanThreshold        int      `toml:"rate_limit_kick_autoban_threshold"`
	RateLimitKickAutobanWindow           int      `toml:"rate_limit_kick_autoban_window"`
	RateLimitKickAutobanDuration         string   `toml:"rate_limit_kick_autoban_duration"`
	RateLimitKickAutobanMinPlaytime      int      `toml:"rate_limit_kick_autoban_min_playtime"`
	RateLimitKickAutobanLenientPlaytime  int      `toml:"rate_limit_kick_autoban_lenient_playtime"`
	RateLimitKickAutobanLenientThreshold int      `toml:"rate_limit_kick_autoban_lenient_threshold"`
	RaidGuardEnabled                     bool     `toml:"raid_guard_enabled"`
	RaidGuardMaxAction                   string   `toml:"raid_guard_max_action"`
	RaidGuardMinPlaytime                 int      `toml:"raid_guard_min_playtime"`
	RaidGuardLenientPlaytime             int      `toml:"raid_guard_lenient_playtime"`
	RaidGuardLenientScale                int      `toml:"raid_guard_lenient_scale"`
	RaidGuardStrictPlaytime              int      `toml:"raid_guard_strict_playtime"`
	RaidGuardStrictScale                 int      `toml:"raid_guard_strict_scale"`
	RaidGuardUnderAttackScale            int      `toml:"raid_guard_under_attack_scale"`
	RaidGuardBanDuration                 string   `toml:"raid_guard_ban_duration"`
	RaidGuardScoreWatch                  int      `toml:"raid_guard_score_watch"`
	RaidGuardScoreChallenge              int      `toml:"raid_guard_score_challenge"`
	RaidGuardScoreSilence                int      `toml:"raid_guard_score_silence"`
	RaidGuardScoreKick                   int      `toml:"raid_guard_score_kick"`
	RaidGuardScoreBan                    int      `toml:"raid_guard_score_ban"`
	RaidGuardObjectionMinMsgs            int      `toml:"raid_guard_objection_min_msgs"`
	RaidGuardObjectionFraction           float64  `toml:"raid_guard_objection_fraction"`
	RaidGuardFastCharPickMs              int      `toml:"raid_guard_fast_charpick_ms"`
	RaidGuardFastSpeechMs                int      `toml:"raid_guard_fast_speech_ms"`
	RaidGuardCharChurnMs                 int      `toml:"raid_guard_char_churn_ms"`
	RaidGuardNameChurnMax                int      `toml:"raid_guard_name_churn_max"`
	RaidGuardCorrWindow                  int      `toml:"raid_guard_corr_window"`
	RaidGuardCorrIPIDs                   int      `toml:"raid_guard_corr_ipids"`
	RaidGuardCorrIPIDsWeak               int      `toml:"raid_guard_corr_ipids_weak"`
	RaidGuardCorrBreadth                 int      `toml:"raid_guard_corr_breadth"`
	RaidGuardCorrMinLen                  int      `toml:"raid_guard_corr_min_len"`
	RaidGuardCorrMaxEntries              int      `toml:"raid_guard_corr_max_entries"`
	RaidGuardArrivalBurst                int      `toml:"raid_guard_arrival_burst"`
	RaidGuardAutoLockdown                bool     `toml:"raid_guard_auto_lockdown"`
	RaidGuardSlurSevereWeight            int      `toml:"raid_guard_slur_severe_weight"`
	RaidGuardSlurFlaggedWeight           int      `toml:"raid_guard_slur_flagged_weight"`
	ModcallCooldown                      int      `toml:"modcall_cooldown"`
	ConnRateLimit                        int      `toml:"connection_rate_limit"`
	ConnRateLimitWindow                  int      `toml:"connection_rate_limit_window"`
	ConnFloodAutoban                     bool     `toml:"conn_flood_autoban"`
	ConnFloodAutobanThreshold            int      `toml:"conn_flood_autoban_threshold"`
	PacketFloodAutoban                   bool     `toml:"packet_flood_autoban"`
	RawPacketRateLimit                   int      `toml:"raw_packet_rate_limit"`
	RawPacketRateLimitWindow             float64  `toml:"raw_packet_rate_limit_window"`
	OOCRateLimit                         int      `toml:"ooc_rate_limit"`
	OOCRateLimitWindow                   int      `toml:"ooc_rate_limit_window"`
	PingRateLimit                        int      `toml:"ping_rate_limit"`
	PingRateLimitWindow                  int      `toml:"ping_rate_limit_window"`
	NewIPIDOOCCooldown                   int      `toml:"new_ipid_ooc_cooldown"`
	NewIPIDICCooldown                    int      `toml:"new_ipid_ic_cooldown"`
	NewIPIDModcallCooldown               int      `toml:"new_ipid_modcall_cooldown"`
	GlobalNewIPRateLimit                 int      `toml:"global_new_ip_rate_limit"`
	GlobalNewIPRateLimitWindow           int      `toml:"global_new_ip_rate_limit_window"`
	IPRetentionDays                      int      `toml:"ip_retention_days"`
	WebAOAllowedOrigin                   string   `toml:"webao_allowed_origin"`
	AutoModEnabled                       bool     `toml:"automod_enabled"`
	AutoModWordlist                      string   `toml:"automod_wordlist"`
	AutoModAction                        string   `toml:"automod_action"`
	AutoModNukeBanDuration               string   `toml:"automod_nuke_ban_duration"`
	RandomSongCooldown                   int      `toml:"random_song_cooldown"`
	RandomSongCooldownDJ                 int      `toml:"random_song_cooldown_dj"`
	RandomSongCooldownMod                int      `toml:"random_song_cooldown_mod"`
	BotBanPlaytimeThreshold              int      `toml:"botban_playtime_threshold"`
	IPHubAPIKey                          string   `toml:"iphub_api_key"`
	EnableTranslator                     bool     `toml:"enable_translator_punishment"`
	TranslatorAPIURL                     string   `toml:"translator_api_url"`
	TranslatorAPIKey                     string   `toml:"translator_api_key"`
	TranslateCooldown                    int      `toml:"translate_cooldown"`
	EnableCasino                         bool     `toml:"enable_casino"`
	EnableAccounts                       bool     `toml:"enable_accounts"`
	RegisterCaptcha                      bool     `toml:"register_captcha"`
	JoinCaptcha                          bool     `toml:"join_captcha"`
	JoinCaptchaStrikes                   int      `toml:"join_captcha_strikes"`
	JoinCaptchaTimeout                   int      `toml:"join_captcha_timeout"`
	JoinCaptchaKinds                     []string `toml:"join_captcha_kinds"`
	JoinCaptchaRemember                  bool     `toml:"join_captcha_remember"`
	JoinCaptchaMinPlaytime               int      `toml:"join_captcha_min_playtime"`
	JoinCaptchaAction                    string   `toml:"join_captcha_action"`
	JoinCaptchaQuestions                 string   `toml:"join_captcha_questions"`
	JoinCaptchaCustomOnly                bool     `toml:"join_captcha_custom_only"`
	JoinCaptchaSecret                    string   `toml:"join_captcha_secret"`
	JoinCaptchaRotate                    int      `toml:"join_captcha_rotate"`
	CaptchaPlugin                        string   `toml:"captcha_plugin"`
	CaptchaPluginTimeout                 int      `toml:"captcha_plugin_timeout"`
	JoinCaptchaPopup                     bool     `toml:"join_captcha_popup"`
	// JoinPopup and JoinPopupMessage are the join popup (see joinpopup.go):
	// an operator-authored welcome message -- rules, a Discord invite, a
	// website -- shown once to any IPID connecting to this server for the
	// very first time ever. Independent of the join captcha above, though it
	// reuses the same client-side AO2 BB dialog to make it unmissable.
	// JoinPopupMessage supports multiple lines (a triple-quoted TOML string);
	// a blank message disables the feature even if JoinPopup is true.
	JoinPopup           bool     `toml:"join_popup"`
	JoinPopupMessage    string   `toml:"join_popup_message"`
	EnableCommunityVote bool     `toml:"enable_community_vote"`
	VoteThreshold       int      `toml:"vote_threshold"`
	VoteDuration        int      `toml:"vote_duration"`
	VoteActions         []string `toml:"vote_actions"`
	VoteMuteDuration    int      `toml:"vote_mute_duration"`
	TypingRacePhrases   []string `toml:"typing_race_phrases"`
	EnableNewspaper     bool     `toml:"enable_newspaper"`
	NewspaperInterval   string   `toml:"newspaper_interval"`
	NewspaperSections   []string `toml:"newspaper_sections"`
	// YouTubePlayPrefix, when non-empty and starting with "http", turns on the
	// /play <youtube-link> integration. The prefix is the URL stem that
	// clients fetch the downloaded MP3 from (e.g. "https://cdn.example.com/yt/").
	// The literal token "{ASSET_URL}" is expanded to ServerConfig.AssetURL at
	// use time so operators don't have to repeat the asset host.
	YouTubePlayPrefix string `toml:"youtube_play_prefix"`
	// YouTubeDownloadDestination is the destination URI for downloaded mp3s.
	// Only file:// (local filesystem) is supported right now — e.g.
	// "file:///var/lib/athena/yt".
	YouTubeDownloadDestination string `toml:"youtube_download_destination"`
	// YouTubeMaxDurationSeconds rejects videos longer than this when probed.
	// 0 falls back to 600 (10 minutes).
	YouTubeMaxDurationSeconds int `toml:"youtube_max_duration_seconds"`
	// YouTubeCookiesPath, when non-empty, is passed to yt-dlp as
	// --cookies <path>. Used to bypass YouTube's bot-detection / age-gate
	// walls by presenting a logged-in session.
	YouTubeCookiesPath string `toml:"youtube_cookies_path"`
	// MaxConnectionGoroutines caps the number of concurrent connection-handling
	// goroutines.  When the pool is full, new connections wait until a slot
	// becomes available rather than spinning up an unbounded number of goroutines.
	// 0 (the default) disables the pool and preserves the original unbounded behaviour.
	MaxConnectionGoroutines int `toml:"max_connection_goroutines"`

	// PingTimeout is the number of seconds a joined client may go without sending
	// a CH (ping) packet before being forcibly disconnected.  0 disables the check.
	PingTimeout int `toml:"ping_timeout"`

	// PlayerLockdownThreshold is the player count at which the server automatically
	// stops accepting new connections.  When the connected player count reaches this
	// value, new join attempts are rejected with a "server is full" message.
	// 0 disables the threshold (only the hard max_players cap applies).
	// Mods can change this at runtime with /setplayerlimit.
	PlayerLockdownThreshold int `toml:"player_lockdown_threshold"`

	// LockdownMinPlaytime is the minimum accumulated playtime, in minutes, a
	// connected IPID needs to survive the purge that runs the instant lockdown
	// is switched on (in-game /lockdown or the Discord bot's /lockdown on).
	// Every currently-connected, non-moderator client below this is kicked
	// immediately -- lockdown's new-connection block only stops the next wave
	// of a flood, it does nothing about the flood that's already inside.
	// 0 disables the purge (lockdown still only blocks new IPIDs, as before).
	// Mods can change this at runtime with /setlockdownplaytime.
	LockdownMinPlaytime int `toml:"lockdown_min_playtime"`

	// SecretSeed is a server-private key used to derive lockdown whitelist
	// passkeys (HMAC-SHA256 of an IPID, keyed by this value) -- a per-IPID
	// credential shown to a player blocked by lockdown that staff can redeem
	// with /lockdown whitelist <passkey> to let them in and permanently
	// exempt them from the playtime purge, without needing them online or
	// needing staff to know their IPID up front. Treat this like a password:
	// anyone who has it can mint a valid passkey for ANY IPID. Leave blank to
	// disable the whole passkey system (the "not a ban" message and
	// /lockdown whitelist <passkey> both go inert; /lockdown add and
	// /lockdown whitelist all are unaffected). Changing this invalidates
	// every previously-issued passkey.
	SecretSeed string `toml:"secretseed"`

	// LockdownString is the message template shown to a player blocked by
	// lockdown (rejected at connect, or kicked by the playtime purge),
	// explaining that it's not a ban and giving them their passkey to relay
	// to staff. The literal token "{id}" is replaced with their passkey; if
	// omitted, the passkey is appended to the end instead. Only used when
	// SecretSeed is set.
	LockdownString string `toml:"lockdownstring"`

	// EnableTUI, when true, starts the read-only terminal dashboard at server
	// launch -- the same effect as passing the -tui CLI flag. The flag still
	// wins if it is explicitly set; this entry is for operators who want the
	// dashboard to be the default without remembering the flag.
	EnableTUI bool `toml:"enable_tui"`
}

type LogConfig struct {
	// BufSize caps the number of lines retained in each area's /modcall
	// report buffer, as a safety valve against unbounded memory use in an
	// extremely active area. Retention is otherwise governed by a rolling
	// 24-hour window (see Area.UpdateBuffer); <= 0 means no cap.
	BufSize              int      `toml:"log_buffer_size"`
	LogLevel             string   `toml:"log_level"`
	LogDir               string   `toml:"log_directory"`
	LogMethods           []string `toml:"log_methods"`
	EnableAreaLogging    bool     `toml:"enable_area_logging"`
	EnableNetworkLogging bool     `toml:"enable_network_logging"`
}

type MSConfig struct {
	Advertise bool   `toml:"advertise"`
	MSAddr    string `toml:"addr"`
}

type DiscordConfig struct {
	BotToken  string `toml:"bot_token"`
	GuildID   string `toml:"guild_id"`
	ModRoleID string `toml:"mod_role_id"`
}

// VoiceConfig controls the optional server-relayed voice-chat feature.
// Every audio frame travels client → Athena → other clients in the same
// area; no peer-to-peer path exists, so peers never learn each other's
// IPs.  Disabled by default.
type VoiceConfig struct {
	EnableVoice             bool `toml:"enable_voice"`
	PTTOnly                 bool `toml:"ptt_only"`
	MaxPeersPerArea         int  `toml:"max_peers_per_area"`
	MaxFrameBytes           int  `toml:"max_frame_bytes"`
	DefaultAreaVoiceAllowed bool `toml:"default_area_voice_allowed"`
	JoinRateLimit           int  `toml:"join_rate_limit"`
	JoinRateLimitWindow     int  `toml:"join_rate_limit_window"`
	FrameRateLimit          int  `toml:"frame_rate_limit"`
	FrameRateLimitWindow    int  `toml:"frame_rate_limit_window"`
	NewIPIDVoiceCooldown    int  `toml:"new_ipid_voice_cooldown"`
}

// Returns a default configuration.
func defaultConfig() *Config {
	return DefaultConfig()
}

// DefaultConfig returns the default server configuration. Exported for testing.
func DefaultConfig() *Config {
	return &Config{
		ServerConfig{
			Addr:                                 "",
			Port:                                 27016,
			AdvertiseHostname:                    "",
			Name:                                 "Unnamed Server",
			Desc:                                 "",
			MaxPlayers:                           100,
			MaxMsg:                               256,
			BanLen:                               "3d",
			EnableWS:                             false,
			WSPort:                               27017,
			EnableWSS:                            false,
			WSSPort:                              443,
			TLSCertPath:                          "",
			TLSKeyPath:                           "",
			ReverseProxyMode:                     false,
			ReverseProxyHTTPPort:                 80,
			ReverseProxyHTTPSPort:                443,
			MCLimit:                              16,
			MaxDice:                              100,
			MaxSide:                              100,
			MaxStatement:                         10,
			RateLimit:                            20,
			RateLimitWindow:                      10,
			RateLimitKickAutoban:                 true,
			RateLimitKickAutobanThreshold:        2,
			RateLimitKickAutobanWindow:           600,
			RateLimitKickAutobanDuration:         "15m",
			RateLimitKickAutobanMinPlaytime:      1200, // 20 hours
			RateLimitKickAutobanLenientPlaytime:  120,  // 2 hours
			RateLimitKickAutobanLenientThreshold: 5,
			RaidGuardEnabled:                     false,
			RaidGuardMaxAction:                   "captcha",
			RaidGuardMinPlaytime:                 1200, // 20 hours -- never acted on
			RaidGuardLenientPlaytime:             120,  // 2 hours -- thresholds doubled
			RaidGuardLenientScale:                200,  // 2h+: needs twice the evidence
			RaidGuardStrictPlaytime:              15,   // under 15m of history = brand new
			RaidGuardStrictScale:                 70,
			RaidGuardUnderAttackScale:            70, // brand new: needs 30% less
			RaidGuardBanDuration:                 "30m",
			RaidGuardScoreWatch:                  40,
			RaidGuardScoreChallenge:              60,
			RaidGuardScoreSilence:                80,
			RaidGuardScoreKick:                   100,
			RaidGuardScoreBan:                    160,
			RaidGuardObjectionMinMsgs:            3,
			RaidGuardObjectionFraction:           0.8,
			RaidGuardFastCharPickMs:              250,
			RaidGuardFastSpeechMs:                1500,
			RaidGuardCharChurnMs:                 1000,
			RaidGuardNameChurnMax:                3,
			RaidGuardCorrWindow:                  30,
			RaidGuardCorrIPIDs:                   4,
			RaidGuardCorrIPIDsWeak:               2,
			RaidGuardCorrBreadth:                 3,
			RaidGuardCorrMinLen:                  15,
			RaidGuardCorrMaxEntries:              4096,
			RaidGuardArrivalBurst:                12,
			RaidGuardAutoLockdown:                false,
			RaidGuardSlurSevereWeight:            40,
			RaidGuardSlurFlaggedWeight:           20,
			ModcallCooldown:                      0,
			ConnRateLimit:                        10,
			ConnRateLimitWindow:                  10,
			ConnFloodAutoban:                     true,
			ConnFloodAutobanThreshold:            6,
			PacketFloodAutoban:                   true,
			RawPacketRateLimit:                   20,
			RawPacketRateLimitWindow:             2,
			OOCRateLimit:                         4,
			OOCRateLimitWindow:                   1,
			PingRateLimit:                        10,
			PingRateLimitWindow:                  5,
			NewIPIDOOCCooldown:                   10,
			NewIPIDICCooldown:                    10,
			NewIPIDModcallCooldown:               60,
			GlobalNewIPRateLimit:                 5,
			GlobalNewIPRateLimitWindow:           10,
			IPRetentionDays:                      0,
			WebAOAllowedOrigin:                   "web.aceattorneyonline.com",
			AutoModEnabled:                       false,
			AutoModWordlist:                      "banned_words.txt",
			AutoModAction:                        "shadow",
			AutoModNukeBanDuration:               "permanent",
			RandomSongCooldown:                   20,
			RandomSongCooldownDJ:                 5,
			RandomSongCooldownMod:                0,
			BotBanPlaytimeThreshold:              120,
			EnableTranslator:                     false,
			TranslatorAPIURL:                     "https://api-free.deepl.com/v2/translate",
			TranslatorAPIKey:                     "",
			TranslateCooldown:                    25,
			EnableCasino:                         false,
			EnableAccounts:                       true,
			RegisterCaptcha:                      true,
			JoinCaptcha:                          false,
			JoinCaptchaStrikes:                   3,
			JoinCaptchaTimeout:                   180,
			JoinCaptchaKinds:                     nil,
			JoinCaptchaRemember:                  true,
			JoinCaptchaMinPlaytime:               300,
			JoinCaptchaAction:                    "mute",
			JoinCaptchaQuestions:                 "captcha_questions.txt",
			JoinCaptchaCustomOnly:                false,
			JoinCaptchaSecret:                    "",
			JoinCaptchaRotate:                    3600,
			CaptchaPlugin:                        "",
			CaptchaPluginTimeout:                 3000,
			JoinCaptchaPopup:                     true,
			JoinPopup:                            false,
			JoinPopupMessage:                     "",
			EnableCommunityVote:                  false,
			VoteThreshold:                        3,
			VoteDuration:                         120,
			VoteActions:                          []string{"kick"},
			VoteMuteDuration:                     300,
			YouTubePlayPrefix:                    "",
			YouTubeDownloadDestination:           "",
			YouTubeMaxDurationSeconds:            600,
			YouTubeCookiesPath:                   "",
		},
		LogConfig{
			BufSize:              0,
			LogLevel:             "info",
			LogDir:               "logs",
			LogMethods:           []string{"stdout"},
			EnableAreaLogging:    false,
			EnableNetworkLogging: false,
		},
		MSConfig{
			Advertise: false,
			MSAddr:    "https://servers.aceattorneyonline.com/servers",
		},
		DiscordConfig{
			BotToken:  "",
			GuildID:   "",
			ModRoleID: "",
		},
		VoiceConfig{
			EnableVoice:             false,
			PTTOnly:                 true,
			MaxPeersPerArea:         6,
			MaxFrameBytes:           4000,
			DefaultAreaVoiceAllowed: true,
			JoinRateLimit:           3,
			JoinRateLimitWindow:     10,
			FrameRateLimit:          60,
			FrameRateLimitWindow:    1,
			NewIPIDVoiceCooldown:    30,
		},
	}
}

// Load reads the server's main configuration file.
func (conf *Config) Load() error {
	_, err := toml.DecodeFile(ConfigPath+"/config.toml", conf)
	if err != nil {
		return err
	}
	return nil
}

// GetConfig returns the server's config options.
func GetConfig() (*Config, error) {
	conf := defaultConfig()
	err := conf.Load()

	if err != nil {
		return nil, err
	}

	return conf, nil
}

// LoadMusic reads the server's music file, returning it's contents.
func LoadMusic() ([]string, error) {
	var musicList []string
	f, err := os.Open(ConfigPath + "/music.txt")
	if err != nil {
		return nil, err
	}
	defer f.Close()
	in := bufio.NewScanner(f)
	for in.Scan() {
		musicList = append(musicList, in.Text())
	}
	if len(musicList) == 0 {
		return nil, fmt.Errorf("empty musiclist")
	}
	if strings.ContainsRune(musicList[0], '.') {
		musicList = append([]string{"Songs"}, musicList...)
	}
	return musicList, nil
}

// LoadCDNs reads the CDN whitelist from cdns.txt.
// Returns an empty slice (no error) if the file does not exist.
// Lines starting with # and blank lines are skipped.
func LoadCDNs() []string {
	f, err := os.Open(ConfigPath + "/cdns.txt")
	if err != nil {
		return nil
	}
	defer f.Close()
	var list []string
	in := bufio.NewScanner(f)
	for in.Scan() {
		line := strings.TrimSpace(in.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		list = append(list, strings.ToLower(line))
	}
	return list
}

// LoadFile reads a server file, returning it's contents.
func LoadFile(file string) ([]string, error) {
	var l []string
	f, err := os.Open(ConfigPath + file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	in := bufio.NewScanner(f)
	lineNo := 0
	skipped := 0
	for in.Scan() {
		lineNo++
		line := in.Text()
		// AO2 list packets (SC, SM, etc.) are emitted on the WebSocket as text
		// frames, which RFC 6455 requires to be valid UTF-8. A single Latin-1 /
		// Shift-JIS / cp1252 byte anywhere in the list aborts the entire frame
		// with a StatusProtocolError close from the browser — the symptom is
		// "every WebAO client disconnects mid-handshake". Skip the bad line and
		// log its number so the operator can fix the source file.
		if !utf8.ValidString(line) {
			logger.LogWarningf("settings: %v line %d contains invalid UTF-8, skipped: %q", file, lineNo, line)
			skipped++
			continue
		}
		// AO2 packets use '#' as the field delimiter and '#%' as the packet
		// terminator. A '#' inside any list entry (character name, music filename,
		// etc.) splits it into two protocol fields, injecting a phantom entry and
		// shifting every subsequent ID by one — the symptom is character-selection
		// failures and dropped IC messages for all characters after the bad entry.
		if strings.ContainsRune(line, '#') {
			logger.LogWarningf("settings: %v line %d contains '#' which breaks AO2 protocol packets, skipped: %q", file, lineNo, line)
			skipped++
			continue
		}
		l = append(l, line)
	}
	if skipped > 0 {
		logger.LogWarningf("settings: %v had %d invalid-UTF-8 line(s) removed; re-save the file as UTF-8 to keep them", file, skipped)
	}
	return l, nil
}

// LoadAreas reads the server's area configuration file, returning it's contents.
func LoadAreas() ([]area.AreaData, error) {
	var conf struct {
		Area []area.AreaData
	}
	_, err := toml.DecodeFile(ConfigPath+"/areas.toml", &conf)
	if err != nil {
		return conf.Area, err
	}
	if len(conf.Area) == 0 {
		return conf.Area, fmt.Errorf("empty arealist")
	}
	return conf.Area, nil
}

// LoadAreas reads the server's role configuration file, returning it's contents.
func LoadRoles() ([]permissions.Role, error) {
	var conf struct {
		Role []permissions.Role
	}
	_, err := toml.DecodeFile(ConfigPath+"/roles.toml", &conf)
	if err != nil {
		return conf.Role, err
	}
	if len(conf.Role) == 0 {
		return conf.Role, fmt.Errorf("empty rolelist")
	}
	return conf.Role, nil
}

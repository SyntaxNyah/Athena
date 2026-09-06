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
	"crypto/md5"
	"encoding/base64"
	"fmt"
	"math/rand"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/MangosArentLiterature/Athena/internal/area"
	"github.com/MangosArentLiterature/Athena/internal/db"
	"github.com/MangosArentLiterature/Athena/internal/logger"
	"github.com/MangosArentLiterature/Athena/internal/packet"
	"github.com/MangosArentLiterature/Athena/internal/permissions"
	"github.com/MangosArentLiterature/Athena/internal/sliceutil"
	"github.com/MangosArentLiterature/Athena/internal/webhook"
)

// Documentation for AO2's network protocol can be found here:
// https://github.com/AttorneyOnline/docs/blob/master/docs/development/network.md

// commandRegex matches valid command names (e.g., /join, /join-tournament, /area_kick, /51), case-insensitively.
var commandRegex = regexp.MustCompile(`(?i)^/[a-z0-9_]+(-[a-z0-9_]+)*`)

// tstNavRegex matches testimony navigation controls (<, >, >N) in IC messages.
// Compiled once at package init to avoid repeated allocation during testimony playback.
var tstNavRegex = regexp.MustCompile(`[<>]([[:digit:]]+)?`)

// maxShownameLength is the maximum number of characters allowed in a showname.
const maxShownameLength = 30

// accountWelcomeMsg is the on-join welcome message shown to unauthenticated players
// when the casino is disabled but the optional account system is enabled
// (`enable_accounts = true`). It advertises the *non-gambling* half of the
// account system: free account creation, wardrobe, default cosmetic tag, and
// playtime tracking. It also documents every tag category available in /shop and
// how to equip a tag with /settag — because with the casino off, tags are free.
//
// Defined as a compile-time constant so there is zero runtime allocation per join.
const accountWelcomeMsg = "👤 Welcome! This server offers an optional free player account.\n\n" +
	"📋 What your account is for:\n" +
	"  • 👗 Wardrobe — save favourite characters and swap between them instantly.\n" +
	"  • 🏷️ Default tag — pick any cosmetic tag from the catalog and wear it next to your name.\n" +
	"  • ⏱️ Playtime tracking — hours persist across sessions and feed /playtime top.\n\n" +
	"🔑 Account commands:\n" +
	"  • /register <username> <password>  — create a free account (username 3–20, password 6+)\n" +
	"  • /login <username> <password>     — sign in on reconnect\n" +
	"  • /account                         — view your profile\n" +
	"  • /wardrobe                        — list favourite characters\n" +
	"  • /favourite <char>                — add/remove a character from the wardrobe\n" +
	"  • /playtime top                    — see the playtime leaderboard\n\n" +
	"🏷️ Default tags — every tag in /shop is FREE to equip:\n" +
	"  • 🎰 gambling   — 30 tags (Gambler, Lucky, High Roller, Whale, Godlike, Infinite…)\n" +
	"  • ⚖️ attorney   — 15 tags (Objection!, Take That!, Hold It!, Prosecutor, Magatama…)\n" +
	"  • 🌸 anime      — 15 tags (Weeb, Senpai, Kawaii, Waifu Haver, Protagonist…)\n" +
	"  • 🎮 gamer      — 15 tags (Noob, Tryhard, Speedrunner, PvP God, Gaming Chair…)\n" +
	"  • 🌷 girly      — 12 tags (Princess, Fairy, Queen, Sparkle, Divine Feminine…)\n" +
	"  • 😂 meme       — 18 tags (Bruh, NPC, Sus, Based, Gigachad, Meme Lord…)\n" +
	"  • 👑 prestige   — 10 tags (Newcomer, Veteran, Grandmaster, Overlord, Absolute Unit…)\n" +
	"  Browse a category:  /shop <category>   (e.g. /shop attorney)\n" +
	"  Equip a tag:        /settag <tag_id>   (e.g. /settag tag_objection)\n" +
	"  Remove your tag:    /settag none\n\n" +
	"💡 Accounts are entirely optional. Skip /register if you just want to play."

// casinoWelcomeMsg is the on-join welcome message shown to unauthenticated players when the
// casino is enabled. Defined as a compile-time constant so there is zero runtime allocation
// on every player join.
const casinoWelcomeMsg = "🎰 Welcome! This server runs the Nyathena Casino — virtual chips, games, jobs & more.\n\n" +
	"📖 Quick navigation:\n" +
	"  • /help          — see every available command\n" +
	"  • /casino        — open the casino dashboard (active tables, balance, game list)\n" +
	"  • /chips         — check your chip balance\n" +
	"  • /jobs          — list all jobs you can work to earn chips\n" +
	"  • /shop          — spend chips on permanent tags & upgrades\n\n" +
	"💰 Ways to earn chips:\n" +
	"  • Everyone starts with 500 chips automatically.\n" +
	"  • Earn 1 chip per hour of playtime (more with /shop passive upgrades!).\n" +
	"  • Work a job: /janitor /busker /paperboy /bailiffjob /clerk  (40–60 min cooldowns).\n" +
	"  • Unscramble events every 30–60 min — type the answer in IC chat to win 10 chips!\n\n" +
	"🍻 THE BAR — /bar menu | /bar buy <drink>\n" +
	"  34 drinks, every one with RISK and HUGE variance. Big wins or big losses!\n" +
	"  beer wine tequila moonshine absinthe mystery poison dragonblood goldenelixir voiddrink\n" +
	"  angelwine devilswhiskey ... use /bar menu to see them all!\n\n" +
	"🛒 Spend your chips at /shop:\n" +
	"  • 115+ cosmetic tags in 7 categories (from 100 chips!) · job passes · passive income\n" +
	"  → /shop <category> to browse  |  /shop buy <id> to purchase\n\n" +
	"📊 Leaderboards: /richest  /playtime top  /unscramble top  /jobtop\n\n" +
	"👗 Wardrobe: /favourite <char> · /wardrobe · /wardrobe <char>  (save favourite characters!)\n\n" +
	"💡 /register <username> <password>  |  /login <username> <password>\n" +
	"🔒 Passwords stored with bcrypt — never in plain text.\n" +
	"🔇 Use /gamble hide to toggle gambling broadcast messages."

// validDeskMods is the set of accepted values for MSPacket.DeskMod.
// Defined at package level to avoid a slice allocation on every IC message.
var validDeskMods = []string{"chat", "0", "1", "2", "3", "4", "5"}

type pktMapValue struct {
	Args     int
	MustJoin bool
	Func     func(client *Client, p *packet.Packet)
}

var PacketMap = map[string]pktMapValue{
	"HI":       {1, false, pktHdid},
	"ID":       {2, false, pktId},
	"askchaa":  {0, false, pktResCount},
	"RC":       {0, false, pktReqChar},
	"RM":       {0, false, pktReqAM},
	"RD":       {0, false, pktReqDone},
	"CC":       {2, true, pktChangeChar},
	"MS":       {15, true, pktIC},
	"MC":       {2, true, pktAM},
	"HP":       {2, true, pktHP},
	"RT":       {1, true, pktWTCE},
	"TT":       {2, true, pktTT},
	"CT":       {2, true, pktOOC},
	"PE":       {3, true, pktAddEvi},
	"DE":       {1, true, pktRemoveEvi},
	"EE":       {4, true, pktEditEvi},
	"PW":       {0, true, pktPW},
	"CH":       {0, false, pktPing},
	"ZZ":       {0, true, pktModcall},
	"MA":       {3, true, pktMA},
	"SETCASE":  {7, true, pktSetCase},
	"CASEA":    {6, true, pktCaseAnn},
	"VS_JOIN":  {0, true, pktVSJoin},
	"VS_LEAVE": {0, true, pktVSLeave},
	"VS_FRAME": {1, true, pktVSFrame},
	"VS_SPEAK": {1, true, pktVSSpeak},
}

// Handles HI#%
func pktHdid(client *Client, p *packet.Packet) {
	hi, err := packet.ParseHI(p.Body)
	if err != nil || strings.TrimSpace(hi.HDID) == "" || client.Uid() != -1 || client.Hdid() != "" {
		return
	}

	// Athena does not store the client's raw HDID, but rather, it's MD5 hash.
	// This is done not only for privacy reasons, but to ensure stored HDIDs will be a reasonable length.
	hash := md5.Sum([]byte(decode(hi.HDID)))
	client.SetHdid(base64.StdEncoding.EncodeToString(hash[:]))
	client.SetHdid(client.Hdid()[:len(client.Hdid())-2]) // Removes the trailing padding.

	if client.CheckBanned(db.HDID) {
		return
	}

	client.Send(&packet.IDClient{PlayerNumber: 0, Software: "Nyathena", Version: encode(version)})
}

// Handles ID#%
func pktId(client *Client, _ *packet.Packet) {
	if client.Uid() != -1 {
		return
	}
	// Body is the client-side ID handshake (software, version). We don't
	// currently store either field, but the parse keeps the handler honest
	// about the wire format.
	client.Send(&packet.PN{
		PlayerCount:       players.GetPlayerCount(),
		MaxPlayers:        config.MaxPlayers,
		ServerDescription: encode(GetServerDesc()),
	})
	client.Send(&packet.FL{Features: []string{
		"noencryption", "yellowtext", "prezoom", "flipping", "customobjections",
		"fastloading", "deskmod", "evidence", "cccc_ic_support", "arup", "casing_alerts",
		"modcall_reason", "looping_sfx", "additive", "effects", "y_offset",
		"expanded_desk_mods", "auth_packet",
	}})

	if config.AssetURL != "" {
		client.Send(&packet.ASS{AssetURL: config.AssetURL})
	}
	sendVoiceCaps(client)
}

// Handles askchaa#%
func pktResCount(client *Client, _ *packet.Packet) {
	raidGuardOnAskchaa(client)
	if client.Uid() != -1 || client.Hdid() == "" {
		return
	}
	if players.GetPlayerCount() >= config.MaxPlayers {
		logger.LogInfo("Player limit reached")
		client.SendSync(&packet.BD{Reason: "This server is currently full."})
		client.conn.Close()
		return
	}
	// Capacity lockdown: reject new connections when the player count has reached
	// the operator-configured threshold (0 = disabled).
	// Known IPs (returning players in the DB) bypass this cap, matching the behaviour
	// of serverLockdown which already exempts previously-seen IPIDs.
	if threshold := int(playerLockdownThreshold.Load()); threshold > 0 && players.GetPlayerCount() >= threshold {
		ipFirstSeenTracker.mu.Lock()
		_, known := ipFirstSeenTracker.times[client.Ipid()]
		ipFirstSeenTracker.mu.Unlock()
		if !known {
			logger.LogInfof("Connection from %v rejected (player capacity lockdown, threshold %v)", client.Ipid(), threshold)
			client.SendSync(&packet.BD{Reason: "This server is not currently accepting new connections."})
			client.conn.Close()
			return
		}
	}
	client.joining = true // This simply exists to prevent skipping the askchaa#% packet and bypassing the player count check.
	client.Send(&packet.SI{
		CharCount:     len(getCharacters()),
		EvidenceCount: len(areas[0].Evidence()),
		MusicCount:    len(getMusicList()),
	})
}

// Handles RC#%
func pktReqChar(client *Client, _ *packet.Packet) {
	raidGuardOnHandshakeStep(client)
	client.Send(&packet.SC{Entries: getCharacters()})
}

// Handles RM#%
func pktReqAM(client *Client, _ *packet.Packet) {
	raidGuardOnHandshakeStep(client)
	// smPacket is a pre-built FantaCode blob (avoids a strings.Join per
	// connection in the common case). JSON-mode clients can't read it, so
	// fall back to the typed-Outgoing path, which goes through BuildJSON.
	// Music names are AO2-escape-encoded here for symmetry with the
	// FantaCode path (buildSMPacket) — every other outbound string the
	// server emits is already encoded, and the encoded form round-trips
	// fine through a JSON string field.
	if client.JSONMode() {
		items := make([]string, 0, len(areas)+len(getMusicList()))
		for _, a := range areas {
			items = append(items, a.Name())
		}
		for _, m := range getMusicList() {
			items = append(items, encode(m))
		}
		client.Send(&packet.SM{Items: items})
		return
	}
	// Enqueued, never written inline. This used to call client.write, which
	// held client.mu across a blocking ~45 KB socket write with no deadline --
	// and Client.Area() takes that same mutex, so every broadcast on the server
	// queued behind any connection that had stopped reading. See
	// TestStuckWriterCannotStallBroadcasts.
	client.sendPrebuilt(getSMPacketBytes())
}

// Handles RD#%
func pktReqDone(client *Client, _ *packet.Packet) {
	raidGuardOnHandshakeStep(client)
	if client.Uid() != -1 || !client.joining || client.Hdid() == "" {
		return
	}
	client.SetUid(uids.GetUid())
	clients.RegisterUID(client)
	client.SetConnectedAt(time.Now())
	client.lastPingNano.Store(time.Now().UnixNano()) // seed so the ping timeout window starts from join time
	players.AddPlayer()
	if config.Advertise {
		updatePlayers <- players.GetPlayerCount()
	}
	client.JoinArea(areas[0])
	client.Send(&packet.DONE{})
	// Send BN after DONE so WebAO's viewport is fully initialized before the
	// background and desk-overlay images are loaded.  Akashi follows the same
	// ordering: HP / FA → DONE → BN.  Sending BN before DONE caused desk
	// images to load against an unrendered viewport, leaving desks invisible
	// on WebAO even when deskmod indicated they should be shown.
	client.Send(&packet.BN{Background: areas[0].Background()})
	// Re-emit VS_CAPS at the end of the join handshake.  pktId already sends
	// it once during the early ID phase, but some clients (notably webAO,
	// which builds its voice subsystem after SI/SC/SM/DONE) ignore packets
	// that arrive before that subsystem exists.  Sending it again here, after
	// DONE and after the client's UID is assigned, guarantees the voice panel
	// can render whether or not the client also caught the early copy.
	sendVoiceCaps(client)
	sendCMArup()
	sendStatusArup()
	sendLockArup()
	// Notify the client of their actual UID so the player list widget filters correctly.
	client.Send(&packet.IDClient{PlayerNumber: client.Uid(), Software: "Nyathena", Version: encode(version)})
	sendPlayerListToClient(client)
	broadcastPlayerJoin(client)
	sendModIPIDsToClient(client) // no-op unless this connection is already BAN_INFO-permissioned at join
	broadcastIPIDToMods(client.Uid(), client.Ipid())
	client.restorePunishments()
	client.restoreRandomCharCurse()
	client.restoreShownamePunishStain()

	// Join popup: shown first, ahead of every other join-sequence message, to
	// a connection whose IPID has never connected to this server before. It's
	// a no-op for anyone else, so it costs nothing to check up front.
	issueJoinPopup(client)

	// Casino on-join setup: seed chip balance and prompt unregistered players.
	// When the casino is off but the account system is enabled, the account
	// welcome message (wardrobe / default tags / playtime tracking) is shown
	// to unauthenticated joiners.
	if config.EnableCasino {
		ipid := client.Ipid()
		go func() {
			if err := db.EnsureChipBalance(ipid); err != nil {
				logger.LogErrorf("Failed to seed chip balance for %v: %v", ipid, err)
			}
		}()
		if !client.Authenticated() {
			client.SendServerMessage(casinoWelcomeMsg)
		}
	} else if config.EnableAccounts && !client.Authenticated() {
		client.SendServerMessage(accountWelcomeMsg)
	}

	// Sent last (after the account/casino welcome message) so the MOTD lands
	// at the bottom of the join sequence, where it's easiest to spot.
	if motd := GetMotd(); motd != "" {
		client.SendMotd(motd)
	}

	// Join captcha: issued last so the question is the final thing in the join
	// sequence, sitting at the bottom of the OOC log where a new player will
	// actually see it rather than scrolled off above the MOTD.
	issueJoinCaptcha(client)

	logger.LogInfof("Client (IPID:%v UID:%v) joined the server", client.Ipid(), client.Uid())

	// Torment reconnect cycle: if this IPID is lagged, restart the disconnect timer
	// immediately. This punishes reconnect attempts and ensures that lag persists
	// even after they manage to get back in.
	if isIPIDTormented(client.Ipid()) {
		go startTormentDisconnect(client)
	}
}

// getRandomFreeChar returns a random free character ID in the client's area,
// or -1 if no characters are available.
func getRandomFreeChar(client *Client) int {
	var free []int
	for i := range getCharacters() {
		if !client.Area().IsTaken(i) {
			free = append(free, i)
		}
	}
	if len(free) == 0 {
		return -1
	}
	return free[rand.Intn(len(free))]
}

// Handles CC#%
func pktChangeChar(client *Client, p *packet.Packet) {
	// Check rate limit first
	if client.CheckRateLimit() {
		client.KickForRateLimit()
		return
	}
	cc, err := packet.ParseCC(p.Body)
	if err != nil {
		return
	}
	newid := cc.CharID
	// WebAO sends -1 to indicate random character selection.
	if newid == -1 {
		newid = getRandomFreeChar(client)
		if newid == -1 {
			return // No free characters available
		}
	}
	if newid < 0 || newid >= len(getCharacters()) {
		return
	}
	if stuckID := client.charStuckID(); stuckID >= 0 && newid != stuckID {
		client.SendServerMessage(fmt.Sprintf("You are character stuck as %v and cannot change characters.", getCharacters()[stuckID]))
		return
	}
	if client.IsTunged() {
		client.SendServerMessage("You have been tunged and cannot change characters until the effect is removed.")
		return
	}
	client.ChangeCharacter(newid)
	raidGuardOnCharPick(client)
}

// Handles MS#%
func pktIC(client *Client, p *packet.Packet) {
	// Welcome to the MS packet validation hell.

	// Check rate limit first
	if client.CheckRateLimit() {
		client.KickForRateLimit()
		return
	}

	// While lockdown is active, a non-moderator under the purge's playtime
	// threshold is silently dropped -- the purge sweep kicks these connections,
	// but a message sent in the gap before that kick lands (or from a
	// low-playtime IPID that reconnected mid-lockdown) must never reach anyone.
	if serverLockdown.Load() && lockdownShouldSilence(client) {
		return
	}

	if !client.CanSpeakIC() { // Literally 1984
		client.SendServerMessage("You are not allowed to speak in this area.")
		return
	}

	// Join captcha: an unverified new IPID cannot speak in character yet. One
	// atomic load when the gate is not up. Placed after the rate-limit and
	// lockdown checks -- which are cheaper and apply to everyone -- but before
	// any packet parsing, so a message from an unverified connection costs the
	// server nothing beyond the nudge.
	if client.joinCaptchaBlocked("speak in character") {
		return
	}

	// Sending an IC message counts as activity for the opt-in /dc idle timer.
	client.dcTouchActivity()

	// Decode the wire-form client packet body into the structured MSPacket
	// type exactly once. From this point on the IC pipeline operates on named
	// fields; nothing else in this function indexes into the packet by slot.
	// The reverse encode happens once at the bottom via ms.ServerArgs().
	ms := packet.ParseMSClient(p.Body)

	// The client's own values, saved before any moderator override below so that
	// state updates (showname, pairInfo, textColor) always reflect what the
	// client actually sent rather than a forced substitute.
	ownCharName := ms.Character
	ownEmote := ms.Emote
	ownTextColor := ms.TextColor
	ownShowname := ms.Showname
	ownFlip := ms.Flip
	ownSelfOffset := ms.SelfOffset
	hasForcedIniswap := false

	// If a moderator has forced a showname for this client, override whatever
	// name the client sent in the packet.
	if forced := client.ForcedShowname(); forced != "" {
		ownShowname = forced
		ms.Showname = forced
	}

	// If a moderator has forced an iniswap character for this client, override
	// the outgoing IC character name and ID. Both values are pre-computed at
	// command invocation so this hot path performs only a single mutex
	// acquisition and two string assignments — no map lookup or int conversion.
	if charName, charIDStr := client.ForcedIniswapInfo(); charName != "" {
		hasForcedIniswap = true
		ownCharName = charName
		ms.Character = charName
		ms.CharID = charIDStr
	}

	if pos := client.Pos(); pos != "" {
		ms.Side = pos
	} else {
		client.SetPos(ms.Side)
	}

	// Check for expired punishments and collect the still-active ones in a single
	// lock acquisition (avoids a second mutex cycle + second time.Now() call).
	expired, punishments := client.CheckExpiredAndGetPunishments()
	if expired {
		client.SendServerMessage("One or more punishments have expired.")
	}

	// Capture the original decoded message before any punishment transforms so
	// it can be (a) used for icwarp backlog history recording and (b) skipped
	// for icwarp replacement when no history is available.
	var originalICMsg string
	if ms.Message != "" {
		originalICMsg = decode(ms.Message)
	}

	// Apply punishment text modifications
	// Note: punishments is a copy of the active punishments
	// State modifications must use UpdatePunishmentState to persist changes
	var sfxCurseActive bool
	var sfxCurseExternalURL string // non-empty when an external http(s) URL sfxcurse fires
	for i := range punishments {
		p := &punishments[i]

		// Apply text modifications
		if ms.Message != "" {
			decodedMsg := decode(ms.Message)
			var modifiedMsg string

			// Use state-aware version for punishments that need it
			if p.punishmentType == PunishmentTorment || p.punishmentType == PunishmentCipher {
				client.UpdatePunishmentState(p.punishmentType, func(ps *PunishmentState) {
					modifiedMsg = ApplyPunishmentToTextWithState(decodedMsg, p.punishmentType, ps)
				})
			} else if p.punishmentType == PunishmentClickbait {
				// Clickbait headlines star the speaker by name.
				modifiedMsg = applyClickbaitWithName(decodedMsg, clientDisplayName(client))
			} else if p.punishmentType == PunishmentMarkov {
				// Markov babble is generated from this area's recent chat history.
				modifiedMsg = applyMarkov(decodedMsg, client.Area())
			} else if p.punishmentType == PunishmentLovebomb {
				// Resolve the target's display name.
				var targetShowname string
				if p.targetUID >= 0 {
					if target, err := getClientByUid(p.targetUID); err == nil {
						targetShowname = clientDisplayName(target)
					}
				}
				if targetShowname == "" {
					// Reservoir-sample one random area member (excluding self) without
					// allocating a full slice — O(N) single pass, zero extra heap.
					var chosen *Client
					n := 0
					clients.ForEach(func(c *Client) {
						if c.Area() == client.Area() && c.Uid() != client.Uid() {
							n++
							if rand.Intn(n) == 0 {
								chosen = c
							}
						}
					})
					if chosen != nil {
						targetShowname = clientDisplayName(chosen)
					}
				}
				modifiedMsg = applyLovebombMessage(targetShowname)
			} else if p.punishmentType == PunishmentThirdPerson {
				displayName := clientDisplayName(client)
				modifiedMsg = applyThirdPersonWithName(decodedMsg, displayName)
			} else if p.punishmentType == PunishmentTranslator {
				modifiedMsg = applyTranslator(decodedMsg, p.customData)
			} else if p.punishmentType == PunishmentICWarp {
				// Only apply the warp when the client is in the area where they
				// were punished. If they've moved away, leave the message intact.
				if p.icWarpArea != nil && p.icWarpArea == client.Area() {
					if past, ok := client.Area().RandomPastICMessage(client.Ipid()); ok {
						modifiedMsg = past
					} else {
						// No history yet in this area — keep the original message.
						modifiedMsg = decodedMsg
					}
				} else {
					modifiedMsg = decodedMsg
				}
			} else {
				modifiedMsg = ApplyPunishmentToText(decodedMsg, p.punishmentType)
			}
			ms.Message = encode(modifiedMsg)
		}

		// SFX curse: replace the IC packet's SfxName with the cursed sound.
		// For standard AO2 asset-server URLs (containing "/base/sounds/") and
		// bare /base/sounds/ local paths, we extract the filename stem (e.g.
		// "aai-cammy-bubble") so that desktop clients resolve it from their
		// local base/sounds/general/ directory and WebAO clients prepend the
		// configured asset URL. For other external http(s) URLs (e.g. Discord
		// CDN, custom hosting), the full URL is forwarded as-is so that
		// clients supporting URL-based audio can stream it directly.
		// SfxDelay is forced to "0" so the sound fires immediately regardless
		// of what the sender's packet contained.
		// sfxCurseActive is set so that a post-NoInterrupt fixup below can
		// ensure the EmoteModifier is in the set {1, 2, 6}; WebAO's
		// chat_tick only plays the emote SFX for those three values, so
		// EmoteModifier 0 or 5 would otherwise silence the sound even though
		// SfxName is correctly populated.
		if p.punishmentType == PunishmentSfxCurse && p.customData != "" {
			ms.SfxName = sfxBareNameFromURL(p.customData)
			ms.SfxDelay = "0"
			sfxCurseActive = true
			// Track external URLs so that an MC fallback packet can be sent after
			// the IC packet. Desktop AO2 clients play HTTP URLs via their media
			// stack (Qt/etc.) when delivered through MC, which is how /play works.
			// The SfxName URL above covers URL-aware clients; MC covers standard ones.
			sfx := p.customData
			if isExternalSfxURL(sfx) {
				sfxCurseExternalURL = sfx
			}
		}

		// Shrink/Grow/Wide: rewrite SelfOffset ("x&y") to lock the vertical
		// or horizontal offset to the value stored in customData. We preserve
		// the other axis from any client-supplied offset to keep things
		// sensible (e.g. /shrink doesn't override /wide).
		if p.punishmentType == PunishmentShrink || p.punishmentType == PunishmentGrow || p.punishmentType == PunishmentWide {
			locked, err := strconv.Atoi(p.customData)
			if err == nil && locked >= -100 && locked <= 100 {
				curX, curY := 0, 0
				if ms.SelfOffset != "" {
					parts := strings.Split(decode(ms.SelfOffset), "&")
					if v, e := strconv.Atoi(parts[0]); e == nil {
						curX = v
					}
					if len(parts) > 1 {
						if v, e := strconv.Atoi(parts[1]); e == nil {
							curY = v
						}
					}
				}
				switch p.punishmentType {
				case PunishmentShrink, PunishmentGrow:
					curY = locked
				case PunishmentWide:
					curX = locked
				}
				ms.SelfOffset = encode(fmt.Sprintf("%d&%d", curX, curY))
			}
		}

		// Handle name modifications
		if p.punishmentType == PunishmentEmoji {
			ms.Message = GetRandomEmoji()
		}
		if p.punishmentType == PunishmentUncannyValley {
			name := ms.Showname
			if strings.TrimSpace(name) == "" && client.CharID() >= 0 && client.CharID() < len(getCharacters()) {
				name = getCharacters()[client.CharID()]
			}
			if name != "" {
				ms.Showname = MutateShowname(name)
			}
		}
	}

	// Protocol-field punishments (teleport/shakecurse/randomflip/forcecolor/
	// nopreanim/forcepreanim) rewrite the packet's non-text fields. Applied
	// before the validators below so every written value gets checked, and
	// before applyHideDisplay so a hidedisplay offset wins over /teleport.
	applyProtocolPunishments(ms, punishments)

	// HideDisplay: push the speaker's own sprite off-screen so only their text
	// shows. Applied here — after the per-punishment loop but before the IC
	// pair-info snapshot taken further down — so the off-screen offset also
	// propagates to anyone paired with the hidden speaker (they stay hidden on
	// the partner's screen too).
	applyHideDisplay(ms, punishments)

	// Global IC warp: if the area has it enabled and this client is not the
	// exempt moderator, replace their message with a random past IC message
	// they sent in this area. Applied after per-client punishments so it
	// stacks naturally with any other active effects.
	if ms.Message != "" && client.Area().ICWarpGlobal() && client.Uid() != client.Area().ICWarpExemptUID() {
		if past, ok := client.Area().RandomPastICMessage(client.Ipid()); ok {
			ms.Message = encode(past)
		}
	}

	if client.IsParrot() { // Bring out the parrot please.
		ms.Message = getParrotMsg()
	}
	if client.IsNarrator() {
		ms.Emote = ""
	}
	if flip := client.CheckAndToggleDanceFlip(); flip != "" {
		ms.Flip = flip
	}
	// EmoteModifier 4 crashes old clients, remap to PREANIM_ZOOM; only {0,1,2,5,6} are valid (matches Akashi).
	emote_mod, err := strconv.Atoi(ms.EmoteModifier)
	if err != nil {
		logger.LogWarningf("dropped MS from IPID:%v UID:%v — EmoteModifier not an integer; value=%q", client.Ipid(), client.Uid(), ms.EmoteModifier)
		return
	}
	if emote_mod == 4 {
		emote_mod = 6
		ms.EmoteModifier = "6"
	}
	if emote_mod != 0 && emote_mod != 1 && emote_mod != 2 && emote_mod != 5 && emote_mod != 6 {
		logger.LogWarningf("dropped MS from IPID:%v UID:%v — EmoteModifier not in {0,1,2,5,6}; value=%d", client.Ipid(), client.Uid(), emote_mod)
		return
	}
	objection, err := ms.Shout()
	if err != nil {
		logger.LogWarningf("dropped MS from IPID:%v UID:%v — ShoutModifier not an integer; raw=%q", client.Ipid(), client.Uid(), ms.ShoutModifier)
		return
	}
	evi, err := strconv.Atoi(ms.Evidence)
	if err != nil {
		logger.LogWarningf("dropped MS from IPID:%v UID:%v — Evidence not an integer; value=%q", client.Ipid(), client.Uid(), ms.Evidence)
		return
	}
	text, err := strconv.Atoi(ms.TextColor)
	if err != nil {
		logger.LogWarningf("dropped MS from IPID:%v UID:%v — TextColor not an integer; value=%q", client.Ipid(), client.Uid(), ms.TextColor)
		return
	}

	if ms.NonInterruptingPreAnim == "" {
		ms.NonInterruptingPreAnim = "0"
	}
	if ms.SfxLooping == "" {
		ms.SfxLooping = "0"
	}
	if ms.Screenshake == "" {
		ms.Screenshake = "0"
	}
	if ms.Additive == "" || client.CharID() != client.Area().LastSpeaker() {
		ms.Additive = "0"
	}
	// Area-level force_nointerrupt mirrors Akashi's forceImmediate: convert PREANIM variants to their non-preanim twin and force immediate=1 so the preanim plays alongside the text.
	if client.Area().NoInterrupt() {
		switch emote_mod {
		case 1, 2:
			emote_mod = 0
			ms.EmoteModifier = "0"
			ms.NonInterruptingPreAnim = "1"
		case 6:
			emote_mod = 5
			ms.EmoteModifier = "5"
			ms.NonInterruptingPreAnim = "1"
		}
	}

	// SFX curse post-fixup: WebAO's chat_tick only triggers emote SFX playback
	// when EmoteModifier is 1, 2, or 6. When the value is 0 (idle) or 5
	// (talking, no preanim), WebAO silently skips the sound even though
	// SfxName is set. This runs after the NoInterrupt block above to avoid
	// being overwritten. We promote the modifier to 1 and clear the preanim
	// name to "-" so no visual preanim animation plays — only the SFX fires.
	if sfxCurseActive && (ms.EmoteModifier == "0" || ms.EmoteModifier == "5") {
		ms.EmoteModifier = "1"
		ms.PreAnim = "-"
	}

	// Decode the message text once; reused for length validation, testimony navigation, and automod.
	msgText := decode(ms.Message)

	// Single lock to obtain the stuck character ID; -1 means not stuck.
	// Used in both iniswap cases below to avoid redundant mutex acquisitions.
	stuckCharID := client.charStuckID()

	// DeskMod "chat" is a legacy alias for "1"; rewrite to match Akashi.
	if ms.DeskMod == "chat" {
		ms.DeskMod = "1"
	}

	switch {
	case !sliceutil.ContainsString(validDeskMods, ms.DeskMod):
		logger.LogWarningf("dropped MS from IPID:%v UID:%v — DeskMod not in validDeskMods; value=%q", client.Ipid(), client.Uid(), ms.DeskMod)
		return
	case !hasForcedIniswap && !strings.EqualFold(getCharacters()[client.CharID()], ms.Character) && !client.Area().IniswapAllowed(): // character name (skip check when forced iniswap)
		client.SendServerMessage("Iniswapping is not allowed in this area.")
		return
	case !hasForcedIniswap && stuckCharID >= 0 && !strings.EqualFold(getCharacters()[stuckCharID], ms.Character): // block iniswap when charstuck unless forced iniswap
		client.SendServerMessage(fmt.Sprintf("You are character stuck as %v and cannot iniswap.", getCharacters()[stuckCharID]))
		return
	case utf8.RuneCountInString(msgText) > config.MaxMsg:
		// Count characters (runes), not bytes. len() returns the UTF-8 byte
		// length, so a multi-byte character (accents, emoji, CJK) or an
		// invisible zero-width character costs up to 4 bytes each and a
		// visually short message could trip the byte-based limit. max_message_length
		// is documented and understood as a character count, so enforce it as one.
		client.SendServerMessage("Your message exceeds the maximum message length!")
		return
	case ms.Message == client.LastMsg():
		logger.LogWarningf("dropped MS from IPID:%v UID:%v — duplicate of LastMsg", client.Ipid(), client.Uid())
		return
	case !hasForcedIniswap && ms.CharID != client.CharIDStr(): // skip check when forced iniswap
		logger.LogWarningf("dropped MS from IPID:%v UID:%v — CharID mismatch; packet=%q client=%q", client.Ipid(), client.Uid(), ms.CharID, client.CharIDStr())
		return
	case objection < 0 || objection > 4:
		logger.LogWarningf("dropped MS from IPID:%v UID:%v — ShoutModifier out of [0,4]; value=%d", client.Ipid(), client.Uid(), objection)
		return
	case evi < 0 || evi > len(client.Area().Evidence()):
		logger.LogWarningf("dropped MS from IPID:%v UID:%v — Evidence id out of range; value=%d max=%d", client.Ipid(), client.Uid(), evi, len(client.Area().Evidence()))
		return
	case ms.Flip != "0" && ms.Flip != "1":
		logger.LogWarningf("dropped MS from IPID:%v UID:%v — Flip not \"0\"/\"1\"; value=%q", client.Ipid(), client.Uid(), ms.Flip)
		return
	case ms.Realization != "0" && ms.Realization != "1":
		logger.LogWarningf("dropped MS from IPID:%v UID:%v — Realization not \"0\"/\"1\"; value=%q", client.Ipid(), client.Uid(), ms.Realization)
		return
	case text < 0 || text > 9: // 0-9 per AO2 protocol (9 = rainbow)
		logger.LogWarningf("dropped MS from IPID:%v UID:%v — TextColor out of [0,9]; value=%d", client.Ipid(), client.Uid(), text)
		return
	case len(ms.Showname) > maxShownameLength:
		client.SendServerMessage("Your showname is too long!")
		return
	case ms.NonInterruptingPreAnim != "0" && ms.NonInterruptingPreAnim != "1":
		logger.LogWarningf("dropped MS from IPID:%v UID:%v — NonInterruptingPreAnim not \"0\"/\"1\"; value=%q", client.Ipid(), client.Uid(), ms.NonInterruptingPreAnim)
		return
	case ms.SfxLooping != "0" && ms.SfxLooping != "1":
		logger.LogWarningf("dropped MS from IPID:%v UID:%v — SfxLooping not \"0\"/\"1\"; value=%q", client.Ipid(), client.Uid(), ms.SfxLooping)
		return
	case ms.Screenshake != "0" && ms.Screenshake != "1":
		logger.LogWarningf("dropped MS from IPID:%v UID:%v — Screenshake not \"0\"/\"1\"; value=%q", client.Ipid(), client.Uid(), ms.Screenshake)
		return
	case ms.Additive != "0" && ms.Additive != "1":
		logger.LogWarningf("dropped MS from IPID:%v UID:%v — Additive not \"0\"/\"1\"; value=%q", client.Ipid(), client.Uid(), ms.Additive)
		return
	}

	// ---------------------------------------------------------------------
	// Content gate: AutoMod banned words in the decoded message AND in the
	// showname (a slur worn as a showname is just as visible as one spoken,
	// and checking both closes an obvious bypass), plus the
	// censored_names.txt showname list.
	//
	// This sits here -- immediately after field validation, before any other
	// handling whatsoever -- rather than further down where it used to live,
	// because everything in between turned out to be a way for the text or
	// the showname to reach somebody:
	//
	//   - the showname is broadcast to EVERY client in a PU packet as part of
	//     the state update below, and that happened before the showname had
	//     been censored at all. A slur worn as a showname rendered in every
	//     client in the room and the censor never got a look at it. That is
	//     the leak this move fixes, and it is not a small one.
	//   - quickdraw, the typing race and the unscramble event all read the
	//     message text, so a censored line could still win a prize.
	//   - the testimony recorder can store and later replay it.
	//
	// Nothing here depends on the punishment transforms applied further down:
	// msgText is decoded exactly once, at the top of this function, and is
	// never reassigned, so checking it here checks the identical string the
	// old placement checked.
	//
	// A shadow trip does NOT abort processing: the message flows through the
	// normal pipeline and is folded into the silenced delivery path below, so
	// the sender gets a well-formed echo -- their client shows the message as
	// sent while no other client ever sees it. A NUKE trip is the opposite and
	// aborts everything immediately, with no echo at all (see
	// applyAutoModNuke).
	//
	// Ordering against the other suppression mechanisms is deliberate and is
	// the point of the whole block: censoring runs before the torment branch,
	// before the stealthmute silencing, and before the escalating
	// kick, so a censored message can never escape through the delayed 50/50
	// torment rebroadcast, and a nuked one never reaches any of them at all.
	// ---------------------------------------------------------------------
	censorShadow := false
	// pendingCensorKick is set when a censor trip requires an escalating kick
	// but the connection must stay open a little longer -- through the
	// shadow-echo/broadcast below -- so the kick is applied once, at the very
	// end of this function, via client.KickForCensorTrip().
	pendingCensorKick := false
	// checkCensored runs one field through the tiered word list. It reports
	// whether the caller must abandon the packet outright; a shadow trip sets
	// censorShadow instead and lets processing continue.
	checkCensored := func(text, source string) (abandon bool) {
		m, result, kick := autoModCheckTiered(client, text, source)
		if m.Matched && m.Entry.Severity == SeverityNuke {
			// Destroys the message and bans the IPID. Nothing else in this
			// function may run afterwards -- not the echo, not the area log,
			// not a single state update.
			return applyAutoModNuke(client, m, source)
		}
		// Everything below nuke feeds the raid guard: saying something flagged
		// is one more piece of evidence about a connection, and combines with
		// the behavioural signals rather than acting on its own. A nuke is
		// excluded above because scoring a connection that has just been
		// banned would be pointless.
		raidGuardOnWordHit(client, m)
		switch result {
		case autoModBlocked:
			if kick {
				client.KickForCensorTrip()
			}
			return true
		case autoModShadow:
			censorShadow = true
			pendingCensorKick = kick
		}
		return false
	}
	// A nuke in EITHER field wins outright, decided before the per-field checks
	// below get a chance to short-circuit one another: without this, a merely
	// default-tier word in the message sets censorShadow and the showname is
	// then never examined, so a nuke word worn as a showname would be answered
	// with a shadow-drop instead of a ban.
	if nukeFieldsOrNothing(client,
		[2]string{msgText, "IC message"},
		[2]string{decode(ms.Showname), "IC showname"},
	) {
		return
	}
	if checkCensored(msgText, "IC message") {
		return
	}
	if !censorShadow && ms.Showname != "" && checkCensored(decode(ms.Showname), "IC showname") {
		return
	}
	if !censorShadow && ms.Showname != "" && checkCensoredShowname(client, decode(ms.Showname)) {
		// checkCensoredShowname has exactly one outcome on a match (shadow-drop
		// + torment-list), unlike the word list's configurable actions, so a
		// match always also warrants the same escalating kick.
		pendingCensorKick = true
		censorShadow = true
	}

	// If force-paired, always sync the wanted CharID to the partner's current CharID,
	// keeping the pair active even when either player changes character or position.
	// Also sync the position so both characters are always broadcast at the same
	// courtroom position, preventing the pairing sprite from breaking on clients.
	if client.ForcePairUID() >= 0 {
		if partner, err := getClientByUid(client.ForcePairUID()); err == nil && partner.CharID() >= 0 {
			ms.OtherCharID = partner.CharIDStr()
			client.SetPairWantedID(partner.CharID())
			partner.SetPairWantedID(client.CharID())
			if pos := partner.Pos(); pos != "" {
				ms.Side = pos
				client.SetPos(pos)
			}
		}
	}

	// If the client used /pair to set a desired pair but has not selected one via the
	// in-client pair button (OtherCharID is absent or -1), inject the server-set pair
	// character ID so the pairing animation activates exactly as if the pair button
	// had been used.
	if (ms.OtherCharID == "" || ms.OtherCharID == "-1") && client.PairWantedID() != -1 {
		ms.OtherCharID = strconv.Itoa(client.PairWantedID())
	}

	// Pairing validation
	if ms.OtherCharID != "" && ms.OtherCharID != "-1" {
		pidStr, _, _ := strings.Cut(ms.OtherCharID, "^")
		pid, err := strconv.Atoi(pidStr)
		if err != nil {
			return
		}
		if pid < 0 || pid > len(getCharacters()) || pid == client.CharID() {
			return
		}
		client.SetPairWantedID(pid)
		pairing := false
		clients.ForEach(func(c *Client) {
			if pairing {
				return
			}
			isForce := client.ForcePairUID() >= 0 && client.ForcePairUID() == c.Uid() &&
				c.ForcePairUID() >= 0 && c.ForcePairUID() == client.Uid()
			// If the client has a stored pair partner, skip any client that isn't
			// that specific partner to prevent false matches from position overlap.
			if client.ForcePairUID() >= 0 && !isForce {
				return
			}
			// Also guard the candidate: if c is already UID-committed to a different partner,
			// it must not be matched by anyone other than that partner.
			if c.ForcePairUID() >= 0 && c.ForcePairUID() != client.Uid() {
				return
			}
			if c.CharID() == pid && c.PairWantedID() == client.CharID() && (isForce || c.Pos() == client.Pos()) {
				pairinfo := c.PairInfo()
				ms.OtherName = pairinfo.name
				ms.OtherEmote = pairinfo.emote
				ms.OtherOffset = pairinfo.offset
				ms.OtherFlip = pairinfo.flip
				pairing = true
			}
		})
		if !pairing {
			// Plain "-1", never "-1^": the "^" pair-order suffix is not parsed
			// by every client (official webAO gets Number("-1^") = NaN and some
			// fork desktop clients drop the whole message — see PR #386). Since
			// /pair injects a wanted id into every message the requester sends,
			// this fallback fires on each of them until the pair is accepted,
			// so a "^" here silently mutes the requester on those clients.
			ms.OtherCharID = "-1"
			ms.OtherName = ""
			ms.OtherEmote = ""
		}
	} else {
		// No pair attempted: ensure OtherName/OtherEmote are empty.
		ms.OtherName = ""
		ms.OtherEmote = ""
	}

	// Offset validation
	if ms.SelfOffset != "" {
		offsets := strings.Split(decode(ms.SelfOffset), "&")
		x_offset, err := strconv.Atoi(offsets[0])
		if err != nil {
			return
		} else if x_offset < -100 || x_offset > 100 {
			return
		}
		if len(offsets) > 1 {
			y_offset, err := strconv.Atoi(offsets[0])
			if err != nil {
				return
			} else if y_offset < -100 || y_offset > 100 {
				return
			}
		}
	}

	// Testimony recorder
	if client.Pos() == "wit" && client.Area().TstState() != area.TRIdle {
		switch client.Area().TstState() {
		case area.TRRecording:
			if client.Area().TstLen() >= config.MaxStatement+1 {
				client.SendServerMessage("Unable to add message: Max statements reached.")
				break
			}
			if client.Area().CurrentTstIndex() == 0 {
				ms.Message = "~~\n-- " + ms.Message + " --"
				ms.TextColor = "3"
				broadcastToArea(client.Area(), &packet.RTPacket{Animation: "testimony1"})
			}
			client.Area().TstAppend(ms.ServerString())
			client.Area().TstAdvance()
		case area.TRInserting:
			if client.Area().TstLen() >= config.MaxStatement {
				client.SendServerMessage("Unable to insert message: Max statements reached.")
				client.Area().SetTstState(area.TRPlayback)
				break
			}
			client.Area().TstInsert(ms.ServerString())
			client.Area().SetTstState(area.TRPlayback)
			client.Area().TstAdvance()
		case area.TRUpdating:
			if client.Area().CurrentTstIndex() == 0 {
				client.SendServerMessage("Cannot edit testimony title.")
				client.Area().SetTstState(area.TRPlayback)
				break
			}
			client.Area().TstUpdate(ms.ServerString())
			client.Area().SetTstState(area.TRPlayback)
		}
	}
	if client.Area().TstState() == area.TRPlayback {
		s := tstNavRegex.FindString(msgText)
		if s != "" {
			if strings.ContainsRune(s, '<') {
				client.Area().TstRewind()
				broadcastToArea(client.Area(), packet.ParseMSServerString(client.Area().CurrentTstStatement()))
				return
			}
			_, idStr, _ := strings.Cut(s, ">")
			id, err := strconv.Atoi(idStr)
			if err != nil {
				client.Area().TstAdvance()
				broadcastToArea(client.Area(), packet.ParseMSServerString(client.Area().CurrentTstStatement()))
				return
			} else {
				if id > 0 && id < client.Area().TstLen() {
					client.Area().TstJump(id)
					broadcastToArea(client.Area(), packet.ParseMSServerString(client.Area().CurrentTstStatement()))
					return
				}
			}
		}
	}

	// Use the client's own values (saved before any moderator-forced showname or
	// iniswap override above) for state updates, so a forced substitute is never
	// written back as the client's own stored state.
	client.SetPairInfo(ownCharName, ownEmote, ownFlip, ownSelfOffset)
	client.SetLastMsg(ms.Message)
	client.SetLastTextColor(ownTextColor)
	newShowname := ownShowname
	if strings.TrimSpace(ownShowname) == "" {
		newShowname = getCharacters()[client.CharID()]
	}
	// Only broadcast a PU showname update when the showname actually changed.
	if client.UpdateShowname(newShowname) {
		broadcastToAll(&packet.PU{ID: client.Uid(), Type: 2, Data: decode(newShowname)})
	}
	client.Area().SetLastSpeaker(client.CharID())

	// Track tournament message count
	if tournamentActive {
		tournamentMutex.Lock()
		if participant, exists := tournamentParticipants[client.Uid()]; exists {
			participant.messageCount++
		}
		tournamentMutex.Unlock()
	}

	// Quickdraw: record the reaction for any active duel.
	quickdrawOnIC(client, msgText)

	// Typing race: check whether the IC message matches the active race phrase.
	typingRaceOnIC(client, msgText)

	// Unscramble: check whether the IC message is the correct answer.
	if config != nil && config.EnableCasino {
		unscrambleOnIC(client, msgText)
	}

	// Torment: ghost or delay the message without the client noticing.
	if !censorShadow && isIPIDTormented(client.Ipid()) {
		handleTormentedIC(client, ms)
		return
	}

	// New-IPID IC cooldown: a brand-new connection must wait before speaking IC.
	// Placed AFTER the rate-limit check at the top of this function and the
	// automod/censor and torment checks above -- NOT before them -- so a flood or
	// slur burst from a fresh connection during the cooldown window still gets
	// judged (and can still be kicked/banned) by those checks instead of being
	// silently shielded by the cooldown. The cooldown itself only ever rejects an
	// otherwise-clean message sent too early.
	if limited, remaining := checkNewIPIDICCooldown(client.Ipid()); limited {
		unit := "seconds"
		if remaining == 1 {
			unit = "second"
		}
		client.SendServerMessage(fmt.Sprintf("New users must wait %d %s before speaking in-character.", remaining, unit))
		return
	}

	// Punishment area: if this area has punishment_area=true, pick a random
	// stateless punishment per IC message and apply it on top of whatever
	// per-client punishments already ran. Nothing is persisted — the effect
	// evaporates the moment the speaker leaves the area. Translator is
	// included in the random pool only if it's fully configured server-wide.
	if client.Area().PunishmentArea() && ms.Message != "" {
		decoded := decode(ms.Message)
		mutated, _ := applyAreaRandomPunishmentText(decoded, translatorEnabled())
		ms.Message = encode(mutated)
	}

	// Mirror area: if this area has mirror=true in its TOML config, reverse the
	// IC message text server-side right before broadcasting. This runs AFTER
	// every punishment transform so the reversal is the last word on the text.
	// The packet wire-format fields are otherwise untouched, so clients can
	// connect and render the message normally — they just see it backwards.
	if client.Area().MirrorArea() && ms.Message != "" {
		mirrored := reverseRunes(decode(ms.Message))
		ms.Message = encode(mirrored)
	}

	// Doki area: per-message chaos rolls (Haschen quote takeovers, zalgo
	// scrambles, dark anagrams, surprise BG swaps). Independent of mirror
	// and punishment_area so they can all stack on the same area if desired.
	if client.Area().DokiArea() && ms.Message != "" {
		decoded := decode(ms.Message)
		res := applyDokiEffect(decoded)
		if res.Text != decoded {
			ms.Message = encode(res.Text)
		}
		if res.SwapBG && len(getBackgrounds()) > 0 {
			bg := getBackgrounds()[rand.Intn(len(getBackgrounds()))]
			client.Area().SetBackground(bg)
			broadcastToArea(client.Area(), &packet.BN{Background: bg})
		}
	}

	// ForceDisplay: if a punished player in this area is pinned, rewrite this
	// (non-moderator) speaker's sprite to that character and clear any pairing,
	// so the whole room renders as the pinned character. Applied last so it has
	// the final word on the outgoing sprite; gated to near-zero cost when unused.
	maybeApplyForceDisplay(client, ms)

	// Encode the structured packet back into wire format exactly once and
	// hand it to the broadcaster. MSPacket implements packet.Outgoing, so
	// Args() is invoked once inside broadcastToAreaFrom.
	//
	// Two delivery punishments intercept here:
	//   - stealthmute: the packet echoes back to ONLY the sender — they see
	//     their message appear normally while the room hears nothing.
	//   - lifo: the packet is buffered and released in reverse arrival order.
	//
	// A censor trip (banned word or censored showname, checked above) is
	// folded in the same way: the triggering message is echoed back to the
	// sender alone, so it looks sent on their side while the room never
	// sees it.
	//
	// Showname punisher (punishment_names.txt): a matching showname stains
	// the speaker's IPID and starts the one-random-punishment-per-minute
	// drip. Unlike the censor checks above it never silences this message —
	// the effects apply from the next message onward.
	if ms.Showname != "" {
		checkPunishmentShowname(client, decode(ms.Showname))
	}

	// Raid guard: score this message now that it has cleared every gate above
	// (rate limit, join captcha, automod/censor, torment) and is otherwise
	// about to reach the room. Placed before the delivery switch below so an
	// action the guard takes here (e.g. a fresh silence/challenge verdict) can
	// still catch this very message via the restricted/silenced routing that
	// switch reads. objection is the ShoutModifier already parsed above --
	// see raidGuardOnIC's doc comment for why it isn't re-parsed here.
	raidGuardOnIC(client, ms, msgText)

	silenced := hasPunishmentType(punishments, PunishmentStealthMute) || censorShadow
	// A captcha-restricted connection is routed away from the room. It counts
	// as silenced from here on so it triggers no traps, contagion or love
	// potions, like any other message the room never actually heard.
	restricted := activeCaptchaRestricted.Load() > 0 && client.captchaRestricted.Load()
	switch {
	case silenced:
		client.Send(ms)
	case restricted:
		deliverRestricted(client, client.Area(), ms)
	case hasPunishmentType(punishments, PunishmentLifo):
		lifoEnqueueIC(client, ms)
	default:
		broadcastToAreaFrom(client.Ipid(), senderBypassesIgnore(client.Perms()), client.Area(), ms)
	}
	silenced = silenced || restricted
	// SFX curse MC fallback: for external http(s) URLs the sfx_name field alone
	// is not enough because standard AO2 desktop clients look for a local file
	// and WebAO concatenates the asset URL with the sound name (producing a
	// garbage path). Sending a one-shot MC packet lets desktop AO2 clients
	// play the URL through their media stack (the same mechanism /play uses),
	// so the cursed sound actually plays for everyone in the area.
	if sfxCurseExternalURL != "" && !silenced {
		broadcastToArea(client.Area(), &packet.MCToClient{
			Name: sfxCurseExternalURL, CharID: client.CharID(),
			Showname: client.Showname(), Looping: "0", Channel: "0", Effects: "",
		})
	}
	// Record the original (pre-punishment) decoded message in the area's icwarp
	// history so future icwarp lookups have something to pick from. Censored
	// messages are deliberately excluded: the area history feeds /icwarp and
	// /markov, which could otherwise regurgitate the slur to the room.
	if originalICMsg != "" && !censorShadow {
		client.Area().RecordICMessage(client.Ipid(), originalICMsg)
	}
	// Mechanic hooks: contagion spread, minefield rolls, silence-bell traps
	// and love potions all key off "a message the room actually heard", so a
	// stealthmuted message never triggers them.
	if !silenced {
		punishmentMechanicsOnIC(client, punishments)
	}
	// Censor-tripped messages get a marker so staff can audit what a silenced
	// speaker tried to say.
	switch {
	case censorShadow:
		addToBuffer(client, "IC", "\""+ms.Message+"\" (censored)", false)
	case restricted:
		// Logged so staff can still read what a restricted connection sent.
		addToBuffer(client, "IC", "\""+ms.Message+"\" (captcha-muted)", false)
	default:
		addToBuffer(client, "IC", "\""+ms.Message+"\"", false)
	}

	// Apply the deferred censor-trip kick (see pendingCensorKick above) now that
	// the shadow echo and buffer log are done -- KickForCensorTrip closes the
	// connection, so nothing below this point may touch client again.
	if pendingCensorKick {
		client.KickForCensorTrip()
	}
}

// reverseRunes returns s with its runes (not bytes) reversed. Used by the
// mirror-area feature so multi-byte UTF-8 characters (accents, emoji, etc.)
// survive the flip intact.
func reverseRunes(s string) string {
	runes := []rune(s)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}

// Handles MC#%
func pktAM(client *Client, p *packet.Packet) {
	// For reasons beyond mortal understanding, this packet serves two purposes: music changes, and area changes.

	// Check rate limit first
	if client.CheckRateLimit() {
		client.KickForRateLimit()
		return
	}

	mc, err := packet.ParseMCFromClient(p.Body)
	if err != nil {
		return
	}
	if client.CharID() != mc.CharID {
		return
	}

	decodedSong := decode(mc.Name)

	// /musicban gate. The check only applies to actual music plays (jukebox
	// entries or streaming URLs) — area-change MC packets fall through to the
	// areaNames branch further down, which we intentionally do NOT block (a
	// music-banned player can still move between rooms). Moderators are exempt.
	// In areas with fewer than musicBanQuietAreaThreshold players the ban is
	// bypassed so banned players can still set the mood in quiet rooms.
	if !permissions.IsModerator(client.Perms()) && isMusicBanned(client.Ipid()) {
		if isMusicURL(decodedSong) || sliceutil.ContainsString(getMusicList(), decodedSong) {
			if client.Area().PlayerCount() >= musicBanQuietAreaThreshold {
				client.SendServerMessage(fmt.Sprintf(
					"You are music-banned. (In areas with fewer than %d people you may still play music.)",
					musicBanQuietAreaThreshold))
				addToBuffer(client, "MUSIC", "Blocked by /musicban.", false)
				return
			}
		}
	}

	// Join captcha: block music plays from an unverified connection. Area
	// changes fall through to the areaNames branch untouched -- moving between
	// rooms broadcasts nothing and is exactly what a confused new player should
	// still be able to do while they work the question out. The atomic load
	// comes first so a verified client never pays for the music-list scan.
	if client.awaitingCaptcha.Load() &&
		(isMusicURL(decodedSong) || sliceutil.ContainsString(getMusicList(), decodedSong)) {
		if client.joinCaptchaBlocked("change the music") {
			return
		}
	}

	if isMusicURL(decodedSong) {
		// A client (typically WebAO) can request a streaming track by sending
		// a raw http(s) URL in the MC packet's song slot. Such a URL is never
		// in the jukebox list, so it must be handled before the music-list and
		// area-name checks below.
		if !client.CanChangeMusic() {
			client.SendServerMessage("You are not allowed to change the music in this area.")
			return
		}
		// Gate on the CDN whitelist exactly like /play does. An un-whitelisted
		// host is rejected with an OOC notice rather than silently dropped.
		if !isAllowedCDN(decodedSong) {
			client.SendServerMessage("Illegal origin: that URL's host is not whitelisted. Add the domain to config/cdns.txt to allow it.")
			return
		}
		name := client.Showname()
		if mc.Showname != "" {
			name = mc.Showname
		}
		effects := "0"
		if mc.Effects != "" {
			effects = mc.Effects
		}
		// Re-broadcast the URL byte-for-byte as it arrived (mc.Name, still in
		// AO2 wire form) so the server never mangles it — recipients decode it
		// back to the exact URL the sender chose.
		addToBuffer(client, "MUSIC", fmt.Sprintf("Changed music to %v.", decodedSong), false)
		playAreaMusic(client.Area(), &packet.MCToClient{
			Name: mc.Name, CharID: mc.CharID, Showname: name,
			Looping: "1", Channel: "0", Effects: effects,
		})
		return
	}
	if sliceutil.ContainsString(getMusicList(), decodedSong) {
		if !client.CanChangeMusic() {
			client.SendServerMessage("You are not allowed to change the music in this area.")
			return
		}
		song := mc.Name
		name := client.Showname()
		effects := "0"
		if !strings.ContainsRune(decodedSong, '.') { // Chosen song is a category, and should stop the music.
			song = "~stop.mp3"
			addToBuffer(client, "MUSIC", "Stopped the music.", false)
		} else {
			addToBuffer(client, "MUSIC", fmt.Sprintf("Changed music to %v.", decodedSong), false)
		}
		if mc.Showname != "" {
			name = mc.Showname
		}
		if mc.Effects != "" {
			effects = mc.Effects
		}
		// Track the current song so /getmusic can re-fetch it for clients
		// whose audio dropped or who joined mid-track.
		playAreaMusic(client.Area(), &packet.MCToClient{
			Name: song, CharID: mc.CharID, Showname: name,
			Looping: "1", Channel: "0", Effects: effects,
		})
	} else if strings.Contains(getAreaNames(), decodedSong) {
		if decodedSong == client.Area().Name() {
			return
		}
		for _, a := range areas {
			if a.Name() == decodedSong {
				if !client.ChangeArea(a) {
					// Mods already received an OOC message from ChangeArea (lock
					// warning or jail notice); only non-mods need this generic reply.
					if !permissions.IsModerator(client.Perms()) {
						client.SendServerMessage("You are not invited to that area.")
					}
					return
				}
				// Hidden quirk for tormented IPIDs: 1/25 chance that the area change
				// message is delayed by 15-45 seconds, making them unsure if they moved.
				if isIPIDTormented(client.Ipid()) && tormentIntn(25) == 0 {
					delayMsg := time.Duration(15+tormentIntn(30)) * time.Second
					time.AfterFunc(delayMsg, func() {
						client.SendServerMessage(fmt.Sprintf("Moved to %v.", a.Name()))
					})
				} else {
					client.SendServerMessage(fmt.Sprintf("Moved to %v.", a.Name()))
				}
				return
			}
		}
	}
}

// Handles HP#%
func pktHP(client *Client, p *packet.Packet) {
	if client.CharID() == -1 || !client.CanJud() {
		client.SendServerMessage("You are not allowed to change the penalty bar in this area.")
		return
	}
	hp, err := packet.ParseHP(p.Body)
	if err != nil {
		return
	}
	if !client.Area().SetHP(hp.Bar, hp.Value) {
		return
	}
	broadcastToArea(client.Area(), &packet.HPPacket{Bar: hp.Bar, Value: hp.Value})

	var side string
	switch hp.Bar {
	case 1:
		side = "Defense"
	case 2:
		side = "Prosecution"
	}
	addToBuffer(client, "JUD", fmt.Sprintf("Set %v HP to %v.", side, hp.Value), false)
}

// Handles RT#%
func pktWTCE(client *Client, p *packet.Packet) {
	if !client.Area().JudgeAllowed() {
		client.SendServerMessage("The judge buttons are disabled in this area.")
		return
	}
	if client.CharID() == -1 || !client.CanJud() {
		client.SendServerMessage("You are not allowed to play WT/CE in this area.")
		return
	}
	rt, err := packet.ParseRT(p.Body)
	if err != nil {
		return
	}
	broadcastToArea(client.Area(), rt)
	addToBuffer(client, "JUD", "Played WT/CE animation.", false)
}

// Handles TT#%
func pktTT(client *Client, p *packet.Packet) {
	if client.CharID() == -1 || !client.CanJud() {
		client.SendServerMessage("You are not allowed to set testimony titles in this area.")
		return
	}
	tt, err := packet.ParseTT(p.Body)
	if err != nil {
		return
	}
	broadcastToArea(client.Area(), tt)
	addToBuffer(client, "JUD", "Set testimony title.", false)
}

// Handles CT#%
func pktOOC(client *Client, p *packet.Packet) {
	// Check rate limit first
	if client.CheckRateLimit() {
		client.KickForRateLimit()
		return
	}

	// Check OOC-specific rate limit per IP; kick on excess to prevent flood even across reconnections.
	if checkIPOOCRateLimit(client.Ipid()) {
		client.KickForRateLimit()
		return
	}

	// While lockdown is active, a non-moderator under the purge's playtime
	// threshold is silently dropped -- see the matching check in pktIC.
	if serverLockdown.Load() && lockdownShouldSilence(client) {
		return
	}

	ct, err := packet.ParseCTFromClient(p.Body)
	if err != nil {
		return
	}

	// Commands are dispatched before OOC-name validation so that /login and
	// other commands are always reachable regardless of new-IPID OOC cooldowns,
	// username validity, or duplicate-name collisions.  Returning players whose
	// IP changed must be able to /login without waiting out any cooldown timer.
	if strings.HasPrefix(ct.Message, "/") {
		decoded := decode(ct.Message)
		match := commandRegex.FindString(decoded)
		command := strings.ToLower(strings.TrimPrefix(match, "/"))
		args := strings.Split(decoded, " ")[1:]
		// Join captcha: an unverified connection may only run the handful of
		// commands it needs to get verified or get help. Everything else is
		// gated because plenty of commands broadcast (/global, /pm, /roll),
		// and a bot that can reach those has not been stopped by the gate.
		if client.awaitingCaptcha.Load() && !joinCaptchaCommandAllowed(command) &&
			client.joinCaptchaBlocked("use that command") {
			return
		}
		ParseCommand(client, command, args)
		return
	}

	// Join captcha: a plain OOC message from an unverified connection is
	// blocked -- but if the player simply typed the answer without the
	// command, take it. The captcha's difficulty is meant to be the question,
	// not remembering to prefix it with /verify.
	if client.awaitingCaptcha.Load() {
		if client.tryJoinCaptchaAnswer(decode(ct.Message)) {
			return
		}
		if client.joinCaptchaBlocked("talk in OOC") {
			return
		}
	}

	// A real (non-command) OOC message counts as activity for the /dc idle timer.
	client.dcTouchActivity()

	username := decode(strings.TrimSpace(ct.Name))
	if username == "" || username == config.Name || len(username) > 30 || strings.ContainsAny(username, "[]") {
		client.SendServerMessage("Invalid username.")
		return
	}
	// Automod check on the OOC username itself — slurs in display names are
	// just as visible as in message bodies, and the autoModCheck handles the
	// configured action consistently. On a shadow trip the message is echoed
	// back to only the sender (under the offending name, so their client
	// looks normal) and dropped for everyone else.
	// A nuke in EITHER the username or the message body wins outright, decided
	// before the per-field handling below can short-circuit: a shadowed
	// username returns from this function before the message is ever checked,
	// so without this sweep a nuke word in the message could be shielded by a
	// gentler word in the name.
	if nukeFieldsOrNothing(client,
		[2]string{username, "OOC username"},
		[2]string{decode(ct.Message), "OOC message"},
	) {
		return
	}
	// Everything gentler feeds the raid guard and then takes the configured
	// action exactly as before.
	nameMatch, nameResult, nameKick := autoModCheckTiered(client, username, "OOC username")
	raidGuardOnWordHit(client, nameMatch)
	switch result, kick := nameResult, nameKick; result {
	case autoModBlocked:
		if kick {
			client.KickForCensorTrip()
		}
		return
	case autoModShadow:
		display := username
		if client.Uid() != -1 {
			display = "[" + strconv.Itoa(client.Uid()) + "] " + display
		}
		if tag := formatTagDisplay(db.GetActiveTag(client.Ipid())); tag != "" {
			display = tag + " " + display
		}
		client.Send(&packet.CTToClient{Name: encode(display), Message: ct.Message, IsFromServer: "0"})
		addToBuffer(client, "OOC", "\""+ct.Message+"\" (censored username)", false)
		if kick {
			client.KickForCensorTrip()
		}
		return
	}
	if utf8.RuneCountInString(ct.Message) > config.MaxMsg {
		// Character (rune) count, not byte count — see the matching IC check in
		// pktIC. A message with multi-byte or invisible zero-width characters
		// shouldn't trip a limit that users read as a character count.
		client.SendServerMessage("Your message exceeds the maximum message length!")
		return
	} else if strings.TrimSpace(ct.Message) == "" {
		return
	}
	var usernameTaken bool
	clients.ForEach(func(c *Client) {
		if c.OOCName() == ct.Name && c != client {
			usernameTaken = true
		}
	})
	if usernameTaken {
		client.SendServerMessage("That username is already taken.")
		return
	}
	client.SetOocName(username)

	// Build OOC display name: [Tag] [UID] OOCName.
	// UID is prepended server-side so it always appears in OOC chat
	// without relying on client-side PU name-matching.
	displayUsername := username
	if client.Uid() != -1 {
		displayUsername = "[" + strconv.Itoa(client.Uid()) + "] " + displayUsername
	}
	if tag := formatTagDisplay(db.GetActiveTag(client.Ipid())); tag != "" {
		displayUsername = tag + " " + displayUsername
	}

	if client.IsJailed() {
		client.SendServerMessage("You are jailed and cannot speak in OOC.")
		return
	}
	if !client.CanSpeakOOC() {
		client.SendServerMessage("You are muted from speaking in OOC.")
		return
	}
	// Check new-IPID OOC cooldown; commands are exempt so new users can still interact with the server.
	if limited, remaining := checkNewIPIDOOCCooldown(client.Ipid()); limited {
		unit := "seconds"
		if remaining == 1 {
			unit = "second"
		}
		client.SendServerMessage(fmt.Sprintf("New users must wait %d %s before using OOC chat.", remaining, unit))
		return
	}
	// Only broadcast the OOC name update once all checks pass, to prevent amplification attacks
	// where bots flood CT packets causing mass PU broadcasts to all connected clients.
	if client.Uid() != -1 {
		broadcastToAll(&packet.PU{ID: client.Uid(), Type: 0, Data: username})
	}
	msg := ct.Message
	// Reject duplicate OOC: if the last message sent in this area is identical, drop silently.
	if last, ok := areaLastOOCMsg.Load(client.Area()); ok {
		if lastStr, ok := last.(string); ok && lastStr == msg {
			return
		}
	}
	areaLastOOCMsg.Store(client.Area(), msg)
	// Automod: check the OOC message for banned words before broadcasting.
	// A shadow trip echoes the message back to only the sender — it looks
	// sent on their side while no other client ever receives it. This runs
	// before the torment branch so a censored message can never leak out
	// through handleTormentedOOC's delayed rebroadcast.
	msgMatch, msgResult, msgKick := autoModCheckTiered(client, decode(msg), "OOC message")
	if msgMatch.Matched && msgMatch.Entry.Severity == SeverityNuke {
		applyAutoModNuke(client, msgMatch, "OOC message")
		return
	}
	raidGuardOnWordHit(client, msgMatch)
	switch result, kick := msgResult, msgKick; result {
	case autoModBlocked:
		if kick {
			client.KickForCensorTrip()
		}
		return
	case autoModShadow:
		client.Send(&packet.CTToClient{Name: encode(displayUsername), Message: msg, IsFromServer: "0"})
		addToBuffer(client, "OOC", "\""+msg+"\" (censored)", false)
		if kick {
			client.KickForCensorTrip()
		}
		return
	}
	// Torment: ghost or delay the OOC message without the client noticing.
	if isIPIDTormented(client.Ipid()) {
		handleTormentedOOC(client, encode(displayUsername), msg)
		return
	}
	// Stealthmute: echo the message back to only the sender so they never
	// notice the room can't hear them. The buffer entry is marked so mods
	// reviewing logs can tell the message was suppressed.
	if client.HasActivePunishment(PunishmentStealthMute) {
		client.Send(&packet.CTToClient{Name: encode(displayUsername), Message: msg, IsFromServer: "0"})
		addToBuffer(client, "OOC", "\""+msg+"\" (stealthmuted)", false)
		return
	}
	// Captcha restriction: same routing the IC path uses.
	if activeCaptchaRestricted.Load() > 0 && client.captchaRestricted.Load() {
		deliverRestricted(client, client.Area(),
			&packet.CTToClient{Name: encode(displayUsername), Message: msg, IsFromServer: "0"})
		addToBuffer(client, "OOC", "\""+msg+"\" (captcha-muted)", false)
		return
	}
	// Raid guard: score this OOC line now that it has cleared automod/censor
	// and torment and is about to reach the room. Every earlier branch above
	// that stealthmutes, captcha-restricts, or torments this message returns
	// before reaching here, so those are never observed -- no separate skip
	// needed.
	raidGuardOnOOC(client, ct.Name, decode(msg))

	broadcastToAreaFrom(client.Ipid(), senderBypassesIgnore(client.Perms()), client.Area(),
		&packet.CTToClient{Name: encode(displayUsername), Message: msg, IsFromServer: "0"})
	addToBuffer(client, "OOC", "\""+msg+"\"", false)
}

// Handles PE#%
func pktAddEvi(client *Client, p *packet.Packet) {
	if !client.CanAlterEvidence() {
		client.SendServerMessage("You are not allowed to alter evidence in this area.")
		return
	}
	pe, err := packet.ParsePE(p.Body)
	if err != nil {
		return
	}
	client.Area().AddEvidence(pe.Name + "&" + pe.Description + "&" + pe.Image)
	broadcastToArea(client.Area(), &packet.LE{Items: client.Area().Evidence()})
	addToBuffer(client, "EVI", fmt.Sprintf("Added evidence: %v | %v", pe.Name, pe.Description), false)
}

// Handles DE#%
func pktRemoveEvi(client *Client, p *packet.Packet) {
	if !client.CanAlterEvidence() {
		client.SendServerMessage("You are not allowed to alter evidence in this area.")
		return
	}
	de, err := packet.ParseDE(p.Body)
	if err != nil {
		return
	}
	client.Area().RemoveEvidence(de.ID)
	broadcastToArea(client.Area(), &packet.LE{Items: client.Area().Evidence()})
	addToBuffer(client, "EVI", fmt.Sprintf("Removed evidence %v.", de.ID), false)
}

// Handles EE#%
func pktEditEvi(client *Client, p *packet.Packet) {
	if !client.CanAlterEvidence() {
		client.SendServerMessage("You are not allowed to alter evidence in this area.")
		return
	}
	ee, err := packet.ParseEE(p.Body)
	if err != nil {
		return
	}
	client.Area().EditEvidence(ee.ID, ee.Name+"&"+ee.Description+"&"+ee.Image)
	broadcastToArea(client.Area(), &packet.LE{Items: client.Area().Evidence()})
	addToBuffer(client, "EVI", fmt.Sprintf("Updated evidence %v to %v | %v", ee.ID, ee.Name, ee.Description), false)
}

// Handles CH#%
// Handles PW#% — area password (not used; server uses invite lists)
func pktPW(_ *Client, _ *packet.Packet) {}

func pktPing(client *Client, _ *packet.Packet) {
	if checkIPPingRateLimit(client.Ipid()) {
		return
	}
	client.lastPingNano.Store(time.Now().UnixNano())
	client.Send(&packet.CHECK{})
}

// Handles ZZ#%
func pktModcall(client *Client, p *packet.Packet) {
	if limited, remaining := checkNewIPIDModcallCooldown(client.Ipid()); limited {
		unit := "seconds"
		if remaining == 1 {
			unit = "second"
		}
		client.SendServerMessage(fmt.Sprintf("New users must wait %d %s before sending a modcall.", remaining, unit))
		return
	}
	if limited, remaining := checkIPModcallCooldown(client.Ipid()); limited {
		unit := "seconds"
		if remaining == 1 {
			unit = "second"
		}
		client.SendServerMessage(fmt.Sprintf("You must wait %d %s before sending another modcall.", remaining, unit))
		return
	}
	setIPModcallTime(client.Ipid())
	zz, _ := packet.ParseZZ(p.Body)
	addToBuffer(client, "MOD", fmt.Sprintf("Called moderator for reason: %v", zz.Reason), false)
	if client.Area().LogSilenced() {
		return
	}
	modcallMsg := fmt.Sprintf("MODCALL\n----------\nArea: %v\nUser: [%v] %v\nShowname: %v\nOOC Name: %v\nIPID: %v\nReason: %v",
		client.Area().Name(), client.Uid(), client.CurrentCharacter(), client.EffectiveShowname(), client.OOCName(), client.Ipid(), zz.Reason)
	out := &packet.ZZ{Reason: modcallMsg}
	clients.ForEach(func(c *Client) {
		if c.Authenticated() && permissions.IsModerator(c.Perms()) {
			c.Send(out)
		}
	})
	if enableDiscord {
		err := webhook.PostModcall(client.CurrentCharacter(), client.EffectiveShowname(), client.OOCName(), client.Ipid(), client.Area().Name(), zz.Reason, client.Uid())
		if err != nil {
			logger.LogError(err.Error())
		}
	}
	logger.WriteReport(client.Area().Name(), client.Area().Buffer())
}

// Handles SETCASE#%
func pktSetCase(client *Client, p *packet.Packet) {
	sc, err := packet.ParseSETCASE(p.Body)
	if err != nil {
		return
	}
	// Existing behaviour: only def/pro/judge/jury feed SetRoleAlert; cm and
	// steno are intentionally ignored. The role indices are 0..3 in that order.
	for i, r := range [4]string{sc.Def, sc.Pro, sc.Judge, sc.Jury} {
		b, err := strconv.ParseBool(r)
		if err != nil {
			return
		}
		client.SetRoleAlert(i, b)
	}
}

// Handles CASEA#%
func pktCaseAnn(client *Client, p *packet.Packet) {
	// Let future generations know I spent far too long trying to make this work.
	// Partially because of my own stupidity, and partially because this is the worst packet in AO2.

	if client.CharID() == -1 || !client.HasCMPermission() {
		client.SendServerMessage("You are not allowed to send case alerts in this area.")
		return
	}
	ca, err := packet.ParseCASEA(p.Body)
	if err != nil {
		return
	}
	// Build the announcement title; the needs fields are forwarded verbatim.
	// The trailing "1" in the FantaCode form preserves the old-client extra-
	// arg workaround. JSON-mode clients don't need that workaround and
	// receive the typed-Outgoing form (which encodes the 6 documented
	// CASEA fields via BuildJSON).
	title := fmt.Sprintf("CASE ANNOUNCEMENT: %v in %v needs players for %v",
		client.CurrentCharacter(), client.Area().Name(), ca.CaseTitle)
	needs := strings.Join(escapeOutgoing("CASEA", []string{ca.NeedDef, ca.NeedPro, ca.NeedJudge, ca.NeedJury, ca.NeedSteno}), "#")
	fantaCodePacket := fmt.Sprintf("CASEA#%v#%v#1#%%", encode(title), needs)
	typedPacket := &packet.CASEA{
		CaseTitle: title,
		NeedDef:   ca.NeedDef, NeedPro: ca.NeedPro,
		NeedJudge: ca.NeedJudge, NeedJury: ca.NeedJury, NeedSteno: ca.NeedSteno,
	}

	// Pre-parse the requested role flags once so we don't re-parse per recipient.
	var alertRoles [4]bool
	for i, r := range [4]string{ca.NeedDef, ca.NeedPro, ca.NeedJudge, ca.NeedJury} {
		b, err := strconv.ParseBool(r)
		if err != nil {
			return
		}
		alertRoles[i] = b
	}

	clients.ForEach(func(c *Client) {
		if c == client {
			return
		}
		for i := 0; i < 4; i++ {
			if alertRoles[i] && c.AlertRole(i) {
				if c.JSONMode() {
					c.Send(typedPacket)
				} else {
					c.write(fantaCodePacket)
				}
				break
			}
		}
	})
}

// Handles MA#<uid>#<duration_minutes>#<reason>#%
// Sent by the AO2 client when a moderator clicks the Ban or Kick button in the
// player-list UI. duration == 0 means kick; -1 means permanent ban; any positive
// value is a timed ban in minutes.
func pktMA(client *Client, p *packet.Packet) {
	if !client.Authenticated() {
		client.SendServerMessage("You are not logged in.")
		return
	}

	targetUID, err := strconv.Atoi(p.Body[0])
	if err != nil {
		return
	}
	durationMins, err := strconv.Atoi(p.Body[1])
	if err != nil {
		return
	}
	if durationMins < -1 {
		durationMins = -1
	}
	reason := strings.TrimSpace(p.Body[2])
	if reason == "" {
		reason = "No reason given."
	}

	isKick := durationMins == 0
	if isKick {
		if !permissions.HasPermission(client.Perms(), permissions.PermissionField["KICK"]) {
			client.SendServerMessage("You do not have permission to kick.")
			return
		}
	} else {
		if !permissions.HasPermission(client.Perms(), permissions.PermissionField["BAN"]) {
			client.SendServerMessage("You do not have permission to ban.")
			return
		}
	}

	target, err := getClientByUid(targetUID)
	if err != nil {
		client.SendServerMessage("User not found.")
		return
	}

	targetIPID := target.Ipid()
	targets := getClientsByIpid(targetIPID)

	if isKick {
		for _, c := range targets {
			c.SendSync(&packet.KK{Reason: reason})
			c.conn.Close()
			if err := webhook.PostKick(c.CurrentCharacter(), c.Showname(), c.OOCName(), c.Ipid(), reason, client.DisplayModName(), c.Uid()); err != nil {
				logger.LogErrorf("while posting kick webhook: %v", err)
			}
		}
		client.SendServerMessage(fmt.Sprintf("Kicked %d client(s).", len(targets)))
		sendPlayerArup()
		addToBuffer(client, "CMD", fmt.Sprintf("Kicked %v from server for reason: %v.", targetIPID, reason), true)
		return
	}

	// Ban path.
	banTime := time.Now().UTC().Unix()
	var until int64
	var untilS, durationStr string
	if durationMins == -1 {
		until = -1
		untilS = "∞"
		durationStr = "perma"
	} else {
		until = time.Now().UTC().Add(time.Duration(durationMins) * time.Minute).Unix()
		untilS = time.Unix(until, 0).UTC().Format("02 Jan 2006 15:04 MST")
		durationStr = fmt.Sprintf("%dm", durationMins)
	}

	banIDByHdid := make(map[string]int)
	for _, c := range targets {
		if _, done := banIDByHdid[c.Hdid()]; done {
			continue
		}
		id, err := db.AddBan(c.Ipid(), c.Hdid(), banTime, until, reason, client.StoredModName())
		if err == nil {
			banIDByHdid[c.Hdid()] = id
		}
	}
	forgetIP(targetIPID)
	for _, c := range targets {
		if id, ok := banIDByHdid[c.Hdid()]; ok {
			c.SendSync(&packet.KB{Reason: fmt.Sprintf("%v\nUntil: %v\nID: %v", reason, untilS, id)})
			if err := webhook.PostBan(c.CurrentCharacter(), c.Showname(), c.OOCName(), targetIPID, c.Uid(), id, durationStr, reason, client.DisplayModName()); err != nil {
				logger.LogErrorf("while posting ban webhook: %v", err)
			}
		} else {
			c.SendSync(&packet.KB{Reason: fmt.Sprintf("%v\nUntil: %v", reason, untilS)})
		}
		c.conn.Close()
	}
	client.SendServerMessage(fmt.Sprintf("Banned %d client(s).", len(targets)))
	sendPlayerArup()
	addToBuffer(client, "CMD", fmt.Sprintf("Banned %v from server for %v: %v.", targetIPID, durationStr, reason), true)
}

// decoder and encoder are package-level, pre-compiled replacers for the AO2 percent-encoding scheme.
// Using package-level vars avoids re-allocating a new strings.Replacer on every encode/decode call.
// strings.Replacer.Replace is safe for concurrent use; these vars must never be reassigned after init.
var (
	decoder = strings.NewReplacer("<percent>", "%", "<num>", "#", "<dollar>", "$", "<and>", "&")
	encoder = strings.NewReplacer("%", "<percent>", "#", "<num>", "$", "<dollar>", "&", "<and>")
)

// decode returns a given AO2-encoded string in its decoded form.
func decode(s string) string {
	return decoder.Replace(s)
}

// encode returns a decoded string in AO2-encoded form.
func encode(s string) string {
	return encoder.Replace(s)
}

// encoderNoAnd is the evidence-packet escape: LE and SC carry pre-joined
// "name&desc&image" items where '&' is a structural delimiter that must reach
// the client intact, so '&' is preserved while '#', '%' and '$' are escaped
// (mirrors Akashi's AOPacket::escapeEvidence).
var encoderNoAnd = strings.NewReplacer("%", "<percent>", "#", "<num>", "$", "<dollar>")

// escapeOutgoing escapes packet content immediately before it is serialized to
// the FantaCode wire format. Every outgoing field must pass through here so a
// chat message can never smuggle a raw '#' (field delimiter) or '%' (packet
// terminator) into the stream and forge packets such as KK (kick) or KB (ban).
//
// LE and SC are the one exception: their items are already pre-joined as
// "name&desc&image"/"name&desc&evidence", where '&' is a structural separator
// that must survive to the client, so those headers leave '&' alone (mirrors
// Akashi's AOPacket::escapeEvidence).
func escapeOutgoing(header string, contents []string) []string {
	esc := encoder
	if header == "LE" || header == "SC" {
		esc = encoderNoAnd
	}
	out := make([]string, len(contents))
	for i, c := range contents {
		out[i] = esc.Replace(c)
	}
	return out
}

// isMusicURL reports whether a music-change name is a streaming http(s) URL
// rather than a jukebox entry or an area name. Detection is by scheme prefix
// only, matching how WebAO submits a custom track in the MC packet's song slot.
func isMusicURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

// isExternalSfxURL reports whether sfx is an external http(s) URL (i.e. not a
// /base/sounds/ asset-server path). External URLs are delivered via MC packet
// because standard AO2 clients cannot play them directly via the sfx_name field.
func isExternalSfxURL(sfx string) bool {
	return (strings.HasPrefix(sfx, "http://") || strings.HasPrefix(sfx, "https://")) &&
		!strings.Contains(sfx, "/base/sounds/")
}

// sfxBareNameFromURL returns the value to send in the AO2 sfx_name field for a
// SFX curse customData entry:
//
//   - Standard AO2 asset-server URLs (http/https containing "/base/sounds/") and
//     bare /base/sounds/ local paths → bare filename stem, no path, no extension.
//     Desktop clients resolve the stem from their local base/sounds/general/ folder;
//     WebAO prepends the configured asset URL and appends ".opus".
//   - All other http(s) URLs (Discord CDN, custom hosting, etc.) → returned
//     as-is so that clients supporting URL-based audio can fetch the file
//     directly without any rewriting.
//
// Examples:
//
//	"https://miku.pizza/base/sounds/general/aai-cammy-bubble.opus" → "aai-cammy-bubble"
//	"/base/sounds/general/aai-cammy-bubble.opus"                   → "aai-cammy-bubble"
//	"aai-cammy-bubble"                                             → "aai-cammy-bubble"
//	"https://cdn.discordapp.com/attachments/123/456/boom.opus"     → "https://cdn.discordapp.com/attachments/123/456/boom.opus"
func sfxBareNameFromURL(s string) string {
	isHTTP := strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
	if isHTTP && !strings.Contains(s, "/base/sounds/") {
		// External URL (e.g. Discord CDN) — send as-is for URL-aware clients.
		return s
	}
	base := path.Base(s) // extracts the last path component, works on URLs too
	if ext := path.Ext(base); ext != "" {
		base = base[:len(base)-len(ext)]
	}
	return base
}

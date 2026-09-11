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
	"fmt"
	"sort"
	"strings"

	"github.com/MangosArentLiterature/Athena/internal/permissions"
	"github.com/MangosArentLiterature/Athena/internal/sliceutil"
)

type Command struct {
	handler    func(client *Client, args []string, usage string)
	minArgs    int
	usage      string
	desc       string
	reqPerms   uint64
	casinoCmd  bool   // when true, command is hidden/disabled if EnableCasino is false
	accountCmd bool   // when true, command requires the account system (works without EnableCasino so long as EnableAccounts is true)
	voiceCmd   bool   // when true, command is hidden/disabled if EnableVoice is false
	category   string // help category (e.g. "general", "casino", "punishment")
	publicHelp bool   // when true, command is listed in /help (and /help <cmd> shows usage) for everyone, even users who lack reqPerms
}

var Commands map[string]Command

// RegisterCommand installs an additional command into the global registry
// after initCommands has already run. It is intended for feature files that
// want to keep their command definition alongside their handler instead of
// editing the monolithic map literal in this file. Panics on duplicate
// registration so the problem is visible at startup rather than at runtime
// when a user first tries the shadowed command.
//
// Must be called AFTER initCommands; typically from a follow-on init hook
// in server.go. Not safe for concurrent use -- the registry is considered
// read-only once the server begins accepting connections.
func RegisterCommand(name string, cmd Command) {
	if Commands == nil {
		panic("RegisterCommand called before initCommands")
	}
	if _, exists := Commands[name]; exists {
		panic("RegisterCommand: duplicate command name " + name)
	}
	Commands[name] = cmd
}

// validateCommands walks the registry after initCommands and panics if any
// entry is missing required fields. Catches accidental paste errors like a
// blank usage string or a nil handler at server startup rather than when a
// player first types the broken command.
func validateCommands() {
	for name, cmd := range Commands {
		if cmd.handler == nil {
			panic("command " + name + " has nil handler")
		}
		if cmd.usage == "" {
			panic("command " + name + " has empty usage")
		}
		if cmd.desc == "" {
			panic("command " + name + " has empty desc")
		}
		if cmd.category == "" {
			panic("command " + name + " has empty category")
		}
	}
}

func initCommands() {
	Commands = map[string]Command{
		"about": {
			handler:  cmdAbout,
			minArgs:  0,
			usage:    "Usage: /about",
			desc:     "Prints Nyathena version and credit information.",
			reqPerms: permissions.PermissionField["NONE"],
			category: "general",
		},
		"8ball": {
			handler:  cmd8Ball,
			minArgs:  1,
			usage:    "Usage: /8ball <question>",
			desc:     "Asks the Magic 8-Ball a yes/no question. Answers come from 8ball.txt or a built-in fallback.",
			reqPerms: permissions.PermissionField["NONE"],
			category: "general",
		},
		"add": {
			handler:  cmdAdd,
			minArgs:  0,
			usage:    "Usage: /add",
			desc:     "Inserts the next IC message from the witness into the testimony after the current statement.",
			reqPerms: permissions.PermissionField["CM"],
			category: "testimony",
		},
		"allowcms": {
			handler:  cmdAllowCMs,
			minArgs:  1,
			usage:    "Usage: /allowcms <true|false>",
			desc:     "Toggles allowing CMs on or off.",
			reqPerms: permissions.PermissionField["MODIFY_AREA"],
			category: "area",
		},
		"allowiniswap": {
			handler:  cmdAllowIniswap,
			minArgs:  1,
			usage:    "Usage: /allowiniswap <true|false>",
			desc:     "Toggles iniswapping on or off.",
			reqPerms: permissions.PermissionField["MODIFY_AREA"],
			category: "area",
		},
		"area": {
			handler:  cmdArea,
			minArgs:  1,
			usage:    "Usage: /area <mute|unmute|rename|unrename>\nmute: Mute everyone in this area except CMs and moderators (IC and OOC).\nunmute: Lift the area mute so people can talk again.\nrename <name>: Rename this area (max 32 characters). It reverts to the name in areas.toml when the room empties or its last CM leaves. Run bare to see the current and configured names.\nunrename: Restore the configured name now instead of waiting.",
			desc:     "Area-wide tools for CMs and moderators. /area mute silences everyone except CMs and moderators, /area unmute reverses it, /area rename <name> names the room for as long as a CM is running it, and /area unrename restores the configured name.",
			reqPerms: permissions.PermissionField["CM"],
			category: "area",
		},
		"areainfo": {
			handler:  cmdAreaInfo,
			minArgs:  0,
			usage:    "Usage: /areainfo",
			desc:     "Prints area settings.",
			reqPerms: permissions.PermissionField["NONE"],
			category: "general",
		},
		"areadesc": {
			handler:  cmdAreaDesc,
			minArgs:  0,
			usage:    "Usage: /areadesc [-c] [description]\n-c: Clear the description.",
			desc:     "Prints or sets the area's entry description shown to players when they join.",
			reqPerms: permissions.PermissionField["NONE"],
			category: "area",
		},
		"arealog": {
			handler:  cmdAreaLog,
			minArgs:  1,
			usage:    "Usage: /arealog <enable|disable>",
			desc:     "Admin: Enables or disables area log silencing. While disabled, messages are not written to the area log file and modcall notifications are not forwarded to moderators or the Discord webhook.",
			reqPerms: permissions.PermissionField["ADMIN"],
			category: "admin",
		},
		"ban": {
			handler:  cmdBan,
			minArgs:  3,
			usage:    "Usage: /ban -u <uid1>,<uid2>... | -i <ipid1>,<ipid2>... [-d duration] <reason>\n-i supports offline IPIDs.",
			desc:     "Bans user(s) from the server. Use -i to ban by IPID (supports offline users).",
			reqPerms: permissions.PermissionField["BAN"],
			category: "moderation",
		},
		"bg": {
			handler:  cmdBg,
			minArgs:  1,
			usage:    "Usage: /bg <background>",
			desc:     "Sets the area's background. DJs may use this once per minute; CMs and MODIFY_AREA have no limit.",
			reqPerms: permissions.PermissionField["NONE"],
			category: "area",
		},
		"bglist": {
			handler:  cmdBgList,
			minArgs:  0,
			usage:    "Usage: /bglist",
			desc:     "Lists all available backgrounds.",
			reqPerms: permissions.PermissionField["NONE"],
			category: "general",
		},
		"charselect": {
			handler:  cmdCharSelect,
			minArgs:  0,
			usage:    "Usage: /charselect [uid1],[uid2]...",
			desc:     "Return to character select.",
			reqPerms: permissions.PermissionField["NONE"],
			category: "general",
		},
		"charstuck": {
			handler:  cmdCharStuck,
			minArgs:  1,
			usage:    "Usage: /charstuck [-d duration] [-r reason] <uid>",
			desc:     "Locks a player to their current character, preventing character changes.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "moderation",
		},
		"charcurse": {
			handler:  cmdCharCurse,
			minArgs:  2,
			usage:    "Usage: /charcurse <uid> <charname>",
			desc:     "Forces a player to a specific character. The player may still change characters freely.",
			reqPerms: permissions.PermissionField["KICK"],
			category: "moderation",
		},
		"charsteal": {
			handler:  cmdCharSteal,
			minArgs:  1,
			usage:    "Usage: /charsteal <uid>",
			desc:     "Takes over the target's current character for yourself, then forces them onto a random free character.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "moderation",
		},
		"charprotect": {
			handler:  cmdCharProtect,
			minArgs:  0,
			usage:    "Usage: /charprotect <on|off>",
			desc:     "Toggles character protection: if you change into an area where someone else is using your claimed character, they get bumped to a random character instead of you.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "moderation",
		},
		"cm": {
			handler:  cmdCM,
			minArgs:  0,
			usage:    "Usage: /cm [uid1],[uid2]...",
			desc:     "Promote to area CM.",
			reqPerms: permissions.PermissionField["NONE"],
			category: "area",
		},
		"doc": {
			handler:  cmdDoc,
			minArgs:  0,
			usage:    "Usage: /doc [-c] [doc]\n-c: Clear the doc.",
			desc:     "Prints or sets the area's document.",
			reqPerms: permissions.PermissionField["NONE"],
			category: "area",
		},
		"delete": {
			handler:  cmdDelete,
			minArgs:  0,
			usage:    "Usage: /delete",
			desc:     "Deletes the current testimony statement.",
			reqPerms: permissions.PermissionField["CM"],
			category: "testimony",
		},
		"dance": {
			handler:  cmdDance,
			minArgs:  0,
			usage:    "Usage: /dance",
			desc:     "Toggles dance mode. Flips your sprite on each message you send.",
			reqPerms: permissions.PermissionField["NONE"],
			category: "general",
		},
		"editban": {
			handler:  cmdEditBan,
			minArgs:  2,
			usage:    "Usage: /editban [-d duration] [-r reason] <id1>,<id2>...",
			desc:     "Changes the reason of ban(s).",
			reqPerms: permissions.PermissionField["BAN"],
			category: "moderation",
		},
		"erp": {
			handler:  cmdErp,
			minArgs:  0,
			usage:    "Usage: /erp",
			desc:     "The AO ERP command. Super fun!",
			reqPerms: permissions.PermissionField["NONE"],
			category: "general",
		},
		"examine": {
			handler:  cmdExamine,
			minArgs:  0,
			usage:    "Usage: /examine",
			desc:     "Starts cross-examination playback.",
			reqPerms: permissions.PermissionField["NONE"],
			category: "testimony",
		},
		"evimode": {
			handler:  cmdSetEviMod,
			minArgs:  1,
			usage:    "Usage: /evimode <mode>",
			desc:     "Sets the area's evidence mode.",
			reqPerms: permissions.PermissionField["CM"],
			category: "area",
		},
		"forcebglist": {
			handler:  cmdForceBGList,
			minArgs:  1,
			usage:    "Usage: /forcebglist <true|false>",
			desc:     "Toggles enforcing the server BG list on or off.",
			reqPerms: permissions.PermissionField["MODIFY_AREA"],
			category: "area",
		},
		"firewall": {
			handler:  cmdFirewall,
			minArgs:  1,
			usage:    "Usage: /firewall <on|off>",
			desc:     "Enables or disables IPHub VPN/proxy screening for new connections. Requires iphub_api_key in config.",
			reqPerms: permissions.PermissionField["BAN"],
			category: "moderation",
		},
		"getban": {
			handler:  cmdGetBan,
			minArgs:  0,
			usage:    "Usage: /getban [-b banid | -i ipid]",
			desc:     "Prints ban(s) matching the search parameters, or prints the 5 most recent bans.",
			reqPerms: permissions.PermissionField["BAN_INFO"],
			category: "moderation",
		},
		"ga": {
			handler:  cmdPlayers,
			minArgs:  0,
			usage:    "Usage: /ga",
			desc:     "Shows players in the current area (shortcut for /players).",
			reqPerms: permissions.PermissionField["NONE"],
			category: "general",
		},
		"gas": {
			handler:  func(client *Client, _ []string, _ string) { cmdPlayers(client, []string{"-a"}, "") },
			minArgs:  0,
			usage:    "Usage: /gas",
			desc:     "Shows players in all areas.",
			reqPerms: permissions.PermissionField["NONE"],
			category: "general",
		},
		"global": {
			handler:  cmdGlobal,
			minArgs:  1,
			usage:    "Usage: /global <message>",
			desc:     "Sends a global message.",
			reqPerms: permissions.PermissionField["NONE"],
			category: "general",
		},
		"g": {
			handler:  cmdGlobal,
			minArgs:  1,
			usage:    "Usage: /g <message>",
			desc:     "Alias of /global — sends a global message.",
			reqPerms: permissions.PermissionField["NONE"],
			category: "general",
		},
		"getmusic": {
			handler:  cmdGetMusic,
			minArgs:  0,
			usage:    "Usage: /getmusic",
			desc:     "Shows the URL of the song currently playing in this area and re-sends the MC packet to just you (useful when your client's audio bugged out).",
			reqPerms: permissions.PermissionField["NONE"],
			category: "general",
		},
		"invite": {
			handler:  cmdInvite,
			minArgs:  1,
			usage:    "Usage: /invite <uid1>,<uid2>... | /invite all",
			desc:     "Invites user(s) to the current area.",
			reqPerms: permissions.PermissionField["CM"],
			category: "area",
		},
		"ignore": {
			handler:  cmdIgnore,
			minArgs:  1,
			usage:    "Usage: /ignore <uid> | /ignore list",
			desc:     "Ignores a user by UID (persists across reconnections), or shows your numbered ignore list with '/ignore list'.",
			reqPerms: permissions.PermissionField["NONE"],
			category: "general",
		},
		"unignore": {
			handler:  cmdUnignore,
			minArgs:  1,
			usage:    "Usage: /unignore <uid|number>",
			desc:     "Removes a permanent ignore. Pass the UID of an online user, or a list number from '/ignore list' to unignore offline users.",
			reqPerms: permissions.PermissionField["NONE"],
			category: "general",
		},
		"jail": {
			handler:  cmdJail,
			minArgs:  1,
			usage:    "Usage: /jail <uid> [area_id] [-d duration] [-r reason]",
			desc:     "Jails a player in the given area (or their current area). They cannot leave and are returned there on reconnect.",
			reqPerms: permissions.PermissionField["BAN"],
			category: "moderation",
		},
		"kick": {
			handler:  cmdKick,
			minArgs:  3,
			usage:    "Usage: /kick -u <uid1>,<uid2>... | -i <ipid1>,<ipid2>... <reason>",
			desc:     "Kicks user(s) from the server.",
			reqPerms: permissions.PermissionField["KICK"],
			category: "moderation",
		},
		"kickarea": {
			handler:  cmdAreaKick,
			minArgs:  1,
			usage:    "Usage: /kickarea <uid1>,<uid2>...",
			desc:     "Kicks user(s) from the current area.",
			reqPerms: permissions.PermissionField["CM"],
			category: "area",
		},
		"kickother": {
			handler:  cmdKickOther,
			minArgs:  0,
			usage:    "Usage: /kickother",
			desc:     "Kicks all other connections sharing your IP (ghost clients).",
			reqPerms: permissions.PermissionField["NONE"],
			category: "general",
		},
		"lock": {
			handler:  cmdLock,
			minArgs:  0,
			usage:    "Usage: /lock [-s]\n-s: Sets the area to be spectatable.",
			desc:     "Locks the current area or sets it to spectatable.",
			reqPerms: permissions.PermissionField["CM"],
			category: "area",
		},
		"adminlock": {
			handler:  cmdAdminLock,
			minArgs:  0,
			usage:    "Usage: /adminlock",
			desc:     "Toggle an admin-only seal on this area: nobody but admins can enter — not even mods or shadow mods with BYPASS_LOCK. Players already inside stay.",
			reqPerms: permissions.PermissionField["ADMIN"],
			category: "admin",
		},
		"admin": {
			handler:  cmdAdmin,
			minArgs:  1,
			usage:    "Usage: /admin <hide|unhide|status>",
			desc:     "Hides or reveals your ADMIN role in /players and /gas for non-admin moderators. Your UID, showname and IPID are always still visible. Lasts until /admin unhide or a server restart.",
			reqPerms: permissions.PermissionField["ADMIN"],
			category: "admin",
		},
		"terminal": {
			handler:  cmdTerminal,
			minArgs:  0,
			usage:    "Usage: /terminal [lines]",
			desc:     "Shows the last N lines (default 50, max 500) of the server's console/log output.",
			reqPerms: permissions.PermissionField["ADMIN"],
			category: "admin",
		},
		"punishaudit": {
			handler:  cmdPunishAudit,
			minArgs:  0,
			usage:    "Usage: /punishaudit <on|off>",
			desc:     "Toggles the OOC alerts you receive when a moderator issues a punishment command.",
			reqPerms: permissions.PermissionField["ADMIN"],
			category: "admin",
		},
		"lockdown": {
			handler:  cmdLockdown,
			minArgs:  0,
			usage:    "Usage: /lockdown | /lockdown add <uid> | /lockdown whitelist all|<passkey> | /lockdown unwhitelist <uid|ipid> | /lockdown exemptlist",
			desc:     "Toggles server lockdown, or whitelists IPIDs. While active, only previously-known IPIDs can connect. Turning lockdown ON also instantly kicks every connected non-moderator whose total playtime is under the /setlockdownplaytime threshold (0 = that part is off). 'whitelist all' covers every area on the server; 'whitelist <passkey>' verifies a lockdown passkey (see secretseed in config.toml), letting that IPID in and permanently exempting it from the playtime purge -- 'unwhitelist'/'exemptlist' manage those exemptions. Lockdown status is broadcast to mods only.",
			reqPerms: permissions.PermissionField["BAN"],
			category: "moderation",
		},
		"botban": {
			handler:  cmdBotBan,
			minArgs:  0,
			usage:    "Usage: /botban",
			desc:     "Bans all spectators with less than 2 minutes of total playtime.",
			reqPerms: permissions.PermissionField["BAN"],
			category: "moderation",
		},
		"setglobalnewiplimit": {
			handler:  cmdSetGlobalNewIPLimit,
			minArgs:  1,
			usage:    "Usage: /setglobalnewiplimit <limit>\nSets the global new-IP rate limit. Use 0 to disable.",
			desc:     "Sets the global new-IP rate limit at runtime.",
			reqPerms: permissions.PermissionField["ADMIN"],
			category: "admin",
		},
		"setglobalipwindow": {
			handler:  cmdSetGlobalIPWindow,
			minArgs:  1,
			usage:    "Usage: /setglobalipwindow <seconds>\nSets the global new-IP rate limit time window in seconds.",
			desc:     "Sets the global new-IP rate limit window at runtime.",
			reqPerms: permissions.PermissionField["ADMIN"],
			category: "admin",
		},
		"setplayerlimit": {
			handler:  cmdSetPlayerLimit,
			minArgs:  1,
			usage:    "Usage: /setplayerlimit <limit>\nSets the player capacity lockdown threshold. New connections are rejected when this many players are connected. Use 0 to disable.",
			desc:     "Sets the player capacity lockdown threshold at runtime.",
			reqPerms: permissions.PermissionField["BAN"],
			category: "moderation",
		},
		"setlockdownplaytime": {
			handler:  cmdSetLockdownPlaytime,
			minArgs:  1,
			usage:    "Usage: /setlockdownplaytime <minutes>\nSets the lockdown purge's minimum total-playtime threshold, in minutes. Turning lockdown on instantly kicks every connected non-moderator below it. Use 0 to disable.",
			desc:     "Sets the lockdown playtime purge threshold at runtime.",
			reqPerms: permissions.PermissionField["BAN"],
			category: "moderation",
		},
		"purgedb": {
			handler:  cmdPurgeDB,
			minArgs:  0,
			usage:    "Usage: /purgedb",
			desc:     "Purges all entries from the known-IP database. Use with caution.",
			reqPerms: permissions.PermissionField["ADMIN"],
			category: "admin",
		},
		"lockbg": {
			handler:  cmdLockBG,
			minArgs:  1,
			usage:    "Usage: /lockbg <true|false>",
			desc:     "Toggles locking the BG on or off.",
			reqPerms: permissions.PermissionField["MODIFY_AREA"],
			category: "area",
		},
		"lockmusic": {
			handler:  cmdLockMusic,
			minArgs:  1,
			usage:    "Usage: /lockmusic <true|false>",
			desc:     "Toggles CM only music on or off.",
			reqPerms: permissions.PermissionField["CM"],
			category: "area",
		},
		"judge": {
			handler:  cmdJudgeButtons,
			minArgs:  1,
			usage:    "Usage: /judge <true|false>",
			desc:     "Toggles the WT/CE judge buttons on or off in this area.",
			reqPerms: permissions.PermissionField["MODIFY_AREA"],
			category: "area",
		},
		"punishmentsafe": {
			handler:  cmdPunishmentSafeArea,
			minArgs:  1,
			usage:    "Usage: /punishmentsafe <true|false>",
			desc:     "Toggles punishment-safe mode: mods/shadow mods/admins cannot punish players in this area (ban/mute/kick still work).",
			reqPerms: permissions.PermissionField["ADMIN"],
			category: "admin",
		},
		"musiclock": {
			handler:  cmdMusicLock,
			minArgs:  0,
			usage:    "Usage: /musiclock",
			desc:     "Locks music in the area so only moderators and CMs can change it.",
			reqPerms: permissions.PermissionField["CM"],
			category: "area",
		},
		"musicunlock": {
			handler:  cmdMusicUnlock,
			minArgs:  0,
			usage:    "Usage: /musicunlock",
			desc:     "Unlocks music in the area, restoring normal music permissions.",
			reqPerms: permissions.PermissionField["DJ"],
			category: "area",
		},
		"login": {
			handler:  cmdLogin,
			minArgs:  2,
			usage:    "Usage: /login <username> <password>",
			desc:     "Logs in as moderator.",
			reqPerms: permissions.PermissionField["NONE"],
			category: "account",
		},
		"logout": {
			handler:  cmdLogout,
			minArgs:  0,
			usage:    "Usage: /logout",
			desc:     "Logs out as moderator.",
			reqPerms: permissions.PermissionField["NONE"],
			category: "account",
		},
		"mkusr": {
			handler:  cmdMakeUser,
			minArgs:  3,
			usage:    "Usage: /mkusr <username> <password> <role>",
			desc:     "Creates a new moderator user.",
			reqPerms: permissions.PermissionField["ADMIN"],
			category: "admin",
		},
		"mod": {
			handler:  cmdMod,
			minArgs:  1,
			usage:    "Usage: /mod [-g] <message>\n-g: Send the message globally.",
			desc:     "Sends a message speaking officially as a moderator.",
			reqPerms: permissions.PermissionField["MOD_SPEAK"],
			category: "moderation",
		},
		"modchat": {
			handler:  cmdModChat,
			minArgs:  1,
			usage:    "Usage: /modchat <message>",
			desc:     "Sends a message to other moderators.",
			reqPerms: permissions.PermissionField["MOD_CHAT"],
			category: "moderation",
		},
		"announce": {
			handler:  cmdAnnounce,
			minArgs:  1,
			usage:    "Usage: /announce <message>",
			desc:     "Broadcasts a server announcement to every connected player. Same as the Discord bot's /announce.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "moderation",
		},
		"motd": {
			handler:  cmdMotd,
			minArgs:  0,
			usage:    "Usage /motd",
			desc:     "Sends the server's message of the day.",
			reqPerms: permissions.PermissionField["NONE"],
			category: "general",
		},
		"move": {
			handler:  cmdMove,
			minArgs:  1,
			usage:    "Usage: /move [-u <uid1,<uid2>...] <area>",
			desc:     "Moves to an area.",
			reqPerms: permissions.PermissionField["NONE"],
			category: "general",
		},
		"modnote": {
			handler:  cmdModnote,
			minArgs:  1,
			usage:    "Usage: /modnote add <ipid> <note> | /modnote list <ipid> | /modnote delete <id>",
			desc:     "Manages per-IPID moderator notes.",
			reqPerms: permissions.PermissionField["BAN_INFO"],
			category: "moderation",
		},
		"mute": {
			handler:  cmdMute,
			minArgs:  1,
			usage:    "Usage: /mute [-ic][-ooc][-m][-j][-d duration][-r reason] <uid1>,<uid2>...\n-ic: Mute IC.\n-ooc: Mute OOC.\n-m: Mute music.\n-j: Mute judge.",
			desc:     "Mutes users(s) from IC, OOC, changing music, and/or judge controls.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "moderation",
		},
		"vmute": {
			handler:  cmdVmute,
			minArgs:  1,
			usage:    "Usage: /vmute [-d seconds] [-r reason] <ipid1>,<ipid2>...",
			desc:     "Mutes an IPID from voice chat.  Any live clients are ejected.",
			reqPerms: permissions.PermissionField["MUTE"],
			voiceCmd: true,
			category: "voice",
		},
		"vunmute": {
			handler:  cmdVunmute,
			minArgs:  1,
			usage:    "Usage: /vunmute <ipid1>,<ipid2>...",
			desc:     "Lifts a voice mute.",
			reqPerms: permissions.PermissionField["MUTE"],
			voiceCmd: true,
			category: "voice",
		},
		"vban": {
			handler:  cmdVban,
			minArgs:  1,
			usage:    "Usage: /vban [-d seconds] [-r reason] <ipid1>,<ipid2>...",
			desc:     "Bans an IPID from voice chat.  Any live clients are ejected.",
			reqPerms: permissions.PermissionField["BAN"],
			voiceCmd: true,
			category: "voice",
		},
		"vunban": {
			handler:  cmdVunban,
			minArgs:  1,
			usage:    "Usage: /vunban <ipid1>,<ipid2>...",
			desc:     "Lifts a voice ban.",
			reqPerms: permissions.PermissionField["BAN"],
			voiceCmd: true,
			category: "voice",
		},
		"vkick": {
			handler:  cmdVkick,
			minArgs:  1,
			usage:    "Usage: /vkick <uid1>,<uid2>...  |  /vkick -i <ipid1>,<ipid2>...",
			desc:     "Ejects one or more clients from voice chat without persistent punishment.",
			reqPerms: permissions.PermissionField["MUTE"],
			voiceCmd: true,
			category: "voice",
		},
		"vlist": {
			handler:  cmdVlist,
			minArgs:  0,
			usage:    "Usage: /vlist",
			desc:     "Lists voice participants in the current area.",
			reqPerms: permissions.PermissionField["NONE"],
			voiceCmd: true,
			category: "voice",
		},
		"vbans": {
			handler:  cmdVbans,
			minArgs:  0,
			usage:    "Usage: /vbans",
			desc:     "Lists active voice mutes and voice bans.",
			reqPerms: permissions.PermissionField["BAN"],
			voiceCmd: true,
			category: "voice",
		},
		"voicearea": {
			handler:  cmdVoiceArea,
			minArgs:  0,
			usage:    "Usage: /voicearea on|off",
			desc:     "Toggles voice chat for the current area.  Ejects participants when turned off.",
			reqPerms: permissions.PermissionField["MODIFY_AREA"],
			voiceCmd: true,
			category: "voice",
		},
		"voicemute": {
			handler:  cmdVoiceMute,
			minArgs:  1,
			usage:    "Usage: /voicemute [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Voice punishment: silently drops every one of the target's voice-chat frames.",
			reqPerms: permissions.PermissionField["MUTE"],
			voiceCmd: true,
			category: "punishment",
		},
		"voicestatic": {
			handler:  cmdVoiceStatic,
			minArgs:  1,
			usage:    "Usage: /voicestatic [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Voice punishment: drops ~60% of the target's voice frames — choppy, breaking up.",
			reqPerms: permissions.PermissionField["MUTE"],
			voiceCmd: true,
			category: "punishment",
		},
		"voicegarble": {
			handler:  cmdVoiceGarble,
			minArgs:  1,
			usage:    "Usage: /voicegarble [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Voice punishment: drops ~88% of the target's voice frames — barely intelligible.",
			reqPerms: permissions.PermissionField["MUTE"],
			voiceCmd: true,
			category: "punishment",
		},
		"voicecutout": {
			handler:  cmdVoiceCutout,
			minArgs:  1,
			usage:    "Usage: /voicecutout [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Voice punishment: gates the target's voice on a walkie-talkie on/off cycle.",
			reqPerms: permissions.PermissionField["MUTE"],
			voiceCmd: true,
			category: "punishment",
		},
		"voicestutter": {
			handler:  cmdVoiceStutter,
			minArgs:  1,
			usage:    "Usage: /voicestutter [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Voice punishment: randomly replays stale frames — a glitchy voice stutter.",
			reqPerms: permissions.PermissionField["MUTE"],
			voiceCmd: true,
			category: "punishment",
		},
		"narrator": {
			handler:  cmdNarrator,
			minArgs:  0,
			usage:    "Usage: /narrator",
			desc:     "Toggles narrator mode on or off.",
			reqPerms: permissions.PermissionField["NONE"],
			category: "general",
		},
		"nointpres": {
			handler:  cmdNoIntPres,
			minArgs:  1,
			usage:    "Usage: /nointpres <true|false>",
			desc:     "Toggles non-interrupting preanims in the current area on or off.",
			reqPerms: permissions.PermissionField["MODIFY_AREA"],
			category: "area",
		},
		"parrot": {
			handler:  cmdParrot,
			minArgs:  1,
			usage:    "Usage: /parrot [-d duration][-r reason] <uid1>,<uid2>...",
			desc:     "Parrots user(s).",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "moderation",
		},
		"play": {
			handler:  cmdPlay,
			minArgs:  1,
			usage:    "Usage: /play <song>",
			desc:     "Plays a song.",
			reqPerms: permissions.PermissionField["DJ"],
			category: "area",
		},
		"players": {
			handler:  cmdPlayers,
			minArgs:  0,
			usage:    "Usage: /players [-a]\n-a: Target all areas.",
			desc:     "Shows players in the current or all areas.",
			reqPerms: permissions.PermissionField["NONE"],
			category: "general",
		},
		"pair": {
			handler:  cmdPair,
			minArgs:  1,
			usage:    "Usage: /pair <uid>",
			desc:     "Sends or accepts a pair request with the specified player.",
			reqPerms: permissions.PermissionField["NONE"],
			category: "general",
		},
		"forcepair": {
			handler:  cmdForcePair,
			minArgs:  2,
			usage:    "Usage: /forcepair <uid1> <uid2>",
			desc:     "Forces two players to pair without requiring mutual consent.",
			reqPerms: permissions.PermissionField["KICK"],
			category: "moderation",
		},
		"forcename": {
			handler:  cmdForceName,
			minArgs:  2,
			usage:    "Usage: /forcename <uid> <name>",
			desc:     "Forces a player to use a specific showname in IC messages.",
			reqPerms: permissions.PermissionField["KICK"],
			category: "moderation",
		},
		"unforcename": {
			handler:  cmdUnforceName,
			minArgs:  1,
			usage:    "Usage: /unforcename <uid>",
			desc:     "Removes a forced showname from a player.",
			reqPerms: permissions.PermissionField["KICK"],
			category: "moderation",
		},
		"charshuffle": {
			handler:  cmdCharShuffle,
			minArgs:  0,
			usage:    "Usage: /charshuffle",
			desc:     "Randomly permutes the character ID of every player in the area (char1↔char2, etc). Originals are remembered — use /uncharshuffle to restore.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "moderation",
		},
		"uncharshuffle": {
			handler:  cmdUnCharShuffle,
			minArgs:  0,
			usage:    "Usage: /uncharshuffle",
			desc:     "Restores every player to the character they had before /charshuffle.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "moderation",
		},
		"nameshuffle": {
			handler:  cmdNameShuffle,
			minArgs:  0,
			usage:    "Usage: /nameshuffle",
			desc:     "Randomly shuffles the shownames of all players in the current area.",
			reqPerms: permissions.PermissionField["KICK"],
			category: "moderation",
		},
		"unnameshuffle": {
			handler:  cmdUnnameShuffle,
			minArgs:  0,
			usage:    "Usage: /unnameshuffle",
			desc:     "Restores all players' own shownames in the current area, undoing any name shuffle.",
			reqPerms: permissions.PermissionField["KICK"],
			category: "moderation",
		},
		"reversename": {
			handler:  cmdReverseName,
			minArgs:  1,
			usage:    "Usage: /reversename <uid[,uid...]> | global",
			desc:     "Reverses a player's showname (Phoenix -> xineohP). Use \"global\" for the whole area; undo with /unreversename.",
			reqPerms: permissions.PermissionField["KICK"],
			category: "moderation",
		},
		"unreversename": {
			handler:  cmdUnreverseName,
			minArgs:  1,
			usage:    "Usage: /unreversename <uid[,uid...]> | global",
			desc:     "Restores a showname reversed by /reversename. Use \"global\" for the whole area.",
			reqPerms: permissions.PermissionField["KICK"],
			category: "moderation",
		},
		"tung": {
			handler:  cmdTung,
			minArgs:  1,
			usage:    "Usage: /tung global [off]",
			desc:     "Forces everyone in your current area to display as \"tung tung sahur\". Use \"off\" to remove.",
			reqPerms: permissions.PermissionField["KICK"],
			category: "moderation",
		},
		"untung": {
			handler:  cmdUntung,
			minArgs:  0,
			usage:    "Usage: /untung global",
			desc:     "Removes the tung effect from everyone in your current area.",
			reqPerms: permissions.PermissionField["KICK"],
			category: "moderation",
		},
		"translate": {
			handler: cmdTranslate,
			minArgs: 2,
			usage: "Usage: /translate <text> <language>\n" +
				"  <language> may be:\n" +
				"    • an English name  — french, spanish, japanese, german, russian, arabic, ...\n" +
				"    • an ISO code      — fr, es, ja, de, ru, ar, zh-CN, ...\n" +
				"    • the keyword      — random  (each word translated into a different language)\n" +
				"\n" +
				"Examples:\n" +
				"  /translate はっはっはっ！！うっせえ、ボケ！！ english\n" +
				"  /translate Good morning! japanese\n" +
				"  /translate Hello world french\n" +
				"\n" +
				"Has a 25-second cooldown per player (configurable via translate_cooldown in config.toml).\n" +
				"Requires enable_translator_punishment = true and translator_api_key set in config.toml.\n" +
				"Moderators bypass the cooldown.",
			desc:     "Translates text into another language using DeepL. Available to all players with a 25-second cooldown.",
			reqPerms: permissions.PermissionField["NONE"],
			category: "general",
		},
		"areainiswap": {
			handler:  cmdAreaIniswap,
			minArgs:  1,
			usage:    "Usage: /areainiswap <character name> | /areainiswap off",
			desc:     "Forces everyone in your current area to iniswap as a chosen character.",
			reqPerms: permissions.PermissionField["KICK"],
			category: "punishment",
		},
		"unpair": {
			handler:  cmdUnpair,
			minArgs:  0,
			usage:    "Usage: /unpair",
			desc:     "Cancels your current pair request or active pairing.",
			reqPerms: permissions.PermissionField["NONE"],
			category: "general",
		},
		"forcerandomchar": {
			handler:  cmdForceRandomChar,
			minArgs:  0,
			usage:    "Usage: /forcerandomchar [uid]",
			desc:     "Forces all players in the current area (or a specific player by UID) to select a random free character.",
			reqPerms: permissions.PermissionField["SHADOW"],
			category: "moderation",
		},
		"forcepos": {
			handler:  cmdForcePos,
			minArgs:  2,
			usage:    "Usage: /forcepos <uid1>,<uid2>,...|all <position>\nAvailable positions: def, pro, wit, jud, hld, hlp, jur, sea",
			desc:     "CM tool: forces one or more players in your area (or everyone with \"all\") into a specific courtroom position, same as /pos but for another player. Not persistent — they can /pos themself elsewhere afterward.",
			reqPerms: permissions.PermissionField["CM"],
			category: "area",
		},
		"curserandomchar": {
			handler:  cmdCurseRandomChar,
			minArgs:  1,
			usage:    "Usage: /curserandomchar <uid>",
			desc:     "Curses a player: their character randomly changes every 1-5 seconds, forever, even across reconnects, until an admin removes it with /uncurserandomchar.",
			reqPerms: permissions.PermissionField["ADMIN"],
			category: "moderation",
		},
		"uncurserandomchar": {
			handler:  cmdUnCurseRandomChar,
			minArgs:  1,
			usage:    "Usage: /uncurserandomchar <uid>",
			desc:     "Removes an active /curserandomchar curse from a player.",
			reqPerms: permissions.PermissionField["ADMIN"],
			category: "moderation",
		},
		"forceunpair": {
			handler:  cmdForceUnpair,
			minArgs:  1,
			usage:    "Usage: /forceunpair <uid>",
			desc:     "Forces a player to unpair from their current pair.",
			reqPerms: permissions.PermissionField["KICK"],
			category: "moderation",
		},
		"pm": {
			handler:  cmdPM,
			minArgs:  2,
			usage:    "Usage: /pm <uid1>,<uid2>... <message>",
			desc:     "Sends a private message.",
			reqPerms: permissions.PermissionField["NONE"],
			category: "general",
		},
		"pos": {
			handler:  cmdPos,
			minArgs:  0,
			usage:    "Usage: /pos [position]",
			desc:     "Shows your current position or changes it to the given position.",
			reqPerms: permissions.PermissionField["NONE"],
			category: "general",
		},
		"poll": {
			handler:  cmdPoll,
			minArgs:  1,
			usage:    "Usage: /poll [question]|[option1]|[option2]|[option3...]",
			desc:     "Creates a poll in the current area.",
			reqPerms: permissions.PermissionField["CM"],
			category: "area",
		},
		"rmusr": {
			handler:  cmdRemoveUser,
			minArgs:  1,
			usage:    "Usage: /rmusr <username>",
			desc:     "Removes a moderator user.",
			reqPerms: permissions.PermissionField["ADMIN"],
			category: "admin",
		},
		"removerole": {
			handler:  cmdRemoveRole,
			minArgs:  1,
			usage:    "Usage: /removerole <username>",
			desc:     "Removes a user's role, resetting them to a default player account. Keeps the account intact (login, chips, playtime, tags) — does NOT delete it like /rmusr.",
			reqPerms: permissions.PermissionField["ADMIN"],
			category: "admin",
		},
		"resetpass": {
			handler:  cmdResetPassword,
			minArgs:  2,
			usage:    "Usage: /resetpass <username> <new_password>",
			desc:     "Resets the password for the given account.",
			reqPerms: permissions.PermissionField["ADMIN"],
			category: "admin",
		},
		"roll": {
			handler:  cmdRoll,
			minArgs:  1,
			usage:    "Usage: /roll [-p] <dice>d<sides>[+/-<modifier>]\n-p: Sets the roll to be private.",
			desc:     "Rolls dice.",
			reqPerms: permissions.PermissionField["NONE"],
			category: "general",
		},
		"randomchar": {
			handler:  cmdRandomChar,
			minArgs:  0,
			usage:    "Usage: /randomchar",
			desc:     "Selects a random free character.",
			reqPerms: permissions.PermissionField["NONE"],
			category: "general",
		},
		"randombg": {
			handler:  cmdRandomBg,
			minArgs:  0,
			usage:    "Usage: /randombg",
			desc:     "Sets the area's background to a random one from the server list. Usable once every 5 seconds.",
			reqPerms: permissions.PermissionField["CM"],
			category: "area",
		},
		"randomsong": {
			handler:  cmdRandomSong,
			minArgs:  0,
			usage:    "Usage: /randomsong",
			desc:     "Plays a random song from the server music list. Tiered cooldown: 20s for regular users, 5s for DJs, no cooldown for moderators (configurable).",
			reqPerms: permissions.PermissionField["NONE"],
			category: "area",
		},
		"rps": {
			handler:  cmdRps,
			minArgs:  1,
			usage:    "Usage: /rps <rock|paper|scissors>",
			desc:     "Play rock-paper-scissors.",
			reqPerms: permissions.PermissionField["NONE"],
			category: "general",
		},
		"coinflip": {
			handler:  cmdCoinflip,
			minArgs:  1,
			usage:    "Usage: /coinflip <heads|tails>",
			desc:     "Challenge another player to a coinflip.",
			reqPerms: permissions.PermissionField["NONE"],
			category: "general",
		},
		"maso": {
			handler:  cmdMaso,
			minArgs:  0,
			usage:    "Usage: /maso [-d duration]\nOptional -d sets how long the effect lasts (default 10m, max 24h). Examples: /maso -d 1h, /maso -d 30m",
			desc:     "Self-apply a random punishment (default 10 min, max 24 h via -d). Type again to reroll to a different punishment.",
			reqPerms: permissions.PermissionField["NONE"],
			category: "punishment",
		},
		"cvote": {
			handler: cmdCvote,
			minArgs: 0,
			usage: "Usage: /cvote <action> <uid> [reason]   — start or add your vote\n" +
				"       /cvote list                       — show all active votes\n" +
				"\n" +
				"Actions players can vote on (server-configurable):\n" +
				"  kick     — disconnect the player from the server\n" +
				"  mute     — silence the player temporarily\n" +
				"  ban      — ban the player (moderator must approve)\n" +
				"  warn     — send the player a formal warning message\n" +
				"  areakick — move the player to the default area\n" +
				"\n" +
				"Votes require a majority to pass; moderators always make the final call.\n" +
				"\n" +
				"Moderator sub-commands:\n" +
				"  /cvote accept <uid>  — enforce a passed vote\n" +
				"  /cvote reject <uid>  — deny a passed vote\n" +
				"  /cvote cancel <uid>  — cancel any active vote",
			desc:     "Community moderation voting. Players vote to kick/mute/ban/warn/areakick; moderators have final say.",
			reqPerms: permissions.PermissionField["NONE"],
			category: "general",
		},
		"setrole": {
			handler:  cmdChangeRole,
			minArgs:  2,
			usage:    "Usage: /setrole <username> <role>",
			desc:     "Changes a moderator user's role.",
			reqPerms: permissions.PermissionField["ADMIN"],
			category: "admin",
		},
		"spectate": {
			handler:    cmdSpectate,
			minArgs:    0,
			usage:      "Usage: /spectate [invite|uninvite <uid1>,<uid2>...]\nToggles spectate mode (CM only). While spectate mode is on, only CMs and invited players may speak in IC — a CM grants speaking with /spectate invite <uid> (or /invite <uid>).",
			desc:       "Toggles spectate mode, or invites/uninvites users to speak in IC during spectate mode.",
			reqPerms:   permissions.PermissionField["CM"],
			category:   "area",
			publicHelp: true,
		},
		"status": {
			handler:  cmdStatus,
			minArgs:  1,
			usage:    "Usage: /status <idle|looking-for-players|lfp|casing|recess|rp|gaming>",
			desc:     "Sets the current area's status. \"lfp\" is a shorthand for looking-for-players.",
			reqPerms: permissions.PermissionField["CM"],
			category: "area",
		},
		"summon": {
			handler:  cmdSummon,
			minArgs:  1,
			usage:    "Usage: /summon <area>",
			desc:     "Summons all users to the specified area.",
			reqPerms: permissions.PermissionField["MOVE_USERS"],
			category: "moderation",
		},
		"swapevi": {
			handler:  cmdSwapEvi,
			minArgs:  2,
			usage:    "Usage: /swapevi <id1> <id2>",
			desc:     "Swaps index of evidence.",
			reqPerms: permissions.PermissionField["NONE"],
			category: "general",
		},
		"testimony": {
			handler:  cmdTestimony,
			minArgs:  0,
			usage:    "Usage: /testimony <record|stop|play|update|insert|delete>\nUse /testimony record to start recording. Witnesses must be in /pos wit for their IC messages to be recorded.",
			desc:     "Manages the area's testimony recorder. Use /testimony record to start recording. Witnesses must be in /pos wit for their IC messages to be captured.",
			reqPerms: permissions.PermissionField["NONE"],
			category: "testimony",
		},
		"testify": {
			handler:  cmdTestify,
			minArgs:  0,
			usage:    "Usage: /testify",
			desc:     "Starts recording IC messages as testimony.",
			reqPerms: permissions.PermissionField["CM"],
			category: "testimony",
		},
		"unban": {
			handler:  cmdUnban,
			minArgs:  1,
			usage:    "Usage: /unban <id1>,<id2>...",
			desc:     "Nullifies ban(s).",
			reqPerms: permissions.PermissionField["BAN"],
			category: "moderation",
		},
		"uncm": {
			handler:  cmdUnCM,
			minArgs:  0,
			usage:    "Usage: /uncm [uid1],[uid2]...",
			desc:     "Removes CM(s) from the current area.",
			reqPerms: permissions.PermissionField["CM"],
			category: "area",
		},
		"update": {
			handler:  cmdUpdate,
			minArgs:  0,
			usage:    "Usage: /update",
			desc:     "Updates the current testimony statement with the next IC message from the witness.",
			reqPerms: permissions.PermissionField["CM"],
			category: "testimony",
		},
		"uninvite": {
			handler:  cmdUninvite,
			minArgs:  1,
			usage:    "Usage: /uninvite <uid1>,<uid2>...",
			desc:     "Uninvites user(s) from the current area.",
			reqPerms: permissions.PermissionField["CM"],
			category: "area",
		},
		"unjail": {
			handler:  cmdUnjail,
			minArgs:  1,
			usage:    "Usage: /unjail <uid1>,<uid2>...",
			desc:     "Releases user(s) from jail.",
			reqPerms: permissions.PermissionField["BAN"],
			category: "moderation",
		},
		"uncharstuck": {
			handler:  cmdUnCharStuck,
			minArgs:  1,
			usage:    "Usage: /uncharstuck <uid1>,<uid2>...",
			desc:     "Removes the character-stuck restriction from user(s).",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "moderation",
		},
		"unlock": {
			handler:  cmdUnlock,
			minArgs:  0,
			usage:    "Usage: /unlock",
			desc:     "Unlocks the current area.",
			reqPerms: permissions.PermissionField["CM"],
			category: "area",
		},
		"unmute": {
			handler:  cmdUnmute,
			minArgs:  1,
			usage:    "Usage: /unmute <uid1>,<uid2>...",
			desc:     "Unmutes user(s).",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "moderation",
		},
		"untorment": {
			handler:  cmdUntorment,
			minArgs:  1,
			usage:    "Usage: /untorment <ipid|all>\nall: purge every IPID from the torment list.",
			desc:     "Removes an IPID from the torment list, or purges the entire list with 'all'.",
			reqPerms: permissions.PermissionField["BAN"],
			category: "moderation",
		},
		"tormentlist": {
			handler:  cmdTormentList,
			minArgs:  0,
			usage:    "Usage: /tormentlist",
			desc:     "Lists every IPID on the torment/lag list, with any connected sessions.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "moderation",
		},
		"censoralerts": {
			handler:  cmdCensorAlerts,
			minArgs:  0,
			usage:    "Usage: /censoralerts <on|off>",
			desc:     "Toggles the OOC alerts you receive when a player trips the word censor.",
			reqPerms: permissions.PermissionField["MOD_CHAT"],
			category: "moderation",
		},
		"vote": {
			handler:  cmdVote,
			minArgs:  1,
			usage:    "Usage: /vote <option_number>",
			desc:     "Vote on the active poll.",
			reqPerms: permissions.PermissionField["NONE"],
			category: "area",
		},
		// Punishment commands - Text Modification
		"whisper": {
			handler:  cmdWhisper,
			minArgs:  1,
			usage:    "Usage: /whisper [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Makes messages only visible to mods and CMs.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"backward": {
			handler:  cmdBackward,
			minArgs:  1,
			usage:    "Usage: /backward [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Reverses character order in messages.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"stutterstep": {
			handler:  cmdStutterstep,
			minArgs:  1,
			usage:    "Usage: /stutterstep [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Doubles every word in messages.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"elongate": {
			handler:  cmdElongate,
			minArgs:  1,
			usage:    "Usage: /elongate [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Repeats vowels in messages.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"uppercase": {
			handler:  cmdUppercase,
			minArgs:  1,
			usage:    "Usage: /uppercase [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Forces messages to UPPERCASE.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"lowercase": {
			handler:  cmdLowercase,
			minArgs:  1,
			usage:    "Usage: /lowercase [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Forces messages to lowercase.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"robotic": {
			handler:  cmdRobotic,
			minArgs:  1,
			usage:    "Usage: /robotic [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Replaces messages with [BEEP] [BOOP] robotic sounds.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"alternating": {
			handler:  cmdAlternating,
			minArgs:  1,
			usage:    "Usage: /alternating [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Makes messages AlTeRnAtInG cAsE.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"fancy": {
			handler:  cmdFancy,
			minArgs:  1,
			usage:    "Usage: /fancy [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Converts messages to Unicode fancy characters.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"uwu": {
			handler:  cmdUwu,
			minArgs:  1,
			usage:    "Usage: /uwu [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Converts messages to UwU speak.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"pirate": {
			handler:  cmdPirate,
			minArgs:  1,
			usage:    "Usage: /pirate [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Converts messages to pirate speech.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"shakespearean": {
			handler:  cmdShakespearean,
			minArgs:  1,
			usage:    "Usage: /shakespearean [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Converts messages to Shakespearean English.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"caveman": {
			handler:  cmdCaveman,
			minArgs:  1,
			usage:    "Usage: /caveman [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Converts messages to caveman grunts.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		// Punishment commands - Visibility/Cosmetic
		"emoji": {
			handler:  cmdEmoji,
			minArgs:  1,
			usage:    "Usage: /emoji [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Replaces name with random emojis.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"invisible": {
			handler:  cmdInvisible,
			minArgs:  1,
			usage:    "Usage: /invisible [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Prevents user from seeing other players' messages.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"hidedisplay": {
			handler:  cmdHideDisplay,
			minArgs:  1,
			usage:    "Usage: /hidedisplay [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Hides the target's own sprite from the IC viewport (their text still shows; funny on pairs).",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"musicban": {
			handler:  cmdMusicBan,
			minArgs:  1,
			usage:    "Usage: /musicban <uid> [-r reason]",
			desc:     "Persistently bans the target's IPID from playing music across sessions (bypassed when their area has <3 people).",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "moderation",
		},
		"musicunban": {
			handler:  cmdMusicUnban,
			minArgs:  1,
			usage:    "Usage: /musicunban <uid|ipid>",
			desc:     "Lifts a /musicban. Accepts a connected target's UID or a raw IPID for offline players.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "moderation",
		},
		"musicbans": {
			handler:  cmdMusicBans,
			minArgs:  0,
			usage:    "Usage: /musicbans",
			desc:     "Lists all active /musicban entries (newest first).",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "moderation",
		},
		"reload": {
			handler:  cmdReload,
			minArgs:  0,
			usage:    "Usage: /reload",
			desc:     "Hot-reloads characters.txt (append-only), music.txt, cdns.txt, backgrounds.txt, parrot.txt, 8ball.txt, banned_words.txt and config.toml motd/desc without restarting.",
			reqPerms: permissions.PermissionField["ADMIN"],
			category: "moderation",
		},
		"shadowundisconnect": {
			handler:  cmdShadowUndisconnect,
			minArgs:  1,
			usage:    "Usage: /shadowundisconnect <uid|ipid|all>",
			desc:     "Lifts a shadow-disconnect. Nothing adds to the list any more (/shadowdisconnect was removed), so this exists to clear entries made before then. Accepts a connected target's UID, a raw IPID for offline players, or \"all\" to clear the entire list.",
			reqPerms: permissions.PermissionField["ADMIN"],
			category: "moderation",
		},
		"shadowdisconnectlist": {
			handler:  cmdShadowDisconnectList,
			minArgs:  0,
			usage:    "Usage: /shadowdisconnectlist",
			desc:     "Lists every shadow-disconnect entry left over from before /shadowdisconnect was removed (newest first). Lift one with /shadowundisconnect.",
			reqPerms: permissions.PermissionField["ADMIN"],
			category: "moderation",
		},
		"forcedisplay": {
			handler:  cmdForceDisplay,
			minArgs:  1,
			usage:    "Usage: /forcedisplay [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Pins the target's character onto every IC message in their area; no other sprite can show.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		// Punishment commands - Timing Effects
		"slowpoke": {
			handler:  cmdSlowpoke,
			minArgs:  1,
			usage:    "Usage: /slowpoke [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Delays messages before sending.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"fastspammer": {
			handler:  cmdFastspammer,
			minArgs:  1,
			usage:    "Usage: /fastspammer [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Rate limits messages heavily.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"pause": {
			handler:  cmdPause,
			minArgs:  0,
			usage:    "Usage: /pause",
			desc:     "Stops testimony recording.",
			reqPerms: permissions.PermissionField["CM"],
			category: "testimony",
		},
		"unlag": {
			handler:  cmdUnlag,
			minArgs:  1,
			usage:    "Usage: /unlag <uid>",
			desc:     "Removes a player's IPID from the lag list.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		// Punishment commands - Social Chaos
		"subtitles": {
			handler:  cmdSubtitles,
			minArgs:  1,
			usage:    "Usage: /subtitles [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Adds confusing subtitles to messages.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"roulette": {
			handler:  cmdRoulette,
			minArgs:  0,
			usage:    "Usage: /roulette join | /roulette [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Join Russian Roulette game, or apply roulette punishment to user(s) (requires MUTE permission).",
			reqPerms: permissions.PermissionField["NONE"],
			category: "minigames",
		},
		"spotlight": {
			handler:  cmdSpotlight,
			minArgs:  1,
			usage:    "Usage: /spotlight [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Announces all actions publicly.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		// Punishment commands - Text Processing
		"censor": {
			handler:  cmdCensor,
			minArgs:  1,
			usage:    "Usage: /censor [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Replaces words with [CENSORED].",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"confused": {
			handler:  cmdConfused,
			minArgs:  1,
			usage:    "Usage: /confused [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Randomly reorders words in messages.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"paranoid": {
			handler:  cmdParanoid,
			minArgs:  1,
			usage:    "Usage: /paranoid [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Adds paranoid text to messages.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"drunk": {
			handler:  cmdDrunk,
			minArgs:  1,
			usage:    "Usage: /drunk [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Slurs and repeats words in messages.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"hiccup": {
			handler:  cmdHiccup,
			minArgs:  1,
			usage:    "Usage: /hiccup [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Interrupts words with 'hic'.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"whistle": {
			handler:  cmdWhistle,
			minArgs:  1,
			usage:    "Usage: /whistle [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Replaces letters with whistles.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"mumble": {
			handler:  cmdMumble,
			minArgs:  1,
			usage:    "Usage: /mumble [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Obscures message text.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		// Punishment commands - Complex Effects
		"spaghetti": {
			handler:  cmdSpaghetti,
			minArgs:  1,
			usage:    "Usage: /spaghetti [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Combines multiple random effects.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"torment": {
			handler:  cmdTorment,
			minArgs:  1,
			usage:    "Usage: /torment [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Cycles through different effects.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"rng": {
			handler:  cmdRng,
			minArgs:  1,
			usage:    "Usage: /rng [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Applies random effect from pool each message.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"essay": {
			handler:  cmdEssay,
			minArgs:  1,
			usage:    "Usage: /essay [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Requires minimum 50 characters.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		// Punishment commands - Advanced
		"haiku": {
			handler:  cmdHaiku,
			minArgs:  1,
			usage:    "Usage: /haiku [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Requires 5-7-5 syllable format.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"autospell": {
			handler:  cmdAutospell,
			minArgs:  1,
			usage:    "Usage: /autospell [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Autocorrects to wrong words.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		// Punishment commands - Animal Sounds
		"monkey": {
			handler:  cmdMonkey,
			minArgs:  1,
			usage:    "Usage: /monkey [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Replaces messages with monkey noises (ook, eek, ooh ooh).",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"snake": {
			handler:  cmdSnake,
			minArgs:  1,
			usage:    "Usage: /snake [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Makes messages hissss like a ssssnake.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"dog": {
			handler:  cmdDog,
			minArgs:  1,
			usage:    "Usage: /dog [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Replaces messages with dog sounds (woof, arf, grr, bork).",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"cat": {
			handler:  cmdCat,
			minArgs:  1,
			usage:    "Usage: /cat [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Replaces messages with cat sounds (meow, purrr~, mrrrow).",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"bird": {
			handler:  cmdBird,
			minArgs:  1,
			usage:    "Usage: /bird [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Replaces messages with bird sounds (tweet, chirp, squawk).",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"cow": {
			handler:  cmdCow,
			minArgs:  1,
			usage:    "Usage: /cow [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Replaces messages with cow sounds (moo, mooo, MOOO).",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"frog": {
			handler:  cmdFrog,
			minArgs:  1,
			usage:    "Usage: /frog [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Replaces messages with frog sounds (ribbit, croak).",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"duck": {
			handler:  cmdDuck,
			minArgs:  1,
			usage:    "Usage: /duck [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Replaces messages with duck sounds (quack, QUACK).",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"horse": {
			handler:  cmdHorse,
			minArgs:  1,
			usage:    "Usage: /horse [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Replaces messages with horse sounds (neigh, whinny, snort) and turns the target into a random uma.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"trex": {
			handler:  cmdTrex,
			minArgs:  1,
			usage:    "Usage: /trex [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Replaces messages with tyrannosaur roars (RAAASRFH).",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"fish": {
			handler:  cmdFish,
			minArgs:  1,
			usage:    "Usage: /fish [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Replaces messages with fish noises (blub, blublublib).",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"lion": {
			handler:  cmdLion,
			minArgs:  1,
			usage:    "Usage: /lion [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Replaces messages with lion sounds (ROAR, grrr, rawr).",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"zoo": {
			handler:  cmdZoo,
			minArgs:  1,
			usage:    "Usage: /zoo [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Applies a random animal sound punishment to each message.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"bunny": {
			handler:  cmdBunny,
			minArgs:  1,
			usage:    "Usage: /bunny [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Replaces messages with bunny sounds (*thump*, *binky!*, *flops*).",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"tsundere": {
			handler:  cmdTsundere,
			minArgs:  1,
			usage:    "Usage: /tsundere [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "It's not like I wanted to punish you, b-baka!! Wraps messages in tsundere denial.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"yandere": {
			handler:  cmdYandere,
			minArgs:  1,
			usage:    "Usage: /yandere [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Hehehe~ wraps messages in obsessive yandere flavour.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"kuudere": {
			handler:  cmdKuudere,
			minArgs:  1,
			usage:    "Usage: /kuudere [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Delivers messages in cold, emotionless monotone.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"dandere": {
			handler:  cmdDandere,
			minArgs:  1,
			usage:    "Usage: /dandere [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Makes messages extremely shy and hesitant with stutters.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"deredere": {
			handler:  cmdDeredere,
			minArgs:  1,
			usage:    "Usage: /deredere [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Wraps messages in over-the-top lovey-dovey sweetness.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"himedere": {
			handler:  cmdHimedere,
			minArgs:  1,
			usage:    "Usage: /himedere [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Makes messages imperious and royalty-like, commoner.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"kamidere": {
			handler:  cmdKamidere,
			minArgs:  1,
			usage:    "Usage: /kamidere [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Delivers messages as a self-proclaimed god to unworthy mortals.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"undere": {
			handler:  cmdUndere,
			minArgs:  1,
			usage:    "Usage: /undere [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Forces messages to agree with everything unconditionally.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"bakadere": {
			handler:  cmdBakadere,
			minArgs:  1,
			usage:    "Usage: /bakadere [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Inserts clumsy, airheaded interjections into every message.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"mayadere": {
			handler:  cmdMayadere,
			minArgs:  1,
			usage:    "Usage: /mayadere [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Wraps messages in eerie, enigmatic mystery. Kukuku~",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"emoticon": {
			handler:  cmdEmoticon,
			minArgs:  1,
			usage:    "Usage: /emoticon [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Forces user to speak only in emoticons (:P, :D, :3, etc.).",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		// Social Torment Punishments
		"lovebomb": {
			handler:  cmdLovebomb,
			minArgs:  0,
			usage:    "Usage: /lovebomb [global [off]] | /lovebomb [-d duration] [-r reason] [-h] [uid1 [uid2]]\n  global           – love-bomb all non-moderators in the area.\n  global off       – remove lovebomb from everyone in the area.\n  -d <duration>    – duration (e.g. 10m, 1h). Default: 10m. Max: 24h.\n  -r <reason>      – optional reason for the log.\n  1 uid            – apply to that uid (random area target per message).\n  2 uids           – uid1 will love-bomb uid2 specifically.",
			desc:     "Forces IC messages to be replaced with silly love declarations. Moderator only.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"unlovebomb": {
			handler:  cmdUnlovebomb,
			minArgs:  1,
			usage:    "Usage: /unlovebomb <uid1>,<uid2>...",
			desc:     "Removes lovebomb punishment from user(s). Moderator only.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"degrade": {
			handler:  cmdDegrade,
			minArgs:  1,
			usage:    "Usage: /degrade [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Forces IC messages to be replaced with degrading self-insults. Moderator only.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"undegrade": {
			handler:  cmdUndegrade,
			minArgs:  1,
			usage:    "Usage: /undegrade <uid1>,<uid2>...",
			desc:     "Removes degrade punishment from user(s). Moderator only.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"tourettes": {
			handler:  cmdTourettes,
			minArgs:  1,
			usage:    "Usage: /tourettes [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Causes random outbursts to be inserted into IC messages (swearing, random objects, nonsense, animal noises). Moderator only.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"slang": {
			handler:  cmdSlang,
			minArgs:  1,
			usage:    "Usage: /slang [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Converts messages to internet slang abbreviations (e.g. 'i don't know' -> 'idk', 'got to go' -> 'gtg').",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"unslang": {
			handler:  cmdUnslang,
			minArgs:  1,
			usage:    "Usage: /unslang <uid1>,<uid2>...",
			desc:     "Removes slang punishment from user(s). Moderator only.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"thesaurusoverload": {
			handler:  cmdThesaurusOverload,
			minArgs:  1,
			usage:    "Usage: /thesaurusoverload [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Forces IC messages to use comically pompous synonyms and smug parentheticals. Moderator only.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"unthesaurusoverload": {
			handler:  cmdUnthesaurusoverload,
			minArgs:  1,
			usage:    "Usage: /unthesaurusoverload <uid1>,<uid2>...",
			desc:     "Removes thesaurusoverload punishment from user(s). Moderator only.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"valleygirl": {
			handler:  cmdValleyGirl,
			minArgs:  1,
			usage:    "Usage: /valleygirl [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Injects valley-girl filler words, vowel stretching, and dramatic tone into IC messages. Moderator only.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"unvalleygirl": {
			handler:  cmdUnvalleygirl,
			minArgs:  1,
			usage:    "Usage: /unvalleygirl <uid1>,<uid2>...",
			desc:     "Removes valleygirl punishment from user(s). Moderator only.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"babytalk": {
			handler:  cmdBabytalk,
			minArgs:  1,
			usage:    "Usage: /babytalk [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Converts IC messages to toddler-style baby talk with phonetic substitutions and stage directions. Moderator only.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"unbabytalk": {
			handler:  cmdUnbabytalk,
			minArgs:  1,
			usage:    "Usage: /unbabytalk <uid1>,<uid2>...",
			desc:     "Removes babytalk punishment from user(s). Moderator only.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"thirdperson": {
			handler:  cmdThirdPerson,
			minArgs:  1,
			usage:    "Usage: /thirdperson [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Forces IC messages into third-person narration using the player's display name, with mood tags. Moderator only.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"unthirdperson": {
			handler:  cmdUnthirdperson,
			minArgs:  1,
			usage:    "Usage: /unthirdperson <uid1>,<uid2>...",
			desc:     "Removes thirdperson punishment from user(s). Moderator only.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"unreliablenarrator": {
			handler:  cmdUnreliableNarrator,
			minArgs:  1,
			usage:    "Usage: /unreliablenarrator [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Makes IC messages sound suspiciously unreliable with hedges, contradictions, and self-doubting commentary. Moderator only.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"ununreliablenarrator": {
			handler:  cmdUnunreliablenarrator,
			minArgs:  1,
			usage:    "Usage: /ununreliablenarrator <uid1>,<uid2>...",
			desc:     "Removes unreliablenarrator punishment from user(s). Moderator only.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"uncannyvalley": {
			handler:  cmdUncannyValley,
			minArgs:  1,
			usage:    "Usage: /uncannyvalley [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Adds glitchy system notes to IC messages and subtly mutates the player's display name each message. Moderator only.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"ununcannyvalley": {
			handler:  cmdUnuncannyvalley,
			minArgs:  1,
			usage:    "Usage: /ununcannyvalley <uid1>,<uid2>...",
			desc:     "Removes uncannyvalley punishment from user(s). Moderator only.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"51": {
			handler:  cmd51,
			minArgs:  1,
			usage:    "Usage: /51 [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Replaces each IC message with a random line from the 51-messages story. Moderator only.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"un51": {
			handler:  cmdUn51,
			minArgs:  1,
			usage:    "Usage: /un51 <uid1>,<uid2>...",
			desc:     "Removes 51 punishment from user(s). Moderator only.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"philosopher": {
			handler:  cmdPhilosopher,
			minArgs:  1,
			usage:    "Usage: /philosopher [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Appends a random deep philosophical question to every IC message. Moderator only.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"unphilosopher": {
			handler:  cmdUnphilosopher,
			minArgs:  1,
			usage:    "Usage: /unphilosopher <uid1>,<uid2>...",
			desc:     "Removes philosopher punishment from user(s). Moderator only.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"poet": {
			handler:  cmdPoet,
			minArgs:  1,
			usage:    "Usage: /poet [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Wraps every IC message in lyrical poetic flourishes. Moderator only.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"unpoet": {
			handler:  cmdUnpoet,
			minArgs:  1,
			usage:    "Usage: /unpoet <uid1>,<uid2>...",
			desc:     "Removes poet punishment from user(s). Moderator only.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"upsidedown": {
			handler:  cmdUpsidedown,
			minArgs:  1,
			usage:    "Usage: /upsidedown [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Flips every IC message upside-down using Unicode characters. Moderator only.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"unupsidedown": {
			handler:  cmdUnupsidedown,
			minArgs:  1,
			usage:    "Usage: /unupsidedown <uid1>,<uid2>...",
			desc:     "Removes upsidedown punishment from user(s). Moderator only.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"sarcasm": {
			handler:  cmdSarcasm,
			minArgs:  1,
			usage:    "Usage: /sarcasm [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Adds sarcastic parenthetical commentary to every IC message. Moderator only.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"unsarcasm": {
			handler:  cmdUnsarcasm,
			minArgs:  1,
			usage:    "Usage: /unsarcasm <uid1>,<uid2>...",
			desc:     "Removes sarcasm punishment from user(s). Moderator only.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"academic": {
			handler:  cmdAcademic,
			minArgs:  1,
			usage:    "Usage: /academic [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Wraps every IC message in overly formal academic language. Moderator only.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"unacademic": {
			handler:  cmdUnacademic,
			minArgs:  1,
			usage:    "Usage: /unacademic <uid1>,<uid2>...",
			desc:     "Removes academic punishment from user(s). Moderator only.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"recipe": {
			handler:  cmdRecipe,
			minArgs:  1,
			usage:    "Usage: /recipe [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Reformats every IC message as a cooking recipe step. Moderator only.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"unrecipe": {
			handler:  cmdUnrecipe,
			minArgs:  1,
			usage:    "Usage: /unrecipe <uid1>,<uid2>...",
			desc:     "Removes recipe punishment from user(s). Moderator only.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"quote": {
			handler:  cmdQuote,
			minArgs:  1,
			usage:    "Usage: /quote [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Wraps IC messages in quotation marks with a 50% chance. Moderator only.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"unquote": {
			handler:  cmdUnquote,
			minArgs:  1,
			usage:    "Usage: /unquote <uid1>,<uid2>...",
			desc:     "Removes quote punishment from user(s). Moderator only.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"translator": {
			handler: cmdTranslator,
			minArgs: 3,
			usage: "Usage: /translator curse [-d duration] [-r reason] [-h] <uid1>,<uid2>...|global <language>\n" +
				"  <target>   may be a comma-separated UID list, OR the keyword:\n" +
				"    • global        — every other player in YOUR current area (you are exempt).\n" +
				"  <language> may be:\n" +
				"    • an English name  — french, spanish, japanese, german, russian, arabic, etc.\n" +
				"    • an ISO code      — fr, es, ja, de, ru, ar, zh-CN, etc.\n" +
				"    • the keyword      — random  (each word is translated into a different language)\n" +
				"\n" +
				"Examples:\n" +
				"  /translator curse 7 french                     — target 7 now speaks French.\n" +
				"  /translator curse 7 random                     — each word of target 7's IC\n" +
				"                                                   messages becomes a random language.\n" +
				"  /translator curse -d 30m -r Spam 7,9 japanese  — target 7 and 9 speak Japanese\n" +
				"                                                   for 30 minutes with a reason.\n" +
				"  /translator curse global french                — every other player in your\n" +
				"                                                   area now speaks French.\n" +
				"  /translator curse global random                — every other player's words\n" +
				"                                                   become random languages.\n" +
				"\n" +
				"Remove with: /untranslator curse <uid>  (or: /untranslator curse global)\n" +
				"Requires enable_translator_punishment = true and translator_api_key set in config.toml.",
			desc:     "Translates a target's IC messages into another language (supports 'random' per-word mode). Moderator only.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"untranslator": {
			handler: cmdUntranslator,
			minArgs: 2,
			usage: "Usage: /untranslator curse <uid1>,<uid2>...|global\n" +
				"Removes the translator punishment applied via /translator curse.\n" +
				"  <target>  may be a comma-separated UID list, OR the keyword:\n" +
				"    • global      — every client on the server currently affected.\n" +
				"\n" +
				"Examples:\n" +
				"  /untranslator curse 7       — clears the translator curse from target 7.\n" +
				"  /untranslator curse global  — clears the translator curse from every\n" +
				"                                affected client on the server.",
			desc:     "Removes the translator punishment from user(s), or every affected client with 'global'. Moderator only.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"unpunish": {
			handler:  cmdUnpunish,
			minArgs:  1,
			usage:    "Usage: /unpunish all\n       /unpunish [-t punishment_type] <uid1>,<uid2>...\n       /unpunish [-t punishment_type] <ipid>\nall: Remove all punishments from every client in your current area.\n-t: Specific punishment type to remove (omit to remove all).\nA non-numeric target is treated as a raw IPID, so offline players can be unpunished too.\nThe full-removal forms also clear the punishment_names.txt showname stain, stopping its per-minute random punishment drip.",
			desc:     "Removes punishment(s) from user(s) by UID or IPID, or all punishments from the entire area with 'all'.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"blockpunishment": {
			handler: cmdBlockPunishment,
			minArgs: 1,
			usage: "Usage: /blockpunishment <uid1>,<uid2>,...\n" +
				"Prevents the specified player(s) from using self-applied chaos commands:\n" +
				"  /maso, /megamaso, /potion, /coinflip\n" +
				"Moderator-issued punishments are unaffected — mods can still punish them normally.\n" +
				"The block is IPID-based and does not persist across server restarts.\n" +
				"Use /unblockpunishment to restore access.",
			desc:     "Prevents a player from using self-applied punishment/chaos commands (/maso, /megamaso, /potion, /coinflip). Mod only.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"unblockpunishment": {
			handler: cmdUnblockPunishment,
			minArgs: 1,
			usage: "Usage: /unblockpunishment <uid1>,<uid2>,...\n" +
				"Restores access to self-applied chaos commands for the specified player(s).\n" +
				"They will be able to use /maso, /megamaso, /potion, and /coinflip again.",
			desc:     "Restores a player's ability to use self-applied chaos commands. Mod only.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"icwarp": {
			handler: cmdICWarp,
			minArgs: 1,
			usage: "Usage: /icwarp [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...\n" +
				"       /icwarp global on\n" +
				"       /icwarp global off\n" +
				"-d: Duration (default 10m, max 24h). -r: Reason for logs.\n" +
				"Per-user: targeted player's IC messages are replaced with a random past message\n" +
				"they sent in the current area (last 24 hours) while the punishment is active.\n" +
				"Global on: applies the same backlog effect to everyone in the area except you.\n" +
				"Global off: disables the area-wide effect.",
			desc:     "Punishes player(s) so their IC messages are replaced with random past messages from the area. /icwarp global on/off affects everyone in the area except the moderator.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"unicwarp": {
			handler:  cmdUniCWarp,
			minArgs:  1,
			usage:    "Usage: /unicwarp <uid1>,<uid2>...",
			desc:     "Removes the icwarp punishment from the specified user(s).",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"stack": {
			handler:  cmdStack,
			minArgs:  2,
			usage:    "Usage: /stack <punishment1> <punishment2> [<punishment3>...] [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Applies multiple punishment effects to user(s) simultaneously.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"randompunishall": {
			handler:  cmdRandomPunishAll,
			minArgs:  0,
			usage:    "Usage: /randompunishall [-d duration] [-r reason] [-h]",
			desc:     "Applies a random punishment to every player currently in the area.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"togglerandompunish": {
			handler:  cmdToggleRandomPunish,
			minArgs:  0,
			usage:    "Usage: /togglerandompunish",
			desc:     "Toggles whether /randompunishall can be used in this area.",
			reqPerms: permissions.PermissionField["CM"],
			category: "punishment",
		},
		"tournament": {
			handler:  cmdTournament,
			minArgs:  1,
			usage:    "Usage: /tournament <start|stop|status>",
			desc:     "Manages punishment tournament mode.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"join-tournament": {
			handler:  cmdJoinTournament,
			minArgs:  0,
			usage:    "Usage: /join-tournament",
			desc:     "Join the active punishment tournament.",
			reqPerms: permissions.PermissionField["NONE"],
			category: "minigames",
		},
		"hotpotato": {
			handler:  cmdHotPotato,
			minArgs:  0,
			usage:    "Usage: /hotpotato | /hotpotato accept | /hotpotato pass",
			desc:     "Start or join a Hot Potato mini-game event. The carrier can use /hotpotato pass to pass the potato randomly.",
			reqPerms: permissions.PermissionField["NONE"],
			category: "minigames",
		},
		"giveaway": {
			handler:  cmdGiveaway,
			minArgs:  1,
			usage:    "Usage: /giveaway start <item> | /giveaway enter",
			desc:     "Start a giveaway or enter an active one.",
			reqPerms: permissions.PermissionField["NONE"],
			category: "minigames",
		},
		"quickdraw": {
			handler:  cmdQuickdraw,
			minArgs:  1,
			usage:    "Usage: /quickdraw <uid> | /quickdraw bullet <uid> | /quickdraw accept | /quickdraw decline\n  /quickdraw <uid>        — Standard duel: both players must type a random word after DRAW!\n  /quickdraw bullet <uid> — Bullet duel: first player to send ANY IC message after DRAW! wins.\n  /quickdraw accept       — Accept an incoming duel challenge.\n  /quickdraw decline      — Decline an incoming duel challenge.",
			desc:     "Challenge another player to a quickdraw duel. Standard mode: type a random word first. Bullet mode (/quickdraw bullet <uid>): first to send any IC message wins. The loser receives a random punishment.",
			reqPerms: permissions.PermissionField["NONE"],
			category: "minigames",
		},
		"russianroulette": {
			handler:  cmdRussianRoulette,
			minArgs:  0,
			usage:    "Usage: /russianroulette | /russianroulette join",
			desc:     "Start or join a Russian Roulette game. The unlucky loser receives a wild random punishment.",
			reqPerms: permissions.PermissionField["NONE"],
			category: "minigames",
		},
		"typingrace": {
			handler:  cmdTypingRace,
			minArgs:  0,
			usage:    "Usage: /typingrace | /typingrace join",
			desc:     "Start a typing race or join an active one. First to type the phrase wins chips. Shows words per second on win.",
			reqPerms: permissions.PermissionField["NONE"],
			category: "minigames",
		},
		"hangman": {
			handler: cmdHangman,
			minArgs: 1,
			usage: "Usage: /hangman start [animals|courtroom|nature|food|random|custom <word>]\n" +
				"       /hangman join\n" +
				"       /hangman guess <letter|word>\n" +
				"       /hangman status\n" +
				"       /hangman stop",
			desc:     "Play Hangman! Host starts a game with a themed or custom secret word. Players guess letters (and optionally the full word). Wrong-guessers are punished on failure.",
			reqPerms: permissions.PermissionField["NONE"],
			category: "minigames",
		},
		// Casino commands
		"bj": {
			handler:   cmdBlackjack,
			minArgs:   0,
			usage:     "Usage: /bj join|bet <amount>|deal|hit|stand|double|split|insurance|status|leave",
			desc:      "Play blackjack. Requires casino to be enabled in the area.",
			reqPerms:  permissions.PermissionField["NONE"],
			casinoCmd: true,
			category:  "casino",
		},
		"poker": {
			handler:   cmdPoker,
			minArgs:   0,
			usage:     "Usage: /poker join|ready|hand|check|call|bet <n>|raise <n>|fold|allin|status|leave",
			desc:      "Play Texas Hold'em poker. Requires casino to be enabled in the area.",
			reqPerms:  permissions.PermissionField["NONE"],
			casinoCmd: true,
			category:  "casino",
		},
		"slots": {
			handler:   cmdSlots,
			minArgs:   0,
			usage:     "Usage: /slots [spin [amount]] | /slots max | /slots jackpot | /slots stats",
			desc:      "Play slot machines. Requires casino to be enabled in the area.",
			reqPerms:  permissions.PermissionField["NONE"],
			casinoCmd: true,
			category:  "casino",
		},
		"croulette": {
			handler:   cmdCasinoRoulette,
			minArgs:   2,
			usage:     "Usage: /croulette bet <red|black|even|odd|low|high|number <n>> <amount>",
			desc:      "Play European roulette. Requires casino to be enabled in the area.",
			reqPerms:  permissions.PermissionField["NONE"],
			casinoCmd: true,
			category:  "casino",
		},
		"baccarat": {
			handler:   cmdBaccarat,
			minArgs:   2,
			usage:     "Usage: /baccarat <player|banker|tie> <amount>",
			desc:      "Play baccarat. Requires casino to be enabled in the area.",
			reqPerms:  permissions.PermissionField["NONE"],
			casinoCmd: true,
			category:  "casino",
		},
		"craps": {
			handler:   cmdCraps,
			minArgs:   3,
			usage:     "Usage: /craps bet <pass|nopass> <amount>",
			desc:      "Play craps. Requires casino to be enabled in the area.",
			reqPerms:  permissions.PermissionField["NONE"],
			casinoCmd: true,
			category:  "casino",
		},
		"crash": {
			handler:   cmdCrash,
			minArgs:   1,
			usage:     "Usage: /crash bet <amount> | /crash cashout",
			desc:      "Play crash. 45s cooldown between rounds; cashout locked for first 5s (instant eject = loss). Requires casino to be enabled.",
			reqPerms:  permissions.PermissionField["NONE"],
			casinoCmd: true,
			category:  "casino",
		},
		"mines": {
			handler:   cmdMines,
			minArgs:   1,
			usage:     "Usage: /mines start <mines> <bet> | /mines pick <n> | /mines cashout | /mines quit",
			desc:      "Play mines. Requires casino to be enabled in the area.",
			reqPerms:  permissions.PermissionField["NONE"],
			casinoCmd: true,
			category:  "casino",
		},
		"keno": {
			handler:   cmdKeno,
			minArgs:   3,
			usage:     "Usage: /keno pick <numbers...> <bet>",
			desc:      "Play keno. Requires casino to be enabled in the area.",
			reqPerms:  permissions.PermissionField["NONE"],
			casinoCmd: true,
			category:  "casino",
		},
		"wheel": {
			handler:   cmdWheel,
			minArgs:   2,
			usage:     "Usage: /wheel spin <bet>",
			desc:      "Spin the prize wheel. Requires casino to be enabled in the area.",
			reqPerms:  permissions.PermissionField["NONE"],
			casinoCmd: true,
			category:  "casino",
		},
		"plinko": {
			handler:   cmdPlinko,
			minArgs:   0,
			usage:     "Usage: /plinko drop <low|med|high> <bet>",
			desc:      "Drop a chip down the Plinko peg board. Risk level controls payout spread (low: 0.3x-2.5x, med: 0.1x-5x, high: 0x-12x). Requires casino to be enabled in the area.",
			reqPerms:  permissions.PermissionField["NONE"],
			casinoCmd: true,
			category:  "casino",
		},
		"bar": {
			handler:   cmdBar,
			minArgs:   0,
			usage:     "Usage: /bar menu | /bar buy <drink>",
			desc:      fmt.Sprintf("Visit the bar! %d drinks each with RISK and wild variance — huge wins or big losses. Use /bar menu to see all drinks.", len(barMenu)),
			reqPerms:  permissions.PermissionField["NONE"],
			casinoCmd: true,
			category:  "casino",
		},
		"rob": {
			handler:   cmdRob,
			minArgs:   0,
			usage:     "Usage: /rob [bank|casino|vault|atm|store|mint|armored|museum]",
			desc:      "Attempt to rob a location for chips. 20% success rate — catastrophic failures drain your chips and may mute you.",
			reqPerms:  permissions.PermissionField["NONE"],
			casinoCmd: true,
			category:  "chips",
		},
		"gamble": {
			handler:   cmdGamble,
			minArgs:   1,
			usage:     "Usage: /gamble hide",
			desc:      "Toggle visibility of gambling broadcast messages in the area chat.",
			reqPerms:  permissions.PermissionField["NONE"],
			casinoCmd: true,
			category:  "casino",
		},
		"chips": {
			handler:   cmdChipsEnhanced,
			minArgs:   0,
			usage:     "Usage: /chips [top [n]] | [area [n]] | [give <uid> <amount>]",
			desc:      "Check your Nyathena Chip balance, view leaderboards, or give chips to another player.",
			reqPerms:  permissions.PermissionField["NONE"],
			casinoCmd: true,
			category:  "chips",
		},
		"richest": {
			handler:   cmdRichest,
			minArgs:   0,
			usage:     "Usage: /richest [n]",
			desc:      "Show the global chip leaderboard (top 10 richest players by default, max 50).",
			reqPerms:  permissions.PermissionField["NONE"],
			casinoCmd: true,
			category:  "chips",
		},
		"casino": {
			handler:   cmdCasino,
			minArgs:   0,
			usage:     "Usage: /casino [status]",
			desc:      "View the casino dashboard or status for the current area.",
			reqPerms:  permissions.PermissionField["NONE"],
			casinoCmd: true,
			category:  "casino",
		},
		"casinoenable": {
			handler:   cmdCasinoEnable,
			minArgs:   1,
			usage:     "Usage: /casinoenable <true|false>",
			desc:      "Enables or disables the casino for this area.",
			reqPerms:  permissions.PermissionField["MODIFY_AREA"],
			casinoCmd: true,
			category:  "casino",
		},
		"casinoset": {
			handler:   cmdCasinoSet,
			minArgs:   2,
			usage:     "Usage: /casinoset <minbet|maxbet|maxtables|jackpot> <value>",
			desc:      "Configures casino settings for this area.",
			reqPerms:  permissions.PermissionField["MODIFY_AREA"],
			casinoCmd: true,
			category:  "casino",
		},
		"grantchips": {
			handler:   cmdGrantChips,
			minArgs:   2,
			usage:     "Usage: /grantchips <uid> <amount>",
			desc:      "Admin: Grant any amount of chips to an online player by UID.",
			reqPerms:  permissions.PermissionField["ADMIN"],
			casinoCmd: true,
			category:  "admin",
		},
		"createtag": {
			handler:  cmdCreateTag,
			minArgs:  2,
			usage:    "Usage: /createtag <id> <display name>",
			desc:     "Mint a new custom cosmetic tag at runtime. Id is 2–32 lowercase letters/digits/underscores; display name is what shows in [brackets] beside the player's name.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "moderation",
		},
		"deletetag": {
			handler:  cmdDeleteTag,
			minArgs:  1,
			usage:    "Usage: /deletetag <id>",
			desc:     "Delete a custom tag and unequip it from anyone wearing it. Built-in shop tags are not affected.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "moderation",
		},
		"grantcustomtag": {
			handler:  cmdGrantCustomTag,
			minArgs:  2,
			usage:    "Usage: /grantcustomtag <username> <tag_id>",
			desc:     "Grant a tag (built-in or custom) to a registered account by username. The recipient can equip it with /settag.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "moderation",
		},
		"revokecustomtag": {
			handler:  cmdRevokeCustomTag,
			minArgs:  2,
			usage:    "Usage: /revokecustomtag <username> <tag_id>",
			desc:     "Revoke a previously granted tag from a registered account.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "moderation",
		},
		"listcustomtags": {
			handler:  cmdListCustomTags,
			minArgs:  0,
			usage:    "Usage: /listcustomtags",
			desc:     "List all staff-defined custom tags.",
			reqPerms: permissions.PermissionField["NONE"],
			category: "general",
		},
		"newspaper": {
			handler:  cmdNewspaper,
			minArgs:  0,
			usage:    "Usage: /newspaper | /newspaper now",
			desc:     "Show newspaper config, or use 'now' to publish an issue immediately. Requires ADMIN.",
			reqPerms: permissions.PermissionField["ADMIN"],
			category: "admin",
		},
		"register": {
			handler:    cmdRegister,
			minArgs:    2,
			usage:      "Usage: /register <username> <password>",
			desc:       "Start creating a free player account (a captcha confirmation is required). Tracks playtime, wardrobe favourites, active tag, and (if the casino is enabled) chip balance.",
			reqPerms:   permissions.PermissionField["NONE"],
			accountCmd: true,
			category:   "account",
		},
		"resetusername": {
			handler:    cmdResetUsername,
			minArgs:    1,
			usage:      "Usage: /resetusername <new-username>",
			desc:       "Rename your logged-in account without losing playtime, chips, wardrobe, or tags. Limited to 3 renames per account.",
			reqPerms:   permissions.PermissionField["NONE"],
			accountCmd: true,
			category:   "account",
		},
		"verify": {
			handler:  cmdVerify,
			minArgs:  1,
			usage:    "Usage: /verify <answer>",
			desc:     "Answer the verification question you were given when you joined, so you can chat.",
			reqPerms: permissions.PermissionField["NONE"],
			category: "general",
		},
		"joincaptcha": {
			handler:  cmdJoinCaptcha,
			minArgs:  1,
			usage:    "Usage: /joincaptcha status | verify <uid> | reset <uid|ipid|all>",
			desc:     "Inspect the join captcha, release a player it caught, or clear stored verifications.",
			reqPerms: permissions.PermissionField["BAN"],
			category: "moderation",
		},
		"raidguard": {
			handler:  cmdRaidGuard,
			minArgs:  1,
			usage:    "Usage: /raidguard status | clear <uid|all> | test <text> | alert <on|off>",
			desc:     "Inspect the raid guard, clear a false positive's score, test how it reads text, or mute its alerts for yourself.",
			reqPerms: permissions.PermissionField["MOD_CHAT"],
			category: "moderation",
		},
		"captcha": {
			handler:    cmdCaptcha,
			minArgs:    1,
			usage:      "Usage: /captcha <token>",
			desc:       "Complete a pending /register by entering the captcha token you were given.",
			reqPerms:   permissions.PermissionField["NONE"],
			accountCmd: true,
			category:   "account",
		},
		"account": {
			handler:    cmdAccount,
			minArgs:    0,
			usage:      "Usage: /account",
			desc:       "View your account profile: username, playtime, and (if the casino is enabled) chip balance.",
			reqPerms:   permissions.PermissionField["NONE"],
			accountCmd: true,
			category:   "account",
		},
		"playtime": {
			handler:  cmdPlaytimeTop,
			minArgs:  0,
			usage:    "Usage: /playtime [top] [page]\n       /playtime add <username> <duration>  (admin)\n       /playtime remove <username> <duration>  (admin)\n       /playtime set <username> <duration>  (admin — e.g. 1000h, 3d12h30m, 90m)",
			desc:     "Show the playtime leaderboard. 25 entries per page; pass a page number to scroll. Includes player and moderator accounts. Admins can also use /playtime add|remove|set <username> <duration> to grant, deduct, or overwrite playtime.",
			reqPerms: permissions.PermissionField["NONE"],
			category: "general",
		},
		"reloadplaytime": {
			handler:  cmdReloadPlaytime,
			minArgs:  0,
			usage:    "Usage: /reloadplaytime",
			desc:     "Admin: re-link every registered account to its IPID and merge any orphaned playtime. Fixes leaderboard entries for accounts created on long-running anonymous IPIDs.",
			reqPerms: permissions.PermissionField["ADMIN"],
			category: "admin",
		},
		"unscramble": {
			handler:   cmdUnscramble,
			minArgs:   0,
			usage:     "Usage: /unscramble [top [n]]",
			desc:      "Check your unscramble wins or view the unscramble leaderboard. Answer active puzzles in IC chat to win chips!",
			reqPerms:  permissions.PermissionField["NONE"],
			casinoCmd: true,
			category:  "chips",
		},
		"jobs": {
			handler:   cmdJobs,
			minArgs:   0,
			usage:     "Usage: /jobs",
			desc:      "List all available jobs that earn small chip rewards.",
			reqPerms:  permissions.PermissionField["NONE"],
			casinoCmd: true,
			category:  "jobs",
		},
		"jobtop": {
			handler:   cmdJobTop,
			minArgs:   0,
			usage:     "Usage: /jobtop [n]",
			desc:      "Show the job earnings leaderboard (top chip earners from jobs).",
			reqPerms:  permissions.PermissionField["NONE"],
			casinoCmd: true,
			category:  "chips",
		},
		"janitor": {
			handler:   cmdJanitor,
			minArgs:   0,
			usage:     "Usage: /janitor",
			desc:      "Work as a janitor to earn chips (45-minute cooldown).",
			reqPerms:  permissions.PermissionField["NONE"],
			casinoCmd: true,
			category:  "jobs",
		},
		"busker": {
			handler:   cmdBusker,
			minArgs:   0,
			usage:     "Usage: /busker",
			desc:      "Busk for tips outside the courthouse to earn chips (30-minute cooldown).",
			reqPerms:  permissions.PermissionField["NONE"],
			casinoCmd: true,
			category:  "jobs",
		},
		"paperboy": {
			handler:   cmdPaperboy,
			minArgs:   0,
			usage:     "Usage: /paperboy",
			desc:      "Deliver newspapers and briefs to earn chips (60-minute cooldown).",
			reqPerms:  permissions.PermissionField["NONE"],
			casinoCmd: true,
			category:  "jobs",
		},
		"bailiffjob": {
			handler:   cmdBailiffJob,
			minArgs:   0,
			usage:     "Usage: /bailiffjob",
			desc:      "Stand guard duty as a bailiff to earn chips (2-hour cooldown).",
			reqPerms:  permissions.PermissionField["NONE"],
			casinoCmd: true,
			category:  "jobs",
		},
		"clerk": {
			handler:   cmdClerk,
			minArgs:   0,
			usage:     "Usage: /clerk",
			desc:      "File paperwork as a clerk to earn chips (90-minute cooldown).",
			reqPerms:  permissions.PermissionField["NONE"],
			casinoCmd: true,
			category:  "jobs",
		},
		"shop": {
			handler:    cmdShop,
			minArgs:    0,
			usage:      "Usage: /shop | /shop <category> | /shop buy <item_id> | /shop items | /shop passes | /shop passive",
			desc:       "Browse the Nyathena Shop: 115+ cosmetic tags, job passes, and passive income upgrades. When the casino is disabled, tags are free to equip via /settag. Categories: gambling attorney anime gamer girly meme prestige.",
			reqPerms:   permissions.PermissionField["NONE"],
			accountCmd: true,
			category:   "chips",
		},
		"settag": {
			handler:    cmdSetTag,
			minArgs:    1,
			usage:      "Usage: /settag <tag_id> | /settag none",
			desc:       "Equip or swap a cosmetic tag. Your active tag appears next to your name in /gas and /players. When the casino is disabled, any tag id is free to equip.",
			reqPerms:   permissions.PermissionField["NONE"],
			accountCmd: true,
			category:   "chips",
		},
		"mytags": {
			handler:    cmdMyTags,
			minArgs:    0,
			usage:      "Usage: /mytags",
			desc:       "Lists every tag you own (built-in and custom) with its id and name, and marks your active one. Use the id with /settag.",
			reqPerms:   permissions.PermissionField["NONE"],
			accountCmd: true,
			category:   "chips",
		},
		"favourite": {
			handler:    cmdFavourite,
			minArgs:    1,
			usage:      "Usage: /favourite <char name>",
			desc:       "Toggle a character in your wardrobe favourites. Add or remove with the same command.",
			reqPerms:   permissions.PermissionField["NONE"],
			accountCmd: true,
			category:   "account",
		},
		"wardrobe": {
			handler:    cmdWardrobe,
			minArgs:    0,
			usage:      "Usage: /wardrobe | /wardrobe <char name or number>",
			desc:       "View your saved favourite characters, or swap to one by name or slot number (e.g. /wardrobe 2).",
			reqPerms:   permissions.PermissionField["NONE"],
			accountCmd: true,
			category:   "account",
		},
		// ── Mafia / Werewolf social deduction minigame ──────────────────────
		"mafia": {
			handler:  cmdMafia,
			minArgs:  0,
			usage:    mafiaUsage,
			desc:     "Social deduction minigame (Mafia/Werewolf). Type /mafia help for subcommands.",
			reqPerms: permissions.PermissionField["NONE"],
			category: "mafia",
		},
		"werewolf": {
			handler:  cmdMafia,
			minArgs:  0,
			usage:    mafiaUsage,
			desc:     "Alias for /mafia — social deduction minigame.",
			reqPerms: permissions.PermissionField["NONE"],
			category: "mafia",
		},
		// ── Lotto scratch card ───────────────────────────────────────────────
		"lotto": {
			handler:   cmdLotto,
			minArgs:   2,
			usage:     "Usage: /lotto buy <ticket_cost>",
			desc:      "Buy an instant scratch-card lottery ticket. Three matching symbols = big win!",
			reqPerms:  permissions.PermissionField["NONE"],
			casinoCmd: true,
			category:  "casino",
		},
		// ── Novelty punishments (additions) ──────────────────────────────────
		"timewarp": {
			handler:  cmdTimewarp,
			minArgs:  1,
			usage:    "Usage: /timewarp [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Shuffles the word order of the target's IC messages.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"morse": {
			handler:  cmdMorse,
			minArgs:  1,
			usage:    "Usage: /morse [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Converts the target's IC messages to Morse code dots and dashes.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"rickroll": {
			handler:  cmdRickroll,
			minArgs:  1,
			usage:    "Usage: /rickroll [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Replaces the target's IC messages with meme-styled lyric-adjacent stand-in lines.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"pickup": {
			handler:  cmdPickup,
			minArgs:  1,
			usage:    "Usage: /pickup [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Replaces every IC message the target sends with a catastrophically cheesy pickup line.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"brainrot": {
			handler:  cmdBrainrot,
			minArgs:  1,
			usage:    "Usage: /brainrot [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Corrupts the target's IC messages with maximum skibidi sigma Italian brainrot energy.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"vowelhell": {
			handler:  cmdVowelhell,
			minArgs:  1,
			usage:    "Usage: /vowelhell [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Replaces every consonant in the target's messages with a random vowel.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"chef": {
			handler:  cmdChef,
			minArgs:  1,
			usage:    "Usage: /chef [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Swedish-Chef filter — bork bork bork!",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"karen": {
			handler:  cmdKaren,
			minArgs:  1,
			usage:    "Usage: /karen [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Wraps each message in escalating entitled complaints and manager demands.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"passiveaggressive": {
			handler:  cmdPassiveAggressive,
			minArgs:  1,
			usage:    "Usage: /passiveaggressive [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Adds chilly, performatively-polite framings and sign-offs. It's fine. Really.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"nervous": {
			handler:  cmdNervous,
			minArgs:  1,
			usage:    "Usage: /nervous [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Sprinkles stuttering, um/uh fillers, and jittery trailing apologies.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"dreamsequence": {
			handler:  cmdDreamSequence,
			minArgs:  1,
			usage:    "Usage: /dreamsequence [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Rewrites IC messages as surreal, dreamlike fragments.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"gordonramsay": {
			handler:  cmdGordonRamsay,
			minArgs:  1,
			usage:    "Usage: /gordonramsay [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Replaces every IC line the target sends with a Gordon Ramsay kitchen tirade.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"cherri": {
			handler:  cmdCherri,
			minArgs:  1,
			usage:    "Usage: /cherri [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Forces The Target To Capitalize Every Word Like Cherri Does.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"albhed": {
			handler:  cmdAlbhed,
			minArgs:  1,
			usage:    "Usage: /albhed [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Transliterates the target's IC messages through the Al Bhed cipher (FFX).",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"clown": {
			handler:  cmdClown,
			minArgs:  1,
			usage:    "Usage: /clown [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Wraps every IC message in clown honks and circus filler. 🤡",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"jester": {
			handler:  cmdJester,
			minArgs:  1,
			usage:    "Usage: /jester [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Wraps messages in theatrical jester flourishes and bell-jingle SFX text.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"joker": {
			handler:  cmdJoker,
			minArgs:  1,
			usage:    "Usage: /joker [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Peppers chaotic laughter throughout every IC message. HAHAHA!",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"mime": {
			handler:  cmdMime,
			minArgs:  1,
			usage:    "Usage: /mime [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Replaces every IC line with a silent mime action — *gestures wordlessly*.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"biblebot": {
			handler:  cmdBiblebot,
			minArgs:  1,
			usage:    "Usage: /biblebot [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Replaces every IC line with a random Bible verse.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"grounded": {
			handler:  cmdGrounded,
			minArgs:  1,
			usage:    "Usage: /grounded [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Replaces every IC line with a GoAnimate-style 'YOU ARE GROUNDED' tirade.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"smugdere": {
			handler:  cmdSmugdere,
			minArgs:  1,
			usage:    "Usage: /smugdere [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Smug, condescending dere — looks down on everyone, obviously.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"deretsun": {
			handler:  cmdDeretsun,
			minArgs:  1,
			usage:    "Usage: /deretsun [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Tsun-leaning dere flavour. The most popular pick.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"bokodere": {
			handler:  cmdBokodere,
			minArgs:  1,
			usage:    "Usage: /bokodere [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Violently affectionate dere — threats interleaved with care.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"thugdere": {
			handler:  cmdThugdere,
			minArgs:  1,
			usage:    "Usage: /thugdere [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Streetwise dere — every message in fam/yo/no-cap idiom.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"teasedere": {
			handler:  cmdTeasedere,
			minArgs:  1,
			usage:    "Usage: /teasedere [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Flirty teasing dere — playful pokes and winks.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"dorodere": {
			handler:  cmdDorodere,
			minArgs:  1,
			usage:    "Usage: /dorodere [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Dirt-stained, twisted dere — smiles through grime.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"hinedere": {
			handler:  cmdHinedere,
			minArgs:  1,
			usage:    "Usage: /hinedere [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Sarcastic, aloof dere — perpetually exasperated.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"hajidere": {
			handler:  cmdHajidere,
			minArgs:  1,
			usage:    "Usage: /hajidere [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Mortified, blushing dere — perpetually embarrassed.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"rindere": {
			handler:  cmdRindere,
			minArgs:  1,
			usage:    "Usage: /rindere [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Cold, distant dere — barely speaks to you.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"utsudere": {
			handler:  cmdUtsudere,
			minArgs:  1,
			usage:    "Usage: /utsudere [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Depressive dere — everything is pointless.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"darudere": {
			handler:  cmdDarudere,
			minArgs:  1,
			usage:    "Usage: /darudere [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Lazy, perpetually-tired dere — yawns through every line.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"butsudere": {
			handler:  cmdButsudere,
			minArgs:  1,
			usage:    "Usage: /butsudere [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Apathetic, deadpan dere — couldn't care less.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"sdere": {
			handler:  cmdSDere,
			minArgs:  1,
			usage:    "Usage: /sdere [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Dominant dere archetype — gives orders.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"mdere": {
			handler:  cmdMDere,
			minArgs:  1,
			usage:    "Usage: /mdere [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Submissive dere archetype — apologetic and meek.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"tsuyodere": {
			handler:  cmdTsuyodere,
			minArgs:  1,
			usage:    "Usage: /tsuyodere [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Strength-obsessed dere — flexes in every sentence.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"omnidere": {
			handler:  cmdOmnidere,
			minArgs:  1,
			usage:    "Usage: /omnidere [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "All deres at once — every IC message picks a random dere flavour for maximum tonal whiplash.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"megamaso": {
			handler:  cmdMegamaso,
			minArgs:  0,
			usage:    "Usage: /megamaso [-d duration]\nOptional -d sets how long each stacked effect lasts (default 10m, max 24h). Examples: /megamaso -d 1h, /megamaso -d 30m",
			desc:     "Self-applied chaos: rolls a random punishment (default 10 min, max 24 h via -d). Each repeat ADDS another random punishment to the stack instead of replacing it.",
			reqPerms: permissions.PermissionField["NONE"],
			category: "punishment",
		},
		"sfxcurse": {
			handler:  cmdSfxCurse,
			minArgs:  2,
			usage:    "Usage: /sfxcurse [-d duration] [-r reason] [-h] global | <uid1>,<uid2>... <sfx-url>",
			desc:     "Forces the target to emit the specified SFX on every IC message. Accepts any streamable http(s) URL (Discord CDN, custom hosting, etc.) or a /base/sounds/ path.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"unsfx": {
			handler:  cmdUnSfx,
			minArgs:  1,
			usage:    "Usage: /unsfx <uid1>,<uid2>...",
			desc:     "Removes the SFX curse from one or more clients.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"shrink": {
			handler:  cmdShrink,
			minArgs:  1,
			usage:    "Usage: /shrink [-d duration] [-r reason] [-h] global | <uid1>,<uid2>... [offset]",
			desc:     "Locks the target's vertical sprite offset into a negative value (default -25). Persists per IC message until /unshrink.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"grow": {
			handler:  cmdGrow,
			minArgs:  1,
			usage:    "Usage: /grow [-d duration] [-r reason] [-h] global | <uid1>,<uid2>... [offset]",
			desc:     "Locks the target's vertical sprite offset into a positive value (default +25). Persists per IC message until /ungrow.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"wide": {
			handler:  cmdWide,
			minArgs:  1,
			usage:    "Usage: /wide [-d duration] [-r reason] [-h] global | <uid1>,<uid2>... [offset]",
			desc:     "Locks the target's horizontal sprite offset (default +50). Persists per IC message until /unwide.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"unshrink": {
			handler:  cmdUnshrink,
			minArgs:  1,
			usage:    "Usage: /unshrink <uid1>,<uid2>...",
			desc:     "Removes the /shrink offset lock.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"ungrow": {
			handler:  cmdUngrow,
			minArgs:  1,
			usage:    "Usage: /ungrow <uid1>,<uid2>...",
			desc:     "Removes the /grow offset lock.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"unwide": {
			handler:  cmdUnwide,
			minArgs:  1,
			usage:    "Usage: /unwide <uid1>,<uid2>...",
			desc:     "Removes the /wide offset lock.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"fromsoftware": {
			handler:  cmdFromSoftware,
			minArgs:  1,
			usage:    "Usage: /fromsoftware [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Censors words from fromsoft.txt in the target's IC messages, replacing each occurrence (including inside larger words) with asterisks (one per letter).",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"profile": {
			handler:  cmdProfile,
			minArgs:  0,
			usage:    "Usage: /profile [uid]",
			desc:     "Shows a profile card (playtime, chips, favourites, active tag) for yourself or another online player.",
			reqPerms: permissions.PermissionField["NONE"],
			category: "general",
		},
		"potions": {
			handler:  cmdPotions,
			minArgs:  0,
			usage:    "Usage: /potions",
			desc:     "Lists every potion you can drink. Effects default to 5 minutes; use /potion -d <duration> to customise.",
			reqPerms: permissions.PermissionField["NONE"],
			category: "general",
		},
		"potion": {
			handler:  cmdPotion,
			minArgs:  0,
			usage:    "Usage: /potion [-d duration] <name|off>\nOptional -d sets effect duration (default 5m, max 24h). Examples: /potion -d 30m drunk, /potion -d 1h pirate\nUse /potions to see the menu. /potion off flushes every active potion.",
			desc:     "Drink a named potion (default 5-minute self-applied effect, customisable via -d up to 24 h) or use /potion off to remove every active potion at once.",
			reqPerms: permissions.PermissionField["NONE"],
			category: "general",
		},
		// ── Wave-2 punishments: text transforms ──────────────────────────
		"zalgo": {
			handler:  cmdZalgo,
			minArgs:  1,
			usage:    "Usage: /zalgo [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "C̴o̷r̶r̸u̵p̷t̶s̸ the target's text with creeping zalgo combining marks.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"leetspeak": {
			handler:  cmdLeetspeak,
			minArgs:  1,
			usage:    "Usage: /leetspeak [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "F0rc35 7h3 74r937 70 5p34k 1n h4x0r 1337speak.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"smallcaps": {
			handler:  cmdSmallcaps,
			minArgs:  1,
			usage:    "Usage: /smallcaps [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "ʀᴇɴᴅᴇʀs ᴇᴠᴇʀʏᴛʜɪɴɢ ᴛʜᴇ ᴛᴀʀɢᴇᴛ sᴀʏs ɪɴ ᴛɪɴʏ ᴄᴀᴘs.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"piglatin": {
			handler:  cmdPiglatin,
			minArgs:  1,
			usage:    "Usage: /piglatin [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Orcesfay ethay argettay otay eakspay inyay Igpay Atinlay.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"vaporwave": {
			handler:  cmdVaporwave,
			minArgs:  1,
			usage:    "Usage: /vaporwave [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Ｒｅｎｄｅｒｓ ｔｅｘｔ ｉｎ ｆｕｌｌｗｉｄｔｈ ａｅｓｔｈｅｔｉｃ (distinct from /fancy).",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"lisp": {
			handler:  cmdLisp,
			minArgs:  1,
			usage:    "Usage: /lisp [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Replatheth every 's' and 'z' with 'th'. Tho thorry.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"spoonerism": {
			handler:  cmdSpoonerism,
			minArgs:  1,
			usage:    "Usage: /spoonerism [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Swaps the starting sounds of adjacent words — shake a tower, anyone?",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"keysmash": {
			handler:  cmdKeysmash,
			minArgs:  1,
			usage:    "Usage: /keysmash [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Injects random asdfjkl; keyboard bursts into the target's messages.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"weeb": {
			handler:  cmdWeeb,
			minArgs:  1,
			usage:    "Usage: /weeb [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Sprinkles 350+ anime romaji over everything — word swaps, honorifics, 'Nani?!' and desu~. Sugoi!",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"politician": {
			handler:  cmdPolitician,
			minArgs:  1,
			usage:    "Usage: /politician [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Great question. The target never answers anything directly again.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"clickbait": {
			handler:  cmdClickbait,
			minArgs:  1,
			usage:    "Usage: /clickbait [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Every message becomes a clickbait headline about the speaker (Number 3 Will SHOCK You).",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"markov": {
			handler:  cmdMarkov,
			minArgs:  1,
			usage:    "Usage: /markov [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Replaces messages with markov-chain babble brewed from the area's own recent chat history.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"alliteration": {
			handler:  cmdAlliteration,
			minArgs:  1,
			usage:    "Usage: /alliteration [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Boldly bends both big and bitty words into bizarrely bombastic alliteration.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"cipher": {
			handler:  cmdCipher,
			minArgs:  1,
			usage:    "Usage: /cipher [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Escalating encryption layers per message: ROT13 → BINARY → BASE64, then the decryption fails and it re-arms.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"medieval": {
			handler:  cmdMedieval,
			minArgs:  1,
			usage:    "Usage: /medieval [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Hark! Rewrites the target's text into Olde-English speak — thee/thou swaps, herald's cries and courtly flourishes (100+ combinations).",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"cheese": {
			handler:  cmdCheese,
			minArgs:  1,
			usage:    "Usage: /cheese [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Replaces every message with one of 100+ statements about cheese. Cheese is, technically, a sauce.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		// ── Wave-2 punishments: protocol / viewport effects ──────────────
		"teleport": {
			handler:  cmdTeleport,
			minArgs:  1,
			usage:    "Usage: /teleport [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "The target's sprite pops to a random screen position on every message.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"shakecurse": {
			handler:  cmdShakecurse,
			minArgs:  1,
			usage:    "Usage: /shakecurse [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Forces the screenshake flag on every one of the target's messages.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"randomflip": {
			handler:  cmdRandomflip,
			minArgs:  1,
			usage:    "Usage: /randomflip [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Coin-flips the target's sprite facing on every message — they can't control which way they look.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"forcecolor": {
			handler:  cmdForceColor,
			minArgs:  2,
			usage:    "Usage: /forcecolor [-d duration] [-r reason] [-h] <global | uid1,uid2...> <0-9 | white|green|red|orange|blue|yellow|rainbow>",
			desc:     "Locks the target's IC text colour — all-red rage, ghostly white, or full rainbow.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"nopreanim": {
			handler:  cmdNoPreanim,
			minArgs:  1,
			usage:    "Usage: /nopreanim [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Strips preanimations from the target's messages — no more dramatic desk slams.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"forcepreanim": {
			handler:  cmdForcePreanim,
			minArgs:  1,
			usage:    "Usage: /forcepreanim [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Forces the target's preanimation to play on every message that names one.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		// ── Wave-2 punishments: delivery, traps & contagion ──────────────
		"lifo": {
			handler:  cmdLifo,
			minArgs:  1,
			usage:    "Usage: /lifo [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Buffers the target's IC messages and releases them in REVERSE order — say three things, they arrive backwards.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"contagious": {
			handler:  cmdContagious,
			minArgs:  2,
			usage:    "Usage: /contagious <type> [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Plague mode: the punishment spreads to anyone who speaks within 5s of an infected player's message. Mods are immune.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"minefield": {
			handler:  cmdMinefield,
			minArgs:  1,
			usage:    "Usage: /minefield [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Every message the target sends has a 1-in-6 chance to detonate a random 2-minute punishment on them.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"randomarea": {
			handler:  cmdRandomArea,
			minArgs:  1,
			usage:    "Usage: /randomarea [-d duration] [-r reason] [-h] global | <uid1>,<uid2>...",
			desc:     "Every 20-45 seconds, force-warps the target to a random open area, on its own — no message needed.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"silencebell": {
			handler:  cmdSilencebell,
			minArgs:  0,
			usage:    "Usage: /silencebell [type] [-d duration]\n       /silencebell off | status\nArms a one-shot trap on your area: the next non-moderator to speak gets cursed (random effect unless a type is given).",
			desc:     "Arms a trap on your area — the next person to speak gets auto-punished, with a dramatic announcement.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		"stealthmute": {
			handler:  cmdStealthMute,
			minArgs:  1,
			usage:    "Usage: /stealthmute [-d duration] [-r reason] global | <uid1>,<uid2>...\nThe target is never notified. Lift with /unpunish -t stealthmute <uid>.",
			desc:     "The target's IC/OOC messages echo back to them but reach nobody else — they never know they're muted.",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "punishment",
		},
		// ── Wave-2 QoL ────────────────────────────────────────────────────
		"punishments": {
			handler:  cmdPunishments,
			minArgs:  0,
			usage:    "Usage: /punishments [uid]\nNo argument: list your own active punishments. With a UID (moderators only): inspect another player.",
			desc:     "Lists active punishments with remaining durations — includes lag, mute and jail. Mods can inspect any player.",
			reqPerms: permissions.PermissionField["NONE"],
			category: "punishment",
		},
		"clients": {
			handler:  cmdClients,
			minArgs:  1,
			usage:    "Usage: /clients <uid>",
			desc:     "Lists every connection sharing the target's IPID (multiclient overview).",
			reqPerms: permissions.PermissionField["MUTE"],
			category: "moderation",
		},
		"lfp": {
			handler:  cmdLfp,
			minArgs:  0,
			usage:    "Usage: /lfp",
			desc:     "Toggles your Looking-For-Pair flag so /pairlist shows you to your area.",
			reqPerms: permissions.PermissionField["NONE"],
			category: "general",
		},
		"pairlist": {
			handler:  cmdPairlist,
			minArgs:  0,
			usage:    "Usage: /pairlist",
			desc:     "Lists everyone in your area flagged Looking-For-Pair via /lfp.",
			reqPerms: permissions.PermissionField["NONE"],
			category: "general",
		},
		"dctime": {
			handler:  cmdDC,
			minArgs:  0,
			usage:    "Usage: /dctime [<minutes> | off | status]   (no number = 1-hour timer)",
			desc:     "Opt-in idle auto-disconnect: kicks ONLY you after the chosen minutes of inactivity (default 1h). IC/OOC resets it. /dctime off to cancel.",
			reqPerms: permissions.PermissionField["NONE"],
			category: "general",
		},
		"dc": {
			handler:  cmdDC,
			minArgs:  0,
			usage:    "Usage: /dc [<minutes> | off | status]   (alias of /dctime; no number = 1-hour timer)",
			desc:     "Alias of /dctime — set your personal idle auto-disconnect timer (affects only you).",
			reqPerms: permissions.PermissionField["NONE"],
			category: "general",
		},
		"toggle": {
			handler:  cmdToggle,
			minArgs:  1,
			usage:    "Usage: /toggle global",
			desc:     "Opt-in OOC-hide toggle: /toggle global stops area OOC chat and /global broadcasts from reaching YOU (run it again to bring them back). Direct messages and staff/system announcements still get through, and it never affects anyone else.",
			reqPerms: permissions.PermissionField["NONE"],
			category: "general",
		},
	}
}

// helpCategory describes a top-level /help section visible to players.
type helpCategory struct {
	name  string
	emoji string
	title string
	desc  string
}

// helpCategoryList defines display order and descriptions for /help categories.
var helpCategoryList = []helpCategory{
	{"general", "🎮", "General", "Movement, chat, dice, info commands."},
	{"area", "🏛️", "Area", "Area management — backgrounds, music, locks, polls."},
	{"testimony", "📝", "Testimony", "Testimony recorder — record and replay witness statements."},
	{"minigames", "🎲", "Mini-games", "Hangman, Hot Potato, Quick Draw, Russian Roulette, Typing Race, giveaways."},
	{"mafia", "🕵️", "Mafia / Werewolf", "Social-deduction minigame with roles, votes and night actions."},
	{"casino", "🎰", "Casino", "Blackjack, Poker, Slots, Roulette, and more."},
	{"chips", "💰", "Chips", "Chip balance, leaderboards, shop, and the bar."},
	{"jobs", "💼", "Jobs", "Earn chips without gambling — cooldown-based jobs."},
	{"account", "👤", "Account", "Register, login, wardrobe, and profile."},
	{"moderation", "🔨", "Moderation", "Mute, kick, ban, jail, and other staff tools."},
	{"voice", "🎙️", "Voice Chat", "Voice-chat moderation and per-area controls.  Requires a WebAO client with WebRTC support."},
	{"punishment", "🎭", "Punishments", "Text-effect and behaviour punishments for players."},
	{"admin", "⚙️", "Admin", "Server configuration, user management, runtime tweaks."},
}

// clientCanUseCommand reports whether the client has permission to use the
// command named name, factoring in the special CM check and any console-
// issued per-account command grant (see command_grants.go) for name.
//
// The grant check is the one place in the entire command surface that has to
// know about every registered command, and it gets that for free: name is
// exactly the Commands map key ParseCommand dispatched on, so a grant of any
// command in the registry is checked here regardless of what permission tier
// that command normally requires -- this is the single chokepoint every
// slash command passes through, both for actually running the command
// (ParseCommand) and for whether /help advertises it.
func clientCanUseCommand(client *Client, name string, cmd Command) bool {
	return permissions.HasPermission(client.Perms(), cmd.reqPerms) ||
		(cmd.reqPerms == permissions.PermissionField["CM"] && client.Area().HasCM(client.Uid())) ||
		(cmd.reqPerms == permissions.PermissionField["DJ"] && client.HasCMPermission()) ||
		clientHasCommandGrant(client, name)
}

// ParseCommand calls the appropriate function for a given command.
func ParseCommand(client *Client, command string, args []string) {
	casinoEnabled := config != nil && config.EnableCasino
	// Account commands are available when either the casino (which uses accounts)
	// or the standalone account system is enabled.
	accountsEnabled := config != nil && (config.EnableCasino || config.EnableAccounts)
	voiceEnabledNow := config != nil && config.EnableVoice

	if command == "help" {
		// /help <category|command> — drill down into a category or show command usage
		if len(args) == 1 {
			cmdName := strings.ToLower(args[0])

			// Check if it's a known category first
			for _, cat := range helpCategoryList {
				if cmdName == cat.name {
					// The "punishment" category has dozens of entries — flat
					// alphabetic listing was unreadable. Render it grouped by
					// sub-theme (text effects, dere, animal, themed quote,
					// timing/visibility, etc.) so players and mods can scan it.
					if cat.name == "punishment" {
						client.SendServerMessage(renderPunishmentHelp(client, casinoEnabled, accountsEnabled, voiceEnabledNow))
						return
					}
					var lines []string
					for name, cmd := range Commands {
						if cmd.category != cat.name {
							continue
						}
						if cmd.casinoCmd && !casinoEnabled {
							continue
						}
						if cmd.accountCmd && !accountsEnabled {
							continue
						}
						if cmd.voiceCmd && !voiceEnabledNow {
							continue
						}
						if clientCanUseCommand(client, name, cmd) || cmd.publicHelp {
							lines = append(lines, fmt.Sprintf("  /%v — %v", name, cmd.desc))
						}
					}
					if len(lines) == 0 {
						client.SendServerMessage(fmt.Sprintf("No accessible commands in the '%v' category.", cat.name))
						return
					}
					sort.Strings(lines)
					header := fmt.Sprintf("%v %v Commands\n%v\n\n", cat.emoji, cat.title, cat.desc)
					client.SendServerMessage(header + strings.Join(lines, "\n") + "\n\nFor detailed usage on any command: /<command> -h")
					return
				}
			}

			// Not a category — try to look up as a specific command
			cmd, exists := Commands[cmdName]
			if exists && !(cmd.casinoCmd && !casinoEnabled) && !(cmd.accountCmd && !accountsEnabled) && !(cmd.voiceCmd && !voiceEnabledNow) {
				if clientCanUseCommand(client, cmdName, cmd) || cmd.publicHelp {
					client.SendServerMessage(cmd.usage)
				} else {
					client.SendServerMessage("You do not have permission to use that command.")
				}
				return
			}

			client.SendServerMessage(fmt.Sprintf("Unknown category or command '%v'.\nType /help to see all available categories.", args[0]))
			return
		}

		// /help (no args) — show category overview

		// Build a context-aware header.
		var header string
		if client.Authenticated() {
			header = fmt.Sprintf("Logged in as: %v\n\n", client.ModName())
			if accountsEnabled {
				header += "👗 Your Wardrobe: /favourite <char> to save | /wardrobe to view | /wardrobe <char or #> to swap\n\n"
			}
		} else if accountsEnabled {
			header = "💡 New here? /register <username> <password> — free account, no extra permissions.\n" +
				"   Already have one? /login <username> <password>\n\n"
		}

		// Compute max label width for aligned formatting.
		maxLabelLen := 0
		for _, cat := range helpCategoryList {
			if l := len("/help " + cat.name); l > maxLabelLen {
				maxLabelLen = l
			}
		}
		labelFmt := fmt.Sprintf("  %%v %%-%dv — %%v", maxLabelLen)

		// Build the category list, showing only categories with at least one accessible command.
		var catLines []string
		for _, cat := range helpCategoryList {
			hasAny := false
			for name, cmd := range Commands {
				if cmd.category != cat.name {
					continue
				}
				if cmd.casinoCmd && !casinoEnabled {
					continue
				}
				if cmd.accountCmd && !accountsEnabled {
					continue
				}
				if cmd.voiceCmd && !voiceEnabledNow {
					continue
				}
				if clientCanUseCommand(client, name, cmd) || cmd.publicHelp {
					hasAny = true
					break
				}
			}
			if hasAny {
				catLines = append(catLines, fmt.Sprintf(labelFmt, cat.emoji, "/help "+cat.name, cat.desc))
			}
		}

		client.SendServerMessage(header + "📖 Help Categories — type /help <category> to explore:\n\n" +
			strings.Join(catLines, "\n") +
			"\n\nFor usage on any specific command: /<command> -h")
		return
	}

	cmd := Commands[command]
	if cmd.handler == nil {
		client.SendServerMessage("Invalid command.")
		return
	}
	// Block casino/account commands when the feature is disabled server-wide.
	if cmd.casinoCmd && !casinoEnabled {
		client.SendServerMessage("The casino and player account system is not enabled on this server.")
		return
	}
	if cmd.accountCmd && !accountsEnabled {
		client.SendServerMessage("The player account system is not enabled on this server.")
		return
	}
	if cmd.voiceCmd && !voiceEnabledNow {
		client.SendServerMessage("Voice chat is not enabled on this server.  Set enable_voice = true in [Voice] to use voice commands.")
		return
	}
	if clientCanUseCommand(client, command, cmd) {
		// Show usage when the user passes -h, UNLESS the command's own
		// usage string documents [-h] as a supported flag (punishment
		// commands use -h for "hidden" — suppress the per-target
		// notification).  In that case, let the handler receive -h.
		if sliceutil.ContainsString(args, "-h") && !strings.Contains(cmd.usage, "[-h]") {
			client.SendServerMessage(cmd.usage)
			return
		} else if len(args) < cmd.minArgs {
			client.SendServerMessage("Not enough arguments.\n" + cmd.usage)
			return
		}
		cmd.handler(client, args, cmd.usage)
	} else {
		client.SendServerMessage("You do not have permission to use that command.")
		return
	}
}

// Handles /about

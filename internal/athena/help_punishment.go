/* Athena - A server for Attorney Online 2 written in Go
   Nyathena fork additions: subcategorized /help punishment renderer.

   The flat alphabetic listing of the punishment category was unreadable
   once the registry passed ~50 entries. This file groups punishment
   commands into sub-themes (text effects, dere archetypes, animal
   filters, themed quotes, etc.) so /help punishment fits on a screen
   and mods can find what they want at a glance. */

package athena

import (
	"fmt"
	"sort"
	"strings"
)

// punishmentSubcategory groups several command names under a heading.
// Order matters: the first sub-block listed renders first.
type punishmentSubcategory struct {
	emoji string
	title string
	desc  string
	cmds  []string // command names matching keys in Commands
}

// punishmentHelpGroups is the canonical sub-grouping for /help punishment.
// Any command in the "punishment" registry category that is not named here
// falls into the "Other / extras" bucket so nothing goes missing.
//
// Global flag: every punishment command that accepts UIDs also accepts the
// keyword "global" in place of a UID list to apply the effect to every
// non-moderator currently in the issuer's area.
var punishmentHelpGroups = []punishmentSubcategory{
	{
		emoji: "💬", title: "Text effects",
		desc: "Rewrite the target's IC text — light, mostly cosmetic. Supports 'global' in place of UIDs.",
		cmds: []string{"whisper", "backward", "stutterstep", "elongate", "uppercase", "lowercase",
			"robotic", "alternating", "fancy", "uwu", "pirate", "shakespearean", "caveman",
			"censor", "fromsoftware", "confused", "paranoid", "drunk", "hiccup", "whistle", "mumble",
			"slang", "cherri", "albhed", "morse", "vowelhell", "upsidedown", "autospell",
			"thesaurusoverload", "valleygirl", "babytalk", "thirdperson",
			"unreliablenarrator", "uncannyvalley", "chef", "karen", "passiveaggressive",
			"nervous", "sarcasm", "academic", "philosopher", "poet", "quote", "spaghetti",
			"essay", "rng", "haiku", "dreamsequence", "timewarp",
			"zalgo", "leetspeak", "smallcaps", "piglatin", "vaporwave", "lisp",
			"spoonerism", "keysmash", "weeb", "politician", "clickbait", "markov",
			"alliteration", "cipher", "medieval"},
	},
	{
		emoji: "🎭", title: "Themed quote replacers",
		desc: "Discard the player's text and substitute a themed line per message. Supports 'global'.",
		cmds: []string{"recipe", "rickroll", "pickup", "brainrot", "gordonramsay", "biblebot",
			"grounded", "mime", "subtitles", "spotlight", "cheese"},
	},
	{
		emoji: "🃏", title: "Persona / personality",
		desc: "Wraps every line in a persona's prefix/suffix flavour. Supports 'global'.",
		cmds: []string{"clown", "jester", "joker", "tourettes", "translator"},
	},
	{
		emoji: "💖", title: "Dere archetypes",
		desc: "Anime-style relationship-trope flavour. /omnidere mixes them all. Supports 'global'.",
		cmds: []string{"omnidere", "tsundere", "yandere", "kuudere", "dandere", "deredere",
			"himedere", "kamidere", "undere", "bakadere", "mayadere",
			"smugdere", "deretsun", "bokodere", "thugdere", "teasedere", "dorodere",
			"hinedere", "hajidere", "rindere", "utsudere", "darudere", "butsudere",
			"sdere", "mdere", "tsuyodere"},
	},
	{
		emoji: "🐾", title: "Animal filters",
		desc: "Replace text with animal sounds. Supports 'global'.",
		cmds: []string{"monkey", "snake", "dog", "cat", "bird", "cow", "frog", "duck",
			"horse", "lion", "trex", "fish", "zoo", "bunny"},
	},
	{
		emoji: "👁", title: "Visibility / cosmetic",
		desc: "Hides the player or alters their visual presentation.",
		cmds: []string{"emoji", "invisible", "shrink", "grow", "wide", "areainiswap",
			"hidedisplay", "forcedisplay", "unshrink", "ungrow", "unwide"},
	},
	{
		emoji: "📡", title: "Protocol / viewport effects",
		desc: "Weaponize the IC packet's non-text fields — offsets, screenshake, flip, colour, preanims. Supports 'global'.",
		cmds: []string{"teleport", "shakecurse", "randomflip", "forcecolor",
			"nopreanim", "forcepreanim"},
	},
	{
		emoji: "⏱", title: "Timing & throughput",
		desc: "Slow, speed up, pace — or reverse — the target's IC chat.",
		// "lag" is deliberately absent: the in-game command was removed, and the
		// torment list can now only be applied from the server console. /unlag
		// stays listed under removal, since lifting one is unrestricted.
		cmds: []string{"slowpoke", "fastspammer", "lifo"},
	},
	{
		emoji: "☣️", title: "Traps & contagion",
		desc: "Punishments that move on their own: plagues, mines, area traps, random warps, and the silent treatment.",
		cmds: []string{"contagious", "minefield", "randomarea", "silencebell", "stealthmute"},
	},
	{
		emoji: "🔊", title: "Audio / SFX",
		desc: "Forces audio side-effects on every IC line.",
		cmds: []string{"sfxcurse", "unsfx"},
	},
	{
		emoji: "🎙", title: "Voice chat",
		desc: "Sabotage a player's server-relayed voice-chat audio — drop, gate, or stutter their Opus frames. Needs enable_voice. Supports 'global'.",
		cmds: []string{"voicemute", "voicestatic", "voicegarble", "voicecutout", "voicestutter"},
	},
	{
		emoji: "💥", title: "Stacking / chaos",
		desc: "Combine multiple effects on a single target. /stack, /lovebomb, and /randompunishall support 'global'.",
		cmds: []string{"stack", "torment", "lovebomb", "degrade",
			"emoticon", "51", "icwarp", "megamaso", "maso",
			"randompunishall", "togglerandompunish", "tournament"},
	},
	{
		emoji: "🧹", title: "Removal & control",
		desc: "Inspect and lift active punishments. /punishments lists what's on a player; /unpunish removes ALL types including lag.",
		cmds: []string{"punishments", "unpunish", "unlag", "unicwarp", "unlovebomb",
			"un51", "unacademic", "unbabytalk", "undegrade",
			"unphilosopher", "unpoet", "unquote", "unrecipe", "unsarcasm",
			"unslang", "unthesaurusoverload", "unthirdperson", "untranslator",
			"ununcannyvalley", "ununreliablenarrator", "unupsidedown", "unvalleygirl"},
	},
	{
		emoji: "🚫", title: "Self-chaos block",
		desc: "Prevent a player from using self-applied chaos commands (/maso, /megamaso, /potion, /coinflip). Mods can still punish them normally.",
		cmds: []string{"blockpunishment", "unblockpunishment"},
	},
}

// renderPunishmentHelp produces the grouped /help punishment output.
// Filters by the caller's permission so mods see staff-only entries while
// regular players see only the self-applied ones (e.g. /maso, /megamaso).
func renderPunishmentHelp(client *Client, casinoEnabled, accountsEnabled, voiceEnabled bool) string {
	// Build a fast lookup of commands available to this client.
	available := make(map[string]Command, len(Commands))
	for name, cmd := range Commands {
		if cmd.category != "punishment" {
			continue
		}
		if cmd.casinoCmd && !casinoEnabled {
			continue
		}
		if cmd.accountCmd && !accountsEnabled {
			continue
		}
		if cmd.voiceCmd && !voiceEnabled {
			continue
		}
		if !clientCanUseCommand(client, name, cmd) {
			continue
		}
		available[name] = cmd
	}

	if len(available) == 0 {
		return "No accessible commands in the 'punishment' category."
	}

	// Track which commands we've already rendered so the "extras" bucket
	// catches anything that fell through the named groups.
	rendered := make(map[string]struct{}, len(available))

	var sb strings.Builder
	sb.WriteString("🎭 Punishment Commands\nText-effect and behaviour punishments. Most require MUTE.\n💡 Global mode: any command that takes UIDs also accepts 'global' to affect every\n   non-moderator in your current area. Example: /tsundere global\nUse /help <command> for usage.\n")

	for _, group := range punishmentHelpGroups {
		var lines []string
		for _, name := range group.cmds {
			cmd, ok := available[name]
			if !ok {
				continue
			}
			lines = append(lines, fmt.Sprintf("  /%v — %v", name, cmd.desc))
			rendered[name] = struct{}{}
		}
		if len(lines) == 0 {
			continue
		}
		sort.Strings(lines)
		sb.WriteString(fmt.Sprintf("\n%v %v\n  %v\n", group.emoji, group.title, group.desc))
		sb.WriteString(strings.Join(lines, "\n"))
		sb.WriteString("\n")
	}

	// Extras bucket: anything in the punishment category that no group
	// explicitly listed (so newly-added punishments aren't silently hidden).
	var extras []string
	for name, cmd := range available {
		if _, seen := rendered[name]; seen {
			continue
		}
		extras = append(extras, fmt.Sprintf("  /%v — %v", name, cmd.desc))
	}
	if len(extras) > 0 {
		sort.Strings(extras)
		sb.WriteString("\n📦 Other / extras\n  Punishments not yet bucketed by /help.\n")
		sb.WriteString(strings.Join(extras, "\n"))
		sb.WriteString("\n")
	}

	sb.WriteString("\nFor detailed usage on any command: /<command> -h")
	return sb.String()
}

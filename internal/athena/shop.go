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
	"strings"

	"github.com/MangosArentLiterature/Athena/internal/db"
	"github.com/MangosArentLiterature/Athena/internal/logger"
)

// shopItemIndex provides O(1) lookup by item ID.
// shopCategoryTagCount provides O(1) lookup of how many tags exist per category.
// Both are computed once at package init from the static shopItems catalog.
var (
	shopItemIndex        map[string]shopItem
	shopCategoryTagCount map[string]int
)

func init() {
	shopItemIndex = make(map[string]shopItem, len(shopItems))
	shopCategoryTagCount = make(map[string]int, len(shopCategories))
	for _, it := range shopItems {
		shopItemIndex[it.id] = it
		if it.kind == shopKindTag {
			shopCategoryTagCount[it.category]++
		}
	}
}

// shopItemKind classifies each item in the shop.
type shopItemKind int

const (
	shopKindTag     shopItemKind = iota // cosmetic tag shown in /gas and /players
	shopKindPass                        // permanent perk (job cooldown / bonus)
	shopKindPassive                     // permanent passive income (extra chips/hour)
)

// shopCategoryInfo describes a browseable tag category.
type shopCategoryInfo struct {
	key         string
	displayName string
	emoji       string
}

// shopCategories lists tag categories in display order.
var shopCategories = []shopCategoryInfo{
	{"gambling", "Gambling", "🎰"},
	{"attorney", "Ace Attorney", "⚖️"},
	{"anime", "Anime", "🌸"},
	{"gamer", "Gamer", "🎮"},
	{"girly", "Girly", "🌷"},
	{"meme", "Silly & Memes", "😂"},
	{"prestige", "Prestige", "👑"},
}

// shopItem describes a single purchasable item.
type shopItem struct {
	id                string
	name              string
	kind              shopItemKind
	category          string // tag category key (tag items only); "" for passes
	price             int64
	description       string
	cooldownReduction int64 // seconds to shave off ALL job cooldowns (passes only)
	jobBonus          int64 // extra chips awarded per job completion (passes only)
	hourlyBonus       int64 // extra chips awarded per hour online (passive only)
}

// shopItems is the master catalog.  All entries are displayed by /shop.
var shopItems = []shopItem{

	// ══════════════════════════════════════════════════════════════════════════
	// 🎰 GAMBLING TAGS (30 total, existing)
	// ══════════════════════════════════════════════════════════════════════════

	{id: "tag_gambler", name: "Gambler", kind: shopKindTag, category: "gambling", price: 1_000,
		description: "The first step on every high roller's journey."},
	{id: "tag_lucky", name: "Lucky", kind: shopKindTag, category: "gambling", price: 2_500,
		description: "Fortune smiles on you."},
	{id: "tag_risk_taker", name: "Risk Taker", kind: shopKindTag, category: "gambling", price: 5_000,
		description: "You live on the edge."},
	{id: "tag_card_shark", name: "Card Shark", kind: shopKindTag, category: "gambling", price: 7_500,
		description: "The cards bend to your will."},
	{id: "tag_high_roller", name: "High Roller", kind: shopKindTag, category: "gambling", price: 10_000,
		description: "Big bets, bigger wins."},
	{id: "tag_patreon", name: "Patreon", kind: shopKindTag, category: "gambling", price: 15_000,
		description: "A proud supporter of the casino."},
	{id: "tag_chip_collector", name: "Chip Collector", kind: shopKindTag, category: "gambling", price: 25_000,
		description: "You have amassed a serious stack."},
	{id: "tag_jackpot", name: "Jackpot", kind: shopKindTag, category: "gambling", price: 35_000,
		description: "You've hit it big."},
	{id: "tag_casino_regular", name: "Casino Regular", kind: shopKindTag, category: "gambling", price: 50_000,
		description: "The staff know your name."},
	{id: "tag_hustler", name: "Hustler", kind: shopKindTag, category: "gambling", price: 75_000,
		description: "Always working the angles."},
	{id: "tag_all_in", name: "All In", kind: shopKindTag, category: "gambling", price: 100_000,
		description: "No half measures."},
	{id: "tag_whale", name: "Whale", kind: shopKindTag, category: "gambling", price: 150_000,
		description: "You move markets with your bets."},
	{id: "tag_bluffer", name: "Bluffer", kind: shopKindTag, category: "gambling", price: 200_000,
		description: "Your poker face is legendary."},
	{id: "tag_odds_defier", name: "Odds Defier", kind: shopKindTag, category: "gambling", price: 300_000,
		description: "Statistics fear you."},
	{id: "tag_dealer", name: "Dealer", kind: shopKindTag, category: "gambling", price: 400_000,
		description: "You know where every card is."},
	{id: "tag_ace", name: "Ace", kind: shopKindTag, category: "gambling", price: 500_000,
		description: "Always the highest card in the deck."},
	{id: "tag_double_down", name: "Double Down", kind: shopKindTag, category: "gambling", price: 600_000,
		description: "You never back away from a good hand."},
	{id: "tag_full_house", name: "Full House", kind: shopKindTag, category: "gambling", price: 750_000,
		description: "A powerful hand, a powerful player."},
	{id: "tag_flush", name: "Flush", kind: shopKindTag, category: "gambling", price: 900_000,
		description: "All the right suits, all the right moves."},
	{id: "tag_lucky_charm", name: "Lucky Charm", kind: shopKindTag, category: "gambling", price: 1_000_000,
		description: "A million chips of proof."},
	{id: "tag_bankroll", name: "Bankroll", kind: shopKindTag, category: "gambling", price: 1_250_000,
		description: "Your wealth speaks for itself."},
	{id: "tag_fortune", name: "Fortune's Fave", kind: shopKindTag, category: "gambling", price: 1_500_000,
		description: "The universe bends your way."},
	{id: "tag_degenerate", name: "Degenerate", kind: shopKindTag, category: "gambling", price: 2_000_000,
		description: "You wear it as a badge of honour."},
	{id: "tag_the_house", name: "The House", kind: shopKindTag, category: "gambling", price: 2_500_000,
		description: "The house always wins, and you are the house."},
	{id: "tag_legendary", name: "Legendary", kind: shopKindTag, category: "gambling", price: 3_000_000,
		description: "Stories are told about players like you."},
	{id: "tag_casino_royale", name: "Casino Royale", kind: shopKindTag, category: "gambling", price: 4_000_000,
		description: "The ultimate casino experience."},
	{id: "tag_diamond", name: "Diamond", kind: shopKindTag, category: "gambling", price: 5_000_000,
		description: "Rare, brilliant, and impossibly valuable."},
	{id: "tag_mythic", name: "Mythic", kind: shopKindTag, category: "gambling", price: 6_000_000,
		description: "Spoken of in hushed reverence."},
	{id: "tag_godlike", name: "Godlike", kind: shopKindTag, category: "gambling", price: 7_500_000,
		description: "Mortals cannot comprehend your wins."},
	{id: "tag_infinite", name: "Infinite", kind: shopKindTag, category: "gambling", price: 10_000_000,
		description: "The ultimate flex. You have truly seen it all."},

	// ══════════════════════════════════════════════════════════════════════════
	// ⚖️ ACE ATTORNEY TAGS (15 tags)
	// ══════════════════════════════════════════════════════════════════════════

	{id: "tag_objection", name: "Objection!", kind: shopKindTag, category: "attorney", price: 500,
		description: "The classic battlecry. Every good attorney has one."},
	{id: "tag_take_that", name: "Take That!", kind: shopKindTag, category: "attorney", price: 500,
		description: "Slam that evidence on the bench."},
	{id: "tag_hold_it", name: "Hold It!", kind: shopKindTag, category: "attorney", price: 500,
		description: "Wait just a moment there!"},
	{id: "tag_attorney", name: "Attorney at Law", kind: shopKindTag, category: "attorney", price: 1_500,
		description: "You passed the bar. Somehow."},
	{id: "tag_detective", name: "Detective", kind: shopKindTag, category: "attorney", price: 2_000,
		description: "On the case and eating burgers."},
	{id: "tag_prosecutor", name: "Prosecutor", kind: shopKindTag, category: "attorney", price: 5_000,
		description: "A perfect win record — until now."},
	{id: "tag_defense", name: "Defense Attorney", kind: shopKindTag, category: "attorney", price: 8_000,
		description: "Fighting for the truth, one contradiction at a time."},
	{id: "tag_co_counsel", name: "Co-Counsel", kind: shopKindTag, category: "attorney", price: 12_000,
		description: "Supporting the defense from the sidelines."},
	{id: "tag_steel_samurai", name: "Steel Samurai", kind: shopKindTag, category: "attorney", price: 25_000,
		description: "Hero of justice and children everywhere!"},
	{id: "tag_magatama", name: "Magatama", kind: shopKindTag, category: "attorney", price: 75_000,
		description: "The Magatama reveals the locks around your secrets."},
	{id: "tag_chords_of_steel", name: "Chords of Steel", kind: shopKindTag, category: "attorney", price: 150_000,
		description: "RAAAHHH! Your voice shakes the courtroom."},
	{id: "tag_ladder_or_step", name: "Ladder or Step?", kind: shopKindTag, category: "attorney", price: 500_000,
		description: "The greatest philosophical debate of our time."},
	{id: "tag_turnabout", name: "Turnabout", kind: shopKindTag, category: "attorney", price: 1_000_000,
		description: "Every case ends in a turnabout when you're involved."},
	{id: "tag_psyche_lock", name: "Psyche-Lock", kind: shopKindTag, category: "attorney", price: 2_500_000,
		description: "The chains of deception cannot hold against you."},
	{id: "tag_godot_coffee", name: "One Thousand Blends", kind: shopKindTag, category: "attorney", price: 5_000_000,
		description: "You've had exactly 1000 cups of coffee today. This one is number 17."},

	// ══════════════════════════════════════════════════════════════════════════
	// 🌸 ANIME TAGS (15 tags)
	// ══════════════════════════════════════════════════════════════════════════

	{id: "tag_weeb", name: "Weeb", kind: shopKindTag, category: "anime", price: 100,
		description: "Embrace the culture. No shame."},
	{id: "tag_otaku", name: "Otaku", kind: shopKindTag, category: "anime", price: 250,
		description: "You've watched more anime than you've slept."},
	{id: "tag_senpai", name: "Senpai", kind: shopKindTag, category: "anime", price: 500,
		description: "Notice me."},
	{id: "tag_kawaii", name: "Kawaii", kind: shopKindTag, category: "anime", price: 1_000,
		description: "Unbearably cute in every possible way."},
	{id: "tag_chibi", name: "Chibi", kind: shopKindTag, category: "anime", price: 1_500,
		description: "Small but incredibly powerful (and adorable)."},
	{id: "tag_isekai", name: "Isekai Survivor", kind: shopKindTag, category: "anime", price: 4_000,
		description: "Reincarnated into a courtroom with a truck-kun origin story."},
	{id: "tag_tsundere", name: "Tsundere", kind: shopKindTag, category: "anime", price: 8_000,
		description: "It's not like I want to be here or anything. Baka."},
	{id: "tag_yandere", name: "Yandere", kind: shopKindTag, category: "anime", price: 12_000,
		description: "I'll do anything for my beloved. Anything."},
	{id: "tag_kuudere", name: "Kuudere", kind: shopKindTag, category: "anime", price: 15_000,
		description: "Cool, calm, emotionally unreachable."},
	{id: "tag_waifu_haver", name: "Waifu Haver", kind: shopKindTag, category: "anime", price: 20_000,
		description: "You have chosen your side and you will defend it."},
	{id: "tag_nakama", name: "Nakama", kind: shopKindTag, category: "anime", price: 35_000,
		description: "The power of friendship is real and it lives here."},
	{id: "tag_sensei", name: "Sensei", kind: shopKindTag, category: "anime", price: 60_000,
		description: "You have much to teach, and even more to learn."},
	{id: "tag_final_boss", name: "Final Boss", kind: shopKindTag, category: "anime", price: 200_000,
		description: "The true challenge was you all along."},
	{id: "tag_transcended", name: "Transcended", kind: shopKindTag, category: "anime", price: 750_000,
		description: "Beyond mortal understanding. You have achieved something."},
	{id: "tag_protagonist", name: "Protagonist", kind: shopKindTag, category: "anime", price: 2_000_000,
		description: "You are the main character and the plot bends around you."},

	// ══════════════════════════════════════════════════════════════════════════
	// 🎮 GAMER TAGS (15 tags)
	// ══════════════════════════════════════════════════════════════════════════

	{id: "tag_noob", name: "Noob", kind: shopKindTag, category: "gamer", price: 100,
		description: "Proudly new. At least you're honest."},
	{id: "tag_gg", name: "GG EZ", kind: shopKindTag, category: "gamer", price: 200,
		description: "Good game. Easy game. No contest."},
	{id: "tag_touch_grass", name: "Touch Grass", kind: shopKindTag, category: "gamer", price: 300,
		description: "A reminder from your past self. You haven't listened."},
	{id: "tag_tryhard", name: "Tryhard", kind: shopKindTag, category: "gamer", price: 500,
		description: "You take every game seriously. Every. Single. One."},
	{id: "tag_afk", name: "AFK", kind: shopKindTag, category: "gamer", price: 750,
		description: "Away from keyboard. Spiritually, at least."},
	{id: "tag_pro_gamer", name: "Pro Gamer", kind: shopKindTag, category: "gamer", price: 3_000,
		description: "A certified professional. Certification may not be real."},
	{id: "tag_speedrunner", name: "Speedrunner", kind: shopKindTag, category: "gamer", price: 10_000,
		description: "Optimised pathing. Any% or bust."},
	{id: "tag_no_lifer", name: "No Lifer", kind: shopKindTag, category: "gamer", price: 20_000,
		description: "You live in the server. You have no other home."},
	{id: "tag_esports", name: "Esports Ready", kind: shopKindTag, category: "gamer", price: 40_000,
		description: "Sponsor me. I'm ready."},
	{id: "tag_pvp_god", name: "PvP God", kind: shopKindTag, category: "gamer", price: 100_000,
		description: "No mercy. No remorse. No defeats."},
	{id: "tag_git_gud", name: "Git Gud", kind: shopKindTag, category: "gamer", price: 200_000,
		description: "You've said this to others. Now it's on your profile."},
	{id: "tag_game_dev", name: "Game Dev", kind: shopKindTag, category: "gamer", price: 400_000,
		description: "Making games instead of playing them. Probably."},
	{id: "tag_world_first", name: "World First", kind: shopKindTag, category: "gamer", price: 1_500_000,
		description: "First in the world to do something impressive. Allegedly."},
	{id: "tag_one_more_run", name: "One More Run", kind: shopKindTag, category: "gamer", price: 4_000_000,
		description: "You said this 7 hours ago. You are still here."},
	{id: "tag_gaming_chair", name: "Gaming Chair", kind: shopKindTag, category: "gamer", price: 8_000_000,
		description: "The source of all your power. Don't question it."},

	// ══════════════════════════════════════════════════════════════════════════
	// 🌷 GIRLY TAGS (12 tags)
	// ══════════════════════════════════════════════════════════════════════════

	{id: "tag_princess", name: "Princess", kind: shopKindTag, category: "girly", price: 200,
		description: "Royalty, obviously."},
	{id: "tag_fairy", name: "Fairy", kind: shopKindTag, category: "girly", price: 500,
		description: "Delicate, magical, and 100% real."},
	{id: "tag_queen", name: "Queen", kind: shopKindTag, category: "girly", price: 2_000,
		description: "Step aside, there's a queen coming through."},
	{id: "tag_sparkle", name: "Sparkle", kind: shopKindTag, category: "girly", price: 4_000,
		description: "Everything you touch turns into glitter. A blessing and a curse."},
	{id: "tag_bubblegum", name: "Bubblegum", kind: shopKindTag, category: "girly", price: 6_000,
		description: "Sweet, bright, and impossible to ignore."},
	{id: "tag_cottagecore", name: "Cottagecore", kind: shopKindTag, category: "girly", price: 10_000,
		description: "Living in a cute cottage surrounded by mushrooms and frogs."},
	{id: "tag_pastel", name: "Pastel Queen", kind: shopKindTag, category: "girly", price: 25_000,
		description: "Every colour is soft. Every vibe is immaculate."},
	{id: "tag_boss_babe", name: "Boss Babe", kind: shopKindTag, category: "girly", price: 60_000,
		description: "Running things. Taking names. Looking great doing it."},
	{id: "tag_enchantress", name: "Enchantress", kind: shopKindTag, category: "girly", price: 150_000,
		description: "Mystical, powerful, and impossible to resist."},
	{id: "tag_sorceress", name: "Sorceress", kind: shopKindTag, category: "girly", price: 400_000,
		description: "You didn't need a magic wand. You never did."},
	{id: "tag_femme_fatale", name: "Femme Fatale", kind: shopKindTag, category: "girly", price: 1_200_000,
		description: "Devastating and utterly unforgettable."},
	{id: "tag_divine_feminine", name: "Divine Feminine", kind: shopKindTag, category: "girly", price: 6_000_000,
		description: "You have transcended the ordinary. A force of nature made manifest."},

	// ══════════════════════════════════════════════════════════════════════════
	// 😂 SILLY & MEME TAGS (20 tags)
	// ══════════════════════════════════════════════════════════════════════════

	{id: "tag_bruh", name: "Bruh", kind: shopKindTag, category: "meme", price: 100,
		description: "Bruh."},
	{id: "tag_npc", name: "NPC", kind: shopKindTag, category: "meme", price: 150,
		description: "Following the script. No agency detected."},
	{id: "tag_sus", name: "Sus", kind: shopKindTag, category: "meme", price: 200,
		description: "Very suspicious. Among us in this courtroom."},
	{id: "tag_no_cap", name: "No Cap", kind: shopKindTag, category: "meme", price: 300,
		description: "Absolutely telling the truth. On everything."},
	{id: "tag_ratio", name: "L+Ratio", kind: shopKindTag, category: "meme", price: 500,
		description: "You have been ratio'd. Or you are the ratio."},
	{id: "tag_skill_issue", name: "Skill Issue", kind: shopKindTag, category: "meme", price: 750,
		description: "The diagnosis is in. The prognosis is practice more."},
	{id: "tag_tung_tung_sahur", name: "Tung Tung Sahur", kind: shopKindTag, category: "meme", price: 1_000,
		description: "Tung tung tung tung sahur. The drum calls. You answered."},
	{id: "tag_based", name: "Based", kind: shopKindTag, category: "meme", price: 2_000,
		description: "Certified based. No further explanation needed."},
	{id: "tag_copium", name: "Copium", kind: shopKindTag, category: "meme", price: 4_000,
		description: "Huffing copium through a garden hose. It's fine."},
	{id: "tag_sigma", name: "Sigma", kind: shopKindTag, category: "meme", price: 7_500,
		description: "You grind. You do not simp. You follow the sigma path."},
	{id: "tag_chronically_online", name: "Chronically Online", kind: shopKindTag, category: "meme", price: 12_000,
		description: "You know every meme format and you use them all wrong."},
	{id: "tag_gigachad", name: "Gigachad", kind: shopKindTag, category: "meme", price: 50_000,
		description: "The jawline. The confidence. The absolute unit."},
	{id: "tag_mrbeast", name: "MrBeast", kind: shopKindTag, category: "meme", price: 75_000,
		description: "You buried 100 chips and gave the rest away. Subscribe."},
	{id: "tag_main_character", name: "Main Character", kind: shopKindTag, category: "meme", price: 100_000,
		description: "The plot revolves around you. The NPCs can tell."},
	{id: "tag_goat", name: "GOAT", kind: shopKindTag, category: "meme", price: 250_000,
		description: "Greatest Of All Time. It's confirmed."},
	{id: "tag_rizz", name: "Infinite Rizz", kind: shopKindTag, category: "meme", price: 500_000,
		description: "Unspoken rizz so powerful it bends reality."},
	{id: "tag_giga_brain", name: "Giga Brain", kind: shopKindTag, category: "meme", price: 1_000_000,
		description: "Thinking on a level most cannot comprehend."},
	{id: "tag_meme_lord", name: "Meme Lord", kind: shopKindTag, category: "meme", price: 2_500_000,
		description: "You have mastered the art. You are the meme."},
	{id: "tag_404_not_found", name: "404 Not Found", kind: shopKindTag, category: "meme", price: 5_000_000,
		description: "Error: player not found. And yet here you are."},
	{id: "tag_exists_confused", name: "Exists & Confused", kind: shopKindTag, category: "meme", price: 9_999_999,
		description: "You are here. You don't know why. You spent 10 million chips on this tag."},

	// ══════════════════════════════════════════════════════════════════════════
	// 👑 PRESTIGE TAGS (10 tags)
	// ══════════════════════════════════════════════════════════════════════════

	{id: "tag_newcomer", name: "Newcomer", kind: shopKindTag, category: "prestige", price: 100,
		description: "Just arrived. Everything is new and exciting."},
	{id: "tag_regular", name: "Regular", kind: shopKindTag, category: "prestige", price: 1_000,
		description: "A known face around here. Reliably present."},
	{id: "tag_veteran", name: "Veteran", kind: shopKindTag, category: "prestige", price: 10_000,
		description: "You have seen things. You remain unfazed."},
	{id: "tag_elite", name: "Elite", kind: shopKindTag, category: "prestige", price: 40_000,
		description: "Not everyone makes it here. You did."},
	{id: "tag_champion", name: "Champion", kind: shopKindTag, category: "prestige", price: 125_000,
		description: "A champion in every sense. Feared and respected."},
	{id: "tag_grandmaster", name: "Grandmaster", kind: shopKindTag, category: "prestige", price: 500_000,
		description: "The highest conventional rank. But there is always further to go."},
	{id: "tag_lord", name: "Lord", kind: shopKindTag, category: "prestige", price: 1_000_000,
		description: "A title earned by those who rule through wealth and presence."},
	{id: "tag_overlord", name: "Overlord", kind: shopKindTag, category: "prestige", price: 3_500_000,
		description: "You are not just a lord. You lord over lords."},
	{id: "tag_demon_king", name: "Demon King", kind: shopKindTag, category: "prestige", price: 7_000_000,
		description: "The final form. All who oppose you have been defeated."},
	{id: "tag_absolute_unit", name: "Absolute Unit", kind: shopKindTag, category: "prestige", price: 10_000_000,
		description: "There are no words. You are simply an absolute unit."},

	// ══════════════════════════════════════════════════════════════════════════
	// 💼 JOB PASSES — Cooldown Reduction (all stack, up to 50 min total)
	// ══════════════════════════════════════════════════════════════════════════

	{id: "pass_quick", name: "Quick Worker Pass", kind: shopKindPass, price: 10_000,
		cooldownReduction: 5 * 60,
		description:       "Permanently reduces ALL job cooldowns by 5 minutes."},
	{id: "pass_speedy", name: "Speedy Pass", kind: shopKindPass, price: 50_000,
		cooldownReduction: 10 * 60,
		description:       "Permanently reduces ALL job cooldowns by an additional 10 minutes."},
	{id: "pass_turbo", name: "Turbo Pass", kind: shopKindPass, price: 150_000,
		cooldownReduction: 15 * 60,
		description:       "Permanently reduces ALL job cooldowns by an additional 15 minutes."},
	{id: "pass_lightning", name: "Lightning Pass", kind: shopKindPass, price: 500_000,
		cooldownReduction: 20 * 60,
		description:       "Permanently reduces ALL job cooldowns by an additional 20 minutes. (Full stack = −50 min, floor 5 min.)"},

	// ══════════════════════════════════════════════════════════════════════════
	// 💼 JOB PASSES — Reward Bonus (all stack, up to +11 chips/job)
	// ══════════════════════════════════════════════════════════════════════════

	{id: "pass_bonus", name: "Bonus Chip Pass", kind: shopKindPass, price: 25_000,
		jobBonus:    1,
		description: "Permanently earn +1 extra chip on every job completion."},
	{id: "pass_extra", name: "Extra Chip Pass", kind: shopKindPass, price: 100_000,
		jobBonus:    2,
		description: "Permanently earn +2 extra chips on every job completion."},
	{id: "pass_lucky_find", name: "Lucky Find Pass", kind: shopKindPass, price: 400_000,
		jobBonus:    3,
		description: "Permanently earn +3 extra chips on every job completion."},
	{id: "pass_jackpot_seeker", name: "Jackpot Seeker Pass", kind: shopKindPass, price: 1_000_000,
		jobBonus:    5,
		description: "Permanently earn +5 extra chips on every job completion. (Full stack = +11 chips/job!)"},

	// ══════════════════════════════════════════════════════════════════════════
	// ⏱️ PASSIVE INCOME — Extra chips per hour online (all stack)
	// ══════════════════════════════════════════════════════════════════════════

	// Passive income passes are additive (not multiplicative) — the "2x/3x/5x/10x"
	// names refer to the total chips/hr when all lower passes are also owned.
	// hourlyBonus is the raw number of extra chips/hr this pass contributes.
	{id: "pass_income_2x", name: "Income Doubler", kind: shopKindPassive, price: 1_500,
		hourlyBonus: 1,
		description: "Permanently earn +1 chip per hour online (doubles the base 1 chip/hr to 2 chips/hr)."},
	{id: "pass_income_3x", name: "Income Tripler", kind: shopKindPassive, price: 5_000,
		hourlyBonus: 1,
		description: "Permanently earn +1 more chip per hour online (adds to Doubler for 3 chips/hr total)."},
	{id: "pass_income_5x", name: "5x Income Booster", kind: shopKindPassive, price: 25_000,
		hourlyBonus: 2,
		description: "Permanently earn +2 more chips per hour online (adds to Doubler+Tripler for 5 chips/hr total)."},
	{id: "pass_income_10x", name: "10x Income Booster", kind: shopKindPassive, price: 100_000,
		hourlyBonus: 5,
		description: "Permanently earn +5 more chips per hour online (10 chips/hr total with all four). Still a slog!"},
}

// shopItemByID returns the item with the given ID, or (shopItem{}, false) if not found.
// Only the built-in catalog is checked here — staff-defined custom tags live
// in the database and are looked up separately via lookupTag.
func shopItemByID(id string) (shopItem, bool) {
	it, ok := shopItemIndex[id]
	return it, ok
}

// lookupTag resolves a tag id to its display name regardless of whether the
// tag is built-in or staff-defined. Returns ("", false) when the id refers to
// no tag at all, or to a non-tag shop item (a pass).
func lookupTag(id string) (string, bool) {
	if id == "" {
		return "", false
	}
	if it, ok := shopItemIndex[id]; ok {
		if it.kind != shopKindTag {
			return "", false
		}
		return it.name, true
	}
	return db.GetCustomTag(id)
}

// formatTagDisplay returns the bracketed tag label, e.g. "[High Roller]".
// Returns "" when tagID is empty or unknown. Resolves both built-in and
// staff-defined custom tags.
func formatTagDisplay(tagID string) string {
	name, ok := lookupTag(tagID)
	if !ok {
		return ""
	}
	return "[" + name + "]"
}

// getPlayerPassBonuses returns all three stacking pass values for an IPID in a
// single database round-trip: total cooldown reduction (seconds), total job
// bonus (chips/job), and total hourly bonus (chips/hr).
func getPlayerPassBonuses(ipid string) (cooldown, job, hourly int64) {
	items, err := db.GetPlayerShopItems(ipid)
	if err != nil {
		logger.LogErrorf("shop: GetPlayerShopItems failed for ipid=%v: %v", ipid, err)
		return
	}
	for _, id := range items {
		if it, ok := shopItemIndex[id]; ok {
			cooldown += it.cooldownReduction
			job += it.jobBonus
			hourly += it.hourlyBonus
		}
	}
	return
}

// getPlayerCooldownReduction returns the total job-cooldown reduction in seconds
// for the given IPID, summing up all purchased cooldown passes.
func getPlayerCooldownReduction(ipid string) int64 {
	cooldown, _, _ := getPlayerPassBonuses(ipid)
	return cooldown
}

// getPlayerJobBonus returns the total extra chips per job completion for the
// given IPID, summing up all purchased job-bonus passes.
func getPlayerJobBonus(ipid string) int64 {
	_, job, _ := getPlayerPassBonuses(ipid)
	return job
}

// getPlayerHourlyBonus returns the total extra chips per hour of online time
// for the given IPID, summing up all purchased passive income passes.
func getPlayerHourlyBonus(ipid string) int64 {
	_, _, hourly := getPlayerPassBonuses(ipid)
	return hourly
}

// ── /shop command ─────────────────────────────────────────────────────────────

// cmdShop handles the /shop command.
//
//	/shop                  — show overview with category list
//	/shop <category>       — browse items in a category
//	/shop buy <id>         — purchase an item
//	/shop items            — list your owned items
//
// When the casino is disabled but the account system is enabled, the shop becomes
// a read-only tag catalog — prices and passes are still listed for reference, but
// /shop buy is blocked and any tag id can be equipped for free with /settag.
func cmdShop(client *Client, args []string, _ string) {
	if len(args) == 0 {
		printShopOverview(client)
		return
	}

	sub := strings.ToLower(args[0])

	// Check if arg matches a category key first.
	for _, cat := range shopCategories {
		if sub == cat.key {
			printShopCategory(client, cat)
			return
		}
	}

	switch sub {
	case "passes":
		printShopPasses(client)
	case "passive":
		printShopPassive(client)
	case "buy":
		if !config.EnableCasino {
			client.SendServerMessage(
				"The chip economy is disabled on this server, so /shop buy is off.\n" +
					"Good news: every tag in /shop can be equipped for free with /settag <tag_id>.\n" +
					"Browse tag categories with /shop gambling | attorney | anime | gamer | girly | meme | prestige.")
			return
		}
		if len(args) < 2 {
			client.SendServerMessage("Usage: /shop buy <item_id>  — use /shop to see category names.")
			return
		}
		shopBuy(client, args[1])
	case "items", "owned", "inventory":
		shopListOwned(client)
	default:
		client.SendServerMessage(
			"Usage:\n" +
				"  /shop                 — shop overview & categories\n" +
				"  /shop <category>      — browse a tag category\n" +
				"    Categories: gambling attorney anime gamer girly meme prestige\n" +
				"  /shop passes          — job passes (cooldown & bonus)\n" +
				"  /shop passive         — passive income upgrades\n" +
				"  /shop buy <item_id>   — purchase an item\n" +
				"  /shop items           — view your owned items & active tag\n" +
				"  /settag <id>|none     — equip or remove your active tag")
	}
}

func printShopOverview(client *Client) {
	casinoOn := config.EnableCasino
	activeTag := db.GetActiveTag(client.Ipid())
	activeDisplay := "(none)"
	if t := formatTagDisplay(activeTag); t != "" {
		activeDisplay = t
	}

	// Fetch owned items once and build a set for O(1) category-ownership counts.
	ownedItems, _ := db.GetPlayerShopItems(client.Ipid())
	ownedSet := make(map[string]struct{}, len(ownedItems))
	for _, id := range ownedItems {
		ownedSet[id] = struct{}{}
	}

	var sb strings.Builder
	if casinoOn {
		bal, _ := db.GetChipBalance(client.Ipid())
		sb.WriteString(fmt.Sprintf("\n🛒 Nyathena Shop — Balance: %d chips | Active tag: %s\n", bal, activeDisplay))
		sb.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		sb.WriteString("Buy permanent cosmetic tags and grinding upgrades with chips!\n")
		sb.WriteString("All purchases are PERMANENT and linked to your account.\n\n")
	} else {
		sb.WriteString(fmt.Sprintf("\n🛒 Nyathena Shop — Active tag: %s\n", activeDisplay))
		sb.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		sb.WriteString("The chip economy is off — every tag below is FREE to equip.\n")
		sb.WriteString("Pick any tag id and run /settag <tag_id> to make it yours.\n\n")
	}

	sb.WriteString("📂 Tag Categories (use /shop <category> to browse):\n")
	for _, cat := range shopCategories {
		count := shopCategoryTagCount[cat.key]
		if casinoOn {
			owned := countOwnedInCategoryFromSet(ownedSet, cat.key)
			sb.WriteString(fmt.Sprintf("  %s %-12s  — %d tags  (owned: %d)  → /shop %s\n",
				cat.emoji, cat.displayName, count, owned, cat.key))
		} else {
			sb.WriteString(fmt.Sprintf("  %s %-12s  — %d tags  → /shop %s\n",
				cat.emoji, cat.displayName, count, cat.key))
		}
	}

	if casinoOn {
		sb.WriteString("\n💼 Upgrades:\n")
		sb.WriteString("  /shop passes   — job cooldown & reward passes\n")
		sb.WriteString("  /shop passive  — passive income upgrades (more chips/hour)\n")

		sb.WriteString("\n🛒 Commands:\n")
		sb.WriteString("  /shop buy <item_id>   — purchase an item by ID\n")
		sb.WriteString("  /shop items           — list your owned items\n")
		sb.WriteString("  /settag <id>|none     — equip or remove your active tag\n")
	} else {
		sb.WriteString("\n🛒 Commands:\n")
		sb.WriteString("  /settag <tag_id>      — equip any tag from any category (free while the casino is off)\n")
		sb.WriteString("  /settag none          — remove your active tag\n")
		sb.WriteString("  /shop <category>      — browse tags and get their ids\n")
	}
	client.SendServerMessage(sb.String())
}

// countOwnedInCategoryFromSet counts how many items in a given tag category
// the player owns, using a pre-built owned-item set to avoid extra DB calls.
func countOwnedInCategoryFromSet(owned map[string]struct{}, cat string) int {
	n := 0
	for _, it := range shopItems {
		if it.kind == shopKindTag && it.category == cat {
			if _, ok := owned[it.id]; ok {
				n++
			}
		}
	}
	return n
}

func printShopCategory(client *Client, cat shopCategoryInfo) {
	ownedItems, _ := db.GetPlayerShopItems(client.Ipid())
	ownedSet := make(map[string]struct{}, len(ownedItems))
	for _, id := range ownedItems {
		ownedSet[id] = struct{}{}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("\n%s %s Tags — /shop buy <item_id> to purchase\n", cat.emoji, cat.displayName))
	sb.WriteString(fmt.Sprintf("  %-28s  %-20s  %-12s  %s\n", "Item ID", "Tag", "Price", "Owned?"))
	sb.WriteString("  " + strings.Repeat("─", 72) + "\n")

	for _, it := range shopItems {
		if it.kind != shopKindTag || it.category != cat.key {
			continue
		}
		owned := ""
		if _, ok := ownedSet[it.id]; ok {
			owned = "✅"
		}
		sb.WriteString(fmt.Sprintf("  %-28s  [%-18s]  %-12d  %s\n",
			it.id, it.name, it.price, owned))
	}
	sb.WriteString("\nBuy with: /shop buy <item_id>    Back to overview: /shop\n")
	client.SendServerMessage(sb.String())
}

func printShopPasses(client *Client) {
	ownedItems, _ := db.GetPlayerShopItems(client.Ipid())
	ownedSet := make(map[string]struct{}, len(ownedItems))
	for _, id := range ownedItems {
		ownedSet[id] = struct{}{}
	}

	var sb strings.Builder
	sb.WriteString("\n💼 Job Passes — Permanent job upgrades\n")
	sb.WriteString(fmt.Sprintf("  %-26s  %-26s  %-12s  %s\n", "Item ID", "Pass Name", "Price", "Benefit"))
	sb.WriteString("  " + strings.Repeat("─", 80) + "\n")

	for _, it := range shopItems {
		if it.kind != shopKindPass {
			continue
		}
		owned := ""
		if _, ok := ownedSet[it.id]; ok {
			owned = " ✅"
		}
		var benefit string
		if it.cooldownReduction > 0 {
			benefit = fmt.Sprintf("−%dm cooldown", it.cooldownReduction/60)
		} else if it.jobBonus > 0 {
			benefit = fmt.Sprintf("+%d chip/job", it.jobBonus)
		}
		sb.WriteString(fmt.Sprintf("  %-26s  %-26s  %-12d  %s%s\n",
			it.id, it.name, it.price, benefit, owned))
	}

	sb.WriteString("\n💡 Cooldown passes stack — max −50 min (5 min floor per job).\n")
	sb.WriteString("💡 Bonus passes stack — max +11 chips/job with all 4 owned.\n")
	sb.WriteString("Buy with: /shop buy <item_id>\n")
	client.SendServerMessage(sb.String())
}

func printShopPassive(client *Client) {
	ownedItems, _ := db.GetPlayerShopItems(client.Ipid())
	ownedSet := make(map[string]struct{}, len(ownedItems))
	var currentBonus int64
	for _, id := range ownedItems {
		ownedSet[id] = struct{}{}
		if it, ok := shopItemIndex[id]; ok {
			currentBonus += it.hourlyBonus
		}
	}

	var sb strings.Builder
	sb.WriteString("\n⏱️ Passive Income Upgrades — Extra chips earned per hour online\n")
	sb.WriteString(fmt.Sprintf("  %-26s  %-26s  %-12s  %s\n", "Item ID", "Pass Name", "Price", "Benefit"))
	sb.WriteString("  " + strings.Repeat("─", 80) + "\n")

	for _, it := range shopItems {
		if it.kind != shopKindPassive {
			continue
		}
		owned := ""
		if _, ok := ownedSet[it.id]; ok {
			owned = " ✅"
		}
		benefit := fmt.Sprintf("+%d chip/hr", it.hourlyBonus)
		sb.WriteString(fmt.Sprintf("  %-26s  %-26s  %-12d  %s%s\n",
			it.id, it.name, it.price, benefit, owned))
	}

	sb.WriteString(fmt.Sprintf("\nBase: 1 chip/hr | Your total: %d chip/hr (base + %d bonus)\n",
		1+currentBonus, currentBonus))
	sb.WriteString("💡 All four stack for a maximum of 10 chips/hr. Still a grind!\n")
	sb.WriteString("Buy with: /shop buy <item_id>\n")
	client.SendServerMessage(sb.String())
}

func shopBuy(client *Client, itemID string) {
	it, ok := shopItemByID(itemID)
	if !ok {
		client.SendServerMessage(fmt.Sprintf("Unknown item '%v'. Use /shop to browse categories.", itemID))
		return
	}

	if db.HasShopItem(client.Ipid(), it.id) {
		if it.kind == shopKindTag {
			client.SendServerMessage(fmt.Sprintf("You already own [%v]. Use /settag %v to equip it.", it.name, it.id))
		} else {
			client.SendServerMessage(fmt.Sprintf("You already own '%v'.", it.name))
		}
		return
	}

	if err := db.PurchaseShopItem(client.Ipid(), it.id, it.price); err != nil {
		client.SendServerMessage(fmt.Sprintf("Purchase failed: %v", err))
		return
	}

	newBal, _ := db.GetChipBalance(client.Ipid())

	switch it.kind {
	case shopKindTag:
		_ = db.SetActiveTag(client.Ipid(), it.id)
		if client.Authenticated() {
			db.SetAccountActiveTag(client.ModName(), it.id) //nolint:errcheck
		}
		client.SendServerMessage(fmt.Sprintf(
			"✅ Purchased [%v] tag for %d chips! It is now your active tag.\n"+
				"Use /settag <id> to switch tags, or /settag none to remove it.\n"+
				"Balance: %d chips", it.name, it.price, newBal))
	case shopKindPass:
		var benefit string
		if it.cooldownReduction > 0 {
			benefit = fmt.Sprintf("job cooldowns reduced by %d min", it.cooldownReduction/60)
		} else if it.jobBonus > 0 {
			benefit = fmt.Sprintf("+%d chip bonus per job", it.jobBonus)
		}
		client.SendServerMessage(fmt.Sprintf(
			"✅ Purchased %v for %d chips!\nPermanent benefit: %v\nBalance: %d chips",
			it.name, it.price, benefit, newBal))
	case shopKindPassive:
		totalHourly := 1 + getPlayerHourlyBonus(client.Ipid())
		client.SendServerMessage(fmt.Sprintf(
			"✅ Purchased %v for %d chips!\nPermanent benefit: +%d chip/hr (you now earn %d chips/hr total)\nBalance: %d chips",
			it.name, it.price, it.hourlyBonus, totalHourly, newBal))
	}
}

func shopListOwned(client *Client) {
	items, err := db.GetPlayerShopItems(client.Ipid())
	if err != nil || len(items) == 0 {
		client.SendServerMessage("You don't own any shop items yet. Use /shop to browse the catalog.")
		return
	}

	activeTag := db.GetActiveTag(client.Ipid())
	var sb strings.Builder
	sb.WriteString("\n🛒 Your Shop Items\n")
	sb.WriteString("  " + strings.Repeat("─", 50) + "\n")

	var tags, passes, passives []string
	var totalCooldown, totalJobBonus, totalHourly int64
	for _, id := range items {
		if it, ok := shopItemByID(id); ok {
			switch it.kind {
			case shopKindTag:
				active := ""
				if id == activeTag {
					active = " ← active"
				}
				tags = append(tags, fmt.Sprintf("  [%-18s] — %s%s", it.name, it.id, active))
			case shopKindPass:
				var benefit string
				if it.cooldownReduction > 0 {
					benefit = fmt.Sprintf("−%d min cooldown", it.cooldownReduction/60)
					totalCooldown += it.cooldownReduction
				} else if it.jobBonus > 0 {
					benefit = fmt.Sprintf("+%d chip/job", it.jobBonus)
					totalJobBonus += it.jobBonus
				}
				passes = append(passes, fmt.Sprintf("  %-28s — %s", it.name, benefit))
			case shopKindPassive:
				passives = append(passives, fmt.Sprintf("  %-28s — +%d chip/hr", it.name, it.hourlyBonus))
				totalHourly += it.hourlyBonus
			}
		}
	}

	if len(tags) > 0 {
		sb.WriteString(fmt.Sprintf("🏷️  Tags (%d owned):\n", len(tags)))
		for _, t := range tags {
			sb.WriteString(t + "\n")
		}
		if activeTag == "" {
			sb.WriteString("  (no active tag — use /settag <id> to equip one)\n")
		}
	}
	if len(passes) > 0 {
		sb.WriteString("💼 Job Passes:\n")
		for _, p := range passes {
			sb.WriteString(p + "\n")
		}
		if totalCooldown > 0 || totalJobBonus > 0 {
			sb.WriteString(fmt.Sprintf("  ⤷ Total: −%d min cooldown | +%d chip/job\n",
				totalCooldown/60, totalJobBonus))
		}
	}
	if len(passives) > 0 {
		sb.WriteString("⏱️  Passive Income:\n")
		for _, p := range passives {
			sb.WriteString(p + "\n")
		}
		sb.WriteString(fmt.Sprintf("  ⤷ Total: %d chips/hr (base 1 + %d bonus)\n", 1+totalHourly, totalHourly))
	}
	client.SendServerMessage(sb.String())
}

// ── /settag command ───────────────────────────────────────────────────────────

// cmdSetTag handles /settag <tag_id> or /settag none.
// Swaps the player's active cosmetic tag.
//
// When the casino is enabled, tags must be purchased in /shop before they can
// be equipped. When the casino is disabled but the account system is enabled,
// ownership checks are waived and any tag id can be equipped for free — the
// server-wide chip economy is off so there is nothing to "buy".
func cmdSetTag(client *Client, args []string, _ string) {
	tagID := args[0]

	if strings.EqualFold(tagID, "none") || tagID == "" {
		if err := db.SetActiveTag(client.Ipid(), ""); err != nil {
			client.SendServerMessage("Failed to clear tag: " + err.Error())
			return
		}
		if client.Authenticated() {
			db.SetAccountActiveTag(client.ModName(), "") //nolint:errcheck
		}
		client.SendServerMessage("🏷️ Your active tag has been removed.")
		return
	}

	// Built-in catalog: kind/category is known so we can give richer errors
	// and respect the casino-disabled "tags are free" rule.
	if it, ok := shopItemByID(tagID); ok {
		if it.kind != shopKindTag {
			client.SendServerMessage(fmt.Sprintf("'%v' is not a tag — it's a pass. Tags are the cosmetic items shown in /gas.", it.name))
			return
		}
		if config.EnableCasino && !db.HasShopItem(client.Ipid(), tagID) {
			client.SendServerMessage(fmt.Sprintf("You don't own [%v]. Purchase it first with: /shop buy %v", it.name, tagID))
			return
		}
		if err := db.SetActiveTag(client.Ipid(), tagID); err != nil {
			client.SendServerMessage("Failed to set tag: " + err.Error())
			return
		}
		if client.Authenticated() {
			db.SetAccountActiveTag(client.ModName(), tagID) //nolint:errcheck
		}
		client.SendServerMessage(fmt.Sprintf("🏷️ Active tag set to [%v]. It will now appear next to your name in /gas.", it.name))
		return
	}

	// Custom tags must always be granted by staff first; the
	// casino-disabled "any tag is free" rule does not apply here.
	if name, ok := db.GetCustomTag(tagID); ok {
		if !db.HasShopItem(client.Ipid(), tagID) {
			client.SendServerMessage(fmt.Sprintf("You haven't been granted [%v]. Ask a moderator to /grantcustomtag you this tag.", name))
			return
		}
		if err := db.SetActiveTag(client.Ipid(), tagID); err != nil {
			client.SendServerMessage("Failed to set tag: " + err.Error())
			return
		}
		if client.Authenticated() {
			db.SetAccountActiveTag(client.ModName(), tagID) //nolint:errcheck
		}
		client.SendServerMessage(fmt.Sprintf("🏷️ Active tag set to [%v]. It will now appear next to your name in /gas.", name))
		return
	}

	client.SendServerMessage(fmt.Sprintf("Unknown tag '%v'. Use /shop <category> to browse available tag ids, or /listcustomtags for staff-defined tags.", tagID))
}

# Casino Commands — Nyathena Chips

This document covers the full casino system, including **Nyathena Chips** (the server's virtual currency), all casino games, leaderboards, and staff configuration commands.

---

## Nyathena Chips

**Nyathena Chips** are a persistent virtual currency tied to your IPID (connection fingerprint).

- Every new connection starts with **500 chips** automatically.
- Balances persist across sessions.
- Chips cannot go negative — you can only spend what you have.
- **Maximum balance: 10,000,000 chips (10 million).** Winnings are capped at this ceiling to prevent runaway inflation.
- **Default maximum bet: 10,000,000 chips (10 million)** per wager when no area-specific limit is configured. Staff can raise or lower this per-area with `/casinoset maxbet`.

### 🔒 Password Security

Player account passwords are stored using **bcrypt** (industry-standard one-way hashing at cost factor 12). Your password is **never stored in plain text** — not in the database, not in logs, and never transmitted after initial registration. Even server administrators cannot read your password.

---

## `/chips` — Currency Commands

| Command | Description |
|---------|-------------|
| `/chips` | Show your current chip balance. |
| `/chips top [n]` | Show the global chip leaderboard (top 10 by default, max 50). |
| `/chips area [n]` | Show the chip leaderboard for players currently in your area. |
| `/chips give <uid> <amount>` | Transfer chips to another player (max 200,000 per transfer, 10-minute cooldown). |
| `/richest [n]` | Shortcut for `/chips top` — show the richest players by chip balance (top 10 by default, max 50). |

### Examples

```
/chips
> 💰 Your balance: 350 Nyathena Chips

/chips top 5
> 🏆 Global Chip Leaderboard (Top 5)
>  1. abc123 — 12000 chips
>  2. def456 — 8400 chips
>  ...

/richest 5
> 🏆 Global Chip Leaderboard (Top 5)
>  1. abc123 — 12000 chips
>  2. def456 — 8400 chips
>  ...

/chips area
> 🏆 Area Chip Leaderboard — Courtroom (Top 10)
>  1. Athena — 5000 chips
>  2. Apollo — 3200 chips

/chips give 3 100
> Sent 100 chips to Phoenix. Your balance: 250 chips.
```

### Anti-abuse constraints for `/chips give`
- Maximum 200,000 chips per transfer.
- 10-minute cooldown between transfers.
- You cannot give chips to yourself.
- **Requires 24 hours of total playtime.** New accounts must reach 24 hours of playtime before they can transfer chips to others.

---

## `/casino` — Dashboard

| Command | Description |
|---------|-------------|
| `/casino` | Display the casino dashboard for your current area (active tables, balance, game list). |
| `/casino status` | Display detailed status: active tables, current players, slots stats, jackpot pool. |

---

## Casino Games

> **All games require the casino to be enabled in the current area** (see staff commands below).  
> All bets are in Nyathena Chips. Minimum and maximum bets are set per-area.

---

### 🃏 Blackjack — `/bj`

Standard 6-deck blackjack with split, double down, and insurance.

| Subcommand | Description |
|------------|-------------|
| `/bj join` | Join or create the blackjack table (up to 6 players). |
| `/bj bet <amount>` | Place your bet before the deal. |
| `/bj deal` | Start the round (any joined player can trigger once all bets are placed). |
| `/bj hit` | Draw another card. |
| `/bj stand` | End your turn. |
| `/bj double` | Double your bet and draw exactly one more card (first action only). |
| `/bj split` | Split a pair into two hands (doubles your bet). |
| `/bj insurance` | Place an insurance side-bet when dealer shows an Ace (costs half your bet). |
| `/bj status` | Show the current state of the table (dealer's visible card, players, hands). |
| `/bj leave` | Leave the table. |

**Rules:**
- Blackjack (Ace + 10-value on deal) pays **3:2**.
- Dealer hits on soft 16, stands on soft 17.
- Insurance pays **2:1**.
- 60-second turn timer — auto-stand on timeout.

**Examples:**
```
/bj join
/bj bet 50
/bj deal
/bj hit
/bj stand
```

---

### ♠️ Poker (Texas Hold'em) — `/poker`

Full Texas Hold'em with blinds, all streets, and hand evaluation.

| Subcommand | Description |
|------------|-------------|
| `/poker join` | Join or create the poker table (up to 9 players, 500-chip buy-in). |
| `/poker ready` | Signal ready to start (game begins when all seated players are ready). |
| `/poker hand` | View your hole cards privately. |
| `/poker check` | Check (pass action, only when no bet is outstanding). |
| `/poker call` | Call the current bet. |
| `/poker bet <amount>` | Place a bet. |
| `/poker raise <amount>` | Raise the current bet. |
| `/poker fold` | Fold your hand. |
| `/poker allin` | Go all-in with your remaining stack. |
| `/poker status` | Show the pot, community cards, and active players. |
| `/poker leave` | Leave the table (fold if in a hand). |

**Rules:**
- Small blind: 25 chips. Big blind: 50 chips.
- Hole cards are shown privately; community cards and pot are broadcast to the area.
- 60-second turn timer — auto-fold on timeout.

---

### 🎰 Slots — `/slots`

| Subcommand | Description |
|------------|-------------|
| `/slots spin [amount]` | Spin the slots with an optional bet (default: 10 chips). |
| `/slots max` | Spin with the area's maximum bet. |
| `/slots jackpot` | View the current jackpot pool amount. |
| `/slots stats` | View area-wide slots statistics (total spins, payout, jackpots hit). |

**Symbols and payouts** (from highest to lowest):

| Combination | Payout |
|-------------|--------|
| 🎰🎰🎰 (Jackpot) | Full jackpot pool |
| 💎💎💎 | 100× bet |
| ⭐⭐⭐ | 50× bet |
| 🍇🍇🍇 | 20× bet |
| 🍊🍊🍊 | 10× bet |
| 🍋🍋🍋 | 5× bet |
| 🍒🍒🍒 | 3× bet |
| 🍒🍒 (any position) | 2× bet |
| Any 2 matching | 1× bet (push) |
| No match | 0 (loss) |

**Jackpot:** 5% of each losing spin is added to the area jackpot pool. Jackpot is only available when the area staff enables it.

**Rate limit:** Maximum 5 spins per 10 seconds.

---

### 🔴 European Roulette — `/croulette`

```
/croulette bet <type> <amount>
```

| Bet Type | Description | Payout |
|----------|-------------|--------|
| `red` | Red numbers | 1:1 |
| `black` | Black numbers | 1:1 |
| `even` | Even numbers (not 0) | 1:1 |
| `odd` | Odd numbers | 1:1 |
| `low` | Numbers 1–18 | 1:1 |
| `high` | Numbers 19–36 | 1:1 |
| `number <n>` | Straight up (0–36) | 35:1 |

**Example:**
```
/croulette bet red 100
/croulette bet number 17 50
```

---

### 🎴 Baccarat — `/baccarat`

```
/baccarat <player|banker|tie> <amount>
```

| Bet | Payout |
|-----|--------|
| `player` | 1:1 |
| `banker` | 0.95:1 (5% commission) |
| `tie` | 8:1 |

---

### 🎲 Craps (Lite) — `/craps`

```
/craps bet <pass|nopass> <amount>
```

**Pass line rules:**
- First roll 7 or 11: **pass wins**.
- First roll 2, 3, or 12: **pass loses** (craps out).
- Any other number becomes the **point**; keep rolling until point repeats (pass wins) or 7 appears (pass loses).

Pass pays **1:1**; don't-pass is the opposite outcome.

---

### 📈 Crash — `/crash`

Bet before launch, then cash out before the multiplier crashes.

```
/crash bet <amount>   — start a round with a bet
/crash cashout        — cash out at the current multiplier
```

- The multiplier grows every second starting at 1.05×.
- The game crashes at a random point between 1.05× and 6×, skewed **very** heavily toward lower values (four-random-product distribution).
- **House edge: 20%** — all payouts are multiplied by 0.80.
- If you don't `/crash cashout` before the crash, you lose your bet.

**⚠️ Anti-cheese rules (no more instant-cashout spam):**
- **Minimum hold time: 5 seconds** — you cannot cash out within the first 5 seconds after betting. Attempting to do so counts as a loss ("rocket explodes on the launchpad").
- **45-second cooldown between bets** — after a round ends (win or lose), you must wait 45 seconds before starting a new crash game.

---

### 💣 Mines — `/mines`

Navigate a minefield for increasing multipliers.

```
/mines start <mines> <bet>  — start a game (1–24 mines on 5×5 grid)
/mines pick <n>             — reveal cell n (1–25)
/mines cashout              — collect your current winnings
/mines quit                 — give up (lose your bet)
```

- Each safe pick increases your multiplier.
- Hitting a mine ends the game and you lose your bet.
- The more mines, the higher the potential multiplier per pick.

---

### 🎱 Keno — `/keno`

Pick numbers and hope they match the draw.

```
/keno pick <n1> <n2> ... <bet>
```

- Pick 1–10 numbers from 1–80.
- 20 numbers are drawn randomly.
- Payout scales by how many of your picks match (the more picks you make, the higher the jackpot for matching all of them).

**Example:**
```
/keno pick 7 14 21 28 35 100
```

---

### 🎡 Prize Wheel — `/wheel`

```
/wheel spin <bet>
```

**Wheel segments (casino-realistic odds, ~92.5% RTP):**

| Multiplier | Probability |
|------------|-------------|
| 0× (miss)  | 60% |
| 1.5×       | 17% |
| 2×         | 13% |
| 3×         | 7%  |
| 5×         | 2%  |
| 10×        | 1%  |

---

### 🔇 Gamble Hide — `/gamble hide`

Toggle whether you see gambling broadcast messages in the area chat.

```
/gamble hide
```

- Run once to **hide** all gambling result announcements (wheel spins, slot wins, roulette results, etc.) from your chat.
- Run again to **show** them again.
- This setting is saved to your account and automatically restored when you log in.

---

### 🍻 The Bar — `/bar`

Visit the bar for drinks with **massive variance** — every drink carries real risk of a big loss or a big gain. No safe options here!

```
/bar menu              — show all available drinks with costs
/bar buy <drink>       — order a drink (costs chips, rolls a random effect)
```

**⚠️ All drinks have risk! You can lose chips on any drink, including the cheap ones.**

| Drink | Cost | Risk Level | Notes |
|-------|------|------------|-------|
| `beer` | 50 | Low-Moderate | 20% chance of a skunked batch (loss) |
| `wine` | 100 | Low-Moderate | 25% sommelier incident risk |
| `whiskey` | 250 | Moderate | 25% drunk loss risk |
| `tequila` | 150 | High | 50/50 — glory or regret |
| `vodka` | 200 | Moderate | 3-way outcome: legend/cough/meh |
| `rum` | 200 | Moderate | 33% cursed pirate risk |
| `gin` | 300 | Moderate | 25% botanical allergy risk |
| `mojito` | 350 | Moderate | 25% sentient mint risk |
| `mead` | 200 | Moderate | 25% monk curse risk |
| `sake` | 400 | High | 5-way outcome including tragic arc |
| `champagne` | 800 | Moderate-High | 25% trophy destruction risk |
| `margarita` | 300 | Moderate | 20% catastrophic brain freeze |
| `moonshine` | 100 | Very High | 50/50 — see future or lose everything |
| `absinthe` | 500 | Very High | 5-way including Green Fairy robbery |
| `fireball` | 300 | High | 33% mouth fire (loss) |
| `jagerbomb` | 250 | Moderate | 25% vibration accident |
| `longisland` | 600 | Extreme | 4–7 random swings, net result unknown |
| `cosmo` | 350 | Moderate | 25% villain robbery risk |
| `pina` | 400 | Moderate | 25% imaginary ocean chip loss |
| `mystery` | 1,000 | Extreme | 5-way: huge jackpot or big loss or nothing |
| `poison` | 50 | Extreme | 85% lose, 15% jackpot of 3k–10k chips |
| `doubletrouble` | 500 | Very High | 50/50 — 3× win or extra loss |
| `dragonblood` | 750 | Very High | 6-way with massive swings |
| `cursedwine` | 600 | Very High | 5-way haunted vintage |
| `goldenelixir` | 2,000 | Extreme | 10% legendary jackpot, 30% brutal loss |
| `roulettebrew` | 400 | Very High | Literally a roulette spin in drink form |
| `blackout` | 300 | Extreme | 8-way: memory gaps, big wins or losses |
| `thundermead` | 450 | Very High | 5-way electrified mead chaos |
| `devilswhiskey` | 350 | Very High | Devil's cut (mostly loss-skewed) |
| `angelwine` | 800 | High | 5-way celestial judgment |
| `ghostshot` | 200 | High | 5-way spectral outcomes |
| `electriclemonade` | 350 | High | 5-way voltage surge |
| `voiddrink` | 1,500 | Extreme | 10% void jackpot, 30% consumed by void |
| `luckybrew` | 250 | Very High | 6-way pure luck — clover decides your fate |

**Examples:**
```
/bar menu
/bar buy beer
/bar buy mystery
/bar buy goldenelixir
/bar buy voiddrink
```

> Tip: use `/gamble hide` to suppress bar public announcements in the area chat.

---

## Earning Chips Without Gambling

Beyond casino games, there are two non-gambling ways to earn chips.

### 🔤 Unscramble Events

Every **30 minutes to 3 hours** (random) the server posts a scrambled word to all players via OOC broadcast. The **first player to type the correct unscrambled word in IC chat** wins **10 chips**. Once a player claims the reward, the round ends immediately and the server waits for the next scheduled interval before posting a new puzzle.

- Puzzles expire after **5 minutes** if nobody answers.
- The prize is fixed at 10 chips per event.
- Words are drawn from a large, varied pool — no two consecutive rounds use the same word.
- The winner's **typing speed** is recorded and announced to the server.
- Wins are tracked per account for a dedicated leaderboard.
- If you register/login and your IPID changes, your unscramble wins are **automatically merged** onto your new connection — you never lose your score.

| Command | Description |
|---------|-------------|
| `/unscramble` | Show your win count and the current active puzzle (if any). |
| `/unscramble top [n]` | Show the top unscramble winners (top 10 by default, max 50). |

**Example:**
```
🔤 UNSCRAMBLE EVENT! Unscramble this word in IC chat to win 10 chips!
   Scrambled: TONYETAR
   You have 5 minutes. First correct answer wins!

> (player types "attorney" in IC)
🎉 UNSCRAMBLE SOLVED! Phoenix typed "attorney" in 4.32s — +10 chips awarded!
```

---

### 💼 Jobs

Type a job command to earn chips. Each job has a **unique cooldown** and **some have random bonus chances** to keep things interesting.

| Command | Base Reward | Cooldown | Interactive Notes |
|---------|-------------|----------|-------------------|
| `/busker` | 2–6 chips | 30 min | **Random tips** (2–6 chips); performance is **announced in area OOC** so others see you! |
| `/janitor` | 3 chips | 45 min | **25% chance** to find a lost coin (+1 bonus chip). |
| `/paperboy` | 3 chips | 60 min | **15% chance** for a generous reader to tip extra (+2 bonus chips). |
| `/clerk` | 4 chips | 90 min | **15% overtime rush** chance (+2 bonus chips). |
| `/bailiffjob` | 5 chips | 2 hours | **10% chance** to catch suspicious activity (+2 bonus chips). |

> **Job passes** from the `/shop` can permanently reduce these cooldowns and increase chip rewards. See [/shop — Nyathena Shop](#-shop--nyathena-shop) below.

Use `/jobs` to see all available jobs with their rewards and cooldowns at a glance.
Use `/jobtop` to see who has earned the most chips from jobs.

**Examples:**
```
/busker
🎸 [Area OOC] Phoenix is busking outside the courthouse, playing a dramatic Phoenix Wright medley!
🎸 The crowd loved your performance! Generous tips flooded in. +6 chips | Balance: 106 chips

/janitor
🧹 You swept the courthouse floors and found a lost coin on the way out! +4 chips | Balance: 110 chips

/janitor (before cooldown expires)
🧹 You are tired. Come back in 43m 12s to work again.
```

---

## 🛒 `/shop` — Nyathena Shop

Spend your chips on **permanent upgrades** that stay linked to your account forever.

| Command | Description |
|---------|-------------|
| `/shop` | Browse the full shop catalog with prices and descriptions. |
| `/shop buy <item_id>` | Purchase an item by its ID. |
| `/shop items` | List all items you currently own (tags, passes, and passive upgrades together). |
| `/mytags` | List just the tags you own — built-in and staff-granted custom alike — with their ids and which one is active. |
| `/settag <tag_id>` | Equip a purchased cosmetic tag visible in `/gas` and `/players`. |
| `/settag none` | Remove your active cosmetic tag. |

### 🏷️ Cosmetic Tags (30 gambling-themed tags)

Tags are permanent cosmetic labels that appear **next to your name** in `/gas` and `/players` — letting everyone see your level of commitment.

| Item ID | Tag | Price |
|---------|-----|-------|
| `tag_gambler` | [Gambler] | 1,000 chips |
| `tag_lucky` | [Lucky] | 2,500 chips |
| `tag_risk_taker` | [Risk Taker] | 5,000 chips |
| `tag_card_shark` | [Card Shark] | 7,500 chips |
| `tag_high_roller` | [High Roller] | 10,000 chips |
| `tag_patreon` | [Patreon] | 15,000 chips |
| `tag_chip_collector` | [Chip Collector] | 25,000 chips |
| `tag_jackpot` | [Jackpot] | 35,000 chips |
| `tag_casino_regular` | [Casino Regular] | 50,000 chips |
| `tag_hustler` | [Hustler] | 75,000 chips |
| `tag_all_in` | [All In] | 100,000 chips |
| `tag_whale` | [Whale] | 150,000 chips |
| `tag_bluffer` | [Bluffer] | 200,000 chips |
| `tag_odds_defier` | [Odds Defier] | 300,000 chips |
| `tag_dealer` | [Dealer] | 400,000 chips |
| `tag_ace` | [Ace] | 500,000 chips |
| `tag_double_down` | [Double Down] | 600,000 chips |
| `tag_full_house` | [Full House] | 750,000 chips |
| `tag_flush` | [Flush] | 900,000 chips |
| `tag_lucky_charm` | [Lucky Charm] | 1,000,000 chips |
| `tag_bankroll` | [Bankroll] | 1,250,000 chips |
| `tag_fortune` | [Fortune's Fave] | 1,500,000 chips |
| `tag_degenerate` | [Degenerate] | 2,000,000 chips |
| `tag_the_house` | [The House] | 2,500,000 chips |
| `tag_legendary` | [Legendary] | 3,000,000 chips |
| `tag_casino_royale` | [Casino Royale] | 4,000,000 chips |
| `tag_diamond` | [Diamond] | 5,000,000 chips |
| `tag_mythic` | [Mythic] | 6,000,000 chips |
| `tag_godlike` | [Godlike] | 7,500,000 chips |
| `tag_infinite` | [Infinite] | 10,000,000 chips |

### 🏷️ The Other Tag Categories (87 more tags)

The table above is only the gambling category. Six more are browsable in game
with `/shop <category>`, which prints each one with its id, name and price:

| Category | Command | Tags |
|----------|---------|------|
| ⚖️ Ace Attorney | `/shop attorney` | 15 |
| 🌸 Anime | `/shop anime` | 15 |
| 🎮 Gamer | `/shop gamer` | 15 |
| 🌷 Girly | `/shop girly` | 12 |
| 😂 Silly & Memes | `/shop meme` | 20 |
| 👑 Prestige | `/shop prestige` | 10 |

A few from the meme category: `tag_bruh` (100), `tag_sus` (200),
`tag_gigachad` (50,000), **`tag_mrbeast` (75,000 — "You buried 100 chips and
gave the rest away. Subscribe.")**, `tag_goat` (250,000),
`tag_exists_confused` (9,999,999).

Tags are **purely cosmetic** — they have no gameplay effect. Buy them all to collect them!

After purchasing a tag, it is **automatically equipped** as your active tag. You can switch at any time with `/settag <tag_id>`.

### 💼 Job Passes — Cooldown Reduction (stackable)

These passes permanently reduce the cooldown on **all** jobs for your account. All four stack, giving up to **50 minutes** of total reduction (with a minimum 5-minute floor per job).

| Item ID | Pass Name | Price | Benefit |
|---------|-----------|-------|---------|
| `pass_quick` | Quick Worker Pass | 10,000 chips | −5 min from all job cooldowns |
| `pass_speedy` | Speedy Pass | 50,000 chips | −10 min additional |
| `pass_turbo` | Turbo Pass | 150,000 chips | −15 min additional |
| `pass_lightning` | Lightning Pass | 500,000 chips | −20 min additional |

> All four owned: **−50 minutes** off every job. Minimum cooldown is always 5 minutes.

### 💼 Job Passes — Reward Bonus (stackable)

These passes permanently increase the chip reward you earn from **every job**. All four stack for up to **+11 chips** per completion.

| Item ID | Pass Name | Price | Benefit |
|---------|-----------|-------|---------|
| `pass_bonus` | Bonus Chip Pass | 25,000 chips | +1 chip per job |
| `pass_extra` | Extra Chip Pass | 100,000 chips | +2 chips per job |
| `pass_lucky_find` | Lucky Find Pass | 400,000 chips | +3 chips per job |
| `pass_jackpot_seeker` | Jackpot Seeker Pass | 1,000,000 chips | +5 chips per job |

> All four owned: **+11 extra chips** on every job completion.

### Examples

```
/shop
> 🛒 Nyathena Shop — Your balance: 5000 chips
> ── 🏷️  Cosmetic Tags ──
>   tag_gambler        [Gambler]     1000 chips
>   ...

/shop buy tag_gambler
> ✅ Purchased [Gambler] tag for 1000 chips! It is now your active tag.
> Balance: 4000 chips

/gas
> [Gambler] [3] Phoenix Wright   ← your tag is shown!

/settag tag_high_roller
> 🏷️ Active tag set to [High Roller].

/settag none
> 🏷️ Your active tag has been removed.

/shop buy pass_quick
> ✅ Purchased Quick Worker Pass for 10000 chips!
> Permanent benefit: job cooldowns reduced by 5 min
```

---

### `/casinoenable <true|false>`
Enable or disable the casino for the current area.

```
/casinoenable true
/casinoenable false
```

### `/casinoset <setting> <value>`
Configure casino settings for the current area.

| Setting | Type | Description |
|---------|------|-------------|
| `minbet` | integer | Minimum bet in chips (0 = no limit) |
| `maxbet` | integer | Maximum bet in chips (0 = no limit) |
| `maxtables` | integer | Maximum simultaneous active tables (0 = no limit) |
| `jackpot` | true/false | Enable or disable the slots jackpot for this area |

**Examples:**
```
/casinoset minbet 10
/casinoset maxbet 500
/casinoset maxtables 2
/casinoset jackpot true
```

---

### `/grantchips <uid> <amount>` *(Admin only)*
Grant any number of chips to an online player by their UID. There are no transfer limits or cooldowns — this command is for admin use only.

```
/grantchips 3 500
> Granted 500 chips to Phoenix. Their new balance: 850 chips.
```

The target player also receives a notification:
```
> An admin granted you 500 Nyathena Chips! Your new balance: 850 chips.
```

---

## `/areainfo` Integration

`/areainfo` now includes a **Casino** line showing:
- Whether the casino is enabled or disabled.
- Bet limits (if configured).
- Current jackpot pool (if jackpot is enabled).

**Example output:**
```
BG: courtroom
Evi mode: Mods
...
Casino: enabled (bet: 10–500), jackpot pool: 1200
```

---

## Anti-spam / Rate Limits

| Feature | Limit |
|---------|-------|
| Slots spins | Max 5 spins per 10 seconds |
| Chips give cooldown | 10 minutes between transfers |
| Chips give max | 200,000 chips per transfer |
| Chips give playtime gate | 24 hours total playtime required |
| BJ/Poker turn timer | 60 seconds — auto-stand/fold |
| Crash cooldown | 45 seconds between bets |
| Crash minimum hold | Must hold for 5 seconds before cashout (instant cashout = loss) |

---

## Persistence

All chip balances are stored in the server's SQLite database (`CHIPS` table). Balances survive server restarts. New IPIDs receive 100 chips on first connection.

# Nyathena — Player Commands Guide

Every command in here is available to regular players (no moderator privileges required). Cooldowns and exceptions are noted inline.

For moderator-only commands, see [`MOD_COMMANDS.md`](MOD_COMMANDS.md). For casino games, see [`CASINO_COMMANDS.md`](CASINO_COMMANDS.md). For the social-deduction Mafia minigame, see [`MAFIA_COMMANDS.md`](MAFIA_COMMANDS.md).

---

## Quick Reference

```
/help                    Browse command categories
/help <category>         List commands in a category (e.g. /help general)
/help <command>          Show usage for a specific command
/about                   Server version + fork credits
```

---

## General / Movement

| Command | Description |
|---------|-------------|
| `/area <name>` | Move to a named area |
| `/areas` | List all areas |
| `/areainfo` | Show settings for the current area |
| `/areadesc` | Show this area's entry description |
| `/ga` | List players in your current area |
| `/gas` | List players in **all** areas (empty areas are hidden) |
| `/players` | Same as /ga |
| `/find <name>` | Find which area a player is in |
| `/pos [pos]` | Show or set your IC position (def, pro, wit, jud, hld, hlp) |
| `/charselect` | Return to character select |
| `/move [-u <uids>] <area>` | Move to an area (the `-u` form needs `MOVE_USERS`) |
| `/randomchar` | Switch to a random free character (5s cooldown — DJs and mods bypass it) |
| `/wardrobe [name\|number]` | View your favourite characters, or swap straight to one |
| `/favourite <char name>` | Add or remove a character from your wardrobe favourites |
| `/dance` | Toggle dance mode (sprite flips on every IC message) |
| `/narrator` | Toggle narrator mode |
| `/dc` / `/dctime [minutes\|off\|status]` | **Opt-in** idle auto-disconnect that affects **only you** — no number arms a 1-hour timer. Any IC or OOC message resets the countdown. Off by default; `/dctime off` cancels. |
| `/motd` | Re-send the server's message of the day |
| `/about` | Server version and fork credits |
| `/kickother` | Kick stale ghost connections sharing your HDID |

---

## Chat

| Command | Description |
|---------|-------------|
| `/global <message>` (or `/g`) | Send a server-wide OOC message. Shows your `[tag]` like local OOC. Filtered exactly like ordinary chat. |
| `/pm <uid> <message>` | Private message a specific player |
| `/ignore <uid>` / `/ignore list` | Permanently ignore a player (survives reconnects), or list who you're ignoring. Real moderators and admins cannot be ignored; shadow mods can. |
| `/unignore <uid\|number>` | Lift an ignore — by UID, or by the number from `/ignore list` for someone offline |
| `/erp` | Toggle the area's ERP mode (if allowed) |
| `/8ball <question>` | Ask the Magic 8-Ball. Answers come from `8ball.txt` or a built-in classic list. |
| `/translate <text> <language>` | Translate text with DeepL (25-second cooldown) |
| `/getmusic` | Show the URL of the song playing in this area and re-send the MC packet to just you (handy when your client's audio bugged out). |
| `/randomsong` | Play a random track. Tiered cooldown: 20s for players, 5s for DJs, none for mods. |
| `/vlist` | List the voice-chat participants in your area (servers with `enable_voice`) |

---

## Pairing

| Command | Description |
|---------|-------------|
| `/pair <uid>` | Request to pair with a player. Mutual `/pair` finalizes the pairing. Messages reference each player's **showname** (in-character name) when set. |
| `/unpair` | Cancel your pair. Full bidirectional reset — clears state on every peer that referenced you, so no desyncs. |
| `/lfp` | Toggle your **Looking-For-Pair** flag. Flagged players show up in `/pairlist`. |
| `/pairlist` | List everyone in your area flagged `/lfp`, with UID, name and character — then `/pair <uid>` away. |

---

## Area, Evidence and Testimony

| Command | Description |
|---------|-------------|
| `/doc [-c] [url]` | Show or set the area's case document |
| `/areadesc [-c] [text]` | Show or set the area's entry description |
| `/bg <background>` | Set the area's background (subject to the area's BG lock; DJs are limited to once a minute) |
| `/bglist` | List every available background |
| `/swapevi <id1> <id2>` | Swap two pieces of evidence |
| `/testimony <record\|stop\|play\|update\|insert\|delete>` | Drive the area's testimony recorder. Witnesses must be in `/pos wit` for their lines to be captured. |
| `/examine` | Start cross-examination playback |
| `/vote <option>` | Vote on the area's active poll |
| `/cvote <action> <uid> [reason]` | Community moderation vote (kick/mute/ban/warn/areakick). Moderators have the final say. |

---

## Running a room (area CM)

`/cm` makes you a **case manager** for the area you are standing in — no moderator role required, as long as the area allows CMs. Everything below is then available to you (and to moderators, who hold the same `CM` permission everywhere).

| Command | Description |
|---------|-------------|
| `/cm [uids]` | Become an area CM (or promote others) |
| `/uncm [uids]` | Step down (or demote others) |
| **`/area rename <name>`** | **Name the room you are in** — `/area rename DR Killing Game`. See below. |
| **`/area unrename`** | Put the configured name back straight away |
| `/area mute` / `/area unmute` | Silence everyone in the room except CMs and moderators (IC and OOC), and lift it. Scoped to the room: leaving the area lifts it automatically. |
| `/lock` / `/unlock` | Lock the area (current occupants are auto-invited) |
| `/lock -s` | Set the area spectatable |
| `/spectate [invite\|uninvite <uids>]` | Toggle spectate mode — everyone may enter and watch, only CMs and invited players may speak IC. Listed in `/help` for every player so anyone can look up what it does. |
| `/invite <uids>` / `/uninvite <uids>` | Grant or revoke entry in a locked area, and IC speaking rights in spectate mode |
| `/kickarea <uids>` | Eject players from the area (they also drop off the invite list, so they cannot walk back in) |
| `/forcepos <uids\|all> <position>` | Stage a scene: push players in your area into a courtroom position |
| `/status <idle\|lfp\|casing\|recess\|rp\|gaming>` | Set the area status shown on the area list (`lfp` is short for `looking-for-players`) |
| `/evimode <any\|cms\|mods>` | Set who may edit evidence |
| `/randombg` | Random background from the server list (once every 5s) |
| `/musiclock` | Restrict music changes to CMs and moderators |
| `/poll [question]\|[option]\|[option]...` | Open a poll in the area |
| `/togglerandompunish` | Toggle the area's random-punishment rolls |

### Naming your room — `/area rename`

```
/area rename DR Killing Game     # the area list now reads "DR Killing Game"
/area rename                     # show the current name and the configured one
/area unrename                   # restore the configured name right now
```

The rename reaches everyone who is already connected — nobody has to reconnect to see it.

**It is a loan.** The name in the server's `areas.toml` is the area's real identity, and it comes back on its own when the room stops belonging to anyone:

- the **last person leaves** the area, or
- the area **loses its last CM** — they ran `/uncm`, moved elsewhere, or disconnected.

So you never have to remember to undo one, and a restart returns every area to its configured name.

**Names are checked before they are accepted:** 1–32 characters, no `#`, `%`, `$` or `&` (AO2 spends those on its packet format), no control characters, not the same as a music track or another area's name, and it goes through the same banned-word filter as IC and OOC chat — including the same evasion-resistant matching and the same consequences. A room name is shown to every connected player, so it is held to exactly the standard a line of chat is.

---

## Accounts (when accounts/casino are enabled)

| Command | Description |
|---------|-------------|
| `/register <username> <password>` | Create a free player account. Captcha confirmation required by default. |
| `/captcha <token>` | Confirm a pending registration |
| `/verify <answer>` | Answer the verification question shown when you joined (only on servers with `join_captcha` on). You can also just type the answer in OOC. |
| `/login <username> <password>` | Sign in to your account |
| `/logout` | Sign out |
| `/account` | View your account info |
| `/profile [uid]` | Show a profile card. DJs get a 💿 vinyl badge. |
| `/playtime` | Show the playtime leaderboard (page 1, 25 entries) |
| `/playtime top <page>` | Browse subsequent pages of 25 |
| `/resetusername <new>` | Rename your account (keeps playtime/chips/wardrobe). Capped at **3 renames per account**. |

---

## Mini-games

| Command | Description |
|---------|-------------|
| `/rps <rock\|paper\|scissors>` | **PvP** rock-paper-scissors. The first call posts an open challenge with a hidden choice; the second player commits blind and the result is announced. 30s window per player. |
| `/coinflip <heads\|tails>` | Area-scoped 30-second PvP coinflip — opposite sides only |
| `/roll <n>d<m>` | Roll dice (e.g. `/roll 2d6`) |
| `/maso [-d duration]` | Apply a random punishment to yourself (default 10 min, max 24 h). Re-roll by typing it again. |
| `/megamaso [-d duration]` | Like `/maso` but **stacking**: each repeat adds another random punishment to the pile (default 10 min per layer, max 24 h). |
| `/hotpotato [accept\|pass]` | Hot Potato: don't be holding it when the timer stops |
| `/quickdraw <uid>` / `/quickdraw bullet <uid>` | Duel another player. Standard: type a random word first. Bullet: first to send any IC message wins. The loser gets a random punishment. |
| `/russianroulette [join]` | Russian Roulette — the unlucky loser takes a wild random punishment |
| `/roulette join` | Area roulette |
| `/typingrace [join]` | First to type the phrase wins chips |
| `/hangman start [theme\|custom <word>]` | Hangman with themed or custom words; wrong guessers are punished on a loss |
| `/giveaway start <item>` / `/giveaway enter` | Run or enter a giveaway |
| `/unscramble [top [n]]` | Your unscramble wins, or the leaderboard. Answer live puzzles in IC to win chips. |

---

## Jobs (earn chips without gambling)

| Command | Cooldown |
|---------|----------|
| `/jobs` | — (lists them all) |
| `/busker` | 30 minutes |
| `/janitor` | 45 minutes |
| `/clerk` | 90 minutes |
| `/paperboy` | 60 minutes |
| `/bailiffjob` | 2 hours |
| `/jobtop [n]` | — (job earnings leaderboard) |

---

## Potions (self-applied effects)

```
/potions                      # menu
/potion <name>                # drink one (default 5 min)
/potion -d <duration> <name>  # custom duration, e.g. /potion -d 30m drunk (max 24h)
/potion off                   # flush every active potion
```

| Potion | Effect |
|--------|--------|
| `drunk` | Slurs and shuffles letters |
| `uwu` | Wewites yowo wowds wike this |
| `shy` | Stuttering, hesitant speech |
| `dramatic` | Shakespearean tongue |
| `pirate` | Yarrr! |
| `poet` | Poetic flourish |
| `caveman` | Talk simple. Words short. |
| `fancy` | Unicode fancy characters |
| `chef` | Swedish-Chef-isms |
| `cherri` | Capitalizes Every Word |
| `omnidere` | Each line picks a random anime dere flavour |
| `zalgo` | C̴o̷r̶r̸u̵p̷t̶s̸ your text with creeping zalgo marks |
| `love` | 💘 Auto-sends a **pair request** to the next player who speaks in your area. Consent preserved — they still accept with `/pair`. If you were already requesting each other, the pair completes instantly. |
| `character` | Auto-rotates your character every 30 seconds |

---

## Punishment Self-Service

| Command | Description |
|---------|-------------|
| `/punishments` | List your own active punishments with remaining durations — including lag, mute, and jail. Great for "wait, why am I still speaking pig latin?" |

---

## Tournament (opt-in)

| Command | Description |
|---------|-------------|
| `/join-tournament` | Join an active punishment tournament. You'll get 2–3 random punishments; whoever sends the most IC messages wins. |

---

## Custom Tags (cosmetic, requires account)

| Command | Description |
|---------|-------------|
| `/shop` | Browse shop tags |
| `/buytag <id>` | Buy a tag (chips required if casino enabled) |
| `/settag <id>` | Equip a tag — shows `[Tag Name]` in `/gas`, `/players`, OOC, `/global` |
| `/cleartag` | Remove your equipped tag |
| `/listcustomtags` | List every staff-minted custom tag |

---

## Notes

- **Permission gating**: Everything in this guide is callable by every connected player, except the "Running a room" section, which needs the `CM` permission — and `/cm` hands that to any player in an area that allows CMs.
- Moderator-only commands (mute, kick, ban, the punishment commands, `/firewall`, `/lockdown`, etc.) are documented in `MOD_COMMANDS.md`.
- **Cooldowns**: Many commands have small per-user cooldowns. The 5s `/randomchar` cooldown is bypassed for DJs and moderators.
- **Tags in OOC**: Local OOC and `/global` both show your equipped tag in front of your name as `[Tag] Name`.

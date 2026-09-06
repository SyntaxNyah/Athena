![Athena logo](resource/logo.png)

# Nyathena

**Nyathena** is a fork of [Athena](https://github.com/MangosArentLiterature/Athena) a lightweight AO2 (Attorney Online 2) server written in Go. It is maintained by [SyntaxNyah](https://github.com/syntaxnyah) with huge credits to [OmniTroid](https://github.com/OmniTroid) fur supporting this project with many bug fixes, improvements and code cleanup. Huge shoutouts to them for the support.  Full credit for the original server, its design, and everything it's built on goes to [MangosArentLiterature](https://github.com/MangosArentLiterature).

This started as a personal server project and kind of spiralled. One punishment command becoming ten, a casino system, tag system, account system, a bunch of mini games and the whole time it kept being *fun* to add things, which is not always how software development goes. That's entirely down to how well Athena is built. Adding a new feature rarely meant slow build times or having to debug for hours in the codebase. It usually just meant writing the thing.

This fork is a **120+ command punishment system** designed to give moderators a ridiculous, embarrassing amount of creative control over players to abuse as much as they want and mess with whether it be; backwards text, forced haiku, dere archetypes, voice corruption, contagious plagues, area traps, reversed message order, coinflip PvP challenges. It's purposefully excessive and a little sadistic, and that's the point. Abuse players to your hearts content! Beyond that: a Discord bot, a casino with 11 games, a full Mafia minigame, server-relayed voice chat over websockets, and a lot of smaller quality-of-life things that accumulated over time.

---

## What's Different From Upstream Athena

### Punishment System (120+ Commands)

This is the main thing Nyathena exists for. All punishment commands support `-d <duration>`, `-r <reason>`, `-h` (hidden — applies silently without notifying the target), and the `global` keyword (applies to every non-moderator in the area). They accept comma-separated UIDs, auto-expire, and stack freely. Check what's on a player (or yourself) anytime with `/punishments [uid]`.

**Text Effects (60):** `/whisper`, `/backward`, `/stutterstep`, `/elongate`, `/uppercase`, `/lowercase`, `/robotic`, `/alternating`, `/fancy`, `/uwu`, `/pirate`, `/shakespearean`, `/caveman`, `/censor`, `/fromsoftware`, `/confused`, `/paranoid`, `/drunk`, `/hiccup`, `/whistle`, `/mumble`, `/slang`, `/cherri`, `/albhed`, `/morse`, `/vowelhell`, `/upsidedown`, `/autospell`, `/thesaurusoverload`, `/valleygirl`, `/babytalk`, `/thirdperson`, `/unreliablenarrator`, `/uncannyvalley`, `/chef`, `/karen`, `/passiveaggressive`, `/nervous`, `/sarcasm`, `/academic`, `/philosopher`, `/poet`, `/quote`, `/spaghetti`, `/essay`, `/rng`, `/haiku`, `/dreamsequence`, `/timewarp`, `/zalgo`, `/leetspeak`, `/smallcaps`, `/piglatin`, `/vaporwave`, `/lisp`, `/spoonerism`, `/keysmash`, `/weeb` (350+ romaji corpus), `/politician`, `/clickbait`, `/markov` (babble brewed from the area's own chat history), `/alliteration`, `/cipher` (escalating ROT13 → BINARY → BASE64)

**Themed Quote Replacers (10):** `/gordonramsay` (60+ kitchen tirades), `/biblebot`, `/grounded`, `/mime`, `/subtitles`, `/spotlight`, `/recipe`, `/rickroll`, `/pickup`, `/brainrot`

**Persona / Personality (5):** `/clown`, `/jester`, `/joker`, `/tourettes`, `/translator` (via DeepL API)

**Dere Archetypes (26):** `/tsundere`, `/yandere`, `/kuudere`, `/dandere`, `/deredere`, `/himedere`, `/kamidere`, `/undere`, `/bakadere`, `/mayadere`, `/smugdere`, `/deretsun`, `/bokodere`, `/thugdere`, `/teasedere`, `/dorodere`, `/hinedere`, `/hajidere`, `/rindere`, `/utsudere`, `/darudere`, `/butsudere`, `/sdere`, `/mdere`, `/tsuyodere`, and `/omnidere` (random dere flavour on every single message — maximum tonal whiplash).

**Animal Filters (12):** `/monkey`, `/snake`, `/dog`, `/cat`, `/bird`, `/cow`, `/frog`, `/duck`, `/horse`, `/lion`, `/zoo`, `/bunny`

**Visibility / Cosmetic (8):** `/emoji`, `/invisible`, `/shrink`, `/grow`, `/wide`, `/areainiswap`, `/hidedisplay` (push the target's own sprite off-screen — funny on pairs), `/forcedisplay` (pin the target's character onto every IC sprite in the area — nobody else's character shows)

**Protocol / Viewport (6):** `/teleport` (sprite pops to random screen positions), `/shakecurse` (forced screenshake), `/randomflip` (coin-flipped sprite facing), `/forcecolor` (locked text colour — all-red rage or full rainbow), `/nopreanim`, `/forcepreanim`

**Timing (3):** `/slowpoke`, `/fastspammer`, `/lifo` (messages release in REVERSE order — say three things, they arrive backwards)

**Traps & Contagion (4):** `/contagious <type>` (plague mode — spreads to anyone who speaks within 5s of an infected player; mods immune), `/minefield` (1-in-6 chance per message to detonate a random 2-minute punishment), `/silencebell` (area trap: the next person to speak gets cursed), `/stealthmute` (messages echo back to the sender but reach nobody — they never know)

**Audio (2):** `/sfxcurse <uid> <sfx-url>` (forces an SFX on every IC message), `/unsfx`

**Stacking / Chaos:** `/stack` (supports many punishments examples including but not limited to;  `global`), `/torment`, `/lovebomb`, `/degrade`, `/emoticon`, `/51`, `/icwarp`, `/randompunishall`, `/togglerandompunish`
```
/stack <type1> <type2> [...] [-d duration] [-r reason] [-h] global | <uid>
```

**Tournament mode:** `/tournament start|status|stop` — volunteers get 2–3 random punishments; most IC messages sent wins.

**Self-applied:** `/maso` (random single effect, rerolls on repeat) and `/megamaso` (each call stacks another random punishment on top — you can pile on as many as you like).

**PvP:** `/coinflip <heads|tails>` — area-scoped 30-second challenge where both players must pick opposite sides.

**Remove:** `/unpunish <uid>` (everything including lag), `/unpunish -t <type> <uid>` (specific), or `/unpunish all` (entire area). Regular mods cannot lift punishments applied by an admin or shadow mod.

### Server-Relayed Voice Chat (`enable_voice = true`)

> **Compatible clients only.** Voice chat requires a WebSocket-capable AO2 client that implements the Nyathena voice protocol. The two known working clients are **[webao.miku.pizza](https://webao.miku.pizza)** (SyntaxNyah's hosted WebAO instance) and [**LemmyAO**](https://github.com/syntaxnyah/lemmyao), SyntaxNyah's fork of the AO2 desktop client. Standard AO2 and unmodified WebAO builds do not support voice and will simply not see the voice channel.

Audio travels client → Athena → area peers. There is no peer-to-peer path, so players never learn each other's IPs from the voice connection. The server relays 20 ms Opus frames at 48 kHz as opaque base64 blobs — it never decodes audio, so the relay is CGO-free and codec-agnostic.

Voice punishment commands (manipulate the frame flow, not the audio signal itself):
- `/voicemute` — drops every frame; silent to the room
- `/voicestatic` — drops ~60% of frames; choppy and breaking up
- `/voicegarble` — drops ~88% of frames; barely intelligible
- `/voicecutout` — gates frames on a ~650 ms on/off cycle; walkie-talkie effect
- `/voicestutter` — randomly replays the previous frame; glitchy voice stutter

Moderation: `/vmute`, `/vban`, `/voicearea on|off`, per-area join and frame rate limits, new-IPID cooldown.

### Casino System (`enable_casino = true`)

Virtual currency (**Nyathena Chips**) stored in SQLite. New players start with 500 chips; cap is 10,000,000.

| Game | Command |
|------|---------|
| Blackjack (6-deck, split/double/insurance, up to 6 players) | `/bj` |
| Texas Hold'em (up to 9 players, 500-chip buy-in) | `/poker` |
| Slots with area jackpot pool | `/slots` |
| European Roulette (single-zero) | `/croulette` |
| Baccarat (player / banker / tie) | `/baccarat` |
| Craps (pass / don't-pass) | `/craps` |
| Crash multiplier | `/crash` |
| Minesweeper grid (1–24 mines, 5×5) | `/mines` |
| Keno (pick 1–10 from 1–80) | `/keno` |
| Prize wheel (~92.5% RTP) | `/wheel` |
| Bar (33 drinks of wildly varying variance) | `/bar` |

Economy: `/chips`, `/chips top`, `/chips give`, `/richest`. Earn passively via jobs (`/busker`, `/janitor`, `/paperboy`, `/clerk`, `/bailiffjob`) or win unscramble events that fire every 30 min–3 h.

Shop (`/shop`): 30 cosmetic tags (1,000–10,000,000 chips), job cooldown reduction passes, job reward bonus passes.

### Discord Bot Integration

A full Discord bot with slash commands, mod-bridge embeds, area/player listings, and live security toggles — so moderators can respond to incidents without being logged into the AO server.

| Discord slash | Effect |
|---------------|--------|
| `/firewall on\|off` | Toggle IPHub VPN/proxy firewall |
| `/lockdown on\|off` | Toggle server-wide new-IPID lockdown. `on` also instantly kicks every connected non-moderator under the configured minimum-playtime threshold — the flood that's already inside, not just the next wave trying to join — and silently drops (never broadcasts) messages from anyone still under that threshold for as long as lockdown stays active. |
| `/lockdown whitelist_all` | Whitelist every currently-connected IPID |

### Mafia Social Deduction Minigame

Full in-server Mafia (4–20+ players) with automatic role pools, day/night phases, lynch voting, night actions, last wills, graveyard, whisper, and phase timers.

**Town:** Villager, Detective, Doctor, Sheriff, Bodyguard, Vigilante, Mayor, Escort  
**Mafia:** Mafia, Shapeshifter, Godfather  
**Neutral:** Jester, Witch, Lawyer, Arsonist, Serial Killer, Survivor

See `MAFIA_COMMANDS.md` for the full reference.

### Potions System

Self-applied fun effects via `/potion <name>` (default 5 min, `-d` to override). `/potions` lists the cabinet; `/potion off` flushes everything.

Available potions: `drunk`, `uwu`, `shy`, `dramatic`, `pirate`, `poet`, `caveman`, `fancy`, `chef`, `cherri`, `omnidere`, `zalgo` (corrupts your own text), `love` (💘 auto-sends a pair request to the next player who speaks — consent preserved), `character` (auto-rotates your sprite every 30 seconds).

### AutoMod

Word-list-based enforcement covering IC text, IC showname, OOC text, and OOC username. Actions: `shadow` (default), `ban`, `kick`, `mute`, or `torment`.

With the default `shadow` action a censored message is **shadow-sent**: the sender's client shows it as sent, but the packet is dropped for every other client — nobody sees the slur. The speaker's IPID is also added to the torment list, and every censor trip alerts online moderators in OOC (mods can opt out per-session with `/censoralerts off`). Inspect the list with `/tormentlist`; lift entries with `/untorment <ipid>` or purge everything with `/untorment all`.

### Join Captcha (`join_captcha = true`)

Makes a previously-unseen IPID answer one question before it can send IC or OOC
messages, run most commands, or change the music. Connecting, looking around,
picking a character and moving between areas all still work while unverified —
it gates speech, not the connection.

It targets raids specifically. A flood arrives from many IPIDs and HDIDs at once,
so bans and per-IP limits are always a step behind it; what none of those
connections can do is answer a question.

- **Nothing to copy.** The answer is never printed in the question. "Type ABC123
  to verify" is beaten by three lines of regex however random ABC123 is, so
  nothing here works that way — the answer always has to be derived (add two
  numbers written as words, reverse a token, count things, continue a sequence,
  pick out non-adjacent characters). Fifteen kinds, several phrasings each, and
  a randomised wrapper around all of them. The property is enforced at
  generation time, not just intended: a draw that would leak its own answer is
  re-rolled.
- **Running out of strikes mutes rather than kicks.** A kick is a loud,
  immediate signal, and there is no reason to hand one out for free. Set
  `join_captcha_action = "kick"` if you would rather disconnect them.
- **Can't be rerolled.** Challenges are derived as HMAC-SHA256(secret, IPID +
  time window), so reconnecting returns the *same* question — a bot whose solver
  only handles arithmetic can't reconnect until it draws one.
- **Can't be beaten from the source.** This repository is public, so the
  built-in questions are too. Write your own in `config/captcha_questions.txt`
  (hot-reloadable, never in the repo), or point `captcha_plugin` at your own
  program and the server stops generating questions entirely — see
  [docs/CAPTCHA_PLUGIN.md](docs/CAPTCHA_PLUGIN.md). In the plugin's token mode
  the answer never enters the server process at all.

**Regulars are never asked.** Moderators, anyone who has passed it once (stored
by IPID, surviving restarts), and any IPID with 5+ hours of accumulated playtime
(`join_captcha_min_playtime`) skip it outright.

The question is shown in OOC and in a client-side popup (desktop AO2 and WebAO).
Players answer with `/verify <answer>`, or by just typing the answer in OOC.
Signing in with `/login` clears it. Staff use `/joincaptcha status`, and
`/joincaptcha verify <uid>` to release a false positive — messages are kept in
the area log, tagged, and staff are alerted whenever it acts.

### IPHub VPN Firewall

When `iphub_api_key` is set, new connections are checked against IPHub. Known IPs are cached permanently so repeat visitors never cost an API call (respects the 1,000 req/day free tier). Toggle in-game or via Discord.

### Custom Tags (Moderator/Admin)

Moderators and admins create cosmetic tags at runtime without touching config files or rebuilding.

```
/createtag <id> <display name>       # MUTE
/grantcustomtag <username> <tag_id>  # MUTE
/revokecustomtag <username> <tag_id> # ADMIN
/deletetag <id>                      # ADMIN
/listcustomtags
```

Custom tags never appear in `/shop` and can only be granted by staff.

### Per-Area Logging

Daily-rotating log files per area under `logs/<AreaName>/`. Format: `[HH:MM:SS] | ACTION | CHARACTER | IPID | HDID | SHOWNAME | OOC_NAME | MESSAGE`. Enable with `enable_area_logging = true`.

### Persistent Pairing

UID-based mutual pairing that survives area and character changes. Pair messages use each player's showname when set. `/unpair` does a full bidirectional reset — no stale pair state left on the peer. Flag yourself `/lfp` (Looking For Pair) and browse the area's flagged players with `/pairlist`.

### Other Additions

- **`/punishments [uid]`** — the punishment dashboard: every active effect with remaining duration; players self-inspect, mods inspect anyone (includes lag/mute/jail and issuer tier)
- **`/clients <uid>`** — every connection sharing a target's IPID at a glance (multiclient overview, MUTE)
- **`/area rename <name>`** — CMs and moderators name the room they are standing in (`/area rename DR Killing Game`). The name is a loan: it reverts to the `areas.toml` default when the room empties or loses its last CM, and it goes through the same word filter as IC and OOC. `/area unrename` restores it early. See [docs/MOD_COMMANDS.md](docs/MOD_COMMANDS.md#renaming-an-area-area-rename)
- **`/area mute` / `/area unmute`** — silence everyone in the room except CMs and moderators; scoped to the room, so leaving lifts it
- **Doki Area Effect** — literature-club-themed chaos per area (`doki_area = true` in `areas.toml`)
- **Persistent `/musicban`** — bans an IPID from playing music across sessions; bypassed in areas with fewer than 3 people. `/musicunban` and `/musicbans` round out the set
- **Hot `/reload`** — atomic, race-free reload of `characters.txt` (append-only), `music.txt`, `cdns.txt`, `backgrounds.txt`, `parrot.txt`, `8ball.txt`, `banned_words.txt` and `config.toml` motd/desc without restarting; also bound to stdin `reload` and `SIGHUP`
- **Shadow Mod Visibility** — shadow mods are invisible in `/gas`/`/players` to non-admins
- **`/charshuffle` / `/uncharshuffle`** — Sattolo-permutes everyone's character sprite; originals remembered
- **`/reversename` / `/unreversename`** — flips showname rune order; stacks cleanly with `/forcename`
- **`/charcurse <uid> <charname>`** — one-time forced character swap
- **`/rps`** — PvP rock-paper-scissors with hidden simultaneous commitment
- **`/randomsong`** tiered cooldown (user / DJ / mod)
- **`/randomchar`** DJ and mod cooldown bypass
- **`/getmusic`** — re-sends the current music packet to the requester's client
- **`/8ball <question>`** — customisable response pool via `config/8ball.txt`
- **`/resetusername`** — account rename (capped at 3 per account)
- **`/playtime top`** pagination (25 per page, pass a page number)
- **`/reloadplaytime`** — admin IPID re-link and orphan playtime merge
- **`/profile`** DJ insignia (💿 shown for players with the DJ permission bit)
- **`/global`** shows the sender's tag in the prefix
- **`/gas`** hides empty areas with a count at the bottom
- Locked-room kick lockout bug fix (kicked players can no longer walk back in)
- Hot Potato, Quick Draw, Chip Giveaway, Area Roulette minigames
- Wardrobe management, in-place server restart via `syscall.Exec`

### Removed

- **`/possess`, `/fullpossess`, `/truepossess`, `/unpossess`, `/shadowdisconnect`, `/lag`** — these applied an effect the person on the receiving end could not see: speaking as somebody while silencing them, a disconnect indistinguishable from a bad router, ghosted messages for no stated reason. Nothing to notice, report or appeal is a bad property for a moderation tool, so they are gone rather than gated. The AutoMod censor still arms the torment list automatically, `/tormentlist` and `/untorment` still manage it, and the server console keeps `torment <ipid>` / `untorment <ipid|all>`. `/shadowundisconnect` and `/shadowdisconnectlist` remain so entries written before the removal can still be lifted.

---

## Inherited Features (Upstream Athena)

- WebAO (plain WebSocket)
- Concurrent client handling
- Moderator user system with configurable TOML roles and permission bitfields
- Privacy-oriented logging
- Testimony recorder
- CLI command parser (`mkusr`, etc.)
- bcrypt password storage

---

## Quick Start

```bash
make build
cp -r config_sample config
# edit config/config.toml, areas.toml, roles.toml, etc.
./bin/athena
# then in the CLI prompt:
mkusr
```

Pass `-c /path/to/config` for a custom config directory. Pass `-nocli` to disable stdin.

Binaries for common platforms are on the [releases page](https://github.com/syntaxnyah/nyathena/releases).

---

## Configuration

Key options beyond upstream defaults (`config/config.toml`):

| Key | Purpose |
|-----|---------|
| `enable_casino` | Enable casino and player accounts |
| `iphub_api_key` | IPHub key for VPN/proxy firewall |
| `automod_enabled` / `automod_action` | AutoMod word enforcement |
| `enable_voice` | Server-relayed voice chat |
| `enable_area_logging` | Per-area rotating log files |
| `webhook_url` | Discord webhook for modcall notifications |
| `punishment_webhook_url` | Discord webhook for ban/kick embeds |
| `[Discord] bot_token` / `guild_id` | Discord bot credentials |

See `CLAUDE.md` for the full configuration reference.

### WSS Setup

**Via reverse proxy (recommended for Cloudflare):**
```toml
enable_webao_secure = true
webao_secure_port   = 443
# leave tls_cert_path / tls_key_path empty
```

**Direct TLS:**
```toml
enable_webao_secure = true
webao_secure_port   = 443
tls_cert_path       = "/path/to/cert.crt"
tls_key_path        = "/path/to/key.key"
```

---

## Building From Source

Go 1.19+ required.

```bash
make build    # produces bin/athena
make test     # go test -v -race ./...
make all      # build + test
make release  # goreleaser
```

---

## License

GNU Affero General Public License v3.0, inherited from upstream Athena.

---

## Credits

### Upstream Athena by MangosArentLiterature

None of this would exist without **[Athena](https://github.com/MangosArentLiterature/Athena)** and [MangosArentLiterature](https://github.com/MangosArentLiterature)'s work on it.

Picking Go for an AO2 server was genuinely one of the best decisions that could have been made. And the result is a codebase that stays readable under the kind of chaotic mess that Nyathena has become. Adding a new command never feels like a chore its super simple and easy.

Spending months adding feature after feature to Nyathena and genuinely enjoying it the whole time  is a testament to how clean the original upstream base is is. 

The package layout makes alot of sense. The addition of using a TOML config is easy to extend. The permission system is flexible without being overengineered and I was able to extend it to things such as full on account systems. Every time something new needed to be integrated in, it went in without too much trouble. The discord bot, the casino, the voice relay, the punishment system with its considerly bloated amounts of code, all of it hooks into structure that MangosArentLiterature laid down first, and all of it hangs together because the base is solid.

If you're running an AO2 server and don't need the chaos layer, **please use upstream Athena**. It's focused, well-tested, and exactly what it sets out to be. Nyathena is the chaotic abusive insanity fork that spiraled way way way further  away from upstream Athena. If you want a solid AO experience without any bloat, it's recommended to use the real thing.

Genuine thanks and full credit to MangosArentLiterature for building something this good to start from.

---

**Nyathena** fork and most additions by [SyntaxNyah](https://github.com/syntaxnyah) with support, extra additions, bug fixes and cleanup from [OmniTroid](https://github.com/OmniTroid).

# Nyathena

## Project Overview

Nyathena is a fork of [Athena](https://github.com/MangosArentLiterature/Athena), a lightweight AO2 (Attorney Online 2) server written in Go. It extends upstream Athena with a large set of original features:

- A full **Discord bot** integration (slash commands, embeds, moderation bridge)
- A **casino system** with 10 distinct games and persistent virtual currency ("Nyathena Chips")
- A **Mafia social-deduction minigame** playable inside any server area
- **120+ punishment commands** for moderators, with stacking, tournaments, contagion/trap mechanics, and a coinflip challenge system
- Persistent pairing, per-area logging, configurable rate limiting, AutoMod, IPHub VPN firewall, and more

Module path (retained from upstream): `github.com/MangosArentLiterature/Athena`

## Tech Stack

| Dependency | Version | Purpose |
|-----------|---------|---------|
| `github.com/bwmarrin/discordgo` | v0.28.1 | Discord bot (Gateway, slash commands, embeds) |
| `github.com/ecnepsnai/discord` | v1.2.1 | Discord webhook support |
| `github.com/BurntSushi/toml` | v1.2.0 | TOML config parsing |
| `modernc.org/sqlite` | v1.18.0 | SQLite database (no cgo required) |
| `golang.org/x/crypto` | — | bcrypt password hashing |
| `nhooyr.io/websocket` | v1.8.7 | WebAO WebSocket support |
| `github.com/gorilla/websocket` | v1.4.2 | Additional WebSocket (indirect) |
| `github.com/xhit/go-str2duration/v2` | v2.0.0 | Duration string parsing for punishment timers |

**Go version requirement:** 1.19

## Architecture

### Entry Point

`athena.go` — parses CLI flags, loads config, then starts goroutines for:
- `athena.ListenTCP()` — AO2 TCP connections
- `athena.StartDiscordBot()` — Discord bot
- `athena.ListenWS()` / `athena.ListenWSS()` — WebAO plain/secure WebSocket
- `athena.ListenInput()` — CLI stdin (unless `-nocli`)

Shutdown via OS signal (`SIGINT`/`SIGTERM`) or a `FatalError` channel. Server restart supported via `syscall.Exec`.

### Internal Packages

| Package | Role |
|---------|------|
| `athena` | Core server logic, all command handlers, casino, mafia, punishments, pairing, jobs, shop, unscramble, hot potato, quick draw, giveaway, roulette, coinflip, AutoMod, IPHub |
| `db` | SQLite wrapper; chip balances, accounts, bans |
| `discord/bot` | Discord bot: slash commands, mod bridge, embeds, area/player listings |
| `logger` | Multi-level structured logger (stdout + log file) |
| `ms` | Master server advertisement |
| `packet` | AO2 protocol packet parsing |
| `permissions` | Role-based permission bitfield system |
| `playercount` | Concurrent player counting |
| `settings` | TOML config loading |
| `sliceutil`, `uidheap`, `uidmanager`, `webhook` | Utilities |

### Database

SQLite at `config/athena.db`. Stores:
- Moderator accounts (bcrypt-hashed passwords)
- Ban records
- Nyathena Chip balances (per IPID)
- Player account registrations
- Shop inventory / purchased items / active tags
- Job cooldowns and playtime tracking
- Unscramble win records

## Build & Run

```bash
make build        # go build -v -o bin/athena athena.go
make test         # go test -v ./...
make all          # build + test
make release      # goreleaser (requires goreleaser installed)
```

```bash
./bin/athena                          # config dir: ./config
./bin/athena -c /path/to/config       # custom config directory
./bin/athena -nocli                   # disable stdin CLI
```

**First run:** after build, copy `config_sample/` to `config/`, edit config files, then launch and run `mkusr` in the CLI to create the first moderator account.

## Configuration

Copy `config_sample/` to `config/` before first run.

### config/config.toml — [Server]

| Key | Default | Description |
|-----|---------|-------------|
| `addr` | `""` | Listen address (blank = all interfaces) |
| `port` | `27016` | TCP port |
| `name` | `"Unnamed Server"` | Server name |
| `description` | — | Server description |
| `motd` | — | Message of the day |
| `max_players` | `100` | Maximum connections |
| `max_message_length` | `256` | Maximum IC/OOC message byte length |
| `default_ban_duration` | `"3d"` | Default ban length |
| `multiclient_limit` | `16` | Max connections per IP |
| `asset_url` | `""` | URL for WebAO assets |
| `webhook_url` | `""` | Discord webhook URL for modcall notifications |
| `webhook_ping_role_id` | `""` | Discord role ID to ping on modcall |
| `punishment_webhook_url` | `""` | Discord webhook for ban/kick embeds |
| `enable_webao` | `false` | Enable plain WebSocket (WebAO) |
| `webao_port` | `27017` | WebSocket port |
| `enable_webao_secure` | `false` | Enable WSS (secure WebSocket) |
| `webao_secure_port` | `443` | WSS port |
| `tls_cert_path` / `tls_key_path` | `""` | TLS cert/key (leave blank for reverse proxy) |
| `webao_allowed_origin` | `"web.aceattorneyonline.com"` | Allowed WebSocket Origin (glob supported, `*` = any) |
| `message_rate_limit` | `20` | Max IC/OOC/music packets per window (0 = off) |
| `message_rate_limit_window` | `10` | Window in seconds |
| `ooc_rate_limit` / `ooc_rate_limit_window` | `4` / `1` | OOC-specific rate limit |
| `connection_rate_limit` / `connection_rate_limit_window` | `10` / `10` | Per-IP connection rate |
| `conn_flood_autoban` | `true` | Auto-ban IPs that flood connections |
| `conn_flood_autoban_threshold` | `6` | Rejections before auto-ban |
| `rate_limit_kick_autoban` | `true` | Auto-ban an IPID that racks up repeated rate-limit or censor kicks (see "Repeat-Offender Autoban" below) |
| `rate_limit_kick_autoban_threshold` | `2` | Base (aggressive-tier) rate-limit kicks before auto-ban |
| `rate_limit_kick_autoban_window` | `600` | Rolling window in seconds for counting kicks toward a threshold |
| `rate_limit_kick_autoban_duration` | `"15m"` | How long the auto-ban lasts |
| `rate_limit_kick_autoban_lenient_playtime` | `120` | Minutes of playtime (2h) for the lenient tier |
| `rate_limit_kick_autoban_lenient_threshold` | `5` | Kicks needed at the lenient tier |
| `rate_limit_kick_autoban_min_playtime` | `1200` | Minutes of playtime (20h) that fully exempts an IPID from the auto-ban (0 = no exemption) |
| `raw_packet_rate_limit` / `raw_packet_rate_limit_window` | `20` / `2` | Raw AO2 packet rate |
| `new_ipid_ooc_cooldown` | `10` | Seconds new IPIDs wait before OOC |
| `new_ipid_ic_cooldown` | `10` | Seconds new IPIDs wait before speaking IC (checked after rate-limit/censor, so a flood during the wait still gets judged — see "Repeat-Offender Autoban" below) |
| `new_ipid_modcall_cooldown` | `60` | Seconds new IPIDs wait before modcall |
| `modcall_cooldown` | `0` | Seconds between modcalls per user |
| `automod_enabled` | `false` | Enable AutoMod banned-word enforcement |
| `automod_wordlist` | `"banned_words.txt"` | Path to banned-words file |
| `automod_action` | `"shadow"` | AutoMod action: `shadow` (shadow-send + torment list), `ban`, `kick`, `mute`, or `torment` |
| `automod_nuke_ban_duration` | `"permanent"` | How long a `nuke`-tier ban lasts; a duration string (`"30m"`, `"3d"`) or `"permanent"` |
| `raid_guard_under_attack_scale` | `70` | Threshold multiplier applied while the server is under coordinated attack (100 = off) |
| `raid_guard_slur_severe_weight` | `40` | Raid-guard points for a `severe`-tier word hit |
| `raid_guard_slur_flagged_weight` | `20` | Raid-guard points for a `default`/`watch`-tier word hit |
| `iphub_api_key` | `""` | IPHub API key for VPN/proxy detection |
| `enable_casino` | `false` | Enable casino and player account system |
| `register_captcha` | `true` | Require captcha on `/register` |
| `join_captcha` | `false` | Require a new IPID to answer a question before it can speak (see "Join Captcha" below) |
| `join_captcha_strikes` | `3` | Failures (blocked messages *and* wrong answers, one shared budget) before the action fires |
| `join_captcha_action` | `"mute"` | `mute` or `kick` |
| `join_captcha_timeout` | `180` | Seconds to answer before striking out; 0 = no timeout |
| `join_captcha_popup` | `true` | Also show the question in a client-side popup (AO2 `BB` packet) |
| `join_captcha_remember` | `true` | Remember a solved IPID so it is never challenged again |
| `join_captcha_min_playtime` | `300` | Minutes of playtime (5 h) that exempts an IPID outright (0 = no exemption) |
| `join_captcha_kinds` | `[]` | Which built-in question kinds to use (empty = all) |
| `join_captcha_secret` | `""` | Key for challenge derivation; falls back to `secretseed`, then a random per-process key |
| `join_captcha_rotate` | `3600` | Seconds one IPID keeps the same question; `-1` disables keying |
| `join_captcha_questions` | `"captcha_questions.txt"` | Operator-written question file (not in the repo) |
| `join_captcha_custom_only` | `false` | Serve only the operator's questions, never the built-in ones |
| `captcha_plugin` | `""` | Path to an external captcha plugin program (see `docs/CAPTCHA_PLUGIN.md`) |
| `captcha_plugin_timeout` | `3000` | Milliseconds to wait for a plugin reply before falling back |

### config/config.toml — [Discord]

| Key | Description |
|-----|-------------|
| `bot_token` | Discord bot token (blank = bot disabled) |
| `guild_id` | Discord server ID for slash command registration |
| `mod_role_id` | Discord role ID allowed to run moderation slash commands |

### Other Config Files

| File | Purpose |
|------|---------|
| `areas.toml` | Area definitions |
| `roles.toml` | Moderator role permissions |
| `characters.txt` | Allowed characters |
| `music.txt` | Music list |
| `8ball.txt` | (Optional) `/8ball` response pool. Falls back to a built-in 20-line classic list if missing or empty. |
| `backgrounds.txt` | Background list |
| `banned_words.txt` | AutoMod word list |
| `parrot.txt` | Parrot command word list |
| `captcha_questions.txt` | (Optional) Operator-written join-captcha questions, `question \| answer[, alt]` per line. Never in the repo, hot-reloadable. |

### Discord Bot Setup

1. Create a bot at https://discord.com/developers/applications
2. Enable the **Message Content** intent
3. Copy the bot token into `[Discord]` → `bot_token`
4. Set `guild_id` to your Discord server ID
5. Optionally set `mod_role_id` to restrict moderation slash commands
6. Invite the bot with `applications.commands` and `bot` scopes

## Features Beyond Base Athena

### Punishment System (120+ Commands)

All punishment commands require `MUTE` permission unless noted. They support `-d <duration>` (max 24 h), `-r <reason>`, `-h` (hidden — suppresses the per-target OOC notification so the punishment applies silently; the issuer summary appends `(hidden)` so the mod can confirm), comma-separated UIDs, and the `global` keyword (applies to every non-moderator in the issuer's area). Multiple types stack on a single player.

The `-h` flag works on every applicator: single-effect commands (`/tsundere 7 -h`, `/tsundere global -h`), `/stack`, `/lovebomb`, `/sfxcurse`, `/shrink` / `/grow` / `/wide`, `/randompunishall` (also suppresses the area-wide "unleashed random chaos" announcement), `/translator curse`, and `/icwarp <uid>`. Self-applied effects (`/megamaso`, `/maso`) and the PvP `/coinflip` mini-game are unaffected since they aren't moderator-issued.

`/help punishment` is grouped by sub-theme (text effects / dere archetypes / animal filters / themed quotes / persona / protocol-viewport / audio / voice / timing / traps & contagion / stacking / removal) so the long list is scannable.

**Inspect:** `/punishments` lists your own active punishments with remaining durations (includes lag, mute, jail). Moderators run `/punishments <uid>` to inspect any player — entries show custom data, reason, and issuer tier.

**Remove:** `/unpunish <uid>` (all including lag) or `/unpunish -t <type> <uid>` (specific — works with `/unpunish -t lag <uid>`). `/unpunish all` clears every player in the area including lag. A non-numeric target is treated as a raw IPID (`/unpunish <ipid>`), clearing persisted punishments for offline players too. Full-removal forms also clear the `punishment_names.txt` showname stain (see "Punished Showname Random-Punishment Drip" below). Self-removal is gated for staff-issued punishments — see "/unpunish Self-Removal Protection" below.

#### Text Effects (60)
`/whisper`, `/backward`, `/stutterstep`, `/elongate`, `/uppercase`, `/lowercase`, `/robotic`, `/alternating`, `/fancy`, `/uwu`, `/pirate`, `/shakespearean`, `/caveman`, `/censor`, `/fromsoftware`, `/confused`, `/paranoid`, `/drunk`, `/hiccup`, `/whistle`, `/mumble`, `/slang`, `/cherri`, `/albhed`, `/morse`, `/vowelhell`, `/upsidedown`, `/autospell`, `/thesaurusoverload`, `/valleygirl`, `/babytalk`, `/thirdperson`, `/unreliablenarrator`, `/uncannyvalley`, `/chef`, `/karen`, `/passiveaggressive`, `/nervous`, `/sarcasm`, `/academic`, `/philosopher`, `/poet`, `/quote`, `/spaghetti`, `/essay`, `/rng`, `/haiku`, `/dreamsequence`, `/timewarp`

Wave-2 additions (14): `/zalgo` (combining-mark corruption), `/leetspeak`, `/smallcaps`, `/piglatin`, `/vaporwave` (ｆｕｌｌｗｉｄｔｈ), `/lisp`, `/spoonerism`, `/keysmash`, `/weeb` (350+ romaji corpus — word swaps, honorifics on names, interjections, desu~ particles), `/politician` (never answers directly), `/clickbait` (headlines star the speaker by name), `/markov` (babble generated from the **area's own recent chat history**; falls back to word shuffle in fresh areas), `/alliteration`, `/cipher` (escalates ROT13 → BINARY → BASE64 per message, then "decryption fails" and re-arms). Expanding transforms clamp output to the server's max IC length (`fitICBudget`) so punished messages are never dropped by the post-transform length check.

`/medieval` — rewrites the target's IC text into Olde-English / medieval speak: ~90 word-for-word swaps (`you`→`thou`, `your`→`thy`, `is`→`be`, `yes`→`aye`, `now`→`anon`…), a random heralds' cry prepended (`Hark!`, `Forsooth,`, `Prithee,`, `Zounds!`…) and a random courtly flourish appended (`…by my troth.`, `…mine liege.`, `…verily.`). The prefix and suffix are each rolled independently on top of the per-word swaps, so a single line has **100+ distinct renderings** (herald × flourish alone is 38 × 30 = 1,140 combinations, asserted by `medievalVariationCount` in tests). Transform in `internal/athena/punishments_medieval_cheese.go`.

#### Themed Quote Replacers (11)
`/gordonramsay`, `/biblebot`, `/grounded`, `/mime`, `/subtitles`, `/spotlight`, `/recipe`, `/rickroll`, `/pickup`, `/brainrot`, `/cheese`

`/cheese` discards the target's text entirely and replaces every message with one of **100+ "statements about cheese"** (facts, puns, varieties, and the running gag that cheese is, technically, a sauce). Same shared punishment plumbing as the others (`-d`/`-r`/`-h`, comma UID lists, `global`, `/stack`, persistence, `/unpunish -t cheese`). Corpus size is pinned by `cheeseLineCount` in tests so it can't silently shrink. Like `/medieval` it requires `MUTE` (mod-only) and is **not** in the self-applied `/maso`/`/megamaso` pool.

#### Animal Filters (15)
`/monkey`, `/snake`, `/dog`, `/cat`, `/bird`, `/cow`, `/frog`, `/duck`, `/horse`, `/lion`, `/bunny`, `/zoo` (a random one of the others per message), plus:

- `/trex` — every word becomes a tyrannosaur roar: `RAAASRFH`, `RAWR`, `GRAAAH`, `*tiny arms flail*`.
- `/fish` — every word becomes a bubble: `blub`, `blublublib`, `glub`, `*bubbles*`.

Both use the same `applyAnimalSounds` word-for-word substitution the older filters use (so a long message stays long), require `MUTE`, and are in the `/zoo`, roulette and punishment-area pools alongside the rest. Transforms live in `internal/athena/punishments_trex_fish.go`.

#### Uma Characters — `/horse`
On a server whose `characters.txt` carries uma characters, `/horse` applies the animal filter above **plus a one-shot swap** onto a random free uma character (a real `Client.ChangeCharacter`, so the area's taken-slot table, `/players` and every client agree). The target may change away afterwards, like `/charcurse`.

The swap belongs to the **`/horse` command**, not to `PunishmentHorse` — so `/stack horse`, `/randompunishall`, the roulette pool and a `punishment_area` roll all apply the sounds without swapping anyone, exactly as before. Keeping it at the command level is what stops a punishment the server rolls *at* a player from silently taking their character away.

**The roster is discovered by marker, not by name.** `getUmaCharIDs` (`internal/athena/uma.go`) collects every slot in `characters.txt` whose name contains `(uma)` or `(uma_h)`, case-insensitively — the two are disjoint substrings, so a name can never be counted twice. An operator adds new horse girls to `characters.txt` with one of those suffixes and `/reload`s; no rebuild and no code change, which is the whole point of the convention. Same-franchise characters carrying a different marker (`gold ship (gbf)`) or none (`still in love`) are deliberately excluded — the marker is the opt-in. The pool is a derived cache of the character list, rebuilt by `setCharacters` in lockstep with the name→ID index (and published before the list, so it can only ever be a subset of what a reader can index), so a `/reload` picks new entries up automatically and readers pay one atomic load. Nothing here runs on the IC hot path — the swap happens once, in the command.

**An empty pool is a silent no-op, never an error.** On a server with no marked characters — the upstream default — `/horse` applies its sounds and swaps nobody. It likewise skips the swap, without reporting a failure, for a target who is spectating, already pinned by `/charstuck` or a forced iniswap (`/tung`), or standing where every uma slot is taken: the swap is additive, so it must never look like the punishment failed. It is driven off the clients `cmdPunishment` reports it *actually* punished (that applicator now returns them), so a target shielded by a punishment-safe area is never swapped either — the two can't disagree about who was spared.

#### Persona / Personality (5)
`/clown`, `/jester`, `/joker`, `/tourettes`, `/translator`

#### Visibility / Cosmetic (8)
`/emoji`, `/invisible`, `/shrink`, `/grow`, `/wide`, `/areainiswap` (vertical/horizontal sprite-offset locks; reverse with `/unshrink` / `/ungrow` / `/unwide`), plus the two viewport-control punishments below.

- `/hidedisplay <uid>` — pushes the target's own sprite off-screen via a fixed self-offset so their text still appears in the chat box but their character does not. Looks especially funny on pairs (only the partner's sprite renders). Standard punishment flags: `-d`/`-r`/`-h`, comma UID lists, `global`, stackable, persists across sessions, removable with `/unpunish -t hidedisplay <uid>`.
- `/forcedisplay <uid>` — pins the target's character onto every non-moderator IC message in their area, overwriting each speaker's outgoing sprite and clearing the pair fields. While active, no other character can show in the viewport — the whole room renders as that one character. Moderators are exempt (their own sprite still shows). The hot-path lookup is gated by an atomic counter, so servers that never use `/forcedisplay` pay nothing on every IC packet. Same flag set as `/hidedisplay`; removable with `/unpunish -t forcedisplay <uid>`.

#### Protocol / Viewport Effects (6)
Weaponize the IC packet's non-text fields. Applied in `pktIC` (`applyProtocolPunishments`, `internal/athena/punishments_protocol.go`) **before** field validation, so every written value is validator-legal by construction. Standard flags, persistence, `/stack`, `/unpunish -t`.
- `/teleport <uid>` — random self-offset per message (±75 x, ±50 y); the sprite pops around the viewport. `/hidedisplay` wins if both are active.
- `/shakecurse <uid>` — forces the screenshake flag on every message
- `/randomflip <uid>` — coin-flips the sprite's horizontal facing per message
- `/forcecolor <uid> <0-9|white|green|red|orange|blue|yellow|rainbow>` — locks IC text colour (stored in customData; persisted via the 0x1F reason convention like `/translator`)
- `/nopreanim <uid>` — strips preanimations (PREANIM modifiers demoted, preanim name cleared)
- `/forcepreanim <uid>` — promotes idle/talk modifiers so the named preanim plays

#### Timing Effects (3)
`/slowpoke`, `/fastspammer`, plus:
- `/lifo <uid>` — buffers the target's IC messages and releases them in **reverse arrival order** (flush at 3 messages or 6 s, whichever first). Implemented in `internal/athena/punishments_lifo.go`; queues are keyed by `*Client` and self-flush, so disconnects can't leak.

#### Traps & Contagion (5)
Mechanic punishments hooked after the IC broadcast (`punishmentMechanicsOnIC`, `internal/athena/punishments_mechanics.go`) with cheap early-outs (atomic gate for love potions, one shared mutex for the area maps) — except `/randomarea`, which has no IC message to hook and instead runs off its own per-connection timer.
- `/contagious <type> <uid|global>` — plague mode: target gets `<type>` + a contagion marker; anyone who speaks within **5 s** of an infected player's message catches both and keeps spreading it. Victims inherit remaining duration and issuer tier. Moderators immune; area gets ☣️ announcements. `contagious`/`lag`/`minefield`/`lifo`/`stealthmute` can't be made contagious.
- `/minefield <uid|global>` — every message has a **1-in-6** chance to detonate a random 2-minute punishment (megamaso pool) with a 💥 announcement.
- `/randomarea <uid|global>` — force-warps the target to a random **open** area (never a `/lock`-ed, `/adminlock`-ed, or punishment-safe one) every **20-45 seconds**, on its own — no message needed to trigger it. A per-connection watcher goroutine (`internal/athena/randomarea_punishment.go`) is armed the moment the punishment is applied (fresh, `/stack`ed, or restored on reconnect via the normal `AddPunishmentBy` path) and exits the instant `/unpunish -t randomarea`, expiry, or disconnect makes `HasActivePunishment` false — mirroring the leak-free shape of `/curserandomchar`'s watcher, but reusing the standard punishment slice/DB instead of a bespoke table since it already carries `-d`/`-r`/`-h`, `global`, `/stack` and persistence for free. A carrier standing in a punishment-safe area (or caught by the console punishment kill switch) is skipped for that tick rather than treated as expired.
- `/silencebell [type] [-d dur]` — arms a one-shot trap on the issuer's area: the next non-moderator to speak is cursed (random unless a type is given). `status` / `off` subcommands. Dramatic 🔔 announcements on arm and trigger.
- `/stealthmute <uid|global>` — the target's IC **and** OOC messages echo back to them but reach nobody; they are never notified (the `-h` semantics are forced). Area logs tag suppressed OOC lines `(stealthmuted)`. Stealthmuted messages never trigger traps/contagion/love potions. Lift with `/unpunish -t stealthmute`.

#### Audio (2)
- `/sfxcurse <uid> <sfx-url>` — forces an SFX file to play on every IC message; URL must be `http(s)://…` or under `/base/sounds/…`
- `/unsfx <uid>` — lifts the SFX curse

#### Voice Chat (5)
Sabotage a player's server-relayed voice-chat audio. Require `enable_voice` and `MUTE`; support `-d`/`-r`/`-h`, comma-separated UIDs, `global`, `/stack`, `/unpunish` and DB persistence exactly like the text punishments. They act on the punished speaker's Opus frame *flow* in the documented `pktVSFrame` hook — the relay never decodes audio, so they stay CGO-free (no pitch-shift). Unlike `/vmute` (which ejects an IPID from voice entirely), these keep the target in the room and just corrupt what the room hears.
- `/voicemute <uid>` — drops every one of the target's voice frames; silent to the room
- `/voicestatic <uid>` — drops ~60% of frames; choppy, breaking up
- `/voicegarble <uid>` — drops ~88% of frames; barely intelligible
- `/voicecutout <uid>` — gates frames on a ~650 ms on/off cycle; walkie-talkie effect
- `/voicestutter <uid>` — randomly replays the previous frame; a glitchy voice stutter

#### Dere Archetypes (25)
Original 10: `/tsundere`, `/yandere`, `/kuudere`, `/dandere`, `/deredere`, `/himedere`, `/kamidere`, `/undere`, `/bakadere`, `/mayadere`

Nyathena additions: `/smugdere`, `/deretsun`, `/bokodere`, `/thugdere`, `/teasedere`, `/dorodere`, `/hinedere`, `/hajidere`, `/rindere`, `/utsudere`, `/darudere`, `/butsudere`, `/sdere`, `/mdere`, `/tsuyodere`

`/omnidere` picks a random dere flavour for **every** IC message from the full pool — maximum tonal whiplash.

#### Punishment Stacking
```
/stack <type1> <type2> [...] [-d duration] [-r reason] [-h] global | <uid1>,<uid2>,...
```

#### Punishment Tournament Mode
Voluntary competitive mode where participants receive 2–3 random punishments; most IC messages sent wins.
```
/tournament start|status|stop   # requires MUTE
/join-tournament                # any user
```

#### `/megamaso` — Self-applied Stack Mode
Self-applied "max chaos" mode. The first call rolls a random punishment; each subsequent `/megamaso` while still under the effect **adds another random punishment to the stack** instead of replacing it. Lets a player pile on as many concurrent effects as they like. Duration defaults to 10 minutes and can be set with `-d` (max 24 h).

```
/megamaso              # any user — default 10 min per layer
/megamaso -d 1h        # each stacked layer lasts 1 hour
```

#### `/maso` — Self-applied Single Effect
Rolls a random punishment from the pool and applies it to yourself. Typing `/maso` again rerolls to a different effect and resets the timer. Duration defaults to 10 minutes and can be set with `-d` (max 24 h).

```
/maso              # any user — default 10 min
/maso -d 30m       # apply for 30 minutes
/maso              # (again while active) reroll
```

#### Coinflip Challenge
Area-scoped 30-second PvP challenge. Players must choose opposite sides.
```
/coinflip <heads|tails>
```

#### Punishment Audit Log
By default, every `ADMIN` gets a live `[AUDIT]` OOC alert whenever a moderator issues a punishment-system command (dere archetypes, text effects, protocol/viewport curses, traps, voice curses, `/stack`, etc.) — naming the issuing moderator (real name, not the shadow-anonymized one — admins always see through `SHADOW`), the punishment, the target(s), and the reason/duration, even when the command was run with `-h` (which only suppresses the *target*-facing notice, never the admin-facing one). Every trip is also written to the server's persistent audit log (`logger.WriteAudit`) so the record survives a restart. The issuing admin (if the issuer is themselves an admin) never gets their own alert. Self-applied effects (`/maso`, `/megamaso`), the showname-punisher drip, and a bystander catching a contagion are never reported since they aren't a moderator issuing a fresh command.

```
/punishaudit <on|off>   # ADMIN — toggle these alerts for your own session (default: on)
```

Implemented in `internal/athena/punishment_audit.go` (`alertPunishmentIssued`), hooked into every punishment command's apply path — `cmdPunishment` (the shared applicator behind the great majority of punishment commands), `/stack`, `/lovebomb`, `/sfxcurse`, `/shrink`/`/grow`/`/wide`, `/forcecolor`, `/contagious`, `/silencebell`, `/translator curse`, `/randompunishall`, `/icwarp`, and `/charcurse`.

### Potions System
Self-applied fun effects accessed via `/potion <name>`. Duration defaults to **5 minutes** and can be set with `-d` (max 24 h): `/potion -d 30m drunk`. `/potions` lists the cabinet; `/potion off` flushes every active potion.

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
| `omnidere` | Each line picks a random dere flavour |
| `zalgo` | C̴o̷r̶r̸u̵p̷t̶s̸ your text with creeping zalgo marks |
| `valleygirl` | Injects valley-girl filler words, vowel stretching, and dramatic tone |
| `love` | 💘 Auto-sends a pair request to the next player who speaks in your area (consent preserved — they still accept with `/pair`; mutual interest completes instantly) |
| `character` | Auto-rotates your character every 30 seconds |

The `character` potion is a per-client goroutine, not a punishment — `/potion off` cancels its rotation cleanly. The `love` potion is likewise special: it arms a per-client flag (atomic-counter-gated on the IC hot path) and `/potion off` disarms it.

### Persistent Pairing
UID-based mutual pairing surviving area/character changes. Pair messages (like every OOC-channel/server message — `/global`, `/pm`, `/roll`, `/rps`, `/8ball`, etc.) label players by their **OOC name** via the shared `oocDisplayName` helper, falling back to showname then character only when the OOC name is blank so the label is never empty.

**Auto-unpair on disconnect.** When either partner leaves the server, `clientCleanup` calls `clearPairLinksOnDisconnect` (mirrors `/unpair`'s full bidirectional scan) while the leaver's UID is still valid, clearing `PairWantedID`/`ForcePairUID` on the leaver **and** on every client that referenced them (by UID or CharID) and notifying the surviving partner. This prevents the IC pair desync where a partner kept a stale pair pointing at the departed player's now-recyclable UID/CharID — and stops a stale pending request from auto-completing against a recycled slot later.

A pending pair request never blocks speech: while waiting for the partner to accept, the requester's IC messages go out with a plain `-1` no-pair value (never the `-1^` order-suffixed form, which some desktop forks drop outright and webAO parses as `NaN`).

`/unpair` is a full bidirectional reset: it clears `PairWantedID` and `ForcePairUID` on every client that references the canceller (by UID or by current/historical CharID), preventing the desync where a stale pair-wanted-id lingered on a peer after the canceller's character changed.

**Discoverability:** `/lfp` toggles a Looking-For-Pair flag; `/pairlist` lists every flagged player in the area with UID, display name and character.
```
/pair <uid>
/unpair
/lfp
/pairlist
```

### Character Curse
One-time forced character swap (requires KICK). Target may freely change afterwards.
```
/charcurse <uid> <charname>
```

### Character Steal (`/charsteal`)
`/charsteal <uid>` (`MUTE`) takes the target's current character for the issuing moderator, then forces the target onto a random free character — the same `getRandomFreeChar` pool `/forcerandomchar`/`/randomchar` draw from. The target is moved off the character first (freeing the slot) before the moderator claims it, since `Area.SwitchChar` refuses to hand out a slot that's still marked taken. Requires the target to be in the caller's area and to actually be using a character (not spectating); a moderator can't target themselves. Not persisted — a one-shot swap, like `/forcepos`. Implemented in `internal/athena/charsteal.go` (`cmdCharSteal`).

```
/charsteal <uid>
```

### Character Protection (`/charprotect`)
`/charprotect <on|off>` (`MUTE`) lets a moderator claim their current character as protected. Each area tracks its own taken-character slots independently, so it's normal for a player in one area and a moderator in another to both be using the same character — but when the protected moderator then changes into that player's area, `ChangeArea` would normally demote the *moderator* to spectator (their held slot is already taken there). With protection armed, the other player is force-moved to a random free character instead, so the moderator keeps their character.

```
/charprotect on
/charprotect off
/charprotect          # reports your current setting
```

Session-only (an `atomic.Bool` on `*Client`) — unlike `/curserandomchar`, there's nothing to persist by IPID, since there's no punishment to evade and a reconnect resetting the toggle is harmless (the moderator just re-enables it). The hook lives in `resolveCharProtectOnJoin` (`internal/athena/charprotect.go`), called from both `ChangeArea` and `forceChangeArea` (the jail-placement path) right before the normal "demote to spectator" branch; it no-ops (falling back to normal behavior) if protection is off, nobody else currently holds the character in the destination area, or there's no free character to bump the holder to.

### Forced Position (`/forcepos`)
CM tool for staging a scene: pushes one or more players in the caller's own area into a specific courtroom position — the same `def`/`pro`/`wit`/`jud`/`hld`/`hlp`/`jur`/`sea` set `/pos` lets a player choose for themself. Gated on the `CM` permission, so both server CM-permission holders and area-designated CMs (`/cm`) can use it, same as `/invite`, `/lock`, and `/spectate`.

```
/forcepos <uid> <position>              # force one player
/forcepos <uid1>,<uid2>,... <position>  # force several players at once
/forcepos all <position>                # force everyone in the caller's area
```

Not a punishment — nothing is persisted or stacked, and the target is free to run `/pos` themself again right after. Targets outside the caller's area are silently skipped (mirroring `/charselect`'s per-UID force branch), so a CM can't reach into a different room. Implemented in `internal/athena/commands_area_admin.go` (`cmdForcePos`), reusing the same `validPositions` list `/pos` validates against.

### Random Character Curse (`/curserandomchar`)
ADMIN-only curse (`internal/athena/curse_randomchar.go`) that forces the target's character to randomly change every 1–5 seconds, forever, until an admin lifts it.

```
/curserandomchar <uid>      # ADMIN — arm the curse
/uncurserandomchar <uid>    # ADMIN — lift it
```

Unlike the `character` potion's per-session auto-rotation, the curse is **persisted by IPID** in the `RANDOMCHAR_CURSES` table (DB migration 23), the same way `/musicban` persists — a curse tied only to the live `*Client` would vanish the instant the target reconnects, since a fresh connection gets a brand-new `*Client`. Instead, `restoreRandomCharCurse` (called from `pktReqDone` right alongside `restorePunishments`) checks the joining client's IPID against the table and re-arms the curse on every join, so **relogging cannot be used to escape it** — nor can a full server restart.

Each armed connection runs a single per-client watcher goroutine that picks a new random 1–5 second interval every tick, swaps to a random free character via the same `getRandomFreeChar`/`ChangeCharacter` path `/randomchar` uses (skips a tick if the target is tunged), and exits cleanly via `client.done` the moment the connection closes — so a cursed player who disconnects can never leak a goroutine, and `/uncurserandomchar` clearing the active flag makes the next tick exit on its own.

### Removed: Possession (`/possess`, `/fullpossess`, `/truepossess`, `/unpossess`)

Gone entirely, along with the whole spoof pipeline that backed them (`possess.go`, the `possessing`/`trueMuted` client fields, and the possession branches in `pktIC`/`pktOOC`). The `grant` console gate that briefly stood in front of them is gone too — see "Removed: Console-Gated Commands" below.

The property they shared is that the effect was **invisible to the person it landed on**: a possession spoke as somebody, spoofed their pairing so the room saw nothing unusual, and silenced their IC, their OOC and their commands so they had no channel to say "that isn't me". Gating that behind shell access made it harder to reach and did not change what it did to its target. A moderation tool that leaves the person on the receiving end nothing to notice, report or appeal is the wrong shape regardless of who can fire it.

Nothing replaces it. `/forcedisplay` (pin one character onto every sprite in the area) and `/charcurse` remain — both are visible to the room and to the target.

### Admin Lock (`/adminlock`)
`/adminlock` (ADMIN) toggles an admin-only seal on the caller's area: an admin-locked area refuses entry to **everyone but administrators** — even moderators and shadow mods who hold `BYPASS_LOCK`, and even players on the invite list. Unlike `/lock`, there is no emergency-bypass or invite escape hatch. Players already inside are not evicted (it blocks new entries only). The check sits at the top of `Client.ChangeArea`, before the normal lock logic. The area is also set to `LockLocked` so it displays as locked in ARUP; a new `area.adminLocked` bool (cleared on `Area.Reset`) carries the extra seal. A non-admin cannot `/unlock` or `/lock` an admin-locked area out from under it — only `/adminlock` (by an admin) lifts it, which reopens the area (`LockFree`, invites cleared). Area 0 cannot be admin-locked.

### Mafia Social Deduction Minigame
Fully featured in-server Mafia (4–20+ players) with automatic role pools, day/night phases, lynch voting, night actions, last wills, graveyard, whisper, and phase timers.

**Roles — Town:** Villager, Detective, Doctor, Sheriff, Bodyguard, Vigilante, Mayor, Escort
**Roles — Mafia:** Mafia, Shapeshifter, Godfather
**Roles — Neutral:** Jester, Witch, Lawyer, Arsonist, Serial Killer, Survivor

Key commands: `/mafia create|join|start|vote|act|tally|graveyard|whisper|will`

See `MAFIA_COMMANDS.md` for the full reference.

### Casino System

Enabled with `enable_casino = true`. Requires player accounts (`/register`).

**Virtual Currency — Nyathena Chips:** New connections start with 500 chips; max 10,000,000. Stored in SQLite.

**Games:**

| Command | Game |
|---------|------|
| `/bj` | Blackjack (6-deck, split, double, insurance; up to 6 players) |
| `/poker` | Texas Hold'em (up to 9 players, 500-chip buy-in) |
| `/slots` | Slots with area jackpot pool |
| `/croulette` | European Roulette (single-zero) |
| `/baccarat` | Baccarat (player / banker / tie) |
| `/craps` | Craps lite (pass / don't-pass) |
| `/crash` | Crash multiplier game |
| `/mines` | Minesweeper-style grid (1–24 mines, 5×5) |
| `/keno` | Keno (pick 1–10 numbers from 1–80) |
| `/wheel` | Prize wheel (~92.5% RTP) |
| `/bar` | Bar with 33 drinks of wildly varying variance |

**Economy:** `/chips`, `/chips top`, `/chips area`, `/chips give`, `/richest`

**Earning Without Gambling:**
- Jobs with cooldowns: `/busker`, `/janitor`, `/paperboy`, `/clerk`, `/bailiffjob`
- Unscramble events every 30 min–3 h (first correct IC answer wins 10 chips)

**Shop (`/shop`):** 117 cosmetic tags across seven categories — gambling (30), Ace Attorney (15), anime (15), gamer (15), girly (12), silly & memes (20, including `tag_mrbeast`), prestige (10) — priced 100–10,000,000 chips, plus job cooldown reduction passes and job reward bonus passes.

**Staff:** `/casinoenable`, `/casinoset`, `/grantchips`

See `CASINO_COMMANDS.md` for the full reference.

### Custom Tags (Moderator/Admin)

Moderators and admins can mint cosmetic tags at runtime without rebuilding the server. Custom tags share the same equip system as the built-in shop tags (`/settag`, `[Name]` shown in `/gas` and `/players`) but have their own DB table (`CUSTOM_TAGS`) and are only ownable via a staff grant — they never appear in `/shop` and never cost chips.

| Command | Permission | Purpose |
|---------|------------|---------|
| `/createtag <id> <display name>` | `MUTE` | Create a new custom tag. Id must be 2–32 lowercase letters/digits/underscores; display name (≤30 chars, no `[` or `]`) is what shows in brackets. Rejects ids that collide with built-in shop tags. |
| `/deletetag <id>` | `ADMIN` | Delete a custom tag and clean up every grant + active equip referencing it. Built-in tag ids are protected. |
| `/grantcustomtag <username> <tag_id>` | `MUTE` | Grant any tag (built-in or custom) to a registered player account. Looks up the account's IPID by username — the player must have logged in at least once so the IPID is linked. Idempotent. Note: since this grants *any* shop tag, not just custom ones, a moderator can use it to hand out a normally chip-priced tag for free. |
| `/revokecustomtag <username> <tag_id>` | `ADMIN` | Revoke a previously granted tag from an account; clears it from their active equip if equipped. |
| `/listcustomtags` | none | List every custom tag with its id, name, creator, and creation date. Visible to all players. |
| `/mytags` | none | Self-service: list *your own* tags — built-in and custom alike — with their ids and which one is active. |

Creation and granting are deliberately the lower `MUTE` tier so moderators can run staff-tag workflows without an admin; deletion and revocation stay `ADMIN`-gated since they're the harder-to-reverse, larger-blast-radius half (a delete unwinds every grant and equip of a tag server-wide).

**Workflow:**
```
mod> /createtag founder ⭐ Founder
mod> /grantcustomtag alice founder
alice> /settag founder       → alice now wears [⭐ Founder]
alice> /mytags                → lists [⭐ Founder]  (id: founder)  ← active
```

**`/mytags` (bug fix along the way).** `/shop items` already listed everything a player owned, but its tag branch resolved names purely through `shopItemByID` — the built-in catalog index — so a custom tag id, which lives in `CUSTOM_TAGS` instead, missed that lookup and was silently dropped from the "🏷️ Tags" section entirely: a player who owned only a custom tag saw `/shop items` claim they owned nothing. `/mytags` is a new, narrower command (just tags, none of `/shop items`'s pass/passive-income noise) built on `lookupTag`, which already correctly falls back to `db.GetCustomTag` — and `shopListOwned` (the code behind `/shop items`) now takes the same fallback, so the two commands can no longer disagree about what counts as an owned tag. Pinned by `TestLookupTagResolvesCustomTags` (`internal/athena/shop_test.go`) and `TestGetPlayerShopItemsIncludesCustomTags` (`internal/db/db_custom_tags_test.go`).

### Per-Area Logging

Daily-rotating log files per area under `logs/<AreaName>/<AreaName-YYYY-MM-DD.txt>`.
Format: `[HH:MM:SS] | ACTION | CHARACTER | IPID | HDID | SHOWNAME | OOC_NAME | MESSAGE`
Actions: IC, OOC, AREA, MUSIC, CMD, AUTH, MOD, JUD, EVI.
Enabled with `enable_area_logging = true` in `[Logging]`.

### Modcall Report Buffer: Time-Based Retention (Bug Fix)
Separate from the persistent per-area daily log files above, each area also keeps an in-memory buffer of recent action lines (IC, OOC, MOD calls, CMD, MUSIC, ...) that gets flushed to a `report-<timestamp>-<area>.log` file (and posted to the report webhook) whenever a player calls a moderator (`/modcall`) — the same buffer backs the Discord bot's per-player log lookup. It previously kept a fixed number of lines (`log_buffer_size`, default 150), so a busy area could rotate through that many lines in well under an hour, leaving mod call reports covering only a few hours instead of the intended full day of context. The buffer now retains every line from the past **24 hours** regardless of count; `log_buffer_size` becomes a safety-valve cap on top of that (`0` = no cap, the new default) to bound memory in an extremely active area. Implemented in `Area.UpdateBuffer`/`Area.Buffer` (`internal/area/areas.go`).

**The in-game `/log <area>` command has been removed.** It printed the raw buffer above as a single OOC message to any `ADMIN`; the buffer is still populated exactly as before (the modcall report file and the Discord bot's per-player log lookup are unaffected), there is simply no in-game command that reads it directly any more.

### AutoMod
Word-list-based automatic enforcement. Covers IC message text, IC showname, OOC message text, OOC username, and `/area rename` (an area name is broadcast to every connected client, so it goes through the identical path — see "`/area rename`" below) — slurs in any of those fields trigger the configured action.

**Severity tiers.** `banned_words.txt` entries are no longer flat. Each line may carry a tier and a match mode, `word | tier | mode`, and an unmarked line means exactly what it always did:

```
badword                 # default tier, substring match -- unchanged behaviour
tranny  | nuke          # message destroyed before anyone sees it, IPID banned
faggot  | severe        # configured action + 40 raid-guard points
getfucked | watch       # delivered normally; scores 20 and alerts staff
rape | severe | word    # whole-word match: hits "rape"/"rapes"/"raped", never "therapeutic"
```

| Tier | Message | Connection | Raid-guard score |
|------|---------|-----------|------------------|
| `watch` | delivered normally | untouched | +20, staff alert |
| `default` (unmarked) | per `automod_action` | per `automod_action` | +20 |
| `severe` | per `automod_action` | per `automod_action` | +40 |
| `nuke` | **destroyed, not even echoed to the sender** | **banned** (`automod_nuke_ban_duration`, default permanent) | none |

A `nuke` anywhere in a packet wins outright, decided across **every** field before the per-field checks can short-circuit one another (`nukeFieldsOrNothing`, `automod_nuke.go`). Without that sweep a merely `default`-tier word in the message body set `censorShadow`, the showname was then never examined, and a `nuke` word worn as a showname would have been answered with a shadow-drop instead of a ban — the same shape existed in OOC, where a shadowed username returns before the message is ever checked. The short-circuiting was harmless while every trip did the same thing and stopped being harmless the moment tiers arrived.

**The operator's tier is the tier that gets enforced (bug fix).** This briefly stopped being true in production, and the way it broke is worth recording. `effectiveWordEntries` merges the flat legacy word list into the tiered one, stamping each legacy entry `SeverityDefault` since it carries no tier of its own — harmless while `loadWordListFile` normalized a tiered line *whole*, because `retard | watch` became the gibberish needle `retardwatch` that no message contains. Then `loadWordListFile` began stripping the `| tier | mode` suffix so `/giveaway` would still filter a word defined only with a tier, which is correct on its own and turned that inert needle into the real word at default severity. Every entry now appeared **twice** — once at the operator's tier, once at `SeverityDefault` — and because the worst match wins, every `watch` entry silently became `default`. On a server with `automod_action = "torment"` that meant real players had messages dropped, were torment-listed and were kicked for words explicitly marked alert-only. Every unit test passed throughout: the parser, the matcher and the action handler were each correct, and the bug lived in the seam between them. `effectiveWordEntries` now skips a legacy entry whose pattern the tiered list already covers, and `automod_tier_integrity_test.go` drives the production load path — the same file into both lists — asserting the tier survives, that `watch` returns `autoModPass` under *every* configured action including `ban`, that a flat-list-only word still filters, and that one file never yields duplicate patterns.

The tier of a hit is the **worst** entry the message matches, never the first found — a line padding a slur with mild words is judged on the slur. A mistyped tier or mode is **skipped with a warning naming the entry**, never guessed at: silently downgrading `nuek` to `default` would leave an operator believing a word is blocked when it is not.

**The `nuke` tier is deterministic, and that is the whole point.** Every other action leaves something behind — `shadow` echoes the message to its sender so their client looks normal, `torment` lets them keep talking into a void. Those are right for a list that mostly holds borderline calls and wrong for the small number of words where there is nothing to weigh up. A nuke destroys the message before it can reach anybody *including the sender* (an echo is both a kindness to someone who might be innocent and a tell — a sender who sees nothing at all learns only what a network drop would tell them) and bans the IPID. Crucially it does **not** go through the raid guard's ban gate and contributes **no** raid-guard score: the guard is careful about banning because it is inferring from behaviour, and none of that reasoning applies when an operator has typed an exact word into a file and said what should happen. Keeping the two mechanisms separate is what stops content rules from quietly weakening the invariant that a lone player cannot be banned by behaviour alone. Implemented in `internal/athena/word_severity.go` (tiers, match modes, worst-match-wins) and `automod_nuke.go` (`applyAutoModNuke`).

**Two match modes, because evasion resistance and precision genuinely pull against each other.** Substring matching runs on `normalizeForFilter`'s output, which discards every non-letter so `n i g g e r`, `n.i.g.g.e.r` and `n1gg3r` all collapse onto one needle — and that is also why word boundaries no longer exist by match time, so `rape` would fire inside `therapeutic` and `loadWordListFile` rejects it outright. A word an operator obviously wants blocked is therefore silently unavailable to them, visible only as a startup warning. `word` mode runs on `normalizeForFilterBoundaries` instead — same case, confusable and leetspeak folding, but separator runs become a single space — so the entry matches whole tokens and their stem-preserving inflections. It skips `collidesWithCommonWords` (that gate protects substring matching, and a boundary match structurally cannot fire mid-word) at the cost of being beatable by spacing evasion, which is the right trade only for entries too generic to match as a substring at all. **Inflection is not stemming**: `rape` catches `rapes` and `raped` but not `raping`, and `tranny` does not catch `trannies` — list the forms you care about.

**Actions:** `shadow` (default), `ban` (permanent, silent), `kick`, `mute`, or `torment` (drop the message + torment list). `shadow` and `torment` both also kick the connection immediately after handling the trip (see "Repeat-Offender Autoban" above) — the message is still dropped/shadow-sent and the IPID still torment-listed exactly as described below, but the offender is disconnected on top of that instead of being left connected with no consequence, and repeated trips count toward the same autoban repeated rate-limit kicks do.

**The word filter itself is independent of `automod_enabled`.** `banned_words.txt` is loaded at startup and on `/reload` regardless of the toggle, so IC, IC showname, OOC message, OOC username, and `/giveaway start <item>` are all checked against it even on a server that never turned `automod_enabled` on. Only the punitive action tier (`ban`/`kick`/`mute`/`torment`) is gated behind `automod_enabled`; with it off, a trip still falls back to the safe `shadow` behavior (drop + torment-list) rather than doing nothing. `/giveaway start` rejects an item containing a banned word outright rather than posting it, and separately caps the item description at 100 characters.

**Name filtering at the broadcast itself.** `pktIC`/`pktOOC` censor a showname and an OOC name when the player sets them, which catches the case that matters most and misses several others — because `PU` is the packet that actually puts a name in front of other people, and it is sent from a dozen places, only two of which are those input paths. The server replays **every connected player's stored OOC name and showname to each newly joining client**, so a name set before a word was added to the list is re-broadcast in full to everyone who joins from then on, long after the input check stopped running; `/forcename` sets a showname that no input path checks; `/reversename`, `/nameshuffle` and `ChangeArea` all re-emit stored names. `nameAllowedInPU` (`internal/athena/pu_name_filter.go`) gates every name-carrying `PU` at `broadcastToAll`/`broadcastToAllClients` and at the join replay, matching through the same tiered word list and the same evasion normalization messages go through.

A match **drops the packet entirely** rather than substituting a placeholder — a name nobody can see is the point, and a placeholder still tells the room somebody tried. A `nuke`-tier match also bans, and bans the **owner** of the name, resolved from the `PU`'s own ID field, never the client the packet was headed for: the join replay carries other people's names to whoever is connecting, so acting on the recipient would ban an arbitrary bystander and re-fire on every join for as long as the name existed. Acting on the owner is correct from every call site and self-limiting, since the first hit closes that connection. Two carve-outs: moderators are never banned by this path, and `/forcename` is refused at the command instead (a moderator typing a slur into someone else's showname is the moderator's doing — banning the target for it would be indefensible). Only PU types 0 and 2 are filtered; type 1 is the character name from `characters.txt` (operator-controlled) and type 3 is an area index, so neither is player-supplied text. `broadcastToArea` — the per-message IC path — does not go through this at all, so nothing here is reachable from the hot path.

**Where the content gate runs (bug fix).** The censor block used to sit part-way down `pktIC`, after the state-update section. That was a leak: the showname is broadcast to **every client** in a `PU` packet as part of that state update, and the showname censor ran roughly forty lines later — so a slur worn as a showname rendered in every client in the room and the filter never got a look at it. Nothing errored and nothing was logged; it simply did not work. The same placement also put the censor *after* `quickdrawOnIC`, `typingRaceOnIC` and `unscrambleOnIC`, so a censored line could still win the unscramble chip prize. The gate now runs immediately after field validation, before any other handling at all — nothing it checks depends on the punishment transforms further down, since `msgText` is decoded exactly once and never reassigned, so it examines the identical string the old placement did. Ordering against the other suppression mechanisms is unchanged and deliberate: censoring runs before the torment branch, before the stealthmute silencing and before the escalating kick, so a censored message can never escape through the delayed torment rebroadcast and a nuked one never reaches any of them. Because this is an ordering property that cannot be observed without standing up a full client, and one that regresses silently, it is pinned against the source itself by `TestContentGatePrecedesEveryLeakInPktIC` (`censor_ordering_test.go`), which fails and names the two landmarks if anything moves above the gate.

**`shadow` (default action):** the censored message is **shadow-sent** — echoed back to the sender so their client shows it as sent, while the packet is dropped for every other client, so nobody ever sees the slur. The speaker's IPID is also added to the torment list (ghost/delayed messages and random silent disconnects, persisted across reconnects), and the connection is kicked (see above). Clean messages are unaffected — only messages that trip the censor are swallowed, but once tripped the speaker stays on the torment list until a moderator lifts it. In `pktIC` the shadow trip is folded into the existing stealthmute `silenced` delivery path (so the sender's echo is a fully-processed, well-formed packet, sent *before* the kick closes the connection) and runs **before** the torment branch, so a censored message can never leak out through `handleTormentedIC`'s delayed rebroadcast; it is also excluded from the area's IC history so `/markov`/`/icwarp` can't regurgitate it. Area logs tag suppressed lines `(censored)`.

**Staff alerts:** every censor trip (any action, and the showname censor below) sends a `[CENSOR]` OOC alert to everyone holding `MOD_CHAT` — who tripped, where, the matched entry, the (truncated) text, and what the server did about it. Every alert carries the opt-out hint; each staff member can mute them per-session with `/censoralerts off` (`/censoralerts on` re-enables; bare `/censoralerts` shows the current state). Manual torment additions (now console-only) deliberately never alert — only censor trips do.

**Torment list tooling:** `/tormentlist` (MUTE) lists every IPID on the torment list with any connected sessions (UID, name, area) or `offline`. `/untorment <ipid>` (BAN) removes one entry; `/untorment all` purges the entire list — every IPID, in memory and in the DB (which also cancels all pending torment disconnect timers). There is no longer any in-game command that *adds* to the list — `/lag` was removed; see "Removed: Console-Gated Commands" below.

**Filter-evasion normalization.** Both `banned_words.txt`/`censored_names.txt` entries and the text being checked are run through `normalizeForFilter` (`internal/athena/text_filter_normalize.go`) before matching, so stylizing, spacing out, or leetspeaking a slur doesn't evade the filter. Only letters survive normalization — everything else is either substituted into a letter or dropped — which defeats:
- stylized Unicode letters — mathematical bold/script/fraktur, fullwidth, circled, superscript, etc. — via NFKD compatibility decomposition (`golang.org/x/text/unicode/norm`), which folds e.g. `𝓷𝓲𝓰𝓰𝓮𝓻` or fullwidth `ｎｉｇｇｅｒ` back to plain `nigger`
- combining marks (accents, "zalgo" corruption) stacked onto letters, and zero-width/format characters (ZWSP, ZWNJ, ZWJ, word joiner, BOM, soft hyphen, ...) inserted mid-word — the latter fall out naturally since only Unicode letters survive
- non-Latin homoglyphs — Cyrillic, Greek, Armenian, and Cherokee letters that render almost identically to a Latin letter (`а`/`е`/`о`/`р`/`с`, `α`/`ο`/`ρ`, Armenian `հ`, Cherokee `Ꮟ`, etc.) — via a table cross-checked against Unicode's own `confusables.txt` (the data UTS #39 security profiles are built from), not hand-guessed
- leetspeak digit/symbol substitutions (`n1gg3r`, `$h1t`) — a small, high-confidence set (`0137458@$!`) that skips ambiguous digits like `2`/`6`/`9` which collide too often with ordinary numbers in chat
- spacing/punctuation insertion (`n i g g e r`, `n.i.g.g.e.r`, `n-i-g-g-e-r`) — any character that isn't a letter after substitution is dropped entirely, so inserted separators never break up a banned substring
- letter stuffing (`niggggger`) — a run of **3 or more** identical consecutive letters collapses down to 2 (not 1), applied identically to both sides of the match

Runs collapse to 2 rather than 1 on purpose: a genuinely double-lettered entry needs to keep both copies, or it silently turns into something dangerous. Collapsing to 1 would shrink `nigger` itself to `niger` (a substring of the common word `nigeria`) and `ngger` to `nger` (a substring of `anger`/`danger`/`finger`/`stronger`/`messenger`/...). Capping at 2 keeps those intact while still defeating obvious 3+-repeat stuffing.

**Load-time safety gate.** Normalization can still turn a specific-looking entry into something dangerously generic — e.g. a postcode fragment `l36` normalizes to `le` (digit `3`→`e` via leetspeak, digit `6` dropped), and `tR0N` normalizes to `tron`, which is a substring of `electronic`/`strong`/`astronomy`. `loadWordListFile` rejects (and logs a warning naming the entry) any normalized result that:
- is shorter than `minNormalizedEntryLen` (4 letters) — below that, a substring match is either unconditional (empty) or broad enough to fire on huge swaths of ordinary chat
- collides with a common English word — `collidesWithCommonWords` (`internal/athena/common_words.go`) checks the normalized entry against an embedded ~10,000-word frequency list ([google-10000-english](https://github.com/first20hours/google-10000-english)) and rejects it if it's a substring of some other, unrelated common word (an entry that equals a real word outright is fine — only being a fragment *inside* a different word counts)

Both checks run once at load time (startup and `/reload`), not on the hot per-message path. This trades word-boundary awareness for evasion resistance: even with both gates, a banned entry can in principle match across what used to be separate words. That's an extension of the false-positive risk substring matching already had (e.g. `ass` inside `class`), not a new category — keep `banned_words.txt`/`censored_names.txt` entries as specific as practical, and check the startup/`/reload` logs for skipped-entry warnings after editing either file.

### OOC Commands That Broadcast (`/global`, `/pm`) — Content Gate (Bug Fix)

`pktOOC` dispatches a slash command and **returns** before it reaches the content gate, the torment branch, the stealthmute branch, the captcha restriction and the raid guard. That is right for the overwhelming majority of commands, which take no free text or address only the caller — and it was wrong for the two that carry arbitrary player text to other people.

`/global` (and its alias `/g`) reaches **every client on the server**; `/pm` reaches any UID list the sender names. Neither was examined by anything. The consequences, all four confirmed against the source:

- **A slur went out in full.** `banned_words.txt` was never consulted, at any tier — a `nuke` word that would have banned the IPID from ordinary OOC broadcast server-wide instead.
- **A stealthmuted player was audible again**, to everyone, through the one command with the largest audience.
- **A tormented player escaped the ghosting.** `isIPIDTormented` was never checked.
- **A connection the join captcha had already stopped could still talk.** The command branch gates on `awaitingCaptcha`, but a connection that has *run out of attempts* carries `captchaRestricted` instead, and nothing on this path consulted it.

The word filter appeared to be working, because it was — on the path those two commands do not take. Only `checkNewIPIDOOCCooldown` was re-checked inside `cmdGlobal`/`cmdPM`, so on a server with a large `new_ipid_ooc_cooldown` the cooldown was the *sole* thing standing between a slur and every connected client, and it lapses on its own: a raider who waits it out, or returns on an IPID the server has already seen, was never filtered at all.

**The raid guard could not see a global either.** `raidGuardOnOOC` is called from `pktOOC` only, below the command branch, so a fan-out conducted entirely through `/g` produced no score, no fingerprint and no cross-IPID correlation. Replaying a real incident (24 globals from 16 IPIDs inside six seconds) through the production correlator: **9 echo hits and one line crossing the strong 4-IPID threshold**, so `SigDupeAcrossIPIDs` — the only signal that satisfies the ban gate — now fires on traffic that was previously invisible to layer 2 in its entirety.

`oocCommandAllowed` (`internal/athena/ooc_command_gate.go`) restores that pipeline in the order `pktOOC` applies it, so the two cannot drift on what a censored, tormented or silenced player sees. Every suppressing branch sends back **exactly the packet that was about to be broadcast**, to the sender alone, so their own client looks normal and the suppression stays undetectable from the inside — the property shadow-sending exists for. Successful globals are also now written to the area buffer, so they appear in modcall reports.

**A verdict stops the message that earned it.** `cmdGlobal` scores the guard *after* the suppression checks — deliberately, so a message the room never heard is never fed to the correlation window — which means `captchaRestricted` gets read before the guard has had its say. `pktIC` has the same ordering and solves it by re-reading the flag after `raidGuardOnIC`; `oocGuardVerdictSuppresses` does the same here. Without it a global that earns a silence, kick or ban is broadcast to every client on the server first, and only the *next* one is stopped — which on the largest audience in the game is the wrong way round.

**And while the server is genuinely under attack, a connection carrying *any* raid evidence is held back from broadcasting at all**, even below the rung that would act on it. A global is the biggest audience a player can reach and it is opt-in, so holding one for the duration of a raid costs an ordinary player nothing — which is not true of area chat, and is why this applies here and nowhere else.

It needs no new safety argument, because it borrows the one under-attack threshold scaling already rests on. It **never creates evidence**: a score of zero — every ordinary player talking through a raid — is untouched at any setting. It requires the under-attack state, which needs cross-IPID correlation or an arrival burst and is **not reachable by one person acting alone**. It respects the playtime tiers, since `raidGuardTier` reports `punishable=false` for moderators and for anyone at or above `raid_guard_min_playtime`, and (like every other guard action) a DB error there fails open. And it is a *hold on one message*, not a punishment: nothing is recorded against the sender, nothing persists, the message is echoed back so their client looks normal, and it lapses on its own when the attack state does. Pinned by `TestGlobalsHeldFromSuspiciousConnectionsDuringAnAttack`, whose subtests assert the two that matter — an ordinary player is never held, and neither is a moderator.

Two deliberate choices. Torment **ghosts** a global rather than routing through `handleTormentedOOC`: that helper re-broadcasts to the sender's *area* after a delay, which for a server-wide global would resurrect the message outside the gate and deliver it to the wrong audience. And `/pm` is gated but **not** raid-guard scored — unlike a global it is not a broadcast, and there is no capture data to calibrate correlating private messages against.

Pinned by `ooc_command_gate_test.go`: the verdicts (clean passes, banned word blocked, spacing evasion normalized, `watch` tier still passes under *every* configured action, stealthmute no longer bypassable), plus two source-order invariants in the shape of `censor_ordering_test.go` — every send in `cmdGlobal`/`cmdPM` must sit after the gate, and `pktOOC`'s command branch must keep returning *before* its own gate, or a global would be filtered twice. The first was verified to fail on the pre-fix code.

### `/8ball`, `/poll`, `/mafia will`, `/mafia whisper` — the Same Gate Bypass, Different Commands (Bug Fix)

A follow-up security scan found the identical shape of bug in four more commands that carry free player text to other people, none of which called `oocCommandAllowed`:

- **`/8ball`** (`reqPerms: NONE`) broadcasts the asked question area-wide via `sendAreaServerMessage`. Now gated on the question text (not the canned answer) the same way `cmdGlobal` gates its message, including `raidGuardOnOOC`/`oocGuardVerdictSuppresses` — both are opt-in slash commands like `/global`, so reusing the under-attack hold is consistent with its own stated reasoning.
- **`/poll`** (`reqPerms: CM` — grantable per-area, not staff-only on most servers) broadcasts the question and options area-wide. Gated the same way, and the check now runs **before** the poll is created and stored in the area (`SetActivePoll`), so a blocked poll never leaves a dangling "active poll" that nobody was ever shown.
- **`/mafia will`** stores free text now and broadcasts it **unattended, later** — whenever its owner dies or is lynched. There is no delivery moment for `oocCommandAllowed`'s torment/stealthmute/captcha suppression branches to hold back, so this doesn't use that gate at all; it mirrors `cmdAreaRename`'s store-now/broadcast-later content check instead (`autoModCheckTiered` → `applyAutoModNuke` on a nuke hit → `raidGuardOnWordHit` for lesser hits), rejecting the content before it is ever stored rather than trying to gate an unattended broadcast that happens minutes or hours later.
- **`/mafia whisper`** delivers free text directly to one other player — structurally a PM. Gated with `oocCommandAllowed` exactly like `/pm`, including `/pm`'s own precedent of **not** raid-guard scoring it (a whisper is not a broadcast, and there's no correlation data to calibrate against).

Pinned by source-order tests in `security_audit_test.go` (`TestFunCommandsConsultTheOOCGate`, `TestMafiaWhisperConsultsTheOOCGate`, `TestMafiaWillIsCheckedBeforeItIsStored`), the same shape as `ooc_command_gate_test.go`'s own invariants: the gate call must appear in the handler's source before any send.

### Join Captcha
`join_captcha = true` makes a previously-unseen IPID answer one question before it can send IC or OOC messages, run most commands, or change the music. Connecting, looking around, picking a character and moving between areas all still work while unverified — this is a gate in front of *speech*, not in front of the connection, because AO2 has no captcha step in its handshake to hook.

It exists for raids specifically. A flood arrives from many IPIDs **and** HDIDs at once, so nothing that keys off identity — bans, cooldowns, per-IP rate limits — can get ahead of it: by the time an address is worth blocking it has been discarded. What every one of those connections has in common is that none of them can answer a question.

**The answer is never printed in the question.** This is the property the whole feature rests on. "Please type ABC123 to verify" is defeated by a regex for whatever follows "type", no matter how random `ABC123` is — so nothing here works that way. The answer always has to be *derived*: add two numbers written as words, reverse a token, take non-adjacent characters out of one, count letters or category members, continue a sequence. Fifteen kinds (`math`, `reverse`, `acronym`, `charpick`, `everyother`, `countletter`, `category`, `sequence`, `nato`, `concat`, `wordlength`, `wordorder`, `sumdigits`, `alphabet`, `oddposition`), several phrasings each, a randomised lead-in and answer hint around all of them, and randomised token presentation (plain / dashed / spaced / mixed case — answers are normalized, so every shape reads the same to a person and none is the same string to a scraper). The invariant is **enforced at generation time**, not merely intended: `challengeAnswerLeaks` re-rolls any draw whose answer would appear in its own prompt, and `TestChallengeAnswerNeverAppearsInPrompt` pins it per kind over hundreds of draws.

**Running out of strikes mutes rather than kicks (`join_captcha_action = "mute"`, the default).** A kick is a loud, immediate signal, and there is no reason to hand one out for free — so the default is to stop the connection's messages reaching the room instead. Set `join_captcha_action = "kick"` to disconnect with an explanation.

Delivery routes through `deliverRestricted` in `pktIC`/`pktOOC`, and a restricted message counts as silenced so it triggers no traps, contagion or love potions. It is written to the area log tagged `(captcha-muted)`, and staff get a `[CAPTCHA]` OOC alert whenever the captcha acts, so it can never act on real players unnoticed.

**Strikes are one shared budget.** A blocked IC message, a blocked OOC message, a blocked command, a blocked music change and a wrong `/verify` all cost one strike out of `join_captcha_strikes` (default 3). Each blocked attempt re-shows the question in OOC and re-sends the popup, since someone who tried to talk in character has demonstrably not read the OOC tab.

**Answering.** The question is sent as an OOC message *and* as a client-side popup (the AO2 `BB` packet — a modal box on desktop AO2, a notice on WebAO; `join_captcha_popup = false` to use OOC only). Players reply with `/verify <answer>`, or by simply typing the answer in OOC — the difficulty is meant to be the question, not remembering the command. Answers are compared through `normalizeCaptchaAnswer` (case, spaces and punctuation ignored; `12` and `twelve` both accepted). Only `/verify`, `/login`, `/help` and `/about` are usable while unverified: most commands broadcast (`/global`, `/pm`, `/roll`), and a gate a bot can talk through is not a gate. Moderators are exempt, and `/login` clears the captcha for anyone — an account with a password is a stronger statement about being a real person than any question, and it is reachable while unverified precisely so a returning player on a new IP is never locked out.

**Who is never asked.** Three exemptions, checked in `issueJoinCaptcha` before a challenge is generated: moderators; any IPID that has already passed one (see persistence below); and any IPID with at least `join_captcha_min_playtime` minutes of accumulated all-time playtime (default 300 = 5 hours), read from the same `KNOWN_IPS.PLAYTIME` figure `/playtime`, the lockdown purge and the autoban tiers use — so all four agree on what "an established player" means. Hours on the server say more about being a real person than any puzzle can, and the captcha is there to sort brand-new connections during a raid, so a regular should never see a question. A DB error while reading playtime means *not* exempt (the captcha is shown) — the opposite of the autoban's fail-open rule, and for the opposite reason: there, failing closed would ban a real player over a hiccup; here, failing open would drop the gate mid-incident, and the worst case of failing closed is that a regular answers one question.

Solved IPIDs are persisted in `JOIN_CAPTCHA_VERIFIED` (DB migration 26) and seeded into an in-memory set at startup, so the check is one map lookup at join and a restart never re-challenges the player base. `join_captcha_remember = false` makes verification last only for the connection. The hot path is a single `atomic.Bool` load per message (`client.awaitingCaptcha`), so a server with the feature off pays essentially nothing.

#### Not beatable from the source
This repository is public, so the built-in questions are public too. Three things address that, in increasing order of strength — and they stack.

1. **A secret key.** Challenges are derived as `HMAC-SHA256(key, ipid ‖ time-bucket)` and that digest drives every random choice a generator makes (`internal/athena/joincaptcha_keyed.go`). Two things follow. A connection **cannot reroll**: reconnecting inside `join_captcha_rotate` (default 1 h) returns the *same* question, so a bot whose solver only handles arithmetic can't disconnect and retry until it draws one — this is the main reason the key exists. And because HMAC-SHA256 is a one-way PRF, observing any number of issued challenges reveals nothing about the key and therefore nothing about what any other address will be asked; two servers running identical code produce unrelated streams. The key comes from `secretseed`, domain-separated (`nyathena-join-captcha-v1`) so it cannot collide with the lockdown passkey MAC that shares that seed; `join_captcha_secret` overrides it; with neither set a random key is generated at startup. `join_captcha_rotate = -1` disables keying and restores independent per-connection draws.
2. **Operator-written questions.** `config/captcha_questions.txt` (`question | answer[, alternative...]`, `#` comments) is not in this repository and cannot be derived from it. Entries are mixed into the built-in pool, weighted by how many there are and capped so they can never crowd the generators out entirely; `join_captcha_custom_only = true` serves nothing else. An entry whose answer appears inside its own question is **rejected at load time** with a warning naming it, since that would be copy-pasteable. Hot-reloadable via `/reload`. If `custom_only` is set but the file is empty the built-ins are used anyway — serving no captcha at all would silently open the gate, which is worse than serving a known one.
3. **A plugin.** `captcha_plugin` points at an external program and the server stops generating questions altogether: it asks the plugin for one, shows it, and passes answers back. The plugin is a separate process speaking newline-delimited JSON on stdin/stdout (`internal/plugin`, protocol in `docs/CAPTCHA_PLUGIN.md`), so it can be written in any language, is not linked into the server (and so not bound by its AGPL licence), cannot crash it, and does not have to be rebuilt when the server's Go version changes. A subprocess rather than Go's `plugin` package on purpose: `buildmode=plugin` demands an identical toolchain and identical dependency versions, so a routine `go get` would break every plugin until each was rebuilt. If the plugin returns only an **opaque token** and no answers, the server calls back to have every attempt judged and the answer never exists inside the server process at all.

If a plugin is missing, down or slow, the built-in questions are used instead — deliberately: a captcha that stops working because a helper crashed would open the gate during exactly the incident it exists for. A plugin failure during *verify* never counts against the player (they may well have been right); they are handed a fresh built-in question instead.

**Staff commands** (`BAN`): `/joincaptcha status` (settings, who is awaiting an answer, who the captcha has acted on), `/joincaptcha verify <uid>` (release a false positive), `/joincaptcha reset <uid|ipid|all>` (clear stored verifications so they are challenged again; accepts a raw IPID so offline players can be reset).

Implemented in `internal/athena/joincaptcha.go` (state, gating, commands), `joincaptcha_challenge.go` (the generators and the leak invariant), `joincaptcha_keyed.go` (the HMAC-derived source), `joincaptcha_custom.go` (operator questions), `joincaptcha_plugin.go` (the plugin bridge) and `internal/plugin` (the subprocess host).

### Censored Showname Shadow-Send
`config/censored_names.txt` lists shownames (or substrings of them, case-insensitive) that nobody is allowed to speak under — independent of `automod_enabled`/`banned_words.txt`. Matching goes through the same `normalizeForFilter` Unicode-bypass normalization as AutoMod. Every IC message a player sends while their showname matches an entry is:
- **shadow-dropped** — echoed back to only the sender (their client shows it as sent) while no other client ever receives it, including the very message that tripped the check.
- **torment-listed** — their IPID goes into the torment set (persisted, the same list the AutoMod censor arms), so they get the ghost/delayed-message treatment and eventual silent disconnects until a moderator removes them (`/untorment <ipid|all>`, `/unpunish -t lag`, or `/unlag`).
- **kicked** — same escalating kick as an AutoMod `shadow`/`torment` trip (see "Repeat-Offender Autoban" above); repeated trips count toward the same shared autoban counter.
- **reported to staff** — the same `[CENSOR]` OOC alert as AutoMod trips, with the `/censoralerts off` opt-out hint.

There is deliberately **no permanent stealthmute anymore**: a player who switches to a clean showname talks normally again (they stay on the torment list until a moderator lifts it), but every message sent under a censored showname is swallowed, kicks the connection, and re-alerts staff. Implemented in `internal/athena/showname_censor.go`, hooked into `pktIC` in `netprotocol.go` right alongside the automod checks — before the torment branch, so a censored message can never leak through the delayed torment rebroadcast. The list is hot-reloadable via `/reload` like `parrot.txt`/`backgrounds.txt`; a missing file simply leaves the feature inactive.

### Punished Showname Random-Punishment Drip
`config/punishment_names.txt` lists shownames (or substrings, case-insensitive, matched through the same `normalizeForFilter` Unicode-bypass normalization as AutoMod) that trigger the **showname punisher**. The first IC message sent under a matching showname **stains the speaker's IPID**: one random punishment from the `/megamaso` pool is applied immediately, and another every minute after that (each lasting 10 minutes, persisted to the DB like any moderator-issued punishment), so effects pile up until staff intervene. Unlike the censored-showname shadow-mute, nothing is silenced — the drip is loud and visible to the target.

The stain sticks to the **IPID, not the showname**: renaming to a clean showname (or reconnecting — the drip watcher re-arms on join, `restoreShownamePunishStain` in `pktReqDone`) does not stop the drip. It stops only when a moderator runs a full-removal `/unpunish` form — `/unpunish <uid>`, `/unpunish <ipid>` (non-numeric targets are treated as raw IPIDs, `/musicunban`-style, so offline players can be cleared too), or `/unpunish all` — all of which clear the stain along with the punishments. After being unpunished, speaking under a different (clean) showname is fine; only using a listed showname again re-triggers the stain. Multiclients sharing a stained IPID are throttled to one combined drip per minute.

Implemented in `internal/athena/showname_punishment.go`, hooked into `pktIC` alongside the censored-showname check. The file is optional (missing = feature off), independent of `automod_enabled`, and hot-reloadable via `/reload`; entries go through `loadWordListFile`'s normalization and safety gates. The per-connection drip watcher goroutine mirrors the leak-free `curseRandomCharWatch` shape (exits on `client.done` or when the stain is cleared).

### Doki Area Effect
Per-area chaos mode for literature-club-themed areas. Enable with `doki_area = true` on the area entry in `areas.toml`. Each IC message rolls independently:
- 1/300 — replace text with a Haschen-themed Monika-style quote
- 1/100 — replace text with one of 130+ original Haschen-themed poems (cute, wholesome, devotional, unhinged, gorey, horror, or letter-scrambled horror; capped at 256 chars)
- 1/200 — replace text with a dark Haschen anagram (unscrambles to a hidden Haschen-themed phrase)
- 1/150 — replace text with a zalgo-corrupted Haschen catchphrase
- 1/100 — zalgo-scramble the player's actual text
- 1/250 — surprise-swap the area background to a random one

Stacks freely with `mirror_area` and `punishment_area` on the same area.

### Punishment-Safe Area (`antipunish`)
Per-area shield that makes a room a safe haven from the punishment system. Enable with `antipunish = true` on an area's entry in `areas.toml`, or toggle it live with the ADMIN-only `/punishmentsafe <true|false>` (also accepts `on`/`off`).

While enabled, **no moderator — regular mod, shadow mod, or admin — can land any punishment-system effect** on a player currently standing in that area: every text effect, dere archetype, themed quote replacer, persona effect, visibility/cosmetic punishment, protocol/viewport curse, timing effect, voice curse, trap/contagion command, `/stack`, `/lovebomb`, `/sfxcurse`, `/forcecolor`, `/translator curse`, `/randompunishall`, `/icwarp`, `/charcurse`, and punishments applied through the Discord bridge are all refused against a target in a punishment-safe area. The issuing moderator gets a message naming which target(s) were shielded; targets outside the area are unaffected by the same command. **Real moderation enforcement is untouched** — `/ban`, `/mute`, and `/kick` work exactly as normal in a punishment-safe area, so actual rule-breakers can still be dealt with.

The shield also closes the indirect paths a punishment could otherwise reach a protected player through: a contagion carrier cannot infect a bystander who is standing in a punishment-safe area, a `/minefield` carrier's mine cannot detonate while they're in one, a `/randomarea` carrier is left in place for any tick where they're currently standing in one (and a safe area is never picked as a warp destination for anyone else's `/randomarea` either), and a `/silencebell` trap cannot be armed in one (nor does it trigger if the area became punishment-safe after arming). `/megamaso`, `/maso`, `/potion`, and `/coinflip` are unaffected since they're self-applied or consensual, not moderator-issued.

```
/punishmentsafe true     # ADMIN — this area now shields players from punishment
/punishmentsafe false    # ADMIN — reopen the area to normal punishment commands
```

Implemented in `internal/athena/punishment_safe_area.go` (the shared `punishmentSafeBlocked`/`partitionPunishmentSafe` guards) and `internal/area/areas.go` (`Area.PunishmentSafe()`/`SetPunishmentSafe`), gated the same way `/judge` gates the WT/CE buttons per area.

### Per-Area Judge Button Toggle
The WT/CE judge buttons (Witness Testimony, Cross Examination and the verdict gavels — the `RT` packet handled by `pktWTCE`) can be disabled per area so players can't spam them. They default to **enabled** (upstream behaviour preserved).

- **Config:** add `judge = false` to an area's entry in `areas.toml` to disable the buttons. Omit the line (or set `judge = true`) to keep them on. The field is tri-state (`*bool`) so an absent value means "enabled".
- **Runtime:** staff with `MODIFY_AREA` run `/judge <true|false>` (also accepts `on`/`off`) to flip the buttons live without a restart.
- When disabled, an attempt to play WT/CE is rejected with *"The judge buttons are disabled in this area."* — the check runs before the existing `CanJud()`/character checks in `pktWTCE`.

### Tiered `/randomsong`
`/randomsong` plays a random track from `music.txt`. Cooldown is tiered:
- regular users — `random_song_cooldown` (default 20 s)
- DJ permission — `random_song_cooldown_dj` (default 5 s)
- moderators — `random_song_cooldown_mod` (default 0 s, unlimited)

### Shadow Mod Visibility
Shadow moderators (`SHADOW` perm bit, no `ADMIN`) are completely hidden from `/gas`/`/players` for non-admin viewers — no "Mod:" line is shown at all. Only admins see them, labelled as `Mod: <name> (shadow)`. Previously the line still rendered as `Mod: Moderator`, which let other moderators infer staff status.

**Shadow mods are `/ignore`-able.** Real moderators and admins can never be silenced with `/ignore` (the command refuses, and their IC/OOC messages bypass every recipient's ignore list). Shadow mods are deliberately exempt from that protection: an un-ignorable sender betrays staff status, so to anyone who ignores them a shadow mod must disappear exactly like a normal player. The single source of truth is `senderBypassesIgnore(perm)` (`internal/athena/client.go`) — `IsModerator(perm) && !IsShadow(perm)` — applied at the `/ignore` guard (`cmdIgnore`) and at all three ignore-list bypass sites (IC broadcast, OOC broadcast, and the buffered `/lifo` release). Pinned by `TestSenderBypassesIgnore`.

### Admin Role Hiding (`/admin hide`)
`/admin hide` (`ADMIN`) lets an admin hide their `ADMIN` role from `/players`/`/gas` for non-admin viewers — their UID, showname, character and IPID are completely unaffected; only the `Mod: <name>` line is suppressed, exactly like a shadow mod is hidden from non-admin viewers. Unlike shadow-mod hiding, other **admins** still see the line, tagged `Mod: <name> (hidden)` so staff oversight is never lost.

```
/admin hide      # ADMIN — hide your ADMIN role from other moderators
/admin unhide    # ADMIN — reveal it again
/admin status    # ADMIN — check whether you're currently hidden
```

The setting is keyed by the moderator's **account name** (not the live connection), so it survives a reconnect or re-login as the same account — it is cleared only by an explicit `/admin unhide` or a server restart (the state is in-memory only, never persisted to disk). Implemented in `internal/athena/admin_hide.go`; the display gate lives alongside the existing shadow-mod check in `writeEntry` inside `cmdPlayers` (`internal/athena/commands_moderation.go`).

### `/unpunish` Self-Removal Protection
Punishments now record the issuer's permission tier (`IssuerSystem`/`IssuerMod`/`IssuerShadow`/`IssuerAdmin`) in the `PUNISHMENTS.ISSUER_TIER` column. `IssuerMod` and `IssuerShadow` sit at the **same protection tier** — a shadow mod carries no more formal authority than a regular moderator, only stealth (they differ elsewhere, e.g. hidden from non-admin `/gas`/`/players`), so a punishment either of them issues is removable by either of them, self included. Only an **admin-issued** punishment is protected from self-removal by anyone below `ADMIN` — `/unpunish <own uid>`, `/unpunish -t <type> <own uid>`, and the self-target slice of `/unpunish all` are all gated on `IssuerAdmin`. Admins bypass the gate entirely. Persists across restarts via DB migration 18.

### `/ban` Registered-Account Alert
Whenever `/ban` (by UID or by IPID) successfully bans an IPID, the server checks whether that IPID is linked to a registered player account (`db.GetUsernameByIPID`, the same account link `/resetusername`/`/playtime`/the casino system use) and, if so, sends every online moderator an OOC `[BAN] IPID <ipid> is linked to registered account "<username>".` alert. This is purely informational — the account itself is never touched (not purged, not banned, not locked, no chips/tags/playtime affected); a ban only ever affects the IPID/HDID pair. It just means staff banning an anonymous-looking connection immediately learn it was actually someone's logged-in account, in case that's relevant to the incident (e.g. watching for that same account resurfacing on a new IP). Implemented in `alertBannedAccountLinks` (`internal/athena/commands_moderation.go`), called once per uniquely-banned IPID at the end of `cmdBan`.

### Repeat-Offender Autoban (Rate Limit + Censor)
A single message-based rate-limit violation (`message_rate_limit`, `ooc_rate_limit` — IC/OOC/music spam, handled by `Client.KickForRateLimit`) has always just resulted in a kick, not a ban, so a bot that reconnects right after being kicked could cycle through kick → reconnect → spam forever with no lasting consequence. The same used to be true of an AutoMod censor trip under the `shadow` or `torment` action (see AutoMod below): the offending message was dropped, but the connection stayed open with no immediate consequence, so a determined troll could keep hammering slurs indefinitely with nothing telling them their messages were failing.

Both are now unified into one repeat-offender system. When `rate_limit_kick_autoban` is enabled (default: on), an IPID that racks up `rate_limit_kick_autoban_threshold` kicks (default: **2**) within a rolling `rate_limit_kick_autoban_window` (default: 600s / 10 minutes) is handed a short automatic ban lasting `rate_limit_kick_autoban_duration` (default: 15 minutes) — long enough to break a spam bot's reconnect loop without being a full moderator-length ban. **The counter is shared across both kick kinds**: a rate-limit kick and a censor-trip kick on the same IPID count toward the exact same spree, so a player who trips one of each is treated as a repeat offender exactly like one who trips the same check twice. A kick that arrives after the window has elapsed starts a fresh count instead of adding to an old one, so the autoban targets an active spam spree, not kicks earned far apart over a connection's whole lifetime on the server.

Every call site of `KickForRateLimit` funnels through the same choke point — `pktIC`, `pktChangeChar`, `pktAM` (music/area-change), and both the general and IP-specific OOC rate limiters in `pktOOC` — so the rate-limit half covers IC and OOC alike, and in every one of those handlers the check runs before any packet parsing or broadcasting, so the message that trips the limit is never broadcast to anyone. The censor half is covered by `Client.KickForCensorTrip`, called from every `autoModCheck` call site (IC message, IC showname, OOC username, OOC message) whenever the configured action is `shadow` or `torment`, **and** from the separate `checkCensoredShowname` (`censored_names.txt`) match in `pktIC` — so a name-based trip escalates exactly like a message-content trip. `kick`/`mute`/`ban` AutoMod actions are untouched (already a stronger consequence on their own). This is only ever wired into the automatic, server-triggered censor path — the console's manual `torment <ipid>` (which calls `addTormentedIP` directly) never counts toward the autoban, since that's a deliberate staff action, not something that should also escalate on its own.

**Three playtime tiers — lenient toward real players, aggressive toward obvious bots.** How many kicks it takes to escalate depends on `rateLimitAutobanThresholdFor`'s read of the IPID's accumulated all-time playtime (the same figure `/playtime` and the Lockdown Playtime Purge use):
- **Aggressive** — below `rate_limit_kick_autoban_lenient_playtime` minutes (default: 2 hours), including a brand-new connection with no playtime at all: the base `rate_limit_kick_autoban_threshold` (default: **2**) applies. This is the tier aimed at the intended target — a fresh connection that's purely there to spam.
- **Lenient** — at or above `rate_limit_kick_autoban_lenient_playtime` (default: 2h): needs `rate_limit_kick_autoban_lenient_threshold` kicks instead (default: **5**, clamped up to at least the base threshold so a misconfigured value can't make this tier *more* aggressive than the base one). Not immune, just harder to trip by accident than a brand-new connection.
- **Fully exempt** — moderators, and any IPID at or above `rate_limit_kick_autoban_min_playtime` (default: 20 hours): never auto-banned by this mechanism no matter how many times it trips the rate limit or censor. Still gets the ordinary kick every time, same as a single violation.

This reuses `lockdownPurgeEligible`'s exact playtime-threshold comparison from the Lockdown Playtime Purge feature (see below) for the top and bottom tier boundaries, since both features are answering the same question — "is this a real player, or a brand-new connection with nothing vouching for it" — for the same reason. A DB error while checking playtime fails open (never bans on a hiccup). Set `rate_limit_kick_autoban_min_playtime = 0` (and `rate_limit_kick_autoban_lenient_playtime = 0`) to disable the playtime tiers entirely and apply the base threshold flatly to everyone.

This is separate from `conn_flood_autoban` (repeated *connection* rejections) and `packet_flood_autoban` (raw AO2 packet flooding, hundreds of packets/second — always bans immediately, no leniency, since no legitimate client ever sends at that rate). All three share the same underlying ban mechanism (`autobanFlooderFor` in `internal/athena/server.go`, called with an explicit duration rather than always using `default_ban_duration`) and record the ban exactly like a moderator-issued one (`db.AddBan`, so `/getban`/`/unban` work on it normally). Every automatic ban from any of the three also triggers the same registered-account alert a manual `/ban` does — see "`/ban` Registered-Account Alert" above, which `autobanFlooderFor` now calls too — so staff immediately see if an autoban caught a real logged-in player and can `/unban` a false positive. Implemented in `internal/athena/server.go` (`registerRateLimitKick`, `rateLimitAutobanThresholdFor`, `rateLimitKickAutobanDuration`) and `internal/athena/client.go` (`escalatingKick`, `KickForRateLimit`, `KickForCensorTrip`).

**New-IPID IC cooldown ordering.** `new_ipid_ic_cooldown` (default: 10s, mirrors `new_ipid_ooc_cooldown`) blocks a brand-new IPID from speaking IC until the cooldown elapses. Unlike the OOC cooldown (checked before AutoMod in `pktOOC`), the IC cooldown check in `pktIC` runs deliberately *after* the rate-limit and AutoMod/censor/torment checks — so a flood or slur burst from a fresh connection during the cooldown window is still judged (and can still trigger the autoban above) by those checks, rather than being silently absorbed by the cooldown. The cooldown itself only ever rejects an otherwise-clean message sent too early.

### Connection-Handling Hardening: Panic Recovery and a Bounded Packet Reader (Bug Fixes)

A security scan of the packet-handling path (`Client.HandleClient`, `internal/athena/client.go`) turned up two gaps below the level of any individual command, meaning either one could be triggered from *any* future handler bug, not just a known one.

**No `recover()` anywhere in the per-connection read loop.** `HandleClient` dispatches every parsed packet to its handler with `v.Func(client, pkt)`, and across the ~189 files backing that handler surface, nothing recovered a panic. Since a panic in an unrecovered goroutine crashes the entire Go process — not just that goroutine — a single malformed-input bug in any one handler took the whole server down for every connected player, not just the connection that triggered it. `HandleClient` now wraps the loop in a `defer` that recovers, logs the panic with the client's IPID/UID, the header of the packet being handled (tracked in `lastHandledHeader`, updated right before each `v.Func` call) and a full stack trace (`logger.LogErrorf` + `logger.WriteAudit`, so the event survives a restart), then closes just that one connection via the existing idempotent `client.markClosed()` rather than letting the loop continue against what may now be partially-mutated state. Every `.mu.Lock()` in this codebase pairs with an immediately-following `defer Unlock()`, and Go runs deferred functions during panic unwind before any recover further up the stack runs, so lock safety holds regardless of where the recover sits. This is deliberately *not* wired into the live `[RAIDGUARD]`/`[CENSOR]`-style staff OOC alerts — a panic is a code bug for a developer to fix, not an actionable moderation verdict a staff member can act on by typing a command. Pinned by `TestHandlerPanicIsRecoveredAndClosesOnlyThatConnection` (`security_audit_test.go`), which is self-verifying in an unusual way: without the fix, the test doesn't just fail — it crashes the entire `go test` binary.

**Unbounded packet read buffer.** The classic (`br.ReadString('%')`) and JSON (`json.NewDecoder(br).Decode(&msg)`) packet readers had no cap on bytes buffered before a terminator or a complete document was seen, bounded only by the ping-timeout watchdog — a client that never sends a terminator could make the buffer grow without limit, a memory-amplification DoS across many concurrent connections. `client.conn` is now wrapped in a `boundedReader` (a small counting `io.Reader`, `maxPacketBytes` = 64 KiB — a local constant, not a config key, well above any legitimate packet) *before* `bufio.NewReader` is constructed, so both the classic and JSON paths are bounded uniformly without touching their parsing logic. The counter resets to a fresh budget at the top of every loop iteration, so each packet-read attempt gets its own 64 KiB rather than the whole connection sharing one. Exceeding the cap returns a sentinel `errPacketTooLarge`, which every read branch checks and responds to by logging a warning (naming the IPID/UID) and closing the connection outright — mirroring the existing packet-flood branch's own close-and-return convention — rather than silently truncating, which could let a partial/attacker-crafted packet parse in a worse way than just refusing it. Pinned by `TestBoundedReaderRejectsOversizedRead`/`TestBoundedReaderAllowsExactlyMaxBytes`/`TestBoundedReaderResetAllowsFreshBudgetPerPacket` (`security_audit_test.go`).

### `/area rename ".."` Path Traversal (Bug Fix)

`/area rename` is reachable by any area CM, not just an admin — and neither `areaNameRejection` (the command's own validator) nor `logger.sanitizeAreaName` (which turns an area name into a log-directory path component) special-cased the literal names `.` or `..`. `sanitizeAreaName` strips `/`, `\`, and a handful of other filesystem-unsafe characters, but neither `.` nor `..` contains any of them, so `/area rename ..` produced a name that survived sanitization intact and `filepath.Join(LogPath, "..")` resolved **one directory above** the configured log path — a CM could make the server write a stray log file (`..-YYYY-MM-DD.txt`) outside `logs/`.

Fixed at both layers, matching this codebase's own AutoMod-tier-integrity lesson (the same validation has to hold at every place data flows through, not just the first place it's checked): `areaNameRejection` now rejects the literal names `.` and `..` outright (exact match only — this is not about names merely containing a dot), and `sanitizeAreaName` substitutes a safe placeholder for either as defense-in-depth for any caller that reaches the log path without going through `areaNameRejection` (e.g. a name loaded straight from `areas.toml`). Pinned by `TestAreaRenameRejections`' new `dot`/`dotdot` cases, `TestSanitizeAreaName`'s new cases, `TestCreateAreaLogDirectoryRejectsTraversal`, and `TestAreaNameRejectionBlocksDotAndDotDot`.

Note: this bug only ever let a name make the server **write** a file outside `logs/` — there is no command anywhere in the codebase that reads an arbitrary file path built from network/player input, so it was never a way to read or exfiltrate logs.

### `/login` Brute-Force Hardening (Bug Fix)

`db.AuthenticateUser` had two gaps: no rate limit on failed attempts at all (bcrypt's own cost slows down a single guess, but a raider running many parallel connections against one IPID had unbounded attempts against a real account), and a minor username-enumeration timing gap — an unknown username skipped `bcrypt.CompareHashAndPassword` entirely (empty stored hash fails the format check almost instantly), so it returned measurably faster than a wrong password against a real, registered username.

Both are fixed. `AuthenticateUser` now always runs a real bcrypt comparison, even for an unknown username — against a package-level `dummyBcryptHash` (generated once at package `init()`, same cost factor as a real password hash) when the row isn't found — so the two failure cases take comparable time. Separately, `checkLoginRateLimit`/`registerFailedLogin`/`clearLoginAttempts` (`internal/athena/server.go`) track failed `/login` attempts per **IPID** (so reconnecting doesn't reset the count) in a rolling window: `maxLoginAttempts` (5) failures within `loginAttemptWindow` (5 minutes) blocks further attempts until the window clears; a successful login clears the counter immediately. Local constants, not config keys — matching how other small safety caps in this codebase (e.g. `maxAreaNameLen`) are sized without an operator-facing knob. Pinned by `TestLoginRateLimit` (`security_audit_test.go`).

### Server Freeze: A Blocking Socket Write Under the Field Mutex (Bug Fix)

A raid froze the server without crashing it, while AutoMod was correctly catching everything the raiders said. The cause was a lock held across blocking I/O, and one connection that had stopped reading was enough to do it.

```go
func (client *Client) write(message string) {
    client.mu.Lock()
    io.WriteString(client.conn, message)   // blocking, no deadline
    client.mu.Unlock()
}

func (client *Client) Area() *area.Area {
    client.mu.Lock()                        // the SAME mutex
    defer client.mu.Unlock()
    return client.area
}
```

`client.mu` is the general field mutex, and `Area()` is what **every broadcast calls on every client** to decide who the recipients are. So a client blocked in `write` holds the mutex that every fan-out on the server needs. `runWriter` clears the write deadline after each of its own writes, so nothing bounded this one: on a socket whose send buffer is full, `io.WriteString` blocks until TCP gives up, and the mutex is never released.

`pktReqAM` used that path to push the ~45 KB music/area list **on join**. A raid is hundreds of connections joining at once, many of them bots that connect and never read — so the raid supplies exactly the input that pins the lock.

`SendPacketSync` had the same shape. It does set a deadline, so it is bounded at `writeDeadline` (30 s) rather than forever — but 30 seconds of holding the mutex every broadcast needs is still a server-wide stall, and the path is reached **in bulk at the worst possible moment**: the lockdown playtime purge delivers its kick reason through `SendPacketSync` to every connection it removes. Turning on lockdown to stop a raid fans that out to hundreds of half-open sockets at once, so **the anti-raid response was itself capable of freezing the server**.

**The fix:** neither path holds `client.mu` across I/O any more. `mu` is taken only long enough to read the conn and the log fields, and a separate `writeMu` preserves the one thing it was providing — that two concurrent inline writes cannot interleave their bytes. `Client.write` also now sets a deadline, so it is bounded even in the fallback case, and `pktReqAM` enqueues the pre-built SM blob (`getSMPacketBytes`, published in lockstep with the string form) through the ordinary asynchronous queue rather than writing inline at all.

Pinned by `write_lock_stall_test.go`, which stands up a connection that blocks in `Write` and asserts an ordinary broadcast still completes — once for the join path and once for the purge path. Both were verified to fail against the pre-fix code (a 3-second timeout) and pass after (instant).

### CharsCheck Fan-Out (Bug Fix)

Separate from the freeze above, and found while investigating it: a real inefficiency in how broadcasts are serialized, worth fixing on its own but **not** on its own an explanation for the freeze (see the measurement at the end of this section).

A packet capture taken during the incident spans **2.16 seconds** and carries **6.1 MB** of outbound payload. The breakdown is lopsided:

| Packet | Sends | Bytes | Share | Avg size |
|--------|------:|------:|------:|---------:|
| **CharsCheck** | 515 | 4,641,175 | **75.8%** | **9,011** |
| ARUP | 4,133 | 520,951 | 8.5% | 126 |
| SC / SM (join lists) | 14 | 790,948 | 13.0% | ~56,000 |
| PU | 7,705 | 119,498 | 2.0% | 15 |
| PR | 2,675 | 25,211 | 0.4% | 9 |

Those 4.6 MB of `CharsCheck` were produced by **eight character changes**. `CharsCheck` carries one entry per character on the server (4,465 here), so it is ~9 KB, and it is broadcast to the whole area on every character change — 64 recipients each, i.e. **0.58 MB per character change with 45 people online**.

`SendPacket` serializes per recipient: it walks the entire argument slice and allocates a fresh buffer for each client. For a packet with four short fields that is irrelevant. For one with 4,465, sending an *identical* payload N times costs N × 4,465 loop iterations and N × 9 KB of immediately-garbage allocation.

Benchmarked at 600 clients, one character change:

| | per recipient | build once | |
|---|---:|---:|---|
| time | 12.65 ms | 0.21 ms | **60x** |
| allocated | 5.76 MB | 83 KB | **69x** |
| allocations | 603 | 5 | 120x |

Rapid character re-rolling is a *documented raid signature* (`SigCharChurn` scores it), so a raid drives precisely the input that maximises this.

**How much of the freeze this actually explains: measurably, not much — and that is worth recording.** Replaying the real character-change timeline from `raid_capture.log` against the real broadcast code (`freeze_sim_test.go`) reports CPU-seconds consumed per wall-second of captured traffic. Above 1.00 the work arrives faster than it can be done and the backlog grows without bound; the measured peak raid rate is 8–10 changes/sec:

| clients | before | after |
|---|---:|---:|
| 118 (the freeze capture's population) | 0.02x | 0.00x |
| 600 | 0.13x | 0.00x |
| 1000 | 0.19x | 0.01x |

So the fan-out never came close to saturating a core at real raid rates, and the first, confident diagnosis — that this *was* the freeze — was wrong. Building the simulation is what showed it, which is the argument for building one rather than extrapolating from a microbenchmark. What the projection does show is allocation: **349 GB churned over two hours** of sustained raiding at 600 clients, against 6.2 GB after. Worth fixing for GC pressure alone; not a freeze.

Two things it was *not*, both already fixed earlier and confirmed still correct here: `SendPacket` is asynchronous (it buffers and pushes to `sendCh`, dropping on overflow rather than blocking a broadcaster on one slow consumer), and `ClientList.ForEach` releases its read lock after snapshotting, before any send. Neither a stuck socket nor lock contention was involved.

`broadcastToAreaOnce` (`internal/athena/broadcast_prebuilt.go`) serializes the packet **once per wire format** and hands every recipient the same immutable buffer via `Client.sendPrebuilt`. Safe because `runWriter` only reads the slice (`conn.Write(buf)`, and `string(buf)` for the network log) and never mutates it, so one buffer can back any number of queued sends. It keeps `SendPacket`'s contract exactly: non-blocking, and a full queue drops rather than blocking or disconnecting. JSON-mode clients get their own single `BuildJSON` blob, and the MS broadcast schema is validated once per packet rather than once per recipient, since the bytes are identical.

Applied to all five `CharsCheck` broadcasts and deliberately nowhere else: ARUP (126 bytes) and PU (15 bytes) are three orders of magnitude smaller per packet, so rebuilding those per recipient costs little and the indirection would not pay for itself. Pinned by `broadcast_prebuilt_test.go`, which asserts the emitted bytes are identical to what the per-recipient path produced, that all recipients genuinely share one backing array, that area scoping still holds, and that a client which is not reading gets its packet dropped instead of stalling the fan-out.

### Raid Guard (Behavioural Fan-Out Detection)
Every rate limit the server had before this feature — `message_rate_limit`, `ooc_rate_limit`, the Repeat-Offender Autoban above, even `conn_flood_autoban` — is keyed on one identity: an IPID, an HDID, a connection. That structurally cannot see a raid, because a raid is not one identity behaving badly, it is a *fan-out*: many identities behaving identically. A real incident on this server put **65 distinct IPIDs** on the box inside **three seconds**, and every single one of them individually stayed under `message_rate_limit` the whole time. Measured against a clean baseline capture, the per-IPID speech rate of a raider (median **2.31 msg/s**) was indistinguishable from an ordinary player mid-conversation (**2.00 msg/s**) — while the *aggregate* rate differed by **41x**. There is no per-IPID threshold that separates those two populations, because on a per-IPID basis they are the same population. Catching this needed something that scores behaviour instead of volume, and something that looks at the server as a whole instead of one connection at a time.

**Two layers.** Layer 1 (`internal/athena/raidguard.go`) scores what one connection did: did it pick a character in under a second, is every one of its messages an objection shout, is it wearing a different OOC name on every line. Layer 2 (`internal/athena/raidguard_corr.go`) scores what the server as a whole is seeing: the same text arriving from many *different* IPIDs inside a few seconds, and bursts of brand-new IPIDs arriving per second. Layer 1 is necessary but cannot see a fan-out — one connection looks the same whether it's alone or one of sixty; layer 2 is the layer that actually sees the shape of a raid, and it's the reason this feature exists rather than just tightening the existing per-IPID limits further.

**Signals, and roughly why each earned its weight** (`raidSignalWeight` in `raidguard.go`, highest first):
- **Handshake anomaly (50)** — an RC/RM/RD (character list, music list, area list requests) arriving before the connection's own `askchaa`. No real client implementation does this; a bot that skips straight to requesting data skips the step that precedes it in every real handshake. Measured in **67% of raid connections vs 0% of baseline**.
- **Handshake replay (45)** — an `askchaa` arriving on a connection that has *already joined*. Alone among these signals it is not a judgement about behaviour but a protocol impossibility: `askchaa` is only ever sent in reply to `PN`, `pktId` is the only place `PN` is emitted, and it returns early once the connection has a UID — which `pktReqDone` (RD) assigns. After RD the server therefore never sends another `PN`, so a client whose `askchaa` is driven by `PN` can never send a second one. A connection that sends one anyway is replaying a canned handshake script instead of reacting to what the server actually said.

  Confirmed at source level against **every client family that reaches this server**, because a signal weighted this highly is only worth having if it cannot fire on a real player: **AO2 desktop 2.11** disconnects its own `server_connected` signal inside `NetworkManager::join_to_server()` *before* shipping the packet, so the slot structurally cannot run twice; **AsyncAO** drives the ladder off `PN` and its `courtroom_test.go` feeds a second `PN` mid-load specifically to assert the reply ladder stays exactly `HI, ID, askchaa, RC, RM, RD`; **LemmyAO / webAO** send it only from `applyServerInfo`, the `PN` handler in `client/handshake.ts`. Measured per connection over two clean and two raid captures: **0/17 webAO, 0/11 LemmyAO, 0/6 AsyncAO, 0/60 clients sending no ID packet** — against **37/122 connections claiming to be `AO2 2.11.0`**, a string the desktop client's own source says cannot do this. The signal is really catching a *lie about which client is connecting*.

  Weighted just under the ordering signal so that on its own it reaches the captcha rung but **never** the kick rung at any tier — on the arithmetic, not merely via `clampDisconnect`'s three-signal floor, which is a backstop for a mis-calibrated weight and is spent if a weight leans on it. It also sits below the watch threshold for anyone with two hours of playtime, so an established regular is untouched by it alone. It fires at *handshake*, before the connection has said anything, which is what makes it the answer to the latency floor described under "Raid mode" below: the first message no longer has to be the evidence.
- **Duplicate text across IPIDs (45)** — the same message (by token shingle, not exact match — see below) seen from `raid_guard_corr_ipids` (default 4) distinct IPIDs inside the correlation window. The one signal a per-IPID limiter is structurally incapable of producing, and measured at **34% of raid messages vs 0% of baseline** once trivially short lines (a shared `/gas` or `/cm`) are excluded from the comparison. This is the only signal that satisfies the ban gate.
- **Text echoed by another IPID (25)** — the weak half of the same evidence, at `raid_guard_corr_ipids_weak` (default 2) distinct IPIDs: somebody else is saying your line, but not yet enough people to call it a fan-out. Weighted deliberately below the watch rung at every playtime tier, so on its own it neither alerts nor acts — it takes a second independent signal to reach the captcha rung — and it **never** satisfies the ban gate, so two players quoting each other cannot arm a ban between them. Measured at 0 across both clean captures (85 messages, 44 distinct players) against 3 on the 2026-08-31 raid and 24 on the 2026-08-27 raid capture. See "Grading the corroboration" below for why it exists.
- **Objection spam (35)** — a high, sustained fraction of a connection's messages carrying a non-zero objection modifier (Objection!/Hold It!/Take That!). Measured at **90% of raid IC packets vs 0% of baseline** carrying the modifier at all, and the raid population was **bimodal**: a raider who used it used it on **100%** of their messages, not some middling fraction. A real trial interleaves dialogue with shouts, so the signal requires both a floor of messages and a high fraction (default 80%, `raid_guard_objection_fraction`) before it fires — one dramatic entrance can never trip it.
- **OOC name / showname churn (30 each)** — a connection cycling through several distinct OOC names or shownames rather than sending the same one on every packet, as a real client does.
- **Fast character pick (25)** and **character churn (25)** — picking a character implausibly soon after connecting (`raid_guard_fast_charpick_ms`, default **250**), or re-rolling faster than a person can read a list of several thousand characters. Measured at **67% of raid char-pickers re-rolling within 1 second vs 0% of baseline**. The pick threshold is bounded by protocol physics rather than taste: a real client cannot pick before it has received the character list, because it builds the picker from that list, and the fastest full handshake in the clean capture was 330 ms. 1000 ms is the obvious-looking value and is wrong — **6 of the 19 real handshakes** in that capture completed inside it, so a fast connection auto-picking from a share link would have tripped the signal. `config_sample/config.toml` shipped 1000 while the code and `settings.DefaultConfig` used 250; the sample has been corrected, so **a server running a config copied from the old sample should set `raid_guard_fast_charpick_ms = 250`**.
- **Said something severely flagged (40)** and **said something flagged (20)** — a `severe`-tier and a `default`/`watch`-tier hit on `banned_words.txt` respectively (see "Severity tiers" under AutoMod above). A slur is strong evidence of intent but *zero* evidence of a coordinated fan-out, so these score and never gate. 40 is chosen against one specific number: the challenge threshold at the strict tier is 60 × 70% = **42**, so anything at or above that would let a single flagged word act on a brand-new connection with no other evidence at all. 40 sits just under it, which makes "a slur alone never rises above an alert" true at *every* tier rather than approximately true — and costs nothing, because 40 plus any other signal (even shouty spam at 10) clears 42 and reaches the captcha rung. That composite — a new connection re-rolling characters, shouting on every line, speaking a second after joining, **and** saying something flagged — is what these exist to catch; the word alone is not. A `nuke`-tier hit fires neither signal: scoring a connection that has just been banned outright would be pointless, and routing content rules into the behavioural ban gate would weaken it. Weights are configurable via `raid_guard_slur_severe_weight` / `raid_guard_slur_flagged_weight`.
- **Fast first speech (20)** — speaking implausibly soon after picking a character. Weakest of the timing signals on purpose: a returning player with a saved character can legitimately be fast, so this is a nudge, never an accusation on its own.
- **Shouty spam (10)** and **global spam while brand new (10)** — content-shape signals, weighted lowest because AO2 is a courtroom game where dramatic capslock is ordinary (`isShoutySpam` requires 25+ characters, ≥80% caps, *and* a repeated word, all three, before it fires at all). Present so they can tip a borderline case, never enough to matter alone.

**Signals that were measured and deliberately rejected — this matters as much as the ones kept**, because a plausible-sounding signal that doesn't actually separate the populations is just a way to false-positive real players:
- **Multiple HDIDs sharing one IPID.** Sounds like multi-accounting, but occurred at statistically identical rates in raid and clean traffic — **4.7% vs 5.0%**. Two people behind the same NAT is exactly as common during a raid as on an ordinary evening.
- **CT:MS ratio** (OOC-to-IC packet ratio). *Inverts* — it did not merely fail to separate the populations, it pointed the wrong way, so using it would have penalized normal chatty players over quiet raiders.
- **Repeated `VS_JOIN`.** Also inverts, and for a codebase-specific reason: Nyathena's own `pktVSFrame`/voice-join handling has an explicit "client is retrying while already joined" branch (`internal/athena/voice.go`), so a legitimate client resending `VS_JOIN` is expected, documented behaviour here — not an attack signature to score against.
- **Evidence / flip / realization / text_color field misuse.** No signal in either dataset. Included in the initial hypothesis because they're all fields a naive bot script might mishandle; in practice neither raiders nor real players misuse them measurably, so scoring on them would have added noise with no separating power.

**Safety design — this is the part that matters most, because the one thing this feature must never do is ban a legitimate player:**
1. **No single signal can ban.** The ban threshold (`raid_guard_score_ban`, default **160**) sits above any one signal's weight (the highest is 50) *and* above any pair of them, so reaching it requires at least three independent things having been true about the same connection at once (`verdictForTier` in `raidguard.go`). With the two word-list signals there are thirteen signals totalling 365; the lenient tier's threshold is 320, so a player with 2h+ playtime remains unbannable by this system on arithmetic rather than on policy.
2. **A ban additionally requires cross-IPID corroboration and a concurrently-detected server-wide raid.** Even a connection that has racked up a ban-level score is only ever banned if `SigDupeAcrossIPIDs` has fired (its *own* text was corroborated from other IPIDs) **and** `raidGuardUnderAttack()` is true (the server is *currently* seeing raid-shaped correlation or arrival-burst evidence, held for 2 minutes after the last piece of it). Neither condition is reachable by one person acting alone, however oddly they behave — so a lone unusual player is **structurally unbannable** by this system no matter what they do. A connection that earns a ban-level score without that corroboration is quarantined (silenced) instead, which is self-service: a real player can free themselves by answering the pending join-captcha question, without needing staff at all. This is enforced in `raidGuardEnforce` (`raidguard.go`), not left to be true by convention.
3. **Four playtime tiers**, read from the exact same `KNOWN_IPS.PLAYTIME` figure `/playtime`, the Lockdown Playtime Purge, and the Repeat-Offender Autoban above all use — so "an established player" means one thing everywhere on this server:
   - **Strict** — under `raid_guard_strict_playtime` minutes (default 15): thresholds scaled to `raid_guard_strict_scale` (default **70%**, i.e. 30% less evidence needed to act). This is the population a raid is actually made of — brand-new connections with no history — so it's the one tier the guard leans *into* rather than away from.
   - **Baseline** — thresholds exactly as configured.
   - **Lenient** — at or above `raid_guard_lenient_playtime` (default 2h): thresholds scaled to `raid_guard_lenient_scale` (default **200%**, twice the evidence needed). Not immune, just materially harder to trip by accident.
   - **Fully exempt** — moderators, and any IPID at or above `raid_guard_min_playtime` (default 20h): never acted on however they behave. Staff still get the `[RAIDGUARD]` alert, so a regular whose account is misbehaving stays visible without the server doing anything to them automatically.
4. **A DB error fails OPEN** (`raidGuardTier` in `raidguard.go`) — a hiccup reading playtime must never ban somebody; the worst case of failing open is one raider message getting through, and that is deliberately the cheaper mistake here.
5. **Off by default** (`raid_guard_enabled = false`), and the default ceiling is `raid_guard_max_action = "captcha"`, **not** `"ban"` — an operator has to deliberately raise the ceiling to `kick` or `ban` before the guard can do anything stronger than quarantine-with-self-service-release. When disabled, the hot path costs one atomic load and nothing else.

Matching is by **token shingle**, not whole-message equality (`raidFingerprints`, `raidguard_corr.go`): raiders routinely vary a line slightly between sends — prepending `/g `, appending another slur — and exact-text matching missed a measured 12 such pairs in the capture that shingling catches. Lines shorter than the correlation floor (default 15 normalised characters, `raid_guard_corr_min_len`) never fingerprint at all, because the baseline capture showed a 57% "duplicate" rate that was entirely two players independently typing `/gas` or `/cm` — short identical lines are what ordinary players legitimately have in common, and correlating on them would flag half the server on a slow evening.

**Grading the corroboration (the slow fan-out).** The original layer-2 threshold was a single number — four distinct IPIDs on one line inside ten seconds — calibrated on a raid at its absolute loudest: 65 IPIDs in three seconds, the same line repeated far past four times, every threshold crossed instantly. A second real incident (2026-08-31, kept as `internal/athena/testdata/raid_20260831_iclog.txt`) had the opposite shape and went **completely undetected by layer 2**: ten IPIDs spread ten *different* slurs over twenty-six seconds, re-using lines twice and three times but never four, and spacing the repeats 16 and 19 seconds apart — outside the ten-second window. Replaying that capture at the shipped defaults produces zero correlation hits, so `SigDupeAcrossIPIDs` never fired, `raidGuardUnderAttack()` never became true, and no ban was ever reachable while the raid ran. That is what "a few messages got in" looked like from the inside. Three changes address it, all validated against both clean captures:

- **The evidence is graded rather than all-or-nothing.** `raid_guard_corr_ipids_weak` (default **2**) fires the 25-point echo signal; `raid_guard_corr_ipids` (default **4**, unchanged) still fires the 45-point `SigDupeAcrossIPIDs`, still marks the server under attack, and is still the only thing that satisfies the ban gate. Splitting them rather than simply lowering the one threshold is what keeps that gate honest — under a single lowered threshold the same event would both mark a connection corroborated *and* flip the server-wide flag, which are supposed to be two independent facts. Replaying the 2026-08-31 raid, the guard now corroborates on its **fourth message, seven seconds in** — the earliest point any detector could, since nothing can see a fan-out before a second party has fanned something out.
- **`raid_guard_corr_window` moved from 10 to 30 seconds**, so repeats 16 and 19 seconds apart correlate. Safe only because of the next change.
- **A fingerprint's tally now expires from its *first* sighting, not its last touch** (`corrEntry`, `raidguard_corr.go`), making the window tumbling rather than sliding. Under last-touch expiry an entry survived as long as anything kept touching it, so an area catchphrase said by a different player every twenty seconds would accumulate IPIDs indefinitely and eventually cross any threshold. Bounding the tally to one window of wall clock means correlation only ever describes what happened *inside* a window — which is the claim the signal is making — and is what makes a longer window safe rather than dangerous.

**Raid mode: tightening thresholds while a raid is actually happening.** The per-connection design has a latency floor by construction — scoring needs a message to score, so a raider's *first* message reaches the room before anything is known about that connection, and across a fan-out that first message times the number of connections is the bulk of what gets through. That is not a detection failure: measured against real captures the guard already acts on essentially every raiding connection, it simply acts one or two messages in. Server-wide evidence, though, arrives far sooner than any individual score — arrival bursts and cross-IPID correlation are visible within a fraction of a second of a fan-out starting. `raid_guard_under_attack_scale` (default **70**) uses that: while `raidGuardUnderAttack()` holds, every connection's thresholds are multiplied down, so a verdict that would have landed on message two lands on message one — and since the guard scores *before* the delivery switch in `pktIC`, that message is suppressed rather than sent. Replaying real raid captures, this cut delivered raid messages by roughly 3–8x with no change in which connections were acted on and no ordinary player affected.

Three properties make it safe to be aggressive here, and they are why this is not simply "lower the thresholds": it **never creates evidence**, only lowers the bar for evidence a connection already produced on its own, so anyone with a score of zero — every ordinary player who is merely talking during a raid — is untouchable at any setting, because scaling multiplies the *threshold* and never the score; it requires the under-attack state, which needs correlation or an arrival burst and is **not reachable by one person acting alone**, so a quiet evening never enters this mode and the cost on normal traffic is structurally zero rather than merely small; and it is multiplicative over the playtime tier rather than replacing it, so the tiers keep their ordering and the fully-exempt tiers never reach it at all. It lapses on its own `raidAttackHold` after the last piece of evidence. Pinned by `TestUnderAttackScaleOnlyTightens` (never loosens, and is inert off-attack), `TestUnderAttackNeverActsWithoutEvidence` (score 0 stays clean at every scale) and `TestUnderAttackDoesNotBypassTheBanGate`.

**Echo breadth.** Depth and breadth are different claims and only one is safe to read as "a raid is happening". Two friends quoting each other are one line echoed by two people however many times they do it — breadth 1, and ordinary. A raid is several *different* lines each echoed by different people at once, a shape no pair of players can produce between them. `raid_guard_corr_breadth` (default **3**) distinct lines echoing simultaneously marks the server under attack, giving `raidGuardUnderAttack()` a route that does not depend on one line being repeated four times. Credited at most once per message, keyed by the fingerprint that matched, so a single long echoed sentence counts once rather than once per shingle. Measured: breadth peaked at **0** across both clean captures and at **11** during the raid capture; the 2026-08-31 raid only reached 2, so this deliberately does not fire on it — the latency win there comes from the echo signal, not from here.

**A signal that was measured and deliberately NOT added: a slow arrival detector.** The obvious companion to the above is a second arrival window ("N new IPIDs per minute" alongside the existing 12-per-second), since 10 IPIDs over 26 seconds cannot reach a per-second burst threshold. Measuring it on the captures killed it: arrival counts taken from a packet log are contaminated by the log's own start, where every already-connected player registers as "new" at their first line — the clean captures show 16 and 21 "new" IPIDs inside ten seconds on that basis. There is consequently no trustworthy figure for what a real slow arrival rate looks like on a busy evening, and a threshold guessed without one would fire on a server restart or a popular night. The 12-per-second burst stays as the only arrival trigger.

**And a change measured and deliberately NOT made: lowering that burst threshold.** A later raid reached a genuine peak of only **7 new IPIDs in a second** (54 IPIDs spread over three minutes, well clear of the capture's own start), so the arrival trigger never fired and the under-attack flag had to come entirely from cross-IPID correlation. Lowering the threshold to catch it looks obvious and is not safe: measured the same way, the two clean captures peak at **6 per second** — one below that raid's genuine peak — because a capture's opening seconds register every already-connected player as an arrival, and a server restart or a busy evening produces the same shape for real. There is no gap between the populations to put a threshold in. The handshake-replay signal above is the answer to that raid instead: it identifies the individual connections directly, needs no server-wide flag to do it, and fires before any of them speaks.

**Kept honest by replay.** `TestSlowFanOutIsCorroboratedEarly` asserts the guard corroborates the 2026-08-31 raid no later than its fourth message; `TestSlowFanOutSparesTheRealPlayers` asserts the four ordinary players talking in that same room — two of them *about* the raid, using its vocabulary — stay completely clean; `TestSlowFanOutCannotReachTheBanGate` asserts the strong signal still never fires on that capture, so the ban gate stays shut on this class of evidence. A third capture, `aftermath_capture.log` (the same server two minutes later: 24 players, agitated, several shouting in caps and using the objection button while discussing what happened), is now replayed by the main validation harness alongside the quiet-evening baseline, because a loud upset room is the traffic a loosened threshold would actually eat. It scores a maximum of 20 with zero correlation hits.

**`/raidguard` command** (`internal/athena/raidguard_cmd.go`):
```
/raidguard status         # settings, live under-attack state, correlation window size,
                           # current echo breadth, whether alerts are on for you, and
                           # every connected client with a nonzero score
/raidguard clear <uid>    # the false-positive escape hatch: wipe one connection's
                           # score and lift any captcha action the guard placed on it
/raidguard clear all      # the same, for every connected client
/raidguard test <text>    # show the normalised form, fingerprints, and shouty-spam
                           # verdict for a line of text without acting on anyone —
                           # read-only, never touches the correlation window
/raidguard alert <on|off> # mute the [RAIDGUARD] staff alerts for your own session
```
**Alerts and who can silence them.** Every `[RAIDGUARD]` alert — a verdict the guard reached on a connection, and the notice that it engaged lockdown on its own — goes to everyone holding `MOD_CHAT`, and now carries the hint `(Disable these alerts for yourself with /raidguard alert off)`. During a real raid these arrive in bulk, which is exactly when a moderator working an unrelated incident needs to be able to quiet them, so `/raidguard alert off` mutes them for that moderator's session (`/raidguard alert on` re-enables; the bare form reports the current state, and `/raidguard status` shows it too). Per-session and never persisted, mirroring `/censoralerts`: a mute taken during one raid can never quietly outlive it, since every fresh connection defaults back to alerts on. The fan-out is centralised in `sendRaidGuardAlert` (`raidguard.go`), which appends the hint and applies the mute in one place, so no alert can go out without telling its reader how to turn it off. `alertRaidGuard` and `alertRaidLockdown` both route through it.

The command as a whole is gated on `MOD_CHAT` rather than `BAN` so that everyone who *receives* the alerts can also turn them off; `status`, `clear` and `test` re-check `BAN` inside the handler, so lowering the registry gate widens nothing but the toggle. Pinned by `raidguard_alerts_test.go`, which asserts a muted moderator receives nothing while an unmuted one does (with the hint), that a non-staff client is never a recipient, and that a `MOD_CHAT`-only holder gets the toggle and a permission refusal on everything else.

`/raidguard clear` is the important one operationally: it resets the connection's accumulated score and fired signals back to a clean slate, and — if the raid guard itself was the reason the connection is currently gated or restricted by the join captcha — releases that restriction through the same `releaseJoinCaptcha` path `/joincaptcha verify` uses, so a cleared player can chat again immediately rather than waiting out a timer.

### Persistent Music Ban (`/musicban`)
`/musicban <uid> [-r reason]` bans the target's IPID from playing music — both jukebox entries (`music.txt`) and streaming URLs — across sessions. Persists via the `MUSIC_BANS` table (DB migration 22). Hot-path check is a single RWMutex map lookup, seeded from the DB at startup.

**Quiet-area carve-out:** if the area has **fewer than 3 people** in it, the ban is **bypassed** and the music change is allowed — banned players can still set the mood in empty/small rooms but can't bother a populated one. Moderators are always exempt. Area-change MC packets are unaffected (a music-banned player can still move between rooms).

| Command | Permission | Behaviour |
|---------|------------|-----------|
| `/musicban <uid> [-r reason]` | `MUTE` | Persistently ban the target's IPID. Idempotent; re-banning overwrites the reason and issuer. |
| `/musicunban <uid\|ipid>` | `MUTE` | Lift a music-ban. Accepts a connected target's UID or a raw IPID, so offline players can still be unbanned. |
| `/musicbans` | `MUTE` | List every active music-ban with reason, issuer and timestamp (newest first). |

### Removed: Shadow Disconnect (`/shadowdisconnect`) — enforcement retained

`/shadowdisconnect` is gone for the same reason possession is: to its target a shadow-ban is indistinguishable from a bad router, forever, with nothing to notice or appeal. The `grant` gate it sat behind is gone with it.

**The enforcement half stays**, so entries written before the removal can still be found and lifted rather than silently outliving the command:

| Command | Behaviour |
|---------|-----------|
| `/shadowundisconnect <uid\|ipid\|all>` | `ADMIN`. Lifts a shadow-disconnect by UID, raw IPID (offline players), or `all`. |
| `/shadowdisconnectlist` | `ADMIN`. Lists every remaining entry with reason, issuer and timestamp (newest first). |

The persisted list (`SHADOW_DISCONNECTS`, DB migration 24), its in-memory cache, and the three enforcement points are unchanged — nothing adds to the list any more, so they only ever act on pre-existing rows:
- `ListenTCP`'s accept loop (`internal/athena/server.go`) — checked the instant the IPID is computed, before a `*Client` exists. A match is a silent `conn.Close()`.
- `HandleWS` (`internal/athena/server.go`) — checked before the WebSocket upgrade; a match responds with a bare `500` and no body.
- `Client.HandleClient` (`internal/athena/client.go`) — a defense-in-depth re-check alongside `CheckBanned`/the torment check.

**Why the close isn't just `conn.Close()`.** Kept because the enforcement path still relies on it. Every other disconnect path in this codebase (`/kick`, `/ban`, `CheckBanned`) sends a typed packet (`KK`/`KB`/`BD`) naming the reason before closing. For a raw TCP AO2 client a bare `conn.Close()` with nothing sent first is already indistinguishable from a network drop; for a WebAO/WSS client it is not, since `client.conn` wraps `nhooyr.io/websocket` and `conn.Close()` performs a *clean* closing handshake (status 1000, "Normal Closure") that a browser reports as deliberate. `shadowDisconnectNow()` (`internal/athena/shadowdisconnect.go`) forces the library's abrupt path — an already-expired write deadline plus a doomed `Write` — closing with no Close frame so a browser sees a generic connection error.

### Removed: Console-Gated Commands (`grant`)

There was briefly a console `grant` gate in front of `/possess`, `/fullpossess`, `/truepossess` and `/shadowdisconnect`: `ADMIN` was not sufficient, each use had to be armed from the server console with `grant <command>`, and one arming bought one use.

All four commands have since been **removed outright**, so the gate has nothing left to guard and `grant` / `grant status` / `grant revoke` are gone from the console along with `console_gate.go`. The reasoning that motivated the gate is what finished the job: the thing wrong with those commands was never who could fire them, it was that their target could not tell they had been fired at all.

**What survived, and is unchanged:**

- **The console-only punishment kill switch.** `punishment <enable|disable|status>` still outranks every in-game tier and is still reachable only from stdin.
- **The torment list.** AutoMod still arms it automatically on a censor trip. `/lag` was removed from the game in the same change as the gate, so the console's `torment <ipid>` is the only remaining manual route in; `untorment <ipid|all>` is the console half of `/untorment`. `addTormentedIP` still has exactly two callers (the AutoMod censor path and the console helper), so `/stack lag`, the roulette pool and `/maso` cannot put anyone on it.
- **Lifting is never restricted.** `/untorment`, `/unlag` and `/shadowundisconnect` all stay in game at their existing permissions, so whoever is online can always undo a listing.

Console verbs live in `internal/athena/cli.go`; the torment helpers in `internal/athena/commands_punishment.go`.

### Hot Config Reload (`/reload`)
`/reload` (in-game, `ADMIN`) atomically re-reads every supported config/data file from disk and swaps it in without restarting the server. Also available as the `reload` CLI command on stdin and via `SIGHUP`.

Each reloadable list lives behind a `sync/atomic.Pointer` (see `internal/athena/livereload.go`), so a swap is a single atomic store — readers on the hot IC path never lock and never see a torn value. A parse error in any one file aborts the whole reload before anything is published, so a bad file never leaves the running server half-updated. Validated end-to-end with `go test -race`.

**Reloaded:**
- `characters.txt` — **append-only** (see safety constraint below)
- `music.txt` — full reload; the pre-built SM packet sent on every client join is rebuilt in lockstep
- `cdns.txt`
- `backgrounds.txt` — `/bglist`'s cached output string is rebuilt in lockstep
- `parrot.txt`
- `8ball.txt` (optional; missing file leaves the current value intact)
- `banned_words.txt` (only when automod is enabled)
- `censored_names.txt` (optional; independent of automod_enabled; missing file leaves the current value intact)
- `config.toml` motd and description (the existing hot-config whitelist)

**`characters.txt` safety constraint — append-only.** Connected AO2 clients reference characters by **slot index**, so inserting in the middle, removing, reordering or renaming an existing slot would silently desync every connected player (the person on slot 2600 would suddenly be on whatever character used to be slot 2595). To prevent that, the reload **validates that every existing slot is unchanged** and only accepts entries appended at the end of the file. If the new file changes any pre-existing slot, the reload is rejected with a precise message naming the first bad slot (e.g. `"characters.txt: slot 12 changed from 'X' to 'Y' — character reload is append-only; add new characters at the END of the file (restart the server to reorder, rename or insert)"`). When new characters are appended, every area's character-slot table is grown first via a new `Area.GrowTaken(n)` method so newly-selectable slots can never panic the IC path with an out-of-bounds.

**NOT reloaded** (would require invasive work and is unsafe without restart): areas, listener ports/addr, rate-limit windows, max_players, roles, the server name. These are still restart-only.

### In-Game Console Log Viewer (`/terminal`)
`/terminal [lines]` (`ADMIN`) prints the last N lines of the server's console/log output as a single OOC message, so an admin without shell access to the host can still check on the server. Defaults to 50 lines when no argument is given; capped at 500 per request.

```
/terminal          # last 50 lines
/terminal 100      # last 100 lines
```

Backed by a bounded in-memory ring buffer (`logger.RecentLines`, capped at 2000 lines) that every `log()` call feeds independently of `LogStdOut`/`LogFile`/the TUI's own tap, so `/terminal` works whether or not stdout or file logging is enabled, and reflects the same `CurrentLevel` filtering as the real console. Implemented in `internal/athena/terminal_log.go` and `internal/logger/logger.go`.

### IPHub VPN Firewall
When `iphub_api_key` is set, moderators can run `/firewall on|off` from in-game **or** from Discord (`/firewall on|off` slash command). New IPs are checked against IPHub; VPN/proxy IPs are rejected. Known IPs are never re-checked (respects 1,000 requests/day free tier).

### Discord Bot Security Toggles
Mirrors the in-game security commands so moderators don't have to be logged in to the AO server during an incident.

| Discord slash | Behaviour |
|---------------|-----------|
| `/firewall on` / `/firewall off` | Toggle the IPHub VPN/proxy firewall. Refuses to enable if `iphub_api_key` is unset. |
| `/lockdown on` / `/lockdown off` | Toggle the server-wide new-IPID lockdown. `on` also runs the lockdown playtime purge below. |
| `/lockdown whitelist_all` | Whitelist every currently-connected IPID so they can rejoin during lockdown. |

### `/announce` — In-Game Server Announcement
Mirrors the Discord bot's `/announce` slash command (`handleAnnounce` → `ServerAdapter.SendAnnouncement`, `internal/discord/bot/communication.go` / `internal/athena/discord_adapter.go`) so a moderator can reach every connected player without the Discord bridge configured or an admin having to relay it from Discord.

```
/announce <message>   # MUTE — broadcasts "[Announcement] <message>" to every connected player
```

Sends a server OOC message (`sendGlobalServerMessage`, the same helper minigame/event broadcasts already use) to every client that has completed the join handshake — identical reach to the Discord version's own `Uid() != -1` filter. Unlike `/global`/`/pm`, it is not routed through `oocCommandAllowed`/AutoMod/the raid guard: like `/modchat`, `/kick`'s reason, or a ban reason, the text is moderator-authored rather than player-submitted, so there is nothing for the word filter to protect against. The issuing moderator gets a confirmation echo and the announcement is written to their area's CMD log. Implemented in `cmdAnnounce` (`internal/athena/commands_moderation.go`).

### Lockdown Playtime Purge
`/lockdown` (in-game or the Discord bot's `/lockdown on`) used to only ever block *new* (previously-unseen) IPIDs from connecting — anyone already inside when a moderator reacted to a raid stayed inside and kept spamming, since by the time lockdown flips on, every flooding connection has already gotten past the join gate and is "known" for the rest of the session.

Turning lockdown **ON** now also runs an immediate purge: every currently-connected, non-moderator client whose **total, all-time playtime** on their IPID (`KNOWN_IPS.PLAYTIME` — the same figure `/playtime` and the botban threshold read) is below a configurable minimum is disconnected on the spot, then has its IPID forgotten so lockdown's own new-connection block keeps it out on any immediate reconnect attempt. A player/IP with a lot of accumulated playtime (e.g. 500 hours) is always exempt no matter how the threshold is set — this targets fresh/throwaway connections, not long-time regulars who happen to be online. Moderators are always exempt regardless of playtime. Turning lockdown **off** never triggers a purge.

The threshold defaults to 30 minutes in `config_sample/config.toml` (`lockdown_min_playtime`, in minutes; 0 disables the purge — lockdown then behaves exactly as it did before this feature) and is runtime-adjustable with `/setlockdownplaytime <minutes>` (`BAN`), mirroring how `/setplayerlimit` adjusts the player-capacity threshold.

**Built for a flood, not a handful.** Each kick runs on its own goroutine rather than looping synchronously: `SendPacketSync` (used to deliver the kick reason) blocks its caller for up to the write deadline, and a raid's connections are exactly the ones likely to be half-open or not reading — looping over a large batch of them on one goroutine would serialize on the slowest socket. Fanning the kicks (and their `db.GetPlaytime` lookups) out concurrently means the purge finishes in about one write-deadline's worth of time no matter how many connections are being cleared, without stalling the command handler or anything else on the server — `ClientList.ForEach` only holds its lock long enough to snapshot the client list, not for the duration of the kicks themselves.

Implemented in `purgeLockdownFloodClients` (`internal/athena/server.go`), called from both `cmdLockdown` (`internal/athena/commands_moderation.go`) and the Discord bridge's `ServerAdapter.SetLockdown` (`internal/athena/discord_adapter.go`) so the purge fires identically regardless of which surface a moderator uses. The pure eligibility check (`lockdownPurgeEligible`) is unit-tested independently of the sweep itself.

**Messages stop before the kick lands.** The purge is a one-shot sweep, so there's a brief window — one DB round-trip per connection, fanned out concurrently — between lockdown switching on and a given flood connection actually being disconnected. Without anything else, that connection could still get a message out during the gap, and a low-playtime IPID that reconnects (as an already-known IP, which lockdown's own join gate still allows through) *after* the sweep already ran would never be caught at all. To close both gaps, `pktIC` and `pktOOC` (`internal/athena/netprotocol.go`) each check `lockdownShouldSilence` right after the existing rate-limit checks, for as long as lockdown stays active: a non-moderator whose total playtime is still under the threshold has that message silently dropped — never broadcast, no feedback to the sender, nothing written to the area log — instead of just waiting for the purge to catch up. It shares the exact eligibility logic the sweep uses (`internal/athena/server.go`), so the two can never disagree about who's exempt, and it costs nothing on servers that never turn lockdown on: the check is gated behind the same `serverLockdown` atomic the join gate already reads, so the moderator check and DB lookup only run while lockdown is actually active.

### Lockdown Whitelist Passkeys
A self-service escape hatch for a legitimate player caught by lockdown. Every message that blocks someone because of lockdown — the connection rejection when a new IPID tries to join, and the playtime-purge kick above — hands them a **passkey**: `<their IPID>:<HMAC-SHA256(secretseed, IPID), truncated to 16 bytes, hex>`. Anyone holding a valid passkey has necessarily been issued it by this exact server, since only the server knows `secretseed` — so `/lockdown whitelist <passkey>` (`BAN`) can trust it purely by recomputing the MAC and comparing, with no pending-request table and no need for staff to already know the player's IPID or for the player to still be online. The IPID half isn't itself secret (moderators already see IPIDs everywhere — `/getban`, `/gas`, area logs, the `[BAN]` account-link alert) — only the signature needs to be unforgeable.

Redeeming a passkey does two things at once: it whitelists the IPID for lockdown's join-gate (the same effect as `/lockdown add`), and it **permanently** exempts that IPID from the playtime purge/silence from then on, regardless of actual playtime — closing the gap where `/lockdown add` alone only fixed the join-gate but left the same IPID kickable/silenceable by a *future* lockdown. Persisted in the `LOCKDOWN_EXEMPT` table (DB migration 25) the same way `/musicban`/`/shadowdisconnect` persist by IPID, seeded into an in-memory set at startup so the hot-path checks (`lockdownShouldSilence`, the join-gate's `checkGlobalNewIPRateLimit`) are a lock-free-ish map lookup, not a DB query.

```
/lockdown whitelist <passkey>     # BAN — verify a passkey; whitelists + permanently exempts its IPID
/lockdown unwhitelist <uid|ipid>  # BAN — revoke a previously-verified exemption
/lockdown exemptlist              # BAN — list every IPID currently exempt via a verified passkey
```

**Off by default, and inert until configured.** The whole system hinges on `secretseed` in `config.toml` (blank by default — disables passkey generation/verification entirely; `/lockdown add` and `/lockdown whitelist all` are unaffected either way). Treat it exactly like a password: anyone who has it can mint a valid passkey for *any* IPID, including ones that were never actually blocked, so it must never be shared outside trusted staff tooling. Changing it invalidates every previously-issued passkey (the signature no longer matches under the new key) without needing any cleanup — old passkeys just stop verifying. The message text itself is configurable via `lockdownstring` (a template with a `{id}` placeholder, defaulting to an explanation that this is not a ban); a template missing `{id}` gets the passkey appended instead, so a misconfigured template can never accidentally swallow the ID.

Implemented in `internal/athena/lockdown_passkey.go` (`lockdownPasskeyFor`/`lockdownPasskeyVerify`/`lockdownPasskeyMessage`, the `LOCKDOWN_EXEMPT` in-memory cache, and the `/lockdown whitelist <passkey>`/`unwhitelist`/`exemptlist` handlers), hooked into the rejection messages in `server.go`'s `ListenTCP`/`HandleWS` and into `purgeLockdownFloodClients`'s kick reason. The HMAC round-trip, tamper/forgery rejection, and the `{id}` template substitution are unit-tested independently of the command plumbing.

### `/charshuffle` and `/uncharshuffle`
Mirrors `/nameshuffle` but operates on character IDs. Randomly permutes everyone's character (char 1 → char 2, char 2 → char 5, etc.) using Sattolo's algorithm so every player ends up on someone else's sprite. Originals are remembered per-client so `/uncharshuffle` puts everyone back exactly. Requires `MUTE`.

### `/reversename` and `/unreversename`
Flips the rune order of a player's showname (`Phoenix` → `xineohP`). `/reversename <uid>` (or a comma-separated UID list) targets specific players; `/reversename global` reverses every player in the caller's area. `/unreversename` restores with the same target forms. The forced showname in effect before the flip is remembered per client, so `/unreversename` restores it exactly — even when the reverse was stacked on top of a `/forcename`. Reverses by runes (multi-byte/accented characters survive) and re-encodes so AO2 escape sequences are never split. Requires `KICK`.

### `/getmusic`
For every player. Prints the URL of the song currently playing in the area **and** re-sends the MC packet to just the requesting client. Useful when a client's audio handling glitched and the song never started — the user can either copy the URL or have the bot poke their player to restart playback locally.

### Music Sync on Area Entry (Bug Fix)
Joining an area — the initial connect into area 0, or any subsequent `/area` change — now sends the area's currently-playing track to the joining client as an MC packet. Previously nothing synced a fresh arrival to music already in progress: the client just sat in silence until the track changed again or the player thought to run `/getmusic` by hand. The fix lives in the shared `JoinArea` (`internal/athena/client.go`), so both the initial join and every area change are covered from one place.

**Silent rooms now stop carried-over music (bug fix).** When the destination area has no `CurrentSong()` — a fresh area that has never played anything (`""`), or one whose music was explicitly stopped (`"~stop.mp3"`) — `JoinArea` now sends an explicit `~stop.mp3` MC packet instead of sending nothing. Previously the empty-`CurrentSong` case sent no packet at all, so a client that walked out of a room with music into a silent room kept playing the old track forever: the server never told the client to stop, so it had no way to know it should (this is why the symptom was client-side but the cause was server-side). The explicit-stop case already worked because `"~stop.mp3"` is a non-empty `CurrentSong`; the gap was only the never-played `""` case. On the very first connect this stop is a harmless no-op since nothing is playing yet.

**Custom tracks are now what gets synced (bug fix).** Only the two MC-packet paths in `pktAM` recorded the area's current song; `/play` (Discord/CDN URLs), `/randomsong` and the YouTube pipeline broadcast their MC packet without recording it. The area therefore stayed pointed at the last **jukebox** entry, so leaving the area and coming back — or any fresh join — resynced the client to that stale jukebox track instead of the custom track actually playing. Every BGM path now goes through the shared `playAreaMusic` helper (`internal/athena/music_sync.go`), which records the track and broadcasts it in one place. One-shot sound effects (the `/sfxcurse` MC fallback, `Looping "0"`) deliberately bypass the helper so they never overwrite the area's BGM.

### `/area` — CM/moderator area tools

Registered as the `area` command with `mute` / `unmute` / `rename` / `unrename` sub-commands, gated on the `CM` permission (which every mod role carries, and which `clientCanUseCommand` also grants to area CMs), so both area CMs and moderators can run all of them. Listed under the `area` help category (`/help area`, `/area -h`).

#### `/area mute` / `/area unmute`
Area-wide moderation for CMs and moderators. `/area mute` silences **everyone in the caller's area except CMs and moderators** — both IC and OOC (`ICOOCMuted`). `/area unmute` **reverses it so people can talk again**. The caller, area CMs, CM-permission holders, and moderators are all exempt.

**Scoped to the room — in-memory only (bug fix).** Unlike a real `/mute`, an `/area mute` is meant to silence someone only while they're actually in that area. It records its origin area (`Client.SetAreaMuted`/`AreaMuteOrigin`), and leaving that area — via `ChangeArea` or `forceChangeArea` (e.g. a jail placement) — automatically lifts it (`Client.liftAreaMuteOnDeparture`). `/area unmute` likewise only reverses a mute whose origin is the caller's own area, so it can never accidentally lift a real `/mute` or one applied by a different room.

The mute lives **purely in memory and is never persisted to the DB**. It used to be persisted by IPID "exactly like `/mute` so it survives a quick reconnect" — but the origin area is in-memory-only, so on reconnect `restorePunishments` restored a plain origin-less `ICOOCMuted` via `SetMuted` (which clears the origin). That origin-less mute couldn't be auto-lifted by leaving the area (`liftAreaMuteOnDeparture` bails when the origin is `nil`), couldn't be matched by `/area unmute` (origin `!= a`), and couldn't be cleared by the CM who applied it (CMs lack the `MUTE` permission `/unmute` requires) — so a CM-muted player who reconnected was **muted forever** until a full moderator intervened. Dropping the persistence fixes that: a disconnect destroys the `*Client` (and its area-mute state) and drops the player into area 0 on reconnect, i.e. they've left the room, so the mute ending is exactly the intended room-scoped behavior. (Legacy `ICOOCMuted` rows written by the old persisting code are indistinguishable from real `/mute`s in the DB and still need a moderator `/unmute` to clear.)

The two commands are a clean inverse pair that never disturbs separate individual mutes: `/area mute` only silences players who are currently unmuted (it won't clobber a deliberate `/mute`), and `/area unmute` only lifts the `ICOOCMuted` state that `/area mute` itself set (an IC-only or OOC-only individual mute, or one from another area, is left intact).

Implemented in `internal/athena/commands_area_mute.go`.

#### `/area rename <name>` / `/area unrename`

Lets whoever is running a room name it after what is actually happening in it — a CM staging a Danganronpa case in "Courtroom 3" runs `/area rename DR Killing Game` and the area list says so.

```
/area rename DR Killing Game     # rename the caller's own area
/area rename                     # report the current name and the configured one
/area unrename                   # restore the configured name immediately
```

**The name is a loan, not a transfer.** `areas.toml` holds the area's real identity and it comes back on its own:
- when the area **empties** (`releaseAreaNameOnEmpty`, called from the same three departure paths that call `Area.Reset()` — `clientCleanup`, `ChangeArea`, `forceChangeArea`), or
- when the area **loses its last CM** (`releaseAreaNameOnLastCMLeaving`, called immediately after every `RemoveCM` — the three departure paths plus both branches of `/uncm`).

Nothing is persisted, so a restart also returns every area to its configured name. That is what stops a rename from being a way to permanently vandalise the area list: nobody has to remember to undo one, and there is no state to clean up.

**The CM-held condition is real state, not a call-site convention.** `Area.SetName` records whether the area had a CM at rename time (`Area.NameHeldByCM`), and the last-CM release checks it. A moderator does not have to be a CM to rename a room, and for a room that never had one "no CMs are left" is true from the moment of the rename — so without that bit a moderator's name would snap back on the next unrelated departure. Such a room keeps its name until it empties instead.

**Three things are derived from an area's name and all of them are republished together** (`republishAreaNames`, under `areaRenameMu`):
- `areaNames`, the `#`-joined string `pktAM` matches an incoming area change against. It moved from a plain package global to an `atomic.Pointer[string]` (`getAreaNames`/`setAreaNames` in `livereload.go`) for this — it is read on the IC/music hot path and is now written at runtime.
- the pre-built **SM** blob every joining client is handed.
- the area list held by clients that are **already connected**, pushed as an **FA** packet.

FA rather than a fresh SM is deliberate: SM is a handshake packet, and a client that receives one mid-session re-runs its join ladder and answers with RD. FA carries the area list on its own and is what area-list changes are normally delivered with. (`pktReqDone` ignores a second RD anyway, so the SM route would not have *broken* anything — it would just have shown everyone a loading screen to change one string.)

**Validation** (`areaNameRejection`) — an area name is not just a label; it is the key clients send back to change area, the string the area list is built from, and the directory area logs are written into:

| Rule | Why |
|------|-----|
| 1–32 runes, non-empty after trimming | The area list is a narrow column in every AO2 client. |
| No `#`, `%`, `$`, `&` | AO2's escape table. Area names are the one player-settable string this server emits **unencoded** (`buildSMPacket` writes them verbatim; `pktAM` compares against them after decoding), so one of these would split the list entry or make the area unreachable by name. |
| No control characters | They break the area list and the area-log line format. |
| Not a music-list entry or an `http(s)` URL | `pktAM` tests the music list before the area names, so the room would become unreachable by name and play a song instead. |
| Not another area's current **or configured** name (case-insensitive) | A configured name returns the instant that area empties, so allowing it would produce a silent duplicate later rather than an error now. Re-checked under `areaRenameMu` so two rooms cannot race onto one name. |

**The name goes through the same word filter as IC and OOC**, which is the point rather than an extra: it is broadcast to every connected client, so there is no argument for a weaker standard than one line of chat. `cmdAreaRename` mirrors `checkCensored` in `pktIC` exactly — `autoModCheckTiered` with source `"area rename"`, `applyAutoModNuke` on a `nuke` match (the rename is destroyed and the IPID banned), `raidGuardOnWordHit` for everything below nuke, and the configured `automod_action` otherwise. `watch` tier still passes, as it does in chat. Under the default `shadow` action the caller is sent exactly the confirmation a successful rename sends while the area is untouched and no other client ever sees the name — shadow semantics preserved.

A new name also gets its area-log directory created (`logger.CreateAreaLogDirectory`); a failure is logged and non-fatal.

Implemented in `internal/athena/commands_area_rename.go`, with `SetName`/`DefaultName`/`Renamed`/`ResetName`/`NameHeldByCM` in `internal/area/areas.go`. `Area.Reset()` deliberately does **not** restore the name — undoing a rename needs a server-wide republish the area package cannot perform — so the athena-side helper handles it at the same call sites. Pinned by `commands_area_rename_test.go`.

### `/8ball <question>`
For every player. Picks an answer from `config/8ball.txt` if present, otherwise from the built-in 20 classic Magic 8-Ball responses. The sample shipped in `config_sample/8ball.txt` adds a few cheeky extras.

### `/resetusername <new-username>`
Lets a logged-in player rename their account without losing their playtime, chips, wardrobe, tags, or anything else tied to their account. Capped at **3 renames per account** (DB column `USERS.USERNAME_RESETS`, migration 19).

### `/removerole <username>` (Admin)
Demotes a user to a default player account **without deleting it**. Sits between `/setrole` (which requires a named role from `roles.toml`) and `/rmusr` (which purges the account entirely): `/removerole` zeroes the account's permissions via `db.ChangePermissions(username, 0)`, so a DJ or moderator becomes a plain registered player while keeping their username, password, linked IPID, chips, playtime, wardrobe and tags. Any connected session signed in to that account is updated live — it stays logged in but drops to zero permissions, and a former moderator is sent `AUTH#-1` to clear the AO2 "logged in as moderator" badge. Requires `ADMIN`. No-ops with a clear message if the target is already a default (zero-permission) player account.

### `/playtime` Pagination
`/playtime top` lists 25 entries per page; pass a page number for the next 25 (`/playtime top 2` = positions 26–50). Both player accounts and moderator accounts (including shadow mods) are eligible — there's no permission filter.

Admins can adjust an account's stored playtime directly: `/playtime add <username> <duration>` grants time, `/playtime remove <username> <duration>` deducts it (clamped at 0), and `/playtime set <username> <duration>` overwrites the total outright. Duration accepts any combination of `ns`/`us`/`ms`/`s`/`m`/`h`/`d`/`w` units (e.g. `1000h`, `3d12h30m`). The target account must have logged in at least once (so its IPID is linked); the change applies immediately to the leaderboard, and the player is notified live if online.

`/reloadplaytime` (admin) re-links every registered account to its IPID and merges any orphaned playtime — fixes the bug where an account created on a long-running anonymous IPID didn't show pre-existing hours on the leaderboard until the server restarted.

### `/profile` DJ Insignia
Players with the `DJ` permission bit (and no moderator privileges) get a 💿 vinyl badge on their `/profile` card so DJs are visible at a glance. Mods are unaffected — they have their own staff lines.

### `/profile` Identity Privacy (Bug Fix)
`/profile <uid>` (viewing someone else) used to resolve the target's linked account purely from `db.GetUsernameByIPID(target.Ipid())`, regardless of whether the target had actually logged in on this connection — an IPID stays linked in the DB long after the session that logged it in ends, so anyone could out an anonymous-looking player as a specific known account off that stale link, with no action from the target. The account line now only resolves for someone else's profile if the target `client.Authenticated()` — i.e. they ran `/login` or `/register` on this connection — otherwise it shows `(guest)` exactly like a never-registered IPID would. Viewing your **own** profile (`/profile` with no args) is unaffected and always resolves your own linked account regardless of authentication, since that's your own information rather than something being outed to a peer. The favourites/wardrobe line cascades from the same `username` value so it's hidden alongside the account for the same case.

**Playtime and the active cosmetic tag are hidden the same way, for the same reason.** Even with the account name gone, the raw playtime figure and equipped tag were still shown unconditionally — both are persisted by IPID like the account link, so a logged-out target's past hours or tag choice (a custom tag can literally name its owner) are just as identifying as the account name: `/playtime top` publicly ranks every registered account by name and exact hours, so "this anonymous connection has 47h32m" is enough to deanonymize someone off that leaderboard alone. Both lines now show `(hidden)` under the exact same condition as the account line (someone else's profile, target not authenticated); self-view and an authenticated target are unaffected either way. Favourites/wardrobe needed no separate fix since it already cascades from the same gated `username` value. Chips are untouched — casino balances aren't gated here since `/chips top`/`/richest` have the identical leaderboard-correlation shape as `/playtime top`, so gating chips on `/profile` alone wouldn't close that path either; fixing it would mean the same treatment for those leaderboards, not just this card. Implemented in `cmdProfile` (`internal/athena/commands_account.go`).

### `/global` Tag Display
`/global` now shows the sender's `[tag]` in the prefix, matching local-OOC formatting. `/g` is a plain alias of `/global` (same permissions, same handler) for players who want a shorter command to type — which is also why gating one and not the other would gate nothing; both go through `oocCommandAllowed` (see "OOC Commands That Broadcast" above).

### `/status lfp` Shorthand
`/status` (CM) sets the current area's AO2 status (`idle`, `looking-for-players`, `casing`, `recess`, `rp`, `gaming`). `lfp` is accepted as a shorthand for `looking-for-players` — `/status lfp` and `/status looking-for-players` set the exact same status.

### `/gas` Empty-Area Suppression
`/gas` (the all-areas player listing) hides empty areas to keep the listing scannable on servers with many areas. The empty-area count is shown at the bottom.

### `/randomchar` DJ/Mod Cooldown Bypass
`/randomchar` keeps its 5-second cooldown for regular users, but **DJs and moderators bypass it entirely** so they can swap freely while running events.

### `/rps` (Player vs Player)
`/rps <rock|paper|scissors>` is now PvP. The first call commits a **hidden** choice and posts an open challenge to the area; the second player commits blind (so they can't game-theory the first move) and the result is resolved. 30-second window per player.

### Locked-Room Kick Lockout (Bug Fix)
Previously, `/lock` added every current occupant to the area's invite list, so a CM kicking a player from the locked area didn't actually keep them out — they'd walk right back in. The kick command now also pulls the target's UID from the invite list, so they can't return until the room is unlocked or they're explicitly re-invited.

### `/invite` in Spectate Mode + Public `/spectate` Help
- **`/invite <uid>` now works in spectate mode.** Previously `/invite` bailed with "This area is unlocked." whenever the area wasn't `/lock`-ed, which trapped CMs who had only enabled **spectate mode** (which leaves the area `LockFree`) and wanted to let someone speak. `cmdInvite` now invites to *enter* in a locked area **and** to *speak* (the spectate-invite list, same as `/spectate invite <uid>`) when spectate mode is on; both apply if both are active. In a genuinely unrestricted area the message now tells the CM how to restrict it (`/lock` for entry, `/spectate` for speaking) instead of failing silently.
- **`/spectate` is now listed in `/help` for everyone.** A new `publicHelp` flag on the `Command` struct lets a command appear in `/help` (and show usage via `/help <cmd>`) regardless of the viewer's permissions. `/spectate` sets it so all players can discover how spectate mode works, even though running it still requires `CM`. The flag is honored at all three `/help` filter sites (category overview, category drill-down, single-command lookup).

### Spectate Mode on the Area List (Bug Fix)
`/spectate` restricts IC speech to CMs and invited players, but it deliberately leaves the area's stored lock alone (it is a gate on *speaking*, not on entry) — so `sendLockArup` reported the raw `LockFree` and every client's area list showed a watch-only room as **FREE**. Nothing anywhere told a player looking at the list that they could enter but not talk, and the CM who had just toggled it saw no change either; a player only found out by trying to speak and being refused. Worse, `cmdSpectate` broadcast no ARUP at all, so even once the list *could* have said something, the toggle never told anyone — the display only refreshed when some unrelated event happened to trigger a lock ARUP.

`areaLockDisplay` (`internal/athena/server.go`) now derives what the lock column advertises: an area that is `LockFree` with spectate mode on reports **SPECTATABLE**, which is precisely AO2's own name for "you may enter and watch, but not take part". A `LockLocked` area keeps reporting **LOCKED** — that is a claim about entry, which spectate mode makes none about, and it is the stronger of the two. `cmdSpectate` now calls `sendLockArup()` on toggle so the change actually reaches clients.

**It is a display derivation, never a write-back**, and that distinction is the whole safety argument: the area's stored lock is untouched, so `/unlock`, the auto-unlock when the last CM leaves, and the `LockSpectatable` branches in `CanSpeakIC`/`CanChangeMusic`/`CanJud` all keep reasoning about the lock the area actually has. Those branches gate on the `/lock` invite list, not the separate spectate-invite list, so actually calling `SetLock(LockSpectatable)` from `/spectate` would have silently changed which players could play music and use the judge buttons — a behaviour change smuggled in under a display fix.

Two consequences of the status becoming visible are fixed alongside it:
- **`autoUnlockIfLastCMGone` now lifts spectate mode too**, which its contract ("unlocks *or un-spectates*") always claimed and never did: its `LockFree` early-out matched exactly the state `/spectate` leaves an area in, so a CM could enable spectate mode, disconnect, and leave everyone still in the room IC-silent with nobody holding the `CM`/mod permission `/spectate` needs to lift it. With the status now on the area list, that room would have advertised **SPECTATABLE** forever.
- **`/unlock` in a spectate-mode area points at `/spectate`** instead of answering the flat "This area is not locked." A CM reading SPECTATABLE off the list cannot tell whether it came from `/lock -s` (which `/unlock` lifts) or `/spectate` (which it does not), so the dead end is one the visible status creates.

Pinned by `spectate_arup_test.go`: what each lock/spectate combination advertises, that the derivation never mutates the stored lock, that the toggle broadcasts (a source-level check, since nothing observes a broadcast without standing up a server), and the auto-unlock behaviour. The toggle-broadcast and auto-unlock tests were verified to fail against the pre-fix code.

### Recipe Step Variety
`/recipe` rotates through Step 1 → Step 4 with **separate verb pools per step** (prep / combine / cook / plate). A stream of `/recipe` lines now reads like a real recipe instead of the same template.

### Server-Relayed Voice Chat
Opt-in voice chat. Every audio frame travels **client → Athena → other clients in the same area** — there is no peer-to-peer path, so peers cannot learn each other's IPs via packet sniffing (which is what TURN was previously used for in the now-removed WebRTC mode). Athena itself sees every peer's IP, exactly as it does for chat packets.

Audio: 20 ms Opus frames at 48 kHz, base64-encoded on the wire. The server treats each frame as an opaque blob and just re-broadcasts it (one fan-out per joined peer). No Opus decode happens server-side, so the relay is codec-agnostic and CGO-free.

The `pktVSFrame` voice-effects hook (between the rate-limit check and the broadcast call) is now used by the **voice-chat punishments** — `/voicemute`, `/voicestatic`, `/voicegarble`, `/voicecutout`, `/voicestutter` — in `internal/athena/voice_punishments.go`. They drop, gate, or stale-repeat a punished speaker's frames; because the relay never decodes audio, they manipulate frame *flow* only. Codec-level DSP such as pitch-shifting would need an Opus decoder and CGO and stays out of scope. See **Voice Chat (5)** under the punishment list above.

Protocol (`internal/athena/voice.go`):
- `VS_CAPS#<enabled>#<ptt>#<max_peers>#<codec>#<sample_rate>#<frame_ms>#<max_frame_bytes>#%` — S→C handshake
- `VS_JOIN` / `VS_LEAVE` / `VS_PEERS` — room membership
- `VS_FRAME` (C→S) / `VS_AUDIO#<from_uid>#<b64>#%` (S→C) — audio relay
- `VS_SPEAK#<uid>#<on_off>#%` — speaking indicator

Moderation: IPID-scoped `/vmute` and `/vban`, per-area `/voicearea on|off`, per-UID join and frame rate limits, new-IPID cooldown — all in `internal/athena/voice_mod.go` and `voice_commands.go`. Disabled by default; toggle with `enable_voice = true` under `[Voice]`.

### Streaming Music URLs
Both the `/play <url>` command and a client-sent **MC** packet carrying a raw `http(s)://` URL (e.g. WebAO typing a custom track) play streaming audio. The URL's host must be in `config/cdns.txt`; an un-whitelisted host is rejected with an OOC **"Illegal origin"** notice rather than being silently dropped. The URL is re-broadcast byte-for-byte — the server never mangles it.

The outgoing `MC` packet's numeric fields (`looping`/`channel`/`effects`) always default to `"0"` at the single serialization point (`packet.MCToClient.Args`). This fixes the regression where `/play` and `/randomsong` emitted an empty effects field (`…##%`), which AO2 clients fail to parse as a number and drop silently.

### MS JSON-Schema Validation
JSON-mode connections have their **MS** (in-character) packets validated against draft-07 JSON schemas vendored in the top-level `schemas/` folder (from [OmniTroid/aolib-schemas](https://github.com/OmniTroid/aolib-schemas)):
- **Inbound** MS is validated against `MSRequest.schema.json` in `packet.ParseJSON`; an invalid packet is rejected and dropped (logged).
- **Outbound** MS is validated against `MSBroadcast.schema.json` in `Client.SendPacket`; a packet that fails the schema is dropped (logged) before reaching a JSON-mode client.

To satisfy the schemas, the JSON-mode MS encoder emits proper types (numbers, booleans, and `{x,y}` offset objects) rather than strings. Schemas are embedded via `//go:embed` in `athena.go` and compiled once at startup (`packet.CompileMSSchemas`). FantaCode (classic desktop AO2) traffic is never validated and is unaffected; if the schemas fail to load, validation is silently disabled. Library: `github.com/santhosh-tekuri/jsonschema/v5`.

### `/punishments` Inspection & `/clients` Multiclient Listing
- `/punishments [uid]` — lists active punishments with remaining durations, custom data, reasons, and (for mod viewers) issuer tiers. Covers the out-of-slice effects too: lag (torment list), mute, jail. No permission needed for self-inspection; the `<uid>` form requires `MUTE`. Implemented in `internal/athena/commands_qol.go`.
- `/clients <uid>` — lists every connection sharing the target's IPID with UID, character, area, OOC name and showname. Requires `MUTE`.

### Self-Service Idle Auto-Disconnect (`/dc` / `/dctime`)
Lets **any** player opt into being automatically disconnected after a chosen stretch of inactivity — the same convenience WebAO's idle timeout offers, but user-controlled. Implemented in `internal/athena/disconnect_timer.go`; no permission required (`general` category).

- **Opt-in:** OFF by default. Nobody is ever auto-disconnected unless they personally enable it.
- **Isolated to the caller:** the per-connection watcher goroutine only ever closes the connection of the client that set it. It sends no packet to anyone else and cannot affect another player. (Both clarifications the requester stressed.)
- **Forgiving:** sending an IC or OOC message resets the countdown (`dcTouchActivity`, hooked in `pktIC`/`pktOOC`), so the timer fires only after genuine AFK inactivity — exactly like the WebAO behaviour it mirrors.
- **Default:** running the command with no number arms a **1-hour** countdown.

| Command | Behaviour |
|---------|-----------|
| `/dctime` (or `/dc`) | Enable with the default 60-minute idle window. |
| `/dctime <minutes>` | Disconnect after `<minutes>` of inactivity (capped at 7 days so the deadline arithmetic can't overflow). |
| `/dctime off` | Cancel the timer. Also accepts `0`, `stop`, `cancel`, `disable`, `none`. |
| `/dctime status` | Show the current setting without changing it. |

`/dc` is a plain alias of `/dctime`. A single watcher goroutine is spawned lazily on first enable (CAS-gated) and lives for the rest of the connection, no-opping while disabled and exiting on `client.done`, so re-enabling never respawns it and there is no start/stop race. The watcher re-checks every 10 s, so the disconnect lands within ~10 s of the deadline — plenty precise for an AFK timer. Documented in `/help` via the command's registry `desc`/`usage`.

### Other Features
- Hot Potato area minigame
- Quick Draw area minigame
- Chip Giveaway system
- Area Roulette
- Wardrobe/character management commands
- `/randomchar`
- In-place server restart via `syscall.Exec`
- Testimony recorder (inherited from upstream Athena)
- `/about` credits SyntaxNyah's fork and full credit to MangosArentLiterature's upstream Athena.

## Testing

```bash
go test -v ./...
```

Tests cover: punishment stacking, coinflip logic, rate limiting (with race detector), persistent pairing, per-area logging benchmarks, giveaway, hot potato, quick draw, and roulette. All tests pass with `-race`.

## License

GNU Affero General Public License v3.0 (inherited from upstream Athena).

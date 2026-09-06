# Nyathena — Moderator Commands Guide

Every command in here requires a moderator-class permission. Required permission bits are noted in the right-hand column of each table. Detailed punishment-by-punishment text effects are documented separately in [`PUNISHMENT_COMMANDS.md`](PUNISHMENT_COMMANDS.md).

For player-facing commands, see [`PLAYER_COMMANDS.md`](PLAYER_COMMANDS.md).

---

## Permission Tiers (overview)

| Tier | Bits typically granted | Notes |
|------|-----------------------|-------|
| `MUTE` | The minimum tier for punishments | All punishment text effects, gag, tournament |
| `KICK` | + ability to remove players from the server | Includes /charcurse |
| `BAN` | + connection-level moderation | /ban, /unban, /firewall |
| `MOVE_USERS` | Move/summon players between areas | /summon |
| `MODIFY_AREA` | Override area settings | BG/music locks, force CMs |
| `BAN_INFO` | View ban records | /getban, /listbans |
| `ADMIN` | Server runtime configuration | /arealog, /reloadplaytime, /deletetag |
| `SHADOW` | Stealth moderator | Hidden from /gas/players for non-admins |

Permission bits are configured in `config/roles.toml`. Multiple bits are granted as a bitfield — see the role definitions for combinations.

---

## Account Management (Mod CLI / `/login`)

| Command | Permission | Description |
|---------|-----------|-------------|
| `/login <username> <password>` | NONE | Sign in to your moderator account |
| `/logout` | NONE | Sign out |
| `mkusr <username> <password>` (CLI) | server stdin | Create the first moderator account |
| `/mkusr <username> <password> <role>` | ADMIN | Create a moderator user |
| `/setrole <username> <role>` | ADMIN | Change a user's role (permission tier) |
| `/removerole <username>` | ADMIN | Strip a user's role, resetting them to a default player account. **Keeps the account** (login, chips, playtime, tags) — does not delete it. Use this to demote a DJ/mod without purging their account. |
| `/resetpass <username> <new_password>` | ADMIN | Reset a user's password |
| `/rmusr <username>` | ADMIN | **Delete** a moderator user account entirely |

---

## Connection-Level Moderation

| Command | Permission | Description |
|---------|-----------|-------------|
| `/ban -u <uid> [-d duration] <reason>` | BAN | Ban by UID. If the IPID is linked to a registered account, every online moderator gets an OOC alert naming it (informational only -- the account itself is never touched). |
| `/ban -i <ipid> [-d duration] <reason>` | BAN | Ban by IPID (works on offline targets). Same account-link OOC alert as above. |
| `/unban <ban-id>` | BAN | Lift a ban |
| `/getban [-b banid \| -i ipid]` | BAN_INFO | Look up bans |
| `/editban [-d duration] [-r reason] <ids>` | BAN | Edit ban metadata |
| `/kick <uid>` | KICK | Disconnect a player |
| `/kickother` | NONE | Kick stale ghost connections sharing your HDID |
| `/firewall on\|off` | BAN | Toggle the IPHub VPN/proxy firewall (requires `iphub_api_key` in config). Also exposed as a Discord slash command. |
| `/lockdown [add <uid>\|whitelist all]` | BAN | Toggle server lockdown, or whitelist players. Turning it ON also instantly kicks every connected non-moderator whose total playtime is under the `/setlockdownplaytime` threshold, and for as long as lockdown stays active, silently drops (never broadcasts) any IC/OOC message from anyone still under that threshold. |
| `/setplayerlimit <n>` | BAN | Set the player-capacity lockdown threshold (new joins rejected once this many are connected; 0 = off) |
| `/setlockdownplaytime <minutes>` | BAN | Set the lockdown purge's minimum total-playtime threshold; 0 disables the purge |
| `/lockdown whitelist <passkey>` | BAN | Verify a lockdown passkey (see `secretseed` in config.toml) — whitelists the IPID it certifies and permanently exempts it from the playtime purge |
| `/lockdown unwhitelist <uid\|ipid>` | BAN | Revoke a previously-verified passkey exemption |
| `/joincaptcha status` | BAN | Show the join captcha's settings, who is still awaiting an answer, and who the captcha has acted on |
| `/joincaptcha verify <uid>` | BAN | Release a player the captcha caught (a false positive), verifying them outright |
| `/joincaptcha reset <uid\|ipid\|all>` | BAN | Clear a stored verification so that IPID is challenged again on its next connection; `all` clears every one |
| `/lockdown exemptlist` | BAN | List every IPID currently exempt via a verified passkey |
| `/tormentlist` | MUTE | List every IPID on the torment list, with any connected sessions |
| `/untorment <ipid\|all>` | BAN | Remove one IPID from the torment list, or `all` to purge the entire list |
| `/shadowundisconnect <uid\|ipid\|all>` | ADMIN | Lift a shadow-disconnect. Nothing adds to that list any more — `/shadowdisconnect` was removed — so this is for clearing entries made before then. |
| `/shadowdisconnectlist` | ADMIN | List every remaining shadow-disconnect entry, newest first |
| `/censoralerts [on\|off]` | MOD_CHAT | Toggle the OOC alerts you receive when a player trips the word censor (per-session; defaults to on) |

Censor trips (AutoMod banned words and `censored_names.txt` shownames) alert every online moderator in OOC. With the default `automod_action = "shadow"`, the offending message is shadow-sent — the sender's client shows it as sent, but no other client ever receives it — and the speaker is put on the torment list. Only censor trips reach the torment list from in game; there is no longer any in-game command that adds to it by hand. The console's `torment <ipid>` does not alert other mods.

---

## Voice Moderation

| Command | Permission | Description |
|---------|-----------|-------------|
| `/mute <uid> [-d duration]` | MUTE | Prevent IC speech |
| `/unmute <uid>` | MUTE | Restore IC speech |
| `/gag <uid>` | MUTE | Replace IC text with gibberish |
| `/ungag <uid>` | MUTE | Remove gag |
| `/parrot <uid> [-d duration]` | MUTE | IC text replaced with random parrot phrase |
| `/oocmute <uid>` | MUTE | Mute from OOC chat |
| `/oocunmute <uid>` | MUTE | Restore OOC chat |

---

## Area Control

| Command | Permission | Description |
|---------|-----------|-------------|
| `/lock` | NONE (CM) | Lock the area; current occupants get auto-invited |
| `/unlock` | NONE (CM) | Unlock the area |
| `/lock -s` | NONE (CM) | Set area to spectatable (joiners enter as spectators) |
| `/adminlock` | ADMIN | Toggle an **admin-only seal**: nobody but admins can enter — not even mods or shadow mods with `BYPASS_LOCK`, and not even invited players. Players already inside are not evicted. A non-admin cannot `/unlock` or `/lock` an admin-locked area; only `/adminlock` (by an admin) lifts it. |
| `/invite <uid>` | NONE (CM) | Invite a UID. In a **locked** area this grants entry; in **spectate mode** it also grants the right to speak in IC (same as `/spectate invite`). Requires the area to be locked or in spectate mode — in a plain unlocked area it explains how to restrict the area first instead of doing nothing. |
| `/uninvite <uid>` | NONE (CM) | Remove from invite list |
| `/kick <uid>` (in-area) | NONE (CM) | Eject a player from the area. Now also pulls them from the invite list, so they can't walk back into a locked room. |
| `/cleararea` | MOVE_USERS | Move all players out of an area to the lobby |
| `/forcemove <uid> <area>` | MOVE_USERS | Force-move a player |
| `/summon <area>` | MOVE_USERS | Summon all players to an area |
| `/jail <uid>` | MUTE | Restrict a player to the jail area |
| `/unjail <uid>` | MUTE | Lift jail |
| `/bg <bg>` | DJ / CM / MODIFY_AREA | Set background (DJs rate-limited to once per minute) |
| `/lockbg true\|false` | MODIFY_AREA | Lock/unlock background changes |
| `/lockmusic true\|false` | MODIFY_AREA | Lock/unlock music changes |
| `/forcebglist true\|false` | MODIFY_AREA | Force the server BG list on this area |
| `/allowiniswap true\|false` | MODIFY_AREA | Permit iniswapping |
| `/allowcms true\|false` | MODIFY_AREA | Permit area CMs |
| `/evimode <mode>` | NONE (CM) | Set evidence mode (any/cms/mods) |
| `/status <status>` | NONE (CM) | Set area status |
| `/spectate [invite\|uninvite <uids>]` | NONE (CM) | Toggle spectate mode, or grant/revoke IC speaking rights while it's on. Listed in `/help` for **all** players (not just CMs) so everyone can discover how spectate mode works, though only CMs can run it. |
| `/area mute` / `/area unmute` | NONE (CM) | Silence everyone in the area except CMs and moderators (IC and OOC), and lift it again. Scoped to the room: leaving the area lifts it automatically. |
| `/area rename <name>` | NONE (CM) | Rename the area you are standing in — see [Renaming an area](#renaming-an-area-area-rename) below. |
| `/area unrename` | NONE (CM) | Restore the area's configured name immediately. |
| `/areadesc [-c] [text]` | NONE | Set/clear area entry description |

---

### Renaming an area (`/area rename`)

A CM running a case in "Courtroom 3" can make the area list say what the room actually is:

```
/area rename DR Killing Game     # the area list now reads "DR Killing Game"
/area rename                     # show the current name and the configured one
/area unrename                   # put the configured name back now
```

Gated on `CM`, which both **area CMs** (anyone who ran `/cm`) and **moderators** hold, so it needs no special role.

**The name is a loan, not a transfer.** The name in `areas.toml` is the area's real identity and it comes back on its own:

- when the **last person leaves** the area, or
- when the area **loses its last CM** (they ran `/uncm`, moved to another area, or disconnected).

A name a moderator set in a room that had no CM at all is kept until the room empties — otherwise it would snap back the moment any unrelated player walked out. Nothing is persisted, so a restart also returns every area to its configured name.

**Renames reach players who are already connected.** The server rebuilds the area-name list and the join packet, and pushes the new list to every client (`FA`), so nobody has to reconnect to see it.

**What a name is refused for**, and why each rule exists — an area name is not just a label, it is the key clients send back to change area and the directory area logs are written into:

| Rule | Reason |
|------|--------|
| 1–32 characters | The area list is a narrow column in every AO2 client. |
| No `#`, `%`, `$` or `&` | AO2 spends those on its packet format; area names are sent unencoded, so one would split the list or make the area unreachable. |
| No control characters | They break the area list and the area-log line format. |
| Not a music-list entry or a stream URL | A music change is matched before an area change, so the room would become unreachable by name. |
| Not another area's current **or configured** name | A configured name comes back the instant that area empties, so allowing it would produce a silent duplicate later rather than an error now. |
| Passes the AutoMod word list | See below. |

**The name goes through the same word filter as IC and OOC** — same tiered `banned_words.txt`, same evasion normalization (spacing, leetspeak, homoglyphs, zalgo), same configured `automod_action`, same `[CENSOR]` staff alert, and the same `nuke` tier that destroys the attempt and bans the IPID. A room name is broadcast to every connected client, so there is no argument for holding it to a weaker standard than a single line of chat. A `watch`-tier word still passes, exactly as it does in chat. Under the default `shadow` action the caller is told the rename worked while the area is untouched and no other client ever sees the name.

---

## Per-Player Modifiers

| Command | Permission | Description |
|---------|-----------|-------------|
| `/charstuck [-d duration] <uid>` | MUTE | Lock to current character |
| `/charcurse <uid> <charname>` | KICK | Force a one-time character swap (target may change afterward) |
| `/forcepair <uid1> <uid2>` | MUTE | Force two players into a UID-tracked pair |
| `/forceunpair <uid>` | MUTE | Break a forced pair |
| `/setrole <uid> <role>` | ADMIN | Set a player's role (permission tier) |
| `/clearpos <uid>` | MOVE_USERS | Clear forced position |
| `/showname <uid> <name>` | MUTE | Force a showname |
| `/clearshowname <uid>` | MUTE | Clear forced showname |
| `/nameshuffle` | MUTE | Randomly permute every player's showname in this area |
| `/unnameshuffle` | MUTE | Restore shuffled shownames |
| `/charshuffle` | MUTE | Randomly permute every player's character in this area (Sattolo's algorithm — guaranteed derangement) |
| `/uncharshuffle` | MUTE | Restore characters to pre-shuffle state |
| `/clients <uid>` | MUTE | List every connection sharing the target's IPID — multiclient overview with UID, character, area, OOC name and showname |
| `/punishments <uid>` | MUTE | Inspect any player's active punishments with remaining durations, issuer tiers, lag/mute/jail status |

---

## Removed: possession and stealth-disconnect

`/possess`, `/fullpossess`, `/truepossess`, `/unpossess` and `/shadowdisconnect` **no longer exist**. They were briefly gated behind a server-console `grant` before being removed outright; the `grant` console verbs are gone too.

What the five had in common is that their effect was invisible to the person it landed on. A possession spoke as somebody and silenced them so they could not say otherwise; a shadow-disconnect looked to its target exactly like a bad router. Neither leaves the person on the receiving end anything to notice, report or appeal — which is a bad property for a moderation tool to have, however it is gated.

Two things deliberately remain:

- **`/shadowundisconnect <uid|ipid|all>` and `/shadowdisconnectlist` (ADMIN)** still work, so any entry written before the command was removed can be found and lifted. Nothing adds to that list any more.
- **The torment list is unchanged.** AutoMod still arms it automatically on a censor trip, and `/tormentlist` / `/untorment` still manage it. What is gone is the ability to apply it by hand from in game (`/lag` was removed in the same change); the console keeps `torment <ipid>` / `untorment <ipid|all>`.

---

## Punishments — Quick Index

> All punishments share these flags: `-d <duration>` (max 24h), `-r <reason>`, `-h` (hidden — apply silently), comma-separated UIDs, and `global` (apply to every non-mod in your area). Multiple punishments stack. Use `/help punishment` in-game for the **subcategorized** browser. Per-effect docs live in `PUNISHMENT_COMMANDS.md`.

| Group | Commands |
|-------|----------|
| Text effects (60) | `/whisper /backward /stutterstep /elongate /uppercase /lowercase /robotic /alternating /fancy /uwu /pirate /shakespearean /caveman /censor /fromsoftware /confused /paranoid /drunk /hiccup /whistle /mumble /slang /cherri /albhed /morse /vowelhell /upsidedown /autospell /thesaurusoverload /valleygirl /babytalk /thirdperson /unreliablenarrator /uncannyvalley /chef /karen /passiveaggressive /nervous /sarcasm /academic /philosopher /poet /quote /spaghetti /essay /rng /haiku /dreamsequence /timewarp /zalgo /leetspeak /smallcaps /piglatin /vaporwave /lisp /spoonerism /keysmash /weeb /politician /clickbait /markov /alliteration /cipher` |
| Themed quote replacers | `/gordonramsay /biblebot /grounded /mime /subtitles /spotlight /recipe /rickroll /pickup /brainrot` |
| Persona / personality | `/clown /jester /joker /tourettes /translator` |
| Dere archetypes (26) | `/tsundere /yandere /kuudere /dandere /deredere /himedere /kamidere /undere /bakadere /mayadere /smugdere /deretsun /bokodere /thugdere /teasedere /dorodere /hinedere /hajidere /rindere /utsudere /darudere /butsudere /sdere /mdere /tsuyodere /omnidere` |
| Animal filters (14) | `/monkey /snake /dog /cat /bird /cow /frog /duck /horse /lion /trex /fish /zoo /bunny` — `/trex` roars RAAASRFH, `/fish` blubs, and `/horse` also swaps the target onto a random uma character |
| Visibility / cosmetic | `/emoji /invisible /shrink /grow /wide /areainiswap /hidedisplay /forcedisplay` (and `/unshrink /ungrow /unwide`) |
| Protocol / viewport (6) | `/teleport /shakecurse /randomflip /forcecolor /nopreanim /forcepreanim` |
| Timing | `/slowpoke /fastspammer /lifo` |
| Audio | `/sfxcurse <uid> <sfx-url>` and `/unsfx` |
| Voice chat (5) | `/voicemute /voicestatic /voicegarble /voicecutout /voicestutter` |
| Traps & contagion (4) | `/contagious <type> /minefield /silencebell /stealthmute` |
| Stacking / chaos | `/stack /torment /lovebomb /degrade /emoticon /51 /icwarp /megamaso /maso /randompunishall /togglerandompunish /tournament` |
| Inspection | `/punishments [uid]` — active punishments with remaining durations (players: self only) |
| Removal | `/unpunish <uid>`, `/unpunish -t <type> <uid>`, `/unpunish all`, `/unlag`, plus per-effect `un-` commands |
| Self-chaos block | `/blockpunishment /unblockpunishment` |

### Hidden flag (`-h`)

Appending `-h` to any punishment command suppresses the OOC notification to the target — the punishment applies silently. The issuer's summary appends `(hidden)` for confirmation. Works on all applicators: single-effect, `/stack`, `/lovebomb`, `/sfxcurse`, `/shrink`/`/grow`/`/wide`, `/randompunishall` (also hides the area-wide announcement), `/translator curse`, and `/icwarp`.

```
/tsundere 7 -h                    # Silent tsundere
/stack backward uwu global -h     # Silent stack on entire area
/randompunishall -h                # No area announcement
```

### `/sfxcurse` example

```
/sfxcurse 12 https://miku.pizza/base/sounds/general/meow.opus
/sfxcurse 12 https://cdn.discordapp.com/attachments/123456789/987654321/boom.opus
/sfxcurse global https://example.com/honk.opus
```
The target's IC packet's SFX field is overwritten with the URL on every line until `/unsfx 12`.

**URL handling:**
- URLs that contain `/base/sounds/` (standard AO2 asset-server paths) have their filename stem extracted and sent to clients, which resolve it locally or via the configured asset URL.
- All other `http(s)://` URLs (Discord CDN, custom hosting, etc.) are forwarded as-is so that clients supporting URL-based audio can stream the file directly.

### `/unpunish` — Full Coverage

`/unpunish` covers **every** active punishment including the torment list (which AutoMod arms on a censor trip; `/lag` itself was removed). Forms:

- `/unpunish <uid>` — removes all punishments, mute, jail, and lag from a specific target.
- `/unpunish -t lag <uid>` — removes only lag.
- `/unpunish all` — wipes all punishments from every player in your current area.

### `/unpunish` Self-Removal Protection

The DB records the issuing tier of every punishment in `PUNISHMENTS.ISSUER_TIER`. A regular moderator **cannot** lift a punishment that an admin or shadow mod placed on them — `/unpunish self`, `/unpunish -t <type> self`, and the self-target slice of `/unpunish all` are all gated. Admins and shadow mods bypass the gate.

---

## Punishment Tournaments

| Command | Permission | Description |
|---------|-----------|-------------|
| `/tournament start\|status\|stop` | MUTE | Run a punishment tournament. Volunteers join via `/join-tournament` and accumulate 2–3 random punishments — most IC messages sent wins. |

---

## Custom Tags (cosmetic, staff-managed)

| Command | Permission | Description |
|---------|-----------|-------------|
| `/createtag <id> <display name>` | MUTE | Mint a new custom tag at runtime |
| `/deletetag <id>` | ADMIN | Delete a custom tag and clean up grants/equips |
| `/grantcustomtag <username> <id>` | MUTE | Grant a tag to an account (account must have logged in once) |
| `/revokecustomtag <username> <id>` | ADMIN | Revoke a granted tag |
| `/listcustomtags` | NONE | List every custom tag |

---

## Music Ban (persistent, per-IPID)

| Command | Permission | Description |
|---------|-----------|-------------|
| `/musicban <uid> [-r reason]` | MUTE | Persistently ban the target's IPID from playing music (both jukebox entries and streaming URLs) across sessions. Idempotent — re-banning overwrites the reason and issuer. |
| `/musicunban <uid\|ipid>` | MUTE | Lift a music-ban. Accepts a connected target's UID or a raw IPID, so offline players can still be unbanned. |
| `/musicbans` | MUTE | List every active music-ban with its reason, issuer, and timestamp (newest first). |

Music bans are stored in the `MUSIC_BANS` table (DB migration 22) and cached in-memory for a single-RWMutex-map-lookup hot path on the MC handler.

**Quiet-area carve-out:** if the area has **fewer than 3 people** in it, the ban is **bypassed** and the music change is allowed — banned players can still set the mood in empty/small rooms but can't bother a populated one. Moderators are always exempt. Area-change MC packets are unaffected.

---

## Server Admin

| Command | Permission | Description |
|---------|-----------|-------------|
| `/arealog enable\|disable` | ADMIN | Toggle area-log silencing for the current area |
| `/reloadplaytime` | ADMIN | Re-link every registered account to its IPID and merge orphaned playtime. Fixes the bug where a fresh account on a long-running anonymous IPID didn't appear on the leaderboard. |
| `/reload` | ADMIN | Hot-reload all supported config/data files at runtime without restarting. See "Hot config reload" below. |
| `/restart` | ADMIN | In-place server restart via `syscall.Exec` |
| `/casinoenable` | ADMIN | Toggle casino in this area |
| `/casinoset <key> <value>` | ADMIN | Configure casino limits / jackpot |
| `/grantchips <username> <amount>` | ADMIN | Add chips to an account |

### Hot config reload (`/reload`)

`/reload` (in-game, ADMIN) atomically re-reads every supported config/data file from disk and swaps it in without restarting. Also available as the `reload` CLI command on stdin and via `SIGHUP`.

Each reloadable list lives behind a `sync/atomic.Pointer` so a swap is a single atomic store — readers on the hot IC path never lock and never see a torn value. A parse error in any one file aborts the whole reload before anything is published, so a bad file never leaves the running server half-updated.

**Reloaded:**
- `characters.txt` — **append-only** (see safety constraint below)
- `music.txt` — full reload; the pre-built SM packet sent on every client join is rebuilt in lockstep
- `cdns.txt`, `backgrounds.txt` (with `/bglist` cache rebuilt), `parrot.txt`
- `8ball.txt` (optional; missing file leaves the current value intact)
- `banned_words.txt` (only when automod is enabled)
- `config.toml` motd and description

**`characters.txt` safety constraint — append-only.** Connected AO2 clients reference characters by **slot index**, so inserting in the middle, removing, reordering or renaming an existing slot would silently desync every connected player. The reload **validates that every existing slot is unchanged** and only accepts entries appended at the end of the file. If the new file changes any pre-existing slot, the reload is rejected with a precise message naming the first bad slot — change those operations require a restart.

**NOT reloaded** (would require invasive work and is unsafe without restart): areas, listener ports/addr, rate-limit windows, max_players, roles, the server name.

### Server console (stdin)

Reachable only with shell access to the host, which is a different and stronger thing than any in-game credential — so it is where the powers that outrank every in-game tier live.

| Console command | Description |
|-----------------|-------------|
| `help` | List the console commands |
| `mkusr <username> <password> <role>` / `rmusr <username>` | Create / delete a moderator account |
| `players` | List connected players |
| `getlog <area>` | Dump an area's report buffer |
| `say <message>` | Broadcast an OOC message to every connected player |
| `reload` | The same hot reload as `/reload` (also on `SIGHUP`) |
| `punishment <enable\|disable\|status>` | Global kill switch for the punishment system. Console-only by design: no in-game or Discord command can reach it. |
| `torment <ipid>` | Add an IPID to the torment list by hand. The only remaining manual route — `/lag` was removed from the game. |
| `untorment <ipid\|all>` | The console half of `/untorment` |

There is no longer a `grant` verb: the four commands it armed have been removed (see [Removed: possession and stealth-disconnect](#removed-possession-and-stealth-disconnect)).

---

## Discord Bot Slash Commands

> Requires the bot's role to have the configured `mod_role_id` (or the player's permission tier to be granted via account linking).

| Slash | Description |
|-------|-------------|
| `/players` | List connected players |
| `/info <player>` | Player info card |
| `/find <player>` | Locate a player's area |
| `/status` | Server status |
| `/mute /unmute /ban /unban /kick /gag /ungag /warn /warnings` | Moderation actions |
| `/parrot /drunk /slowpoke /roulette /spotlight /whisper /stutterstep /backward` | Apply punishments |
| `/pm /announce /announce_player` | Communication |
| `/forcemove /cleararea /lock /unlock` | Area control |
| `/logs /auditlog /banlist` | Audit & logs |
| `/firewall on\|off` | Toggle IPHub VPN screening |
| `/lockdown on\|off\|whitelist_all` | Toggle server lockdown / whitelist all currently-connected players. `on` also instantly kicks every connected non-moderator under the lockdown playtime purge threshold (`/setlockdownplaytime` in-game, 0 = off), and silently drops messages from anyone still under it for as long as lockdown stays on. |
| `/restart` | Restart the server (Admin only) |

---

## Stealth Mode

Shadow moderators (`SHADOW` perm bit, no `ADMIN`) are hidden from `/gas` and `/players` for non-admin viewers — no `Mod:` line is shown at all. Only admins see anything for a shadow mod, labelled `Mod: <name> (shadow)`.

Shadow mods are still visible on `/playtime top` (the leaderboard does not filter by permission), and they are NOT exempt from `/unpunish` self-removal protection — which means a regular mod cannot lift a shadow-mod-issued punishment on themselves.

**`/hide`** — vanishing entirely from `/players`, `/gas`, and room player counts — is ADMIN-only. `/hide` again to reappear. Shadow mods (without the ADMIN sentinel) cannot `/hide`, and neither can regular moderators.

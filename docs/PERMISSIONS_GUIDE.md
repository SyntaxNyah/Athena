# Nyathena — Permission System Guide

This is the complete reference for how access control works on a Nyathena server: the role system every Athena-family server has, and the account-based command grant system Nyathena adds on top of it. If you just want to know "how do I let this one player use this one command," skip straight to [Worked Examples](#worked-examples) — everything there is copy-pasteable.

This guide is for the **server owner** — whoever has shell/SSH access to the machine running the server. Some of what's here (`roles.toml`, `/mkusr`) can be done by any `ADMIN` moderator in-game; the command-grant half is deliberately console-only and cannot be done by anyone without shell access, no matter how high their in-game permissions are.

---

## Table of Contents

- [The two layers, in one paragraph](#the-two-layers-in-one-paragraph)
- [Layer 1: Roles (`roles.toml`)](#layer-1-roles-rolestoml)
- [Layer 2: Account Command Grants (`grantcmd`)](#layer-2-account-command-grants-grantcmd)
- [Worked Examples](#worked-examples)
- [Finding a command's exact name](#finding-a-commands-exact-name)
- [Player accounts vs. moderator accounts](#player-accounts-vs-moderator-accounts)
- [Security notes](#security-notes)
- [Troubleshooting](#troubleshooting)
- [Quick reference](#quick-reference)

---

## The two layers, in one paragraph

**Roles** (`config/roles.toml`) are coarse and role-based: an account is assigned a role, the role carries a bundle of permission bits (`MUTE`, `KICK`, `BAN`, `ADMIN`, ...), and every command behind one of those bits opens up together. This is the right tool for "this account is a moderator" or "this account is an admin."

**Command grants** (`grantcmd`, console-only) are fine and account-based: one specific command, for one specific account, independent of whatever role that account has. This is the right tool for "this one account should be able to run this one command, and nothing else changes about what they can do." A grant never touches roles or permission bits — it's a completely separate, additive mechanism. An account's role permissions and its command grants are just added together at the moment it tries to run a command; neither one knows the other exists.

You will normally reach for roles when setting up your regular staff team, and reach for grants when you have a one-off, narrower need that doesn't justify (or actively shouldn't get) a full role.

---

## Layer 1: Roles (`roles.toml`)

### Permission bits

| Bit | What it grants |
|-----|-----------------|
| `CM` | CM permissions in any area |
| `KICK` | Kick users from the server |
| `BAN` | Ban users from the server |
| `BYPASS_LOCK` | Bypass locked areas |
| `MOD_EVI` | Modify evidence when an area's evidence mode is "mods" |
| `MODIFY_AREA` | Modify area settings (BG/music locks, force CMs, etc.) |
| `MOVE_USERS` | Move/summon users to different areas |
| `MOD_SPEAK` | Speak officially as a moderator with `/mod` |
| `BAN_INFO` | View server bans (`/getban`) |
| `MOD_CHAT` | Use `/modchat` |
| `MUTE` | Mute and parrot users; the baseline tier for punishment commands |
| `LOG` | Unused (kept for `roles.toml` backward compatibility) |
| `DJ` | Set area entry descriptions, play music without area-CM status |
| `SHADOW` | Marks the role as an anonymous moderator — appears as "Moderator" to non-admins everywhere |
| `ADMIN` | Every permission, unconditionally |

### `roles.toml` format

```toml
[[Role]]
name = "moderator"
permissions = ["CM", "KICK", "BAN", "BYPASS_LOCK", "MOD_EVI", "MODIFY_AREA", "MOVE_USERS", "MOD_SPEAK", "BAN_INFO", "MOD_CHAT", "MUTE", "LOG", "DJ"]

[[Role]]
name = "shadowmod"
permissions = ["CM", "KICK", "BAN", "BYPASS_LOCK", "MOD_EVI", "MODIFY_AREA", "MOVE_USERS", "MOD_SPEAK", "BAN_INFO", "MOD_CHAT", "MUTE", "LOG", "SHADOW", "DJ"]

[[Role]]
name = "dj"
permissions = ["DJ"]

[[Role]]
name = "admin"
permissions = ["ADMIN"]
```

This file is loaded at startup and is **not** hot-reloadable — a change requires a restart. Add as many roles as you like (a "junior-mod" role with just `MUTE`+`KICK`, say); nothing in Nyathena hardcodes the four sample roles above.

### Assigning roles

```
mkusr alice hunter2 moderator      # server console (stdin) — creates the FIRST admin account
```
or, once at least one admin exists, in-game:
```
/mkusr bob hunter2 moderator       # ADMIN — create a new moderator account
/setrole bob shadowmod             # ADMIN — change an existing account's role
/removerole bob                    # ADMIN — strip the role, keep the account (login, chips, playtime intact)
/rmusr bob                         # ADMIN — delete the account entirely
```

A role change takes effect immediately for any connected session logged into that account.

---

## Layer 2: Account Command Grants (`grantcmd`)

A grant lets one account use one specific in-game command, regardless of its role. It works identically on:
- a **plain registered player** (created via in-game `/register`, zero permission bits) — gives them exactly one moderation-shaped power and nothing else, or
- an **existing moderator** who is missing one command their role doesn't cover — gives them that one extra command without promoting them to a broader role.

A grant is purely additive: it can only ever add access, never remove it, and it never appears as an extra permission bit anywhere. A granted account still shows up as whatever role it actually has in `/gas`, `/players`, shadow-mod labelling, and every other role-driven display — the only thing that changes is that one extra command now works for them.

**Every command in the server is grantable.** There are roughly 250 registered commands, and the grant check sits at the single chokepoint every one of them passes through to run — so nothing needs to be added to a command to make it grantable; it already is, the day it's added to the server.

### The three console commands

Typed at the server's stdin console (the same prompt `mkusr`/`reload`/`torment` are typed at), **not** in-game:

```
grantcmd <username> <command1>[,<command2>,...]
revokecmd <username> <command1>[,<command2>,...]
revokecmd <username> all
grants [username]
```

| Command | Does |
|---------|------|
| `grantcmd <username> <cmds>` | Grants one or more commands (comma-separated, no spaces) to an account |
| `revokecmd <username> <cmds>` | Revokes one or more specific commands |
| `revokecmd <username> all` | Revokes every grant the account currently holds |
| `grants` | Lists every account on the server that currently holds any grant |
| `grants <username>` | Lists just one account's grants, with who granted each one and when |

There is deliberately **no in-game equivalent** of any of these three. Handing out access to any command on the server, to any account, is exactly the kind of broad power this codebase keeps behind shell access rather than an in-game credential — see the note on the removed `grant` console gate in [`MOD_COMMANDS.md`](MOD_COMMANDS.md) for the design precedent this follows.

---

## Worked Examples

Every example below is a real command you can type at the server console as-is (swap in your own usernames).

### Example 1 — A trusted regular player who should be able to `/ban`, nothing else

You have a player, `alice`, who's around during hours your mod team usually isn't, and you trust her to deal with an obvious raider — but you don't want to make her a moderator (no mod chat, no mute/kick/punishment commands, no `/gas` mod visibility, nothing).

```
grantcmd alice ban
```

That's it. `alice` can now run `/ban -u <uid> <reason>` exactly like a moderator with the `BAN` bit would, and nothing else about her account changes — she still looks like a plain player everywhere else on the server.

To take it back later:
```
revokecmd alice ban
```

### Example 2 — A moderator who's missing one or two admin commands

`bob` has the `moderator` role (`MUTE`, `KICK`, `BAN`, etc.) but not `ADMIN`, and you want him to be able to check the live console log and purge the known-IP database during an incident, without handing him full `ADMIN` (which would also give him things like `/mkusr`, `/setrole`, `/adminlock`, and hiding his own admin status).

```
grantcmd bob terminal,purgedb
```

`bob` can now run `/terminal` and `/purgedb`. He is still exactly a `moderator`-role account in every other respect.

### Example 3 — Letting a CM-for-the-night run the testimony recorder without full CM status

`defense_attorney` is a regular player running a trial in one area tonight and needs the testimony-recorder commands, but you don't want to hand them general CM powers (kicking people from the area, locking it, etc.) via `/cm`.

```
grantcmd defense_attorney testify,add,delete,update,testimony,examine
```

Note `examine` doesn't actually need a grant (it has no permission requirement at all — anyone can play back existing testimony), it's included here just so the same player can also start cross-examination; the rest (`testify`/`add`/`delete`/`update`/`testimony`) are the ones that are otherwise CM-only.

### Example 4 — Giving an event DJ the ability to summon people to a stage area

`eventdj` has the `dj` role (just the `DJ` bit) and is running tonight's event. You want them able to pull everyone into the stage area with `/summon`, which normally needs `MOVE_USERS`.

```
grantcmd eventdj summon
```

When the event's over:
```
revokecmd eventdj summon
```

### Example 5 — Granting several commands at once, and checking what's live

```
grantcmd carol kick,mute,charcurse
grants carol
```

`grants carol` prints each of the three, who granted them (`console`) and when, so you have an audit trail without needing to remember what you handed out.

### Example 6 — Auditing the whole server before a review

```
grants
```

Lists every account that currently holds any grant at all, one line per account with all of its granted commands — the fastest way to answer "who has extra access on this server right now, beyond their role."

### Example 7 — Revoking everything from an account at once

`carol` is stepping back from helping out. Rather than revoking three separate commands:

```
revokecmd carol all
```

### Example 8 — Renaming doesn't lose the grant

If `alice` later renames her account with in-game `/resetusername alice2 <password>`, her `ban` grant from Example 1 moves with her automatically — no console action needed. `grants alice2` will show it; `grants alice` will show nothing.

---

## Finding a command's exact name

`grantcmd` takes the command name **without** the leading `/` — e.g. `ban`, not `/ban`. That name has to match exactly what a player types after the slash in-game:

- In-game, `/help <category>` lists every command in a category by name, and `/help <command>` shows one command's usage.
- The name you grant is the *literal* thing typed — `/gas` and `/players` are two separate commands (even though `/gas` is shorthand for `/players -a`), and `/global` and `/g` are two separate commands (even though they're aliases of the same handler). If you want a granted account to be able to use both an alias and its full name, grant both: `grantcmd alice global,g`.
- Command names are case-insensitive when granting (`grantcmd bob BAN` and `grantcmd bob ban` do the same thing) — they're normalized to lowercase before being stored.

---

## Player accounts vs. moderator accounts

Both live in the exact same account system and `grantcmd` treats them identically — the only difference is how the account was created:

- **Player accounts** are created by the player themselves, in-game, with `/register <username> <password>`. They start with zero permission bits.
- **Moderator accounts** are created by an admin, either at the console (`mkusr <username> <password> <role>`, used to bootstrap the very first admin) or in-game (`/mkusr <username> <password> <role>`, requires `ADMIN`). They start with whatever bits the named role carries.

`grantcmd` doesn't care which kind of account it's granting to — a player account with a grant and a moderator account with a grant both just get one extra command layered on top of whatever their role already gives them (zero, for a plain player account).

**Important: usernames are case-sensitive.** `grantcmd` matches the account's actual stored username exactly, the same way `/login` does. If a player registered as `Alice`, `grantcmd alice ban` will fail (no such account) — you need `grantcmd Alice ban`. `grantcmd` checks the account exists before granting anything, so a typo like this fails loudly with an error rather than silently granting nothing to nobody.

---

## Security notes

- **Console-only means console-only.** No in-game command, at any permission level including `ADMIN`, can grant, revoke, or list command grants. If you see a moderator claiming they can do this from in-game, something is wrong (or someone has shell access to the box they shouldn't).
- **A grant is scoped to exactly one command name.** Granting `ban` does not grant `unban`, `kick`, or anything else that sounds related — each one is its own grant.
- **A grant never elevates what a command itself is allowed to do.** For example, a grant only ever gives an account the ability to *run* a command — it doesn't change any of that command's own internal rules. A punishment-safe area still blocks punishment commands from a granted account exactly as it would from a real moderator; an admin-locked area still can't be entered or unlocked except by a real admin, even with every command imaginable granted.
- **Grants take effect immediately**, server-wide, with no restart and no `/reload` — the in-memory cache is rebuilt the moment `grantcmd`/`revokecmd` runs.
- **Grants are stored in the server's database** (`ACCOUNT_COMMAND_GRANTS` table) and survive restarts.

---

## Troubleshooting

**"I granted the command but the player still gets 'You do not have permission to use that command.'"**
1. Check the username matches exactly, including case: `grants <username>` — if it prints nothing, the grant either doesn't exist under that exact name or was never created.
2. Check the command name matches exactly what they're typing, including which alias: granting `global` does not cover `/g`.
3. Confirm the player is actually **logged into that account** on their current connection — a grant is checked against the account you're currently authenticated as (via `/login` or a fresh `/register`), not against an IPID or a past session. If they reconnected and haven't logged back in, the grant won't apply yet.
4. A small number of commands (documented in `CLAUDE.md` under "Account-Based Command Grants") have an internal check that a grant deliberately does *not* bypass — most notably the admin-lock on `/lock`/`/unlock`, which stays admin-only no matter what's granted, by design.

**"I revoked a command but `grants <username>` still shows it."**
This shouldn't happen — `revokecmd` rebuilds the live cache in the same call. If it does, check the server log around the time of the `revokecmd` call for an error (a DB write failure would leave the grant in place and say so).

---

## Quick reference

```
# Roles (in-game, ADMIN, or console mkusr for the first account)
/mkusr <username> <password> <role>
/setrole <username> <role>
/removerole <username>
/rmusr <username>

# Command grants (server console / stdin only)
grantcmd <username> <command1>[,<command2>,...]
revokecmd <username> <command1>[,...]
revokecmd <username> all
grants
grants <username>
```

See [`MOD_COMMANDS.md`](MOD_COMMANDS.md) for the full moderator command list and [`PLAYER_COMMANDS.md`](PLAYER_COMMANDS.md) for the player-facing one — any command name in either of those tables is a valid `grantcmd` target.

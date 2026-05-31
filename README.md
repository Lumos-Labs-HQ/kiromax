# kiromax

Manage multiple kiro-cli accounts on a single machine. Supports flat sessions and guilds — groups of 10 accounts sharing a 500-credit pool.

## How it works

Each account is stored as a `.sqlite3` file. The active account is always `~/.local/share/kiro-cli/data.sqlite3`, which kiro-cli reads directly.

On every swap kiromax copies the target session file to `data.sqlite3` and records `used_at` so the same account isn't reused in the same month.

## Directory layout

```
~/.local/share/kiro-cli/
├── data.sqlite3          ← live active session (kiro-cli reads this)
└── kiro_data/
    ├── guild_alpha/      ← guild: 10 accounts = 500 credits
    │   ├── 1.sqlite3
    │   ├── 2.sqlite3
    │   └── ...
    ├── guild_beta/
    │   └── ...
    └── 1.sqlite3         ← flat sessions (no guild)
```

## Guilds

A guild is a folder of 10 accounts. Each account gives 50 free credits, so a guild has 500 credits total.

- Within a guild, `watch` auto-swaps to the next account when one hits 50 credits.
- The guild total is tracked as a running 0→500 counter.
- Switching to the next guild is always **manual** — you decide when to move on.

### Building a guild

Log into each account in kiro-cli one at a time, running `capture` after each login:

```bash
kiromax guild create alpha

# log into account 1 in kiro-cli, then:
kiromax capture alpha

# log into account 2 in kiro-cli, then:
kiromax capture alpha

# repeat until 10 accounts captured
```

`capture` auto-numbers the files (`1.sqlite3`, `2.sqlite3`, …). The current session is not affected — it just copies it.

### Guild commands

```
kiromax guild list              List all guilds with session status and credit totals
kiromax guild create <name>     Create a new guild directory
kiromax guild add <g> <file>    Copy a session sqlite3 into a guild
kiromax guild swap              Manually advance to the next guild
kiromax guild reset [<name>]    Unend all sessions in a guild (or all guilds)
kiromax guild credits [<name>]  Show credit usage for the active or named guild
```

## Flat session commands

```
kiromax list              List flat sessions with status
kiromax swap              Auto-swap to next unused flat session this month
kiromax use <id>          Force swap to a specific session by ID or filename
kiromax end <id>          Mark a session as ended (skipped by swap)
kiromax reset [<id>]      Unend flat sessions
kiromax credits [<id>]    Show live credit usage (defaults to active session)
```

## Other commands

```
kiromax capture <guild>         Snapshot current login into a guild (auto-numbered)
kiromax login                   Log in to a new kiro account
kiromax continue / c            Open conversation picker to resume any previous chat
kiromax watch [--interval N]    Poll credits every N min (default 5), auto-swap within guild
kiromax swap --guild            Manually advance to next guild (alias for guild swap)
```

## Watch mode

`watch` runs in the foreground and polls the active account's credits on an interval:

- Prints `account: X/50 | guild: Y/500` every tick.
- Auto-swaps to the next account in the guild when one hits 50.
- When the guild is exhausted it prints a reminder to run `kiromax guild swap` — it does **not** switch guilds automatically.

```bash
kiromax watch --interval 3
```

## Session lifecycle

- `ended` flag — set automatically when an account is swapped out; skipped by future swaps.
- `used_at` — timestamp of last activation; flat `swap` skips accounts used this calendar month.
- `reset` clears the ended flag, making sessions available again.

## Notes

- Requires `kiro-cli-chat` on PATH for the `continue` command.
- Session files must be readable and writable by the current user.
- For the active session, `credits` always reads from the live `data.sqlite3` so refreshed tokens are picked up automatically.

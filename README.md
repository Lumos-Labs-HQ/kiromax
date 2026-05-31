# kiromax

Manage multiple kiro-cli accounts on a single machine. Swap between sessions, track monthly usage, and keep a unified conversation history across all accounts.

## How it works

Each account is stored as a `.sqlite3` file in `~/.local/share/kiro-cli/kiro_data/`. The active account is `~/.local/share/kiro-cli/data.sqlite3`.

On every swap kiromax:
1. Saves the current `data.sqlite3` back to the active session file (preserves chat history and refreshed tokens).
2. Merges conversation history from all sessions into the target session, so `--resume` works regardless of which account is active.
3. Copies the target session file to `data.sqlite3`.

This means you can switch accounts mid-month and continue any previous conversation with `kiromax continue`.

## Setup

Create the session directory and drop your session files in:

```bash
mkdir -p ~/.local/share/kiro-cli/kiro_data
# copy or move existing data.sqlite3 files there, named 1.sqlite3, 2.sqlite3, etc.
```

## Commands

```
kiromax list              List all sessions with status
kiromax swap              Mark current session as ended, switch to next unused this month
kiromax use <id>          Switch to a specific session by ID or name
kiromax end <id>          Mark a session as ended (skipped by swap)
kiromax reset [<id>]      Unend all sessions (or one), clearing used_at
kiromax credits [<id>]    Show live credit usage (defaults to active session)
kiromax continue          Open the conversation picker to resume any previous chat
kiromax c                 Alias for continue
```

## Session lifecycle

- `swap` picks the next session that is not ended and was not used this calendar month.
- `reset` clears both the ended flag and used_at, making sessions available again.
- Sessions are identified by numeric ID (position in sorted file list) or filename.

## Notes

- Requires `kiro-cli-chat` to be on PATH for the `continue` command.
- Session files must be readable and writable by the user running kiromax.
- The `credits` command reads the OAuth token stored in the session DB. For the active session it always reads from the live `data.sqlite3` in case kiro-cli has refreshed the token.

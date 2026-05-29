# kiromax

kiromax is a small command-line tool for managing multiple kiro-cli SQLite session
files. It helps rotate sessions by copying a chosen session file to the live
`data.sqlite3` used by `kiro-cli`, marking sessions as ended/unended, and
querying live credit usage (when a social access token is present).

## Overview

- Each session is stored as `<id>.sqlite3` in the `kiro_data` directory.
- The live profile used by `kiro-cli` is `data.sqlite3` (location configured
  by the `dataDB` constant in the code).
- `kiromax swap` marks the current active session as ended and selects the
  next session that has not been used during the current month.



## Configuration

By default `main.go` uses these paths (edit the constants in the source if
needed):

- `dataDB` — `/home/username/.local/share/kiro-cli/data.sqlite3`
- `kiroDataDir` — `/home/username/.local/share/kiro-cli/kiro_data`

Ensure the user running `kiromax` has read/write access to those files and
directories.

## Commands & Usage

- `kiromax list`
  - Lists session files found in `kiroDataDir` with status, ended flag, and
    last-used time.

- `kiromax swap`
  - Auto-swap to the next available session. Marks the current active session
    as ended, then copies the next unused session file to `data.sqlite3` and
    records metadata.

- `kiromax use <id>`
  - Force swap to a specific session ID (copies `<id>.sqlite3` to `data.sqlite3`).

- `kiromax end <id>`
  - Mark the session `<id>` as ended (it will be skipped by `swap`).

- `kiromax reset [<id>]`
  - Without an ID: unend all sessions (make them available for swapping).
  - With an ID: unend the specified session.

- `kiromax credits <id>`
  - Fetch live credit usage for the session using a social access token stored
    inside that session's DB under `auth_kv` key `kirocli:social:token`.

## Examples

List sessions:

```bash
./kiromax list
```

Auto-swap (marks current active session ended and picks next unused):

```bash
./kiromax swap
```

Force use session `3`:

```bash
./kiromax use 3
```

Check credits for session `3` (requires token stored in the session DB):

```bash
./kiromax credits 3
```

## Use cases

- Rotate multiple kiro-cli accounts/profiles on a single machine.
- Automatically cycle sessions once per calendar month.
- Track which sessions were used and when via the `used_at` metadata.
- Query remote usage/credits for sessions that contain a saved social token.

## Implementation notes

- The tool reads and writes a simple meta table `kiromax_meta` inside each
  SQLite file. It also trusts the `auth_kv` table to contain JSON tokens for
  the `credits` command.
- Swapping is done by copying bytes from the session file to `data.sqlite3`.
  This means file permissions for `data.sqlite3` must allow this operation.

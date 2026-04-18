# wallhaven-rotator — Design

Daily wallpaper rotator for Hyprland that pulls from [Wallhaven](https://wallhaven.cc)'s
`hot` feed, with a favorites workflow and [Walker](https://github.com/abenz1267/walker)
integration.

## Goal

- Rotate the desktop wallpaper once per day from Wallhaven's hot feed, filtered
  by user-configured tags / ratios / resolution.
- Let the user mark the current wallpaper as a favorite, which moves it out of
  the rotation pool into a kept collection.
- Let the user switch between modes: **daily** (fresh Wallhaven pick every day)
  and **favorites** (rotate through saved favorites only, one per day).
- Never re-pick a wallpaper we've already shown (dedup by Wallhaven ID).
- Delete non-favorited wallpapers after they leave the screen — disk stays lean.
- Expose all actions via Walker with one grouped menu entry plus direct
  top-level shortcuts.

Non-goals (for v1): multi-monitor per-monitor wallpapers, NSFW content,
user-hosted favorites sync, GUI.

## Layout on disk

```
~/.config/wallhaven-rotator/
├── config.toml        # user-editable; see schema below
├── history.db         # sqlite: every wallpaper ever shown
├── state.toml         # runtime state (mode, last-rotated date, current ID)
├── current.jpg        # symlink → file currently displayed
├── current.meta       # json sidecar for the current wallpaper (id, tags, url)
└── favorites/
    ├── <id>.jpg
    └── <id>.meta
```

Rationale: symlink `current.jpg` is the stable target for hyprlock and any
other consumer — they never need to know where the actual file lives.

## Config schema (`config.toml`)

```toml
# What to search for. Empty list = any.
[filter]
query      = ""                   # freeform search query (Wallhaven `q` param); empty = no text filter
tags       = []                   # additional tag IDs (strings)
categories = ["general"]          # general | anime | people
purity     = ["sfw"]              # sfw only in v1
sorting    = "hot"                # hot | toplist | views | favorites | random
min_width  = 3840
min_height = 1600
ratios     = ["21x9", "32x9"]     # empty = any
colors     = []                   # hex codes, e.g. ["000000"]

# Rotation behavior.
[rotation]
mode                 = "daily"    # daily | favorites
delete_non_favorites = true       # purge current file when next rotation picks one
trigger              = "first-login"  # first-login | timer
timer_time           = "07:00"    # only used if trigger = "timer"

# Where things live. Defaults resolve to XDG paths.
[paths]
# (optional overrides — normally leave unset)
# config_dir    = "~/.config/wallhaven-rotator"
# favorites_dir = "~/.config/wallhaven-rotator/favorites"
# current_link  = "~/.config/wallhaven-rotator/current.jpg"

# How to apply the wallpaper.
[apply]
command = "awww"                  # awww | swww | hyprpaper
transition_type     = "grow"
transition_duration = "0.8"
transition_fps      = "60"
```

## Commands

Subcommands of `wallhaven-rotator`. Each exits non-zero on failure so Walker /
scripts can react.

| Command | Behavior |
|---|---|
| `next` | Fetch + apply a new wallpaper right now (respects current mode). Advances rotation. |
| `favorite` | Move current wallpaper into favorites/. Copies the file so the symlink keeps working. |
| `unfavorite [id]` | Remove a favorite. Defaults to current if no id given. |
| `mode <daily\|favorites>` | Switch rotation mode. Persists to `state.toml`. |
| `info` | Print current wallpaper metadata (id, tags, uploader, URL, ratio, favorited?). |
| `open` | Open current wallpaper's Wallhaven page in default browser. |
| `status` | Print mode, last rotation timestamp, history count, favorites count. |
| `daemon` | First-login-of-day check. Rotates if today's date ≠ last-rotated date. Called from hypr autostart. |
| `list-favorites` | List favorites with id + tags. |

Every command is idempotent-safe: `next` twice in a row simply pulls two
different wallpapers; `favorite` on an already-favorited wallpaper is a no-op.

## Wallhaven integration

- API base: `https://wallhaven.cc/api/v1`
- Endpoints used:
  - `GET /search` — list hot results matching filters, paginated (24/page).
  - `GET /w/{id}` — single wallpaper detail (rarely needed — search already
    returns enough).
- User-Agent header required (empty UA gets 403). We send
  `wallhaven-rotator/<version> (+https://github.com/jcleira/wallhaven-rotator)`.
- Rate limit is ~45 req/min unauthenticated — well within our envelope
  (1–2 req per rotation).

Selection algorithm for `next` in `daily` mode:
1. Load history → set of seen IDs.
2. Page through `hot` results with the filter, skipping seen IDs.
3. Pick the first unseen hit. If no unseen hit in the first 5 pages, widen
   the filter (drop `ratios`, then drop `min_width`/`min_height`, then fall
   back to `toplist` sorting).
4. Download the image, write to favorites/ or cache, record in history,
   update `current.jpg` symlink.

`favorites` mode: just rotate through `favorites/*.jpg` in a stable order,
advancing by one per day.

## Storage — history DB

`history.db` — sqlite, single table:

```sql
CREATE TABLE shown (
  id            INTEGER PRIMARY KEY,   -- wallhaven id (base36 → int)
  shown_on      TEXT    NOT NULL,      -- ISO date, YYYY-MM-DD
  url           TEXT    NOT NULL,      -- wallhaven page URL
  image_url     TEXT    NOT NULL,      -- direct image URL
  ratio         TEXT,
  width         INTEGER,
  height        INTEGER,
  tags          TEXT,                  -- comma-separated
  favorited_at  TEXT                   -- ISO timestamp; null = not favorited
);
```

## Rotation trigger

**Default: first-login.** Hypr autostart runs `wallhaven-rotator daemon` on
session start. The daemon:

1. Reads `state.toml:last_rotated_date`.
2. If `last_rotated_date == today`, exit 0 (nothing to do).
3. Otherwise run `next`. If it fails (e.g. offline), log and exit 0 — we'll
   retry at next login.

No systemd timer needed. Simple, robust, no duplicate rotations. Users who
want a fixed-time rotation can set `trigger = "timer"` and we'll emit a
user-level systemd unit to `~/.config/systemd/user/`.

## Walker integration

Walker has a "modules" config with custom command providers. We'll ship:

- A top-level entry `wallpaper` that opens a submenu with: `next`, `favorite`,
  `unfavorite`, `info`, `open`, `switch mode`, `status`.
- Direct top-level entries that shortcut the most-used actions:
  - `wallpaper next`
  - `wallpaper favorite`
  - `wallpaper switch mode`

Implementation: a small JSON provider script (`walker/wallpaper.sh`) in the
rotator repo, plus a snippet the user pastes into
`~/.config/walker/config.toml` (or that the dotfiles repo installs).

## Hyprland wiring (lives in the dotfiles repo)

- `linux/.config/hypr/hyprlock.conf:20` → change path to
  `~/.config/wallhaven-rotator/current.jpg`.
- `linux/.config/hypr/conf/autostart.conf` → add
  `exec-once = wallhaven-rotator daemon`.
- Existing `linux/.config/hypr/scripts/wallpaper.sh` becomes obsolete —
  remove it (or keep as fallback when rotator is not installed).

## Go project layout

```
wallhaven-rotator/
├── cmd/
│   └── wallhaven-rotator/
│       └── main.go           # subcommand dispatch
├── internal/
│   ├── wallhaven/            # HTTP client
│   ├── config/               # TOML load/save
│   ├── history/              # sqlite store
│   ├── rotate/               # selection + apply
│   ├── favorites/            # add/remove/list
│   └── apply/                # awww/swww/hyprpaper wrappers
├── walker/
│   └── wallpaper.sh          # Walker provider
├── docs/
│   └── install.md
├── DESIGN.md                 # this file
├── README.md
├── LICENSE
├── go.mod
└── go.sum
```

Dependencies (minimize, keep it a real Go project):

- `github.com/BurntSushi/toml` — config + state.
- `modernc.org/sqlite` — pure-Go sqlite, no cgo. (Alternative:
  `github.com/mattn/go-sqlite3` if we want the canonical driver and don't
  mind cgo. Opting for pure-Go for easier cross-platform builds.)
- stdlib `net/http`, `encoding/json` for Wallhaven API.

## Decisions (signed off)

- Default `filter.query = ""` — no text filter out of the box; hot feed at
  your monitor's resolution/ratio, SFW only. User can add a query later.
- Favorites rotate in filename-sorted order (stable, predictable).
- `next` in `favorites` mode advances favorites, does not fetch from
  Wallhaven.
- Install via `go install github.com/jcleira/wallhaven-rotator/cmd/wallhaven-rotator@latest`
  → binary lands in `~/go/bin/wallhaven-rotator`.

## Plan of work

1. This design doc (reviewed, signed off).
2. Scaffold: go mod, layout, empty main.go wired to Cobra.
3. Wallhaven client + history DB (testable in isolation).
4. `next`, `info`, `status` — the minimum viable loop.
5. `favorite`, `unfavorite`, `mode`, `list-favorites`.
6. `daemon` + hypr autostart + hyprlock symlink in dotfiles.
7. Walker integration.
8. README with install steps.

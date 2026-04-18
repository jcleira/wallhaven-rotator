# wallhaven-rotator

Daily wallpaper rotator for Hyprland backed by [Wallhaven](https://wallhaven.cc).
Rotates once per day from the hot feed (filtered by your config), lets you
favorite keepers into a saved collection, and swaps to a *favorites-only*
rotation mode when you're happy with what you have. Actions exposed as
Walker commands.

See [`DESIGN.md`](./DESIGN.md) for the architecture.

## Install

Requires Go 1.21+ and [`awww`](https://github.com/jcleira/awww) (or `swww` /
`hyprpaper`) running in your session.

```bash
go install github.com/jcleira/wallhaven-rotator/cmd/wallhaven-rotator@latest
```

On first run, a default config is written to
`$XDG_CONFIG_HOME/wallhaven-rotator/config.toml`.

## Commands

```
wallhaven-rotator next                 # fetch + apply a fresh wallpaper
wallhaven-rotator favorite             # save current to favorites/
wallhaven-rotator unfavorite           # remove current from favorites/
wallhaven-rotator mode <daily|favorites>
wallhaven-rotator info                 # JSON metadata for current
wallhaven-rotator open                 # open source page on wallhaven.cc
wallhaven-rotator status               # mode, last rotation, counts
wallhaven-rotator list-favorites       # list saved favorites
wallhaven-rotator daemon               # run from Hypr autostart; rotates once/day
wallhaven-rotator config-path          # print the config dir
```

## Config

All fields are optional — omit or leave empty to accept defaults.

```toml
[filter]
query      = ""                   # Wallhaven ?q=; empty = no text filter
categories = ["general"]          # general | anime | people
purity     = ["sfw"]              # v1 is SFW-locked
sorting    = "hot"                # hot | toplist | views | favorites | random
min_width  = 3840
min_height = 1600
ratios     = ["21x9", "32x9"]

[rotation]
mode                 = "daily"    # or "favorites"
delete_non_favorites = true
trigger              = "first-login"

[apply]
command             = "awww"      # awww | swww | hyprpaper
transition_type     = "grow"
transition_duration = "0.8"
transition_fps      = "60"
```

A symlink `~/.config/wallhaven-rotator/current.jpg` always points at the
displayed wallpaper. Point `hyprlock`'s background at that symlink and your
lockscreen will track your desktop automatically.

## Hyprland autostart

```
# ~/.config/hypr/conf/autostart.conf
exec-once = wallhaven-rotator daemon
```

The daemon is a silent no-op if already rotated today, so it's safe on every
login.

## Walker integration

Ship [`walker/wallpaper-menu.sh`](./walker/wallpaper-menu.sh) on your PATH and
bind it to a Hyprland keybind:

```
# ~/.config/hypr/conf/keybinds.conf
bind = SUPER, W, exec, wallpaper-menu.sh
bind = SUPER SHIFT, W, exec, wallhaven-rotator next
bind = SUPER CTRL, W, exec, wallhaven-rotator favorite
```

The script pipes an action list into `walker --dmenu` and dispatches to
`wallhaven-rotator`. Notifications (via `notify-send` / `mako`) surface
results.

## License

MIT — see [LICENSE](./LICENSE).

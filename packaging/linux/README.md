# Linux system packaging

Install these files into the package staging root (paths shown for a
`/usr` prefix):

| Source | Installed path |
| --- | --- |
| Built `qbtremotego` executable | `/usr/bin/qbtremotego` |
| `packaging/linux/qbtremotego.desktop` | `/usr/share/applications/qbtremotego.desktop` |
| `internal/resources/assets/app_light.svg` | `/usr/share/icons/hicolor/scalable/apps/qbtremotego.svg` |

Use mode 0755 for the executable and 0644 for the desktop entry and SVG.
Reuse the source SVG directly; no separate copy of the artwork is maintained
here. Follow the distribution's usual desktop database and icon cache hooks.
The launcher expects the executable to be on PATH. These are source-tree
packaging inputs; the existing GoReleaser binary archives do not include them.

The visible launcher does not advertise MIME types. User-selected magnet and
torrent associations are registered at runtime through the hidden
`$XDG_DATA_HOME/applications/qbtremotego-handler.desktop` entry and
`xdg-mime`. If XDG_DATA_HOME is unset, the app uses `~/.local/share`.
Package installation must not register defaults or modify users' desktop
entries. Autostart remains a separate user entry at
`$XDG_CONFIG_HOME/autostart/qbtremotego.desktop` (normally
`~/.config/autostart/qbtremotego.desktop`).

## Migration from the old runtime handler

Previous versions wrote the hidden handler as
`$XDG_DATA_HOME/applications/qbtremotego.desktop`, which can shadow the
system launcher. On desktop integration sync, the app removes that file only
if it is a regular file matching the complete old generated format: the exact
fields and ordering, a quoted absolute executable path, and one of the three
supported nonempty MIME lists. The executable may be at an older location.
Cleanup also runs when both handlers are disabled.

Edited entries, symlinks, and other unrecognized files are preserved and produce
a warning with their path for manual review. They may continue to shadow the
system launcher. Do not remove them automatically in package scripts.

Cleanup requires running the updated app; an old hidden entry may prevent the
launcher from appearing before that first run. Users can run `qbtremotego`
from a terminal to trigger integration sync. Enabled associations are then
registered under the new handler name. Registration tools remain best-effort:
if `xdg-mime` is unavailable or fails, existing defaults may still reference
the old name. Disabled associations are not rewritten, and the migration does
not broadly edit users' `mimeapps.list` files.

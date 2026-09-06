# Arch Linux / AUR packaging

This directory contains the packaging inputs for the `qbtremotego` AUR
package. Nothing here is a directly buildable `PKGBUILD`: the package on the
AUR is rendered from the `PKGBUILD.in` template for each stable `X.Y.Z`
release tag by the Woodpecker workflow in `.woodpecker/aur.yaml`, and the
rendered result lives at `https://aur.archlinux.org/packages/qbtremotego`.

Files in this directory:

| File | Role |
| --- | --- |
| `PKGBUILD.in` | `PKGBUILD` template with `@PKGVER@`, `@BUILDDATE@` and `@SHA256SUM@` placeholders |
| `LICENSE` | Arch's canonical 0BSD text, verbatim (required by `pkgctl license check`) |
| `LICENSES/0BSD.txt` | Symlink to `LICENSE` (the layout `pkgctl license` expects) |
| `REUSE.toml` | REUSE declarations for the packaging sources |
| `README.md` | This document |

The upstream program this package builds is MIT-licensed (see the repository
root `LICENSE`); the 0BSD files cover only the packaging sources themselves.

## Canonical inputs

- **Source archive**: `https://git.skobk.in/api/v1/repos/skobkin/qBtRemoteGo/archive/<tag>.tar`
  (Forgejo API route). Web routes and release-asset downloads of
  `git.skobk.in` are gated for anonymous clients, but the API route is not.
  The plain `.tar` is byte-deterministic across requests — verified by
  downloading it repeatedly and comparing sha256 — while the `.tar.gz`
  variant is not (its gzip layer varies), so the package pins the `.tar`
  with a real `sha256sums` entry.
- The Forgejo archive extracts to a single non-versioned top-level directory
  `qbtremotego/`. To prevent stale files from surviving between successive
  package versions, the template marks the tarball `noextract` and
  `prepare()` extracts it explicitly with `bsdtar --strip-components=1`
  into a fresh `$srcdir/qbtremotego-$pkgver` directory. All build functions
  operate on that versioned directory.
- A GitHub mirror exists, but its archive root is `qBtRemoteGo-<tag>/` and it
  is not the canonical origin; it is documented here only as a fallback
  (harmless under `--strip-components=1`, but the URL and checksum should
  always come from the Forgejo API route).
- **Desktop file** `packaging/linux/qbtremotego.desktop` →
  `/usr/share/applications/qbtremotego.desktop` and **icon**
  `internal/resources/assets/app_light.svg` →
  `/usr/share/icons/hicolor/scalable/apps/qbtremotego.svg` are installed
  0644; the binary goes to `/usr/bin/qbtremotego` 0755 and the upstream MIT
  license text to `/usr/share/licenses/qbtremotego/LICENSE`. See
  `packaging/linux/README.md` for the desktop-integration rationale: the
  package must not register MIME defaults or touch user desktop entries, so
  there is no `.install` script and no `MimeType=` in the launcher.

## Rendered fields

| Placeholder | Rendered from | Notes |
| --- | --- | --- |
| `@PKGVER@` | `CI_COMMIT_TAG` | Only plain `X.Y.Z` tags pass the workflow's gate |
| `@SHA256SUM@` | sha256 of the downloaded source `.tar` | Real checksum, verified end-to-end by `makepkg --verifysource` |
| `@BUILDDATE@` | `date -u -d "@$CI_COMMIT_TIMESTAMP" +%Y-%m-%dT%H:%M:%SZ` | RFC3339 UTC of the tagged commit. `CI_COMMIT_TIMESTAMP` is stable across workflow re-runs, which a release `created_at` is not guaranteed to be. |

`pkgrel` is pinned to `1` in the template. Any manual AUR-side `pkgrel`
bump gets reverted by the automation on the next release — mirror such
changes into `PKGBUILD.in` instead.

## Dependency determination record

Derived empirically in a clean Arch chroot (`pkgctl build -c`, so only
declared dependencies are ever present), iterated from
`makedepends=('go')` with `depends=()` until the build converged. Each
iteration failed on exactly one missing piece:

1. `X11/Xlib.h` → `libx11`
2. `X11/extensions/Xrandr.h` → `libxrandr`
3. `X11/Xcursor/Xcursor.h` → `libxcursor`
4. `X11/extensions/Xinerama.h` → `libxinerama`
5. `X11/extensions/XInput2.h` → `libxi`
6. `GL/glx.h` → `libglvnd` (on current Arch, `libglvnd` ships the GL
   headers and `gl.pc` — `mesa` is not needed at all)
7. build + headless `go test ./...` succeeded with only these declared

Post-build analysis of the resulting binary:

- `ldd` resolves `libGL.so.1` / `libGLdispatch.so.0` / `libGLX.so.0`
  (`libglvnd`), `libX11.so.6` (`libx11`), plus `libxcb`/`libXau`/`libXdmcp`,
  which are transitive through `libx11` and therefore not listed
  (`glibc`/`gcc-libs` are base and never listed).
- The embedded GLFW (go-gl/glfw v3.4 X11 backend) **dlopen**s its X11
  extension libraries at runtime — `strings` on the binary shows
  `libXcursor.so.1`, `libXi.so.6`, `libXinerama.so.1`, `libXrandr.so.2`
  (and non-critical `libXxf86vm.so.1`, `libXrender.so.1`), invisible to
  `ldd` and namcap. They are declared in `depends` because cursor themes,
  monitor enumeration and input handling degrade without them. `libXxf86vm`
  is intentionally not declared: it is only consulted for gamma adjustment
  and is designed to be optional.

Final arrays:

```sh
depends=(
	'libx11' 'libxcursor' 'libxrandr' 'libxinerama' 'libxi' 'libglvnd'
	'hicolor-icon-theme'   # icon hierarchy for /usr/share/icons/hicolor
)
makedepends=('go')   # X11/GL packages are already in depends and ship the headers
optdepends=(
	'xdg-utils: registering magnet and torrent file associations at runtime'
	'desktop-file-utils: refreshing the desktop database for the runtime handler'
	'org.freedesktop.secrets: storing connection credentials in the system keyring'
)
```

`optdepends` rationale (from the source audit in
`internal/platform/integration_linux.go`): the app executes `xdg-mime`
(`xdg-utils`) and `update-desktop-database` (`desktop-file-utils`) at
runtime but treats both as best-effort — failures are logged and ignored.
Credential storage uses `zalando/go-keyring` over the D-Bus Secret Service
API (pure Go, no ELF linkage), expressed as the `org.freedesktop.secrets`
virtual provider so any compatible keyring satisfies it. The app functions
without all three, hence optdepends rather than depends.

Re-run the derivation when the Fyne/glfw stack changes: render a minimal
`PKGBUILD` (`depends=()`, `makedepends=('go')`), `pkgctl build -c`, add the
mapped missing piece, repeat, then re-check `ldd` + `strings <binary> |
grep '^libX'` against the arrays above.

## Packaging-source licensing

The AUR submission guidelines license package sources 0BSD; this matches the
current Arch scheme (`pkgctl license`): a root `LICENSE` carrying the
canonical Arch 0BSD text byte-for-byte (the tool compares it against its
embedded copy, so the copyright holder is not edited there — the packaging
files' copyright is declared in `REUSE.toml` instead), `LICENSES/0BSD.txt`
as a symlink to it, and `REUSE.toml` declaring
`SPDX-License-Identifier = "0BSD"` for the packaging files. `REUSE.toml`
also covers the AUR-side names (`PKGBUILD`, `.SRCINFO`) — paths that do not
exist in a given layout are ignored per the REUSE specification, so the
same file is valid in this directory and in the published AUR repository.

Validate with:

```sh
pkgctl license check   # in a directory holding PKGBUILD + LICENSE + LICENSES/ + REUSE.toml
```

## Local validation

The template can be exercised locally without touching the AUR. The fake
version `0.0.0` and the rendered artifacts are throwaway — never committed.

```sh
WORK=/tmp/qbtremotego-pkgtest
rm -rf "$WORK" && mkdir -p "$WORK"
git -C /path/to/qBtRemoteGo archive --format=tar --prefix=qbtremotego/ -o "$WORK/qbtremotego-0.0.0.tar" HEAD
cd "$WORK"
SHA=$(sha256sum qbtremotego-0.0.0.tar | awk '{print $1}')
sed -e 's|@PKGVER@|0.0.0|g' -e "s|@SHA256SUM@|$SHA|g" \
    -e 's|@BUILDDATE@|1970-01-01T00:00:00Z|g' \
    /path/to/qBtRemoteGo/packaging/arch/PKGBUILD.in > PKGBUILD
bash -n PKGBUILD
# The pre-seeded tar matches the source alias filename, so makepkg skips the
# network download and verifies the local file against the rendered checksum.
makepkg --printsrcinfo > .SRCINFO
makepkg --verifysource
pkgctl build -c     # authoritative: clean chroot, declared deps only
namcap PKGBUILD
namcap qbtremotego-0.0.0-1-x86_64.pkg.tar.zst
bsdtar -tvf qbtremotego-0.0.0-1-x86_64.pkg.tar.zst usr/bin usr/share
```

`pkgctl build` requires `devtools` and rebuilds the chroot copy with `-c`.
A plain `makepkg -sf` on a developer workstation is NOT sufficient for
dependency work: the host typically has the X11/GL stack installed already,
which hides undeclared dependencies.

## AUR publication flow

`.woodpecker/aur.yaml` publishes the package automatically:

- Runs only on `tag` events, only after the `ci` workflow has passed for the
  same commit (`depends_on: [ci]`), and its first step refuses any tag that
  is not a plain `X.Y.Z` release.
- Downloads the source archive for the tag from the canonical URL, renders
  `PKGBUILD.in` (`@PKGVER@` ← tag, `@SHA256SUM@` ← real checksum,
  `@BUILDDATE@` ← `CI_COMMIT_TIMESTAMP` as RFC3339 UTC), generates
  `.SRCINFO` and verifies the source checksum end-to-end as an unprivileged
  build user.
- Pushes `PKGBUILD`, `.SRCINFO`, `LICENSE`, `REUSE.toml` and the
  `LICENSES/0BSD.txt` symlink to `ssh://aur@aur.archlinux.org/qbtremotego.git`
  (`master` only). If the rendered output already matches the AUR, nothing
  is committed.
- Commits are deterministic: message `qbtremotego <tag>-<pkgrel>`, author
  hardcoded in the workflow, author/committer dates derived from
  `CI_COMMIT_TIMESTAMP`. The AUR host key is pinned by fingerprint — SSH
  host-key verification is never disabled, and the private key is only ever
  passed through a Woodpecker secret.

### Manual one-time setup

1. Generate an Ed25519 keypair dedicated to this automation and register the
   public key with the AUR account that should own the package
   (AUR → My Account → SSH keys).
2. Store the private key as a Woodpecker secret named `AUR_SSH_PRIVATE_KEY`
   on this repository, enabled for tag events. Do not reuse it elsewhere and
   never print it in logs.
3. Confirm the `qbtremotego` name is still free on the AUR (it was not taken
   at the time of writing); the first push of a suitable tag creates the
   package base.
4. Cut the first stable tag only **after** these commits are on `master` —
   Woodpecker reads workflow files from the tagged commit, so a tag on an
   earlier commit would not trigger the workflow.

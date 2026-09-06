# Arch Linux / AUR packaging

Packaging inputs for the `qbtremotego` AUR package. Nothing here is a
directly buildable `PKGBUILD`: the package on the AUR is rendered from
`PKGBUILD.in` for each stable `X.Y.Z` release tag by the Woodpecker workflow
`.woodpecker/aur.yaml` and lives at
https://aur.archlinux.org/packages/qbtremotego.

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

- **Source archive**:
  `https://git.skobk.in/api/v1/repos/skobkin/qBtRemoteGo/archive/<tag>.tar`
  (Forgejo API route). Web routes of `git.skobk.in` are gated for anonymous
  clients, but the API route is not. The plain `.tar` is byte-deterministic
  across requests (verified by repeated downloads), while the `.tar.gz`
  variant is not — its gzip layer varies — so the checksum pin uses `.tar`.
- The archive extracts to a single non-versioned top-level directory
  `qbtremotego/`. The template therefore marks it `noextract`, and
  `prepare()` extracts it explicitly with `bsdtar --strip-components=1`
  into a fresh `$srcdir/qbtremotego-$pkgver` so stale files cannot survive
  between package versions.
- A GitHub mirror exists (`archive/refs/tags/<tag>.tar.gz`, archive root
  `qBtRemoteGo-<tag>/`); it is documented as a fallback only — URL and
  checksum always come from the canonical route above.
- Install paths are in `package()`. The package deliberately registers no
  MIME defaults and ships no `.install` script; see
  `packaging/linux/README.md` for the desktop-integration rationale.

## Rendered fields

| Placeholder | Rendered from | Notes |
| --- | --- | --- |
| `@PKGVER@` | `CI_COMMIT_TAG` | Only plain `X.Y.Z` tags pass the workflow's gate |
| `@SHA256SUM@` | sha256 of the downloaded source `.tar` | Verified end-to-end by `makepkg --verifysource` |
| `@BUILDDATE@` | `CI_COMMIT_TIMESTAMP` as RFC3339 UTC | Stable across workflow re-runs, unlike a release `created_at` |

`pkgrel` is pinned to `1`. A manual AUR-side bump gets reverted by the
automation on the next release — mirror such changes into `PKGBUILD.in`.

## Dependencies

Derived empirically in a clean Arch chroot (`pkgctl build -c`, so only
declared dependencies are ever present): starting from `depends=()` and
`makedepends=('go')`, one package was added per failed build — the X11 and
X11-extension headers (`libx11`, `libxrandr`, `libxcursor`, `libxinerama`,
`libxi`), then GL headers, which current Arch ships in `libglvnd` (so
`mesa` is not needed at all).

The embedded GLFW (go-gl/glfw v3.4 X11 backend) also **dlopen**s its X11
extension libraries at runtime — `libXcursor.so.1`, `libXi.so.6`,
`libXinerama.so.1`, `libXrandr.so.2` — which is invisible to `ldd` and
namcap (`strings <binary>` shows them). They are declared because cursor
themes, monitor enumeration and input handling degrade without them.
`libXxf86vm` is intentionally not declared: it is only consulted for gamma
adjustment and is designed to be optional.

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

`optdepends` rationale (source audit in
`internal/platform/integration_linux.go`): the app runs `xdg-mime`
(`xdg-utils`) and `update-desktop-database` (`desktop-file-utils`)
best-effort, logging and ignoring failures, and stores credentials over the
D-Bus Secret Service API (pure Go, no ELF linkage) — expressed as the
`org.freedesktop.secrets` virtual provider so any compatible keyring
satisfies it. The app functions without all three.

Re-derive when the Fyne/glfw stack changes: render a minimal `PKGBUILD`
(`depends=()`, `makedepends=('go')`), `pkgctl build -c`, add the mapped
missing package, repeat, then re-check `strings <binary> | grep '^libX'`.

## Packaging-source licensing

Arch's current scheme (`pkgctl license`): a root `LICENSE` carrying the
canonical Arch 0BSD text byte-for-byte (the tool compares against its
embedded copy — the packaging files' copyright is declared in `REUSE.toml`
instead), `LICENSES/0BSD.txt` as a symlink to it, and `REUSE.toml` declaring
`SPDX-License-Identifier = "0BSD"`. The annotations also cover the AUR-side
names (`PKGBUILD`, `.SRCINFO`); paths missing from a given layout are
ignored per the REUSE specification, so one file serves both. Validate in a
directory holding `PKGBUILD` + `LICENSE` + `LICENSES/` + `REUSE.toml`:

```sh
pkgctl license check
```

## Local validation

The fake version `0.0.0` and rendered artifacts are throwaway — never
committed.

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
# The pre-seeded tar matches the source alias filename, so makepkg verifies
# the local file against the rendered checksum without downloading.
makepkg --printsrcinfo > .SRCINFO
makepkg --verifysource
pkgctl build -c     # authoritative: clean chroot, declared deps only
namcap PKGBUILD qbtremotego-0.0.0-1-x86_64.pkg.tar.zst
bsdtar -tvf qbtremotego-0.0.0-1-x86_64.pkg.tar.zst usr/bin usr/share
```

A plain `makepkg -sf` on a workstation is **not** sufficient for dependency
work: the host typically has the X11/GL stack installed already, which hides
undeclared dependencies.

## AUR publication flow

`.woodpecker/aur.yaml` runs on `tag` events only, after `ci` passed for the
same commit (`depends_on: [ci]`), and refuses tags that are not plain
`X.Y.Z`. It downloads the source archive for the tag, renders the template
(tag → `@PKGVER@`, real checksum → `@SHA256SUM@`, `CI_COMMIT_TIMESTAMP` →
`@BUILDDATE@`), generates `.SRCINFO` and verifies the checksum end-to-end as
an unprivileged build user, then pushes `PKGBUILD`, `.SRCINFO`, `LICENSE`,
`REUSE.toml` and the `LICENSES/0BSD.txt` symlink to
`ssh://aur@aur.archlinux.org/qbtremotego.git` (`master` only). If the
rendered output already matches the AUR, nothing is committed.

Commits are deterministic: message `qbtremotego <tag>-<pkgrel>`, author
hardcoded in the workflow, author/committer dates derived from
`CI_COMMIT_TIMESTAMP`. The AUR host key is pinned by fingerprint — SSH
host-key verification is never disabled — and the private key only ever
passes through a Woodpecker secret.

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

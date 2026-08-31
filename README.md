# qBtRemoteGo [![build status](https://ci.skobk.in/api/badges/skobkin/qBtRemoteGo/status.svg?events=push%2Ctag)](https://ci.skobk.in/repos/skobkin/qBtRemoteGo)[![latest release](https://git.skobk.in/skobkin/qBtRemoteGo/badges/release.svg)](https://git.skobk.in/skobkin/qBtRemoteGo/releases)

qBtRemoteGo is a native desktop client for remotely controlling qBittorrent through its Web API.

It focuses on a fast add-and-monitor workflow for magnets and `.torrent` files, with a compact GUI and desktop integration.

## Screenshots

![main window](docs/screenshot_main_window.webp)

<details>
<summary>Add torrent</summary>

![adding torrents](docs/screenshot_add_window.webp)

</details>

## Installation

[![Download latest release](docs/button_download.svg)](https://git.skobk.in/skobkin/qBtRemoteGo/releases)

Check the [Releases](https://git.skobk.in/skobkin/qBtRemoteGo/releases) section for the latest downloads.

> [!NOTE]
> Releases are hosted on my private [Forgejo](https://en.wikipedia.org/wiki/Forgejo) instance.

## Connection & authentication

qBtRemoteGo supports two authentication methods (selected in *Settings → Connection → Authentication*):

- **Username & password** — the classic WebUI login; works with any qBittorrent version.
- **API key** — stateless authentication for qBittorrent **v5.2.0 or newer**. Generate a key in
  qBittorrent (*Preferences → WebUI → API Key*) and paste it into the app. The key is sent as an
  `Authorization: Bearer` header, so the client never performs a login. Note that API-key
  authentication therefore cannot normally coexist with reverse-proxy HTTP Basic Auth, which uses
  the same header.

Keep in mind:

- Rotating the key in qBittorrent invalidates the previous key immediately; paste the new key into the app.
- Switching back to password auth does not revoke the server-side key — delete or regenerate it in
  qBittorrent if it is no longer needed.
- Only the credentials for the active method are kept: switching methods means re-entering them.

Credential storage modes (applies to passwords and API keys alike):

- **System keychain** (default) — credentials are stored in the OS keyring, never on disk.
- **Plain text config file** — credentials are stored unencrypted in `config.json`.
- **Session only** — credentials are kept in memory for the current run only and are not restored on the next launch.


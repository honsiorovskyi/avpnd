# avpnd

A tiny local HTTP wrapper around the AWS VPN Client CLI (`aws-vpn-client`), so a browser extension can connect, disconnect and show the status of one VPN profile. It adds no logic of its own: each endpoint runs the matching CLI command. The AWS client does everything else (SAML login in the browser, the VPN itself).

`avpnd` runs as your normal user. No root is needed.

## Build and run

```bash
make build
./avpnd --list-profiles          # profiles known to the AWS client
./avpnd -profile AWS_VPN_EU      # serve on 127.0.0.1:35002
```

The binary is named `avpnd`, not `vpnd`, because macOS ships its own `/usr/sbin/vpnd`.

Flags:

- `-profile <name>`: profile to manage (required unless `--list-profiles`)
- `--list-profiles`: print `aws-vpn-client list-profiles` and exit
- `-listen <addr>`: default `127.0.0.1:35002`. **Port 35001 is refused**: the AWS client's daemon binds it for its SAML callback, and if something else holds it, `connect` fails with "The authentication flow could not be initiated. A required network resource is unavailable."
- `-bin <path>`: CLI path, default `/Applications/AWS VPN Client/AWS VPN Client.app/Contents/MacOS/aws-vpn-client`
- `-v`: also log the CLI output of status polls (connect/disconnect output is always logged)

## HTTP API

GET or POST (OPTIONS preflight is answered; other methods get 405). CORS echoes the request `Origin`.

| Endpoint | Runs | Response |
|---|---|---|
| `/connect` | `connect --profile-name P` | `204`. The CLI returns in about 1s with the profile `WaitingForIdentity` and opens the SAML login in the browser; the AWS daemon finishes connecting in the background (typically a few seconds). |
| `/disconnect` | `disconnect --profile-name P` | `204` (about 1s). |
| `/status` | `get-connection-status --profile-name P` | `200` `{"status":"connected","raw":"Connected"}` |

If the CLI fails (for example "Already connected to profile"), the response is `502` with `{"error": "<the CLI's message>"}`.

`status` is one of `connected`, `connecting`, `disconnecting`, `disconnected`. `raw` is the CLI's own state, which maps like this:

| `raw` | `status` |
|---|---|
| `Connected` | `connected` |
| `Disconnecting` | `disconnecting` |
| `NotConnected`, `Disconnected` | `disconnected` |
| `Connecting`, `WaitingForIdentity`, `Reconnecting`, anything unknown | `connecting` |

Every CLI call writes several lines to the AWS client's own log (`~/.config/AWSVPNClient/logs`), which rotates at 1MB, so `/status` is cached for 1s and concurrent calls share one CLI run. Connect and disconnect clear the cache.

Stopping `avpnd` (SIGINT/SIGTERM) does not touch the VPN.

## Browser extension

`extension/` is a toolbar button for Firefox and Chrome (adapted from the `awsvpn` example). The badge shows `on` / `off` / `...` / `err`, and clicking it connects or disconnects through `avpnd` on `127.0.0.1:35002`. It polls every 1s while connecting or disconnecting and every 3s otherwise. It also closes the tab the SAML flow ends on (`http://127.0.0.1:35001/`).

One script (`extension/src/background-script.js`) serves both browsers; they differ only in manifest. Firefox gets Manifest V2 (`manifest.firefox.json`) and Chrome gets Manifest V3 (`manifest.chrome.json`), because Chrome no longer runs MV2 extensions.

```bash
make extensions                  # build/extensions/avpnd-{firefox,chrome}-<version>.zip, plus unzipped build/extensions/{firefox,chrome}/
make extensions VERSION=1.2.3    # set the version
```

Load an unzipped build for development:

- Firefox: `about:debugging` → This Firefox → Load Temporary Add-on → `build/extensions/firefox/manifest.json`
- Chrome: `chrome://extensions` → Developer mode → Load unpacked → `build/extensions/chrome`

If you change `-listen`, update `AVPND` in `extension/src/background-script.js` and the host permission in both manifests.

## Versioning

Versions are [semantic](https://semver.org) git tags (`vMAJOR.MINOR.PATCH`); the tag is the only place a version is written down. GoReleaser stamps it into the binary and into both extension manifests.

```bash
make version                  # current version, from git describe
make bump                     # tag the next patch version (v0.1.0 -> v0.1.1)
make bump BUMP=minor          # v0.1.1 -> v0.2.0   (BUMP=major for X.0.0)
git push origin v0.2.0        # the tag push publishes a release
./avpnd --version
```

The usual way to ship is one command:

```bash
make release                  # if main has commits since the last tag: vet + test, bump patch, push main and the tag
make release BUMP=minor       # same, with a minor (or major) bump
make release DRY_RUN=1        # only show what would be released
```

It does nothing when there are no new commits, and refuses to run off `main`, with a dirty tree, without an `origin` remote, or when `main` is behind `origin/main`. Commits and tag go in one atomic push; if it fails the local tag is removed.

`make bump` only creates the tag locally, and refuses on a dirty tree or when HEAD is already tagged. Pre-release tags (`v1.0.0-rc1`) are ignored when computing the next version.

## CI and releases

- `.github/workflows/ci.yml` runs on pushes to `main` and on PRs: `gofmt`, `go vet`, `go test -race`, build, and the extension zips (uploaded as a workflow artifact).
- `.github/workflows/release.yml` runs on tags `v*`. Push a version tag (see Versioning) to publish a release.

  GoReleaser (`.goreleaser.yaml`) builds `avpnd` for macOS (arm64 and x86_64), archives it, and attaches `avpnd-firefox-<v>.zip`, `avpnd-chrome-<v>.zip` and `checksums.txt` to the GitHub release.
- `make snapshot` is a local dry run (needs `goreleaser` and a git remote).

The extension zips are unsigned. Firefox release builds only install signed add-ons (submit the zip to addons.mozilla.org, or load it temporarily as above), and Chrome users load the unzipped folder via "Load unpacked" unless it is published on the Chrome Web Store.

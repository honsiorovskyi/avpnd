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

`extension/` is a Firefox/Chromium toolbar button (adapted from the `awsvpn` example). The badge shows `on` / `off` / `...` / `err`, and clicking it connects or disconnects through `avpnd` on `127.0.0.1:35002`. It polls every 1s while connecting or disconnecting and every 3s otherwise. It also closes the tab the SAML flow ends on (`http://127.0.0.1:35001/`).

- Firefox: `about:debugging` → This Firefox → Load Temporary Add-on → `extension/manifest.json`
- Chromium: `chrome://extensions` → Developer mode → Load unpacked → `extension/`

If you change `-listen`, update `AVPND` in `extension/src/background-script.js` and the host permission in `extension/manifest.json`.

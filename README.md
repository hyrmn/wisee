# Wi-See

*See your Wi-Fi.* Wi-See is a small Go web app that draws the devices on your
home network as a live map of bubbles. Devices that are online right now
**pulse**; devices seen in earlier scans stay on the map **dimmed and dashed**,
with a note of when they were last seen. Each service orbiting a device gets
its own bubble and icon.

> Wi-See was heavily inspired by
> [hearthlight](https://github.com/jmkelly/hearthlight) — if you like this,
> you'll like that.

![Go](https://img.shields.io/badge/Go-%3E%3D1.24-00ADD8?logo=go&logoColor=white)
[![License: Unlicense](https://img.shields.io/badge/license-Unlicense-blue.svg)](https://unlicense.org/)

## What you get

- **One static Go binary** — UI, styles, scripts and HTMX are embedded. SQLite
  runs in-process (pure Go, no CGo), so the binary is fully self-contained.
- **Unprivileged discovery** — ARP-cache wake-up, a TCP-RST liveness sweep and
  a curated port probe. No root, no raw sockets.
- **A map you can arrange** — drag bubbles where you want them; positions are
  persisted in SQLite. Pan and zoom freely.
- **Service bubbles** — HTTP/HTTPS (with page-title and TLS-certificate
  fingerprinting), SSH banners, SMB, printers, Plex/Jellyfin, Chromecast,
  Home Assistant, MQTT and more, each with a distinct icon.
- **Device kinds** — routers, servers, computers, phones, printers, NAS boxes,
  media players and IoT gear, classified from vendors (curated MAC OUI table),
  hostnames and observed services.
- **HTMX-driven UI** — the map layer, stats and detail panel update with
  server-rendered fragments; no frontend build step, works without internet
  access (HTMX is vendored).
- **History forever** — devices are never deleted, they just fade.

## Quick start

Requirements: Go 1.24 or newer. Real scanning works on Linux, macOS and
Windows without elevated privileges.

```sh
git clone <your-remote-url> wisee
cd wisee
make demo      # sample network, no scanning — good for a first look
# Open http://127.0.0.1:8090
```

To scan your real LAN:

```sh
make run
# Open http://127.0.0.1:8090
```

To reach it from other machines on your network:

```sh
./bin/wisee -listen 0.0.0.0:8090
```

A scan of a `/24` normally finishes in a few seconds; the map refreshes itself
every 5 seconds. The **Scan now** button forces a rescan.

## Flags

| Flag           | Default            | Meaning                                          |
| -------------- | ------------------ | ------------------------------------------------ |
| `-listen`      | `127.0.0.1:8090`   | HTTP listen address.                             |
| `-db`          | `wisee.db`         | SQLite database path.                            |
| `-interval`    | `60s`              | Time between scans. Minimum 5s.                  |
| `-cidr`        | auto               | Comma-separated IPv4 CIDRs to scan.              |
| `-ports`       | built-in list      | Extra TCP ports, e.g. `8000,9000-9010`.          |
| `-timeout`     | `350ms`            | Per-connection timeout.                          |
| `-concurrency` | `16`               | Hosts probed in parallel.                        |
| `-demo`        | `false`            | Serve sample data without scanning.              |
| `-version`     |                    | Print the version and exit.                      |

`WISEE_LISTEN` and `WISEE_DB` override `-listen` and `-db` defaults.

## How discovery works

One scan does the following:

1. Enumerates the IPv4 LAN interfaces (virtual ones like Docker bridges and
   VPN tunnels are skipped). Networks larger than a `/24` are capped to the
   local `/24`; `-cidr` overrides auto-detection.
2. Sends one UDP packet to each address to wake the kernel's ARP cache.
3. Connects to a usually-closed TCP port on every address. A fast
   "connection refused" (RST) proves the host is alive even when it exposes
   no services.
4. Reads the native ARP table for MAC addresses and extra candidates:
   `/proc/net/arp` on Linux, `arp -an` on macOS, `arp -a` on Windows.
5. Probes a curated list of ~40 common TCP ports on each candidate and
   fingerprints open web ports (HTTP page title, TLS certificate name) and
   SSH (banner).
6. Resolves reverse-DNS names, maps known MAC prefixes to vendors and
   classifies the device kind.
7. Merges everything into SQLite. Devices and services not seen in this scan
   flip to "offline" but stay on the map with their last-seen time.

## Security

Wi-See defaults to local-only (`127.0.0.1:8090`) and has **no
authentication**. The inventory exposes hostnames, IP and MAC addresses and
open ports. Only bind to `0.0.0.0` on networks you trust, or put a reverse
proxy with authentication in front of it. Do not expose it to the internet.

Active probing can trigger intrusion-detection logging on managed networks.
Only scan networks you own or administer.

## Development

```sh
make demo    # sample-data UI (no scanning)
make test    # unit tests
make check   # formatting, vet and tests (what CI runs)
make build   # bin/wisee
```

Layout:

- `internal/discovery` — interface enumeration, liveness sweep, ARP parsing,
  port probes, fingerprinting, classification.
- `internal/store` — SQLite inventory: merge scans, derive online/offline,
  persist bubble positions.
- `internal/web` — HTTP handlers, HTML templates, icons, embedded static
  assets (CSS/JS/vendored HTMX).
- `internal/demo` — sample network for UI work.

## Releases

CI runs `make check` and `make build` on every push and pull request. Tagged
commits (`v*`) get a GitHub release with prebuilt Linux (amd64/arm64), macOS
(amd64/arm64) and Windows (amd64) archives plus checksums:

```sh
git tag v0.1.0
git push origin v0.1.0
```

## Acknowledgements

Wi-See was heavily inspired by
[hearthlight](https://github.com/jmkelly/hearthlight) by
[@jmkelly](https://github.com/jmkelly) — the live network map, the
pulse-for-the-living / fade-for-the-absent device treatment, and the
unprivileged discovery approach all trace back to it. Many thanks to its
author.

## License

[The Unlicense](LICENSE) — public domain. Do whatever you like.

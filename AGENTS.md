# AGENTS.md

Guidance for agents working on **Wi-See** — a small Go web app that draws the
home network as a live map of bubbles. Online devices pulse, previously-seen
devices fade with a last-seen note, services orbit their device with distinct
icons.

## Tech stack

- **Go 1.27+** (`go 1.27.3` in go.mod). Module path: `wisee`.
- **SQLite** via `modernc.org/sqlite` (pure Go, **no CGo**). Do not switch to
  `mattn/go-sqlite3`: the release pipeline cross-compiles with
  `CGO_ENABLED=0`, and the single static binary is a core feature.
- **HTMX 2.0.4**, vendored at `internal/web/static/htmx.min.js`. No CDN, no
  npm, no frontend build step. The app must keep working fully offline.
- `html/template` for all rendering; Go 1.22+ method+wildcard `ServeMux`
  routes. No external web framework, no router dependency.

## Layout

```
main.go                     flags, scan loop, graceful shutdown
internal/discovery/         unprivileged LAN discovery (no root)
  discovery.go              scan orchestration
  net.go                    interface enum, ARP wake, TCP-RST liveness sweep
  arp.go                    /proc/net/arp + `arp -an`/`arp -a` parsing
  ports.go                  port probe + fingerprint (HTTP title / TLS CN / SSH banner)
  classify.go               curated OUI vendor table + kind classification
internal/store/store.go     SQLite inventory; devices are never deleted
internal/web/
  server.go                 handlers + view models + layout math
  icons.go                  inline SVG path maps (device kinds + services)
  templates/                index / devices fragment / detail fragment
  static/                   app.css, app.js, vendored htmx.min.js
internal/demo/demo.go       sample network for `-demo`
Makefile                    build / run / demo / test / check / dist
.github/workflows/          ci.yml (make check+build), release.yml (tag v*)
VERSION                     version string, injected via -ldflags main.version
```

## Build / test / run

- `make check` — gofmt -s -l, go vet, go test ./... (what CI runs; keep green)
- `make test` — unit tests
- `make build` — `bin/wisee`
- `make demo` — sample network UI, no scanning
- `make run` — real scan
- `make dist GOOS=... GOARCH=...` — cross-compile one release binary

Tests use in-memory SQLite (`store.Open(":memory:")`). Run `gofmt -s -w .`
before finishing; `make check` fails on unformatted files.

## Non-obvious conventions (read before editing)

- **Map is HTML divs, not SVG**, on purpose: HTMX innerHTML fragment swaps of
  raw SVG elements hit namespace parsing issues. Device bubbles, service
  orbits, and edges are absolutely-positioned divs inside `#viewport`;
  inline SVG is only used for icons *inside* those HTML elements.
- **JS/CSS contract**: `static/app.js` relies on `.device.hub` (the hub
  bubble), `.edge` + `id="edge-<deviceID>"`, `#viewport`, and toggles
  `zoom-detail` on `#map`. The HTMX poll target is `#viewport` with
  `hx-trigger="load, every 5s, refresh"`; drag-end POSTs then fire
  `htmx.trigger(viewport, "refresh")`. `htmx:beforeSwap` cancels swaps
  mid-drag. Preserve these hooks if you change the markup.
- **Out-of-band swaps**: the devices fragment response includes `#stats` and
  `#scan-state` with `hx-swap-oob="true"`; keep those present.
- **deviceVM.Services shadows Device.Services** (`[]serviceVM` vs
  `[]store.Service`). Any handler rendering `detail` must populate it with
  `orbit(dev.Services)`, or services silently render empty.
- **Template funcs are typed**: `label` takes `store.Device`, so call it as
  `{{label .Device}}` from a `deviceVM`, not `{{label .}}`.
- **Layout**: hub = first router, else self. Unpositioned devices use a
  deterministic phyllotaxis spiral sorted by ID; dragged positions persist in
  the DB (`has_pos`).
- **Inventory semantics**: every scan starts by zeroing all `online` flags,
  then merges observations (keyed by MAC, else IP for MAC-less rows). Devices
  and services are never deleted — they just go dim.
- **Interface filtering**: extend `virtualPrefixes` in `discovery/net.go`
  when a new virtual adapter type must be skipped (docker, WSL, VPN, ...).
- **Cross-OS behavior**: gateway detection is Linux-only
  (`/proc/net/route`); other OSes fall back to the `.1` + ports heuristic.
  Keep it that way or implement a real per-OS path — don't leave stubs.

## Release process

- Push to `main` runs CI. Tag `v*` and push to trigger `release.yml`.
- Artifacts: one `binaries-<goos>-<goarch>` artifact **per matrix job**
  (upload-artifact@v4 forbids same-name uploads across jobs — this was a real
  bug once), merged with `pattern: binaries-*` + `merge-multiple: true`.
- Version comes from the tag (`${{ github.ref_name }}`), injected via
  `-ldflags "-s -w -X main.version=..."`. Keep `VERSION` in sync locally.

## Boundaries

- No auth / no CSRF tokens by design (loopback default). If you expose
  `-listen 0.0.0.0`, consider the security implications (see README Security).
- Only scan networks you own or administer. Keep the curated port list small.
- `InsecureSkipVerify` in fingerprinting is intentional for LAN certs.
- License: **The Unlicense** (public domain). New files should fit that.

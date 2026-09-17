# Research: favicons, icon discovery, Caddy, Tailscale Serve, Compose

Target: self-hosted personal nav dashboard (Go + SQLite, Svelte SPA), Docker, tailnet-only.
Live probes run **2026-09-17**; versions checked the same day. **[verified]** = measured/primary doc, **[unverified]** = flagged.

---

## 1. Google favicon service

### 1.1 `s2/favicons` is now only a redirect shim [verified]

```
GET https://www.google.com/s2/favicons?sz=64&domain=github.com
 -> 301 Location: https://t0.gstatic.com/faviconV2?client=SOCIAL&type=FAVICON
                       &fallback_opts=TYPE,SIZE,URL&url=http://github.com&size=64
```
- It **never returns image bytes itself**; the 301 body is `text/html` (~330 B). **Follow redirects or you store HTML as a "favicon"** — the most likely bug here.
- It rewrites `domain`->`url`, `sz`->`size`, injects `client=SOCIAL&type=FAVICON&fallback_opts=TYPE,SIZE,URL`.
- Bare domain -> `http://<domain>` (scheme-less input gets `http`, not `https`).
- Host **rotates across `t0`–`t3.gstatic.com`**; bytes identical. Don't cache keyed on the redirect URL.

### 1.2 Parameters [verified]

| Param | Behaviour |
|---|---|
| `domain` | Bare host OK. Full URL **incl. path** OK (`https://github.com/features` forwarded verbatim). `https://` scheme OK. **Rejects host:port** (`github.com:443` -> final 404). |
| `sz` | Omitted -> `size=16`. **A hint, not exact** — snapped to a bucket. `sz=0` -> **404 globe**. `sz=abc` -> 404 globe. `sz=-1` -> 16x16. |

`sz` buckets measured on `github.com` (icon set 16/24/32): **16 -> 16x16; 24 -> 24x24; 32/48/64/128/256 -> 32x32; and 8,15,17,31,33,60,63,65,100,127,129,200,512,1024,999,-1,(empty) -> 16x16 fallback.**

`sz` is respected only for exact buckets; everything else silently degrades to **16px**. `sz=999` does **not** error — it gives 16px. `sz=0` **is** an error, not "native size". Exact whitelist **[unverified]** (single domain, empirical).

**Subdomains are not resolved to apex**: `domain=www.google.com` is its own site; no documented subdomain->apex fallback. You pick the host.

### 1.3 Missing-icon detection — use HTTP **status** [verified]

- **No icon -> HTTP `404`**, body is a **726-byte 16x16 RGBA PNG** (default globe), `Content-Type: image/png`.
- That globe is a **stable sentinel**: byte-identical across `example.com`, `nonexistent-abc-999.com`, `foo.invalid`, `test-noicon-xyz.org` (md5 `b8a0bf372c762e966cc99ede8682bc71`). `iana.org` -> 200, different body.
- **No `ETag`** on the 200 **or** the 404 (count 0 both) — you cannot dedupe by ETag.
- 200: `Cache-Control: public, max-age=604800` (7d) + `Expires` + `Last-Modified`. The **404 carries no `Cache-Control` at all.**
- Provenance: the 200 carries `content-location` with the real source (e.g. `https://github.githubassets.com/favicons/favicon.svg`) — proving Google does full HTML discovery and rasterises SVG/ICO to PNG.

**Verdict: `status == 404` means "no icon". Don't use content-length heuristics** (works today, undocumented).

### 1.4 Content-Type / UA / limits, and the direct call [mixed]

- **Final gstatic response is always `image/png`**, even for SVG/ICO origins — reliable. **The `s2` hop is `text/html`** — unreliable for sniffing.
- **User-Agent not required** [verified] — works with an empty UA (couldn't test a fully absent header).
- **Rate limits: none documented.** Google publishes **no** docs, ToU, or SLA for either endpoint; the "unofficial, no guarantees" framing is secondary-source + my probe, **not Google** [unverified]. Probe: 30 rapid requests -> 29× `200`/`404`, no throttling. For a personal app that fetches once and caches server-side this is negligible — but **cache to SQLite, set a UA, and cache 404s as negative results.**
- **Call this directly** (skips the shim; one fewer hop, unambiguous `Content-Type`):
  `https://t2.gstatic.com/faviconV2?client=SOCIAL&type=FAVICON&fallback_opts=TYPE,SIZE,URL&url=https://DOMAIN&size=64`
  `url` **requires a scheme** here (bare `github.com` -> 404). `fallback_opts` is effectively mandatory (omitting it -> 404 even for `github.com`).

### 1.5 Fallback services — measured 2026-09-17 [verified liveness]

| Service | URL | Has icon (github.com) | Missing domain | Status |
|---|---|---|---|---|
| DuckDuckGo | `icons.duckduckgo.com/ip3/<domain>.ico` | 200 `image/x-icon`, 6518 B (**byte-identical to origin `/favicon.ico`** -> cached proxy) | **404** + PNG body | Operating |
| icon.horse | `icon.horse/icon/<domain>` | 200 `image/png`, 26 KB | **200** + 2302 B placeholder | Operating |
| unavatar.io | `unavatar.io/<domain>` | 200 `image/png`, **519 B — identical to Google's** (likely Google-backed) | 200 `image/svg+xml` | Operating |
| Clearbit | `logo.clearbit.com/<domain>` | **connection fails** | — | **DEAD — NXDOMAIN** |
| favicon.im | `favicon.im/<domain>` | 200, redirects to `a.favicon.im`, **`image/svg+xml`** 959 B | 200 SVG placeholder | Operating |

All are free / no API key. **Clearbit is permanently gone** — HubSpot shut it down **2025-12-08**; the host no longer resolves ([changelog](https://developers.hubspot.com/changelog/upcoming-sunset-of-clearbits-free-logo-api)). Do not plan around it.

Only **Google and DuckDuckGo** signal failure by status. icon.horse, unavatar.io and favicon.im all return **`200` with a placeholder** — indistinguishable from success. favicon.im returns **SVG**, which breaks Go `png.Decode`.

**Recommended chain:** (1) Google `faviconV2` (404 = miss, cache negative) -> (2) DuckDuckGo `ip3/<domain>.ico` -> (3) self-hosted discovery (§2: `/favicon.ico` + parse `<link rel="icon">`) -> (4) generated monogram (offline, deterministic). Use icon.horse/unavatar/favicon.im only as a manual "try harder" button, never automated. Their free-tier limits are **[unverified]** (unpublished).

---

## 2. HTML icon discovery

**Algorithm** — resolve every `href` against the **document base URL** (honour `<base href>`), then:

1. `<link rel="icon" type="image/svg+xml">` — prefer SVG; `sizes="any"` is used for SVG.
2. `<link rel="icon" sizes="...">` — `sizes` is space-separated (`"16x16 32x32"`); pick best/largest match. Then `<link rel="shortcut icon">` (legacy; the `shortcut` token is ignored, treated as `icon`).
3. `<link rel="apple-touch-icon">` (and `apple-touch-icon-precomposed`) — usually 180x180, a good hi-res source. Then `<link rel="mask-icon" href color>` — Safari pinned tab, **monochrome SVG**, usually a poor nav icon. Finally `/favicon.ico` at origin root — browsers request it even with no link tag at all.

Chrome quirk (still current): a `.ico` link needs explicit `sizes="32x32"` or Chrome may prefer the ICO over an SVG ([Evil Martians 2026 guide](https://evilmartians.com/chronicles/how-to-favicon-in-2021-six-files-that-fit-most-needs)).

**[unverified]** I did **not** retrieve WHATWG spec clause text for the `icon` link type, and MDN's `/Web/HTML/Reference/Attributes/rel/icon` **404s**. Cite [WHATWG link types](https://html.spec.whatwg.org/multipage/links.html#rel-icon) rather than a quoted clause; the ordering above is browser-behaviour consensus.

**Go library:** **`github.com/mat/besticon`** — **actively maintained** [verified]: latest **v3.23.0** (2026-09-06), pushed 2026-09-06, 997★, not archived. It implements this exact pipeline (HTML fetch, `<link rel>` parsing, size selection, `/favicon.ico` fallback) plus an HTTP service + cache — run as a sidecar or vendor its `iconfinder` package. Alternatives are not credible: `chius-me/favicon-fisher` (2026-09-06 but 3★), `pomdtr/fetch-favicon` (2023), `thinkerou/favicon` (Gin middleware, no discovery).

---

## 3. Caddy in Docker

- **Current: `v2.11.4`, released 2026-06-03** [verified, GitHub releases API]. **Caddy is still v2.x — there is no Caddy v3.** v2.11.0 added automatic `Host`-header rewriting for HTTPS upstreams.
- **Image:** official Docker Hub image `caddy`; Alpine variant is the tag-suffix form **`caddy:<version>-alpine`**. Either works; **pin an explicit tag**.
- **Volumes** [verified]: `/etc/caddy` (Caddyfile — mount the **directory**, not the file; [caddy-docker#364](https://github.com/caddyserver/caddy-docker/issues/364)), `/data` (**must persist** — ACME certs/keys/account), `/config` (autosaved config).
- Official compose baseline ([docs/running](https://caddyserver.com/docs/running)): ports `80:80`, `443:443`, `443:443/udp`; volumes `./conf:/etc/caddy`, `./site:/srv`, `caddy_data:/data`, `caddy_config:/config`. Reload: `docker compose exec -w /etc/caddy caddy caddy reload`.

**Minimal Caddyfile** — if the Go app serves both API and SPA, this is the whole config:
```caddyfile
nav.tailnet-name.ts.net {
	encode zstd gzip
	reverse_proxy app:8080
}
```
If instead Caddy serves the SPA statically and proxies the API (official pattern, confirmed via Context7 `/websites/caddyserver_caddyfile` -> [patterns](https://caddyserver.com/docs/caddyfile/patterns)):
```caddyfile
nav.tailnet-name.ts.net {
	encode zstd gzip
	handle /api/* { reverse_proxy app:8080 }
	handle {
		root * /srv
		try_files {path} /index.html
		file_server
	}
}
```

**`reverse_proxy` directives worth setting** [verified, official docs]:
- **`header_up`: set nothing.** `X-Forwarded-For/Proto/Host` are set **automatically**, and incoming client values are **ignored to prevent spoofing**. Add `trusted_proxies` only if another proxy sits in front.
- **Health checks:** `health_uri /healthz`, `health_interval 30s` (default), `health_timeout 5s`; pair with `lb_try_duration 5s` — **retries are off by default**.
- **Timeouts:** `transport http { dial_timeout 3s; response_header_timeout 30s; read_timeout 30s; write_timeout 30s }` (only `dial_timeout` has a 3s default; the rest default to no timeout).
- **SSE:** `flush_interval -1` (low-latency, no buffering). Caddy **already** flushes immediately when the response is `Content-Type: text/event-stream` or `Content-Length` is unknown, so `-1` is belt-and-braces. Don't set `response_buffers` when streaming.
- **Streams/WebSockets:** `stream_timeout 24h`, `stream_close_delay 5m` (delays forcible close on reload).
- **SPA fallback:** `try_files {path} /index.html` (requires `root`); last item may be `=404` ([try_files](https://caddyserver.com/docs/caddyfile/directives/try_files)).

---

## 4. Tailscale Serve / Funnel

### 4.1 The syntax in the brief is outdated [verified]

Docs: *"The CLI commands for both Tailscale Funnel and Tailscale Serve have changed in the 1.52 version of the Tailscale client."* The positional form `tailscale serve https / http://127.0.0.1:8080` is **pre-1.52**.

Current: `tailscale serve [flags] <target>` ([reference](https://tailscale.com/docs/reference/tailscale-cli/serve))
```bash
tailscale serve --bg 8080                                # simplest
tailscale serve --bg --https=443 http://127.0.0.1:8080   # explicit
tailscale serve status [--json] ; tailscale serve reset
tailscale serve --https=443 <target> off                 # disable (same flags required)
```
Flags: `--bg` (persists across reboot / `tailscale down|up`), `--http=<port>`, `--https=<port>` (default), `--set-path=<path>`, `--tcp=<port>`, `--tls-terminated-tcp=<port>`, `--proxy-protocol=<1|2>`, `--accept-app-caps`, `--service=<vip>`, `--tun`, `--yes`.

**Critical constraint:** *"only `http://127.0.0.1` is supported for proxies."* Serve cannot proxy to a non-loopback address.

### 4.2 serve vs funnel, certs, identity, security
- **serve** = **tailnet-only**; **funnel** = **public internet**. Funnel traffic gets **no identity headers**. The same port **cannot** be both — the most recent command wins. Funnel is **enabled by default** in a tailnet; disable it on the device if unused. For a nav app: **serve only**.
- Serve **requires HTTPS certificates enabled** in the tailnet; certs are auto-provisioned for `<device>.<tailnet>.ts.net` and **DNS names are restricted to that domain**. The CLI offers a consent page that enables HTTPS for you. In-container the domain is exposed as **`${TS_CERT_DOMAIN}`**.
- **Identity headers [verified]:** Serve injects, and **strips inbound spoofed copies**, `Tailscale-User-Login`, `Tailscale-User-Name`, `Tailscale-User-Profile-Pic`. **Not populated for tagged devices.** `--accept-app-caps` also forwards `Tailscale-App-Capabilities` (v1.92+). **You can authenticate the dashboard off `Tailscale-User-Login` with no separate login system.**
- Serve binds **only to the tailnet** — no public exposure, no open port. But it does **not** authenticate beyond tailnet membership: **anyone on your tailnet** (incl. users you share a node with) can reach it. Tailscale ACLs apply as for any service.
- Tailscale's guidance: the backend should **listen only on localhost**, so nobody can bypass Serve and forge identity headers. **Therefore bind the Go app to `127.0.0.1:8080`, never `0.0.0.0`.**
- Extra app auth is **not strictly required** for a single-user tailnet; recommended if you share nodes. Trusting `Tailscale-User-Login` is the low-effort middle ground.

### 4.3 Serve in a container
Yes — the `tailscale/tailscale` image runs `tailscaled` + `containerboot`. Two paths:
1. **Declarative (supported):** `TS_SERVE_CONFIG=/config/serve.json`, mounted as a **directory** (docs: *"you must mount it as a directory (not an individual file)"*). Generate via `tailscale serve status --json`:
```json
{
  "TCP": { "443": { "HTTPS": true } },
  "Web": { "${TS_CERT_DOMAIN}:443": { "Handlers": { "/": { "Proxy": "http://127.0.0.1:8080" } } } },
  "AllowFunnel": { "${TS_CERT_DOMAIN}:443": false }
}
```
2. **Imperative:** `docker exec` in and run `tailscale serve`. Socket defaults to `/var/run/tailscale/tailscaled.sock` (`TS_SOCKET` to override).

**[unverified]** `--net=host` is not documented as a supported path. Serve is a `tailscaled` LocalAPI feature, so a host-networked container could reach the host socket, but the documented approach is sidecar + `TS_SERVE_CONFIG`. The `TS_SERVE_CONFIG` JSON schema **has no official reference docs** (acknowledged in [tailscale#19511](https://github.com/tailscale/tailscale/issues/19511)).

### 4.4 Official container env vars [verified, [docker-params](https://tailscale.com/docs/features/containers/docker/docker-params)]
- **`TS_AUTHKEY`** — auth key (or OAuth secret, which requires `TS_EXTRA_ARGS=--advertise-tags=tag:...`).
- **`TS_STATE_DIR=/var/lib/tailscale`** + volume — without it **every restart creates a new node**. Also set **`TS_AUTH_ONCE=true`**, because `TS_AUTH_ONCE` **defaults to `false`** (forces re-login every start).
- **`TS_USERSPACE` defaults to `true`** (userspace networking) — so the `cap_add: [net_admin, net_raw]` + `/dev/net/tun` in most blog examples are only needed when you set `TS_USERSPACE=false`. **This corrects most online examples.**
- Also `TS_HOSTNAME`, `TS_EXTRA_ARGS`, `TS_ACCEPT_DNS`, `TS_ENABLE_HEALTH_CHECK` / `TS_ENABLE_METRICS` (v1.78+, on `TS_LOCAL_ADDR_PORT`, default `[::]:9002`).

---

## 5. Docker Compose

### 5.1 Shape A — Tailscale sidecar, app shares its netns (**recommended**)
```yaml
services:
  tailscale:
    image: tailscale/tailscale:latest
    hostname: nav
    environment: [TS_AUTHKEY=${TS_AUTHKEY}, TS_STATE_DIR=/var/lib/tailscale,
                  TS_AUTH_ONCE=true, TS_SERVE_CONFIG=/config/serve.json]
    volumes:
      - ts-state:/var/lib/tailscale
      - ./tailscale/config:/config     # directory, not a file
    cap_add: [net_admin, net_raw]      # needed only if TS_USERSPACE=false
    restart: unless-stopped
  app:
    build: .
    network_mode: service:tailscale    # app shares tailscaled's netns
    depends_on: [tailscale]
    restart: unless-stopped
volumes: { ts-state: }
```
The app listens on `127.0.0.1:8080` **inside the shared netns**; Serve proxies to it. **No `ports:` anywhere** — nothing published to host or LAN, strictly stronger than "publish to 127.0.0.1 only". Tailscale terminates TLS with an auto-provisioned `.ts.net` cert. **No Caddy needed.**

### 5.2 Shape B, recommendation, and notes
**Shape B — separate Caddy + Tailscale** is justified only if you want Caddy's static-file serving, compression, or header/routing logic. Caddy must be the service sharing the netns (`network_mode: service:tailscale`), app on an internal compose network, and Caddy must run with **`auto_https off`** — otherwise Caddy's ACME and Tailscale's cert provisioning fight over port 443.
**Shape A is simplest and most robust** for "personal nav, tailnet-only, automatic TLS" — Tailscale already provides TLS termination and the `.ts.net` name; Caddy adds a second cert path and an extra hop for no benefit. Add Caddy later only if it must serve the SPA bundle itself.
- The **official** Tailscale Compose page ([connect-docker-container](https://tailscale.com/docs/features/containers/docker/how-to/connect-docker-container), validated 2026-03-23) does **not** use `network_mode: service:` — it publishes `"8080:80"` and tests via `http://localhost:8080`, despite prose claiming the containers share the network. The **[blog post](https://tailscale.com/blog/docker-tailscale-guide)** and community write-ups show the real `network_mode: service:<ts>` sidecar. **Treat the blog pattern as the actual recipe.**
- Host-only publishing: use `"127.0.0.1:8080:8080"`, **not** `"8080:8080"` (which binds `0.0.0.0`). A published port bypasses the tailnet entirely — never publish to `0.0.0.0` on a host with a public IP.

---

## Unverified / flagged
1. **No official Google docs, ToU, or rate limit** for `s2/favicons` / `faviconV2`; "unofficial" is secondary-source + empirical, not Google policy.
2. **WHATWG clause text not retrieved** for the `icon` link type; MDN's `rel/icon` path 404s. §2.1 ordering is browser-behaviour consensus.
3. **`sz` accepted-bucket whitelist** is empirical (github.com only), undocumented.
4. **`tailscale serve` with `--net=host`** — undocumented; inferred from Serve being a LocalAPI feature.
5. **`TS_SERVE_CONFIG` JSON schema has no official reference docs** (upstream #19511).
6. **Free-tier rate limits** for icon.horse / unavatar.io / favicon.im unpublished; only liveness measured.
7. The 404-globe sentinel and its 726-byte length are **observed behaviour**, not a documented contract.

## Sources
- **Google / icons:** live probes of `google.com/s2/favicons` + `t2.gstatic.com/faviconV2` (2026-09-17); [Evil Martians favicon guide](https://evilmartians.com/chronicles/how-to-favicon-in-2021-six-files-that-fit-most-needs); [WHATWG link types](https://html.spec.whatwg.org/multipage/links.html#rel-icon); `mat/besticon` via GitHub API. Clearbit: [HubSpot changelog](https://developers.hubspot.com/changelog/upcoming-sunset-of-clearbits-free-logo-api).
- **Caddy:** [reverse_proxy](https://caddyserver.com/docs/caddyfile/directives/reverse_proxy), [try_files](https://caddyserver.com/docs/caddyfile/directives/try_files), [patterns](https://caddyserver.com/docs/caddyfile/patterns), [running](https://caddyserver.com/docs/running), [Docker Hub](https://hub.docker.com/_/caddy); version via GitHub releases API.
- **Tailscale:** [serve CLI](https://tailscale.com/docs/reference/tailscale-cli/serve), [Tailscale Serve](https://tailscale.com/docs/features/tailscale-serve), [Docker params](https://tailscale.com/docs/features/containers/docker/docker-params), [Compose guide](https://tailscale.com/docs/features/containers/docker/how-to/connect-docker-container), [blog guide](https://tailscale.com/blog/docker-tailscale-guide), [#19511](https://github.com/tailscale/tailscale/issues/19511).

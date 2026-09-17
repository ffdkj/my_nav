# Go Backend Grounding — chi v5 + modernc SQLite + sqlc + go:embed + Docker

**Target:** small single-user JSON REST API for a personal navigation dashboard; serves an embedded Svelte SPA.
**Method:** Context7 MCP primary; versions from **live `proxy.golang.org/@latest`** + **GitHub Releases API** + `go.dev/VERSION`.
**§1, §2, §3, §5 were actually compiled and executed on `go1.26.7`** (sqlc v1.31.1 binary downloaded and run for real). §4/§6/§7/§8 are doc/source-grounded but not executed — flagged inline.
⚠️ **`web_search` was unavailable (no API key)** — no search-engine corroboration. Every version is from a machine-readable upstream endpoint, never memory.

## 0. Versions (live)

| Module / tool | Version | Date | Note |
|---|---|---|---|
| Go toolchain (local) | **go1.26.7** | — | what the snippets ran on |
| Go latest (`go.dev/VERSION?m=text`) | **go1.27.1** | 2026-08-28 | Go 1.26 = N-1, still supported |
| `github.com/go-chi/chi/v5` | **v5.3.2** | 2026-08-20 | |
| `github.com/go-chi/cors` | **v1.2.2** | 2025-07-01 | separate module |
| `modernc.org/sqlite` | **v1.59.0** | 2026-09-15 | pulls `modernc.org/libc v1.75.7`, `modernc.org/memory v1.12.1` |
| `github.com/mattn/go-sqlite3` | v1.14.52 | 2026-09-05 | CGO, comparison only |
| **sqlc** (CLI) | **v1.31.1** | 2026-04-22 | latest release (v1.30.0 was 2025-09-01) |
| `github.com/pressly/goose/v3` | **v3.28.0** | 2026-09-02 | |
| `github.com/golang-migrate/migrate/v4` | **v4.20.1** | 2026-09-09 | |
| `ariga.io/atlas` | **v1.3.0** | 2026-07-29 | |
| `golang.org/x/image` | **v0.46.0** | 2026-09-08 | webp decode; **no ICO** |
| `github.com/biessek/golang-ico` | v0.0.0-20250805151044-6d8ea19fb761 | 2025-08-05 | ICO decode |
| `github.com/fyne-io/image` | **v0.1.1** | 2025-03-27 | `.../image/ico` subpackage |

## 1. chi v5

**⚠️ `middleware.RealIP` is deprecated and has three security advisories.** Verbatim from `chi/middleware/realip.go` @ master:

> `Deprecated: RealIP is vulnerable to IP spoofing — it mutates r.RemoteAddr to the leftmost X-Forwarded-For value, or to True-Client-IP / X-Real-IP whether or not your infrastructure actually sets them. See GHSA-3fxj-6jh8-hvhx, GHSA-rjr7-jggh-pgcp, GHSA-9g5q-2w5x-hmxf.`
> `Use [ClientIPFromHeader], [ClientIPFromXFF], [ClientIPFromXFFTrustedProxies] or [ClientIPFromRemoteAddr] and read the IP with [GetClientIP] instead. These never mutate r.RemoteAddr.`

It still compiles in v5.3.2 (deprecation only), so it is a silent trap — **do not use it in a new project.**

```go
r := chi.NewRouter()
r.Use(middleware.RequestID)
r.Use(middleware.ClientIPFromRemoteAddr) // no proxy. Behind one: ClientIPFromHeader("CF-Connecting-IP") or ClientIPFromXFFTrustedProxies(n)
r.Use(middleware.Recoverer)
r.Use(middleware.Timeout(15 * time.Second))
// ip := middleware.GetClientIP(r.Context())
```
CORS is a separate module and must be top-level: `r.Use(cors.Handler(cors.Options{AllowedOrigins: []string{"http://localhost:5173"}, AllowedMethods: []string{"GET","POST","PUT","DELETE","OPTIONS"}, AllowedHeaders: []string{"Accept","Content-Type"}, MaxAge: 300}))`

### SPA mount excluding `/api/*` — two gotchas, both reproduced

1. chi's `NotFound` takes an **`http.HandlerFunc`**, not `http.Handler`. Returning `http.Handler` from a helper is a hard compile error: `cannot use spaHandler() (value of interface type http.Handler) as http.HandlerFunc value`.
2. **`r.NotFound(spa)` alone leaks `/api/*` into the SPA.** Verified: `GET /api/nope` returned `200 text/html` (the index) instead of 404. Fix — put a catch-all *inside* the `/api` group so unknown API paths never reach the root not-found handler.

```go
r.Route("/api", func(api chi.Router) {
	r.Get("/links", h.ListLinks)
	api.NotFound(jsonNotFound)          // JSON 404 for unknown /api paths
	api.HandleFunc("/*", jsonNotFound)  // catch-all, same handler
})
r.NotFound(spa)                         // SPA fallback for everything else
r.Get("/*", spa)
r.Get("/", spa)
```
Verified with a real `httptest` server:

| path | status | Content-Type | Cache-Control |
|---|---|---|---|
| `/api/links` | 200 | `application/json` | — |
| `/api/nope` | **404** | `application/json` | — |
| `/assets/app.css` | 200 | `text/css; charset=utf-8` | `public, max-age=31536000, immutable` |
| `/settings/general` (SPA route) | 200 | `text/html; charset=utf-8` | `no-cache` |

## 2. `modernc.org/sqlite` (CGO-free)

**Driver name is `"sqlite"`.** `sql.Open("sqlite", dsn)`, blank import `_ "modernc.org/sqlite"`. **No build tag needed** — it registers a normal `database/sql` driver, so it works with sqlc's `sql_package: "database/sql"` output (verified end-to-end, §3). Canonical repo is **gitlab.com/cznic/sqlite**; GitHub `modernc-org/sqlite` is a mirror. DSN pragmas use `_pragma=name(value)` and may repeat; shorthands also exist (`_journal_mode`, `_busy_timeout`, `_fk`, `_sync`, `_query_only`).

```go
dsn := "file:/data/nav.db?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)" +
       "&_pragma=foreign_keys(1)&_pragma=synchronous(NORMAL)"
db, err := sql.Open("sqlite", dsn)   // + db.SetMaxOpenConns(1) to serialize writes
```
Verified PRAGMA readback: `journal_mode=wal`, `busy_timeout=5000`, `foreign_keys=1`, `synchronous=1`.

> **Critical:** `journal_mode=WAL` persists in the database file, but **`busy_timeout`, `foreign_keys` and `synchronous` are per-connection**. With a pooled `*sql.DB` they must be in the DSN (applied to every new conn) — a one-shot `db.Exec("PRAGMA foreign_keys=ON")` configures only whichever single connection it lands on.

**Limitations vs `mattn/go-sqlite3`:** pure Go (SQLite's C transpiled with `ccgo/v4`) ⇒ **no CGO, trivial cross-compilation, `CGO_ENABLED=0` static binaries** — but no C loadable extensions, larger binary, and slower. Upstream's own benchmark: 1.17×–5.84× slower per-op (e.g. `select_with_index` CGo 0.002 ms vs pure-Go 0.014 ms; `insert` 1.17×). Irrelevant for a single-user dashboard. Other pure-Go option noted upstream: `github.com/ncruces/go-sqlite3` (wasm2go).

## 3. sqlc

- **v1.31.1.** Go + **SQLite is officially `Beta`**; MySQL/PostgreSQL are `Stable`. Beta ≠ unusable — it generated and ran correctly here, but expect edge-case gaps on complex queries.
- **The engine name is `"sqlite"`, not `sqlc.sqlite`.** There is no `sqlc.` prefix anywhere in the config; the official SQLite tutorial uses `engine: "sqlite"`.

```yaml
version: "2"                     # use v2 — see the emit_pointers note below
sql:
  - engine: "sqlite"
    queries: "internal/db/sql/query.sql"
    schema: "internal/db/sql/migrations"   # a DIRECTORY enables migration parsing
    gen:
      go:
        package: "db"
        out: "internal/db"
        sql_package: "database/sql"        # default; required pairing for modernc
        emit_json_tags: true
        emit_empty_slices: true            # :many returns []T not nil
        emit_interface: true               # emits Querier + var _ Querier = (*Queries)(nil)
        emit_pointers_for_null_types: true # nullable -> *string / *int64
```
- **Annotations:** `-- name: <Name> <cmd>` with `:one`, `:many`, `:exec`, `:execresult`, `:execrows`, `:execlastid`. SQLite uses `?` placeholders.
- **`emit_pointers_for_null_types` + SQLite — a doc contradiction, resolved empirically.** The *v1* config docs say it applies only to PostgreSQL with `sql_package: pgx/v4|v5`. The *v2* docs say: *"Currently only supported for PostgreSQL if `sql_package` is `pgx/v4` or `pgx/v5`, **and for SQLite**."* I ran v1.31.1 with `engine: sqlite` + `sql_package: database/sql` and it works: nullable `group_id`/`icon`/`notes` came out as `*int64` / `*string`. **Use config v2.**
- **Overrides / nullable columns:** `db_type` overrides are **nullability-specific** — one entry applies to either nullable *or* non-nullable, never both, so declare two entries for one type. `column` overrides take precedence over `db_type`. `pointer: true` is **nested inside `go_type`**, not at override top level — top-level fails with `field pointer not found in type opts.Override` (hit this for real).

```yaml
overrides:
  - db_type: "text"                                      # non-nullable TEXT
    go_type: "string"
  - db_type: "text"                                      # nullable TEXT -> *string
    nullable: true
    go_type: { type: "string", pointer: true }           # same db_type, second entry
```
- **`schema:` vs `queries:`** — `schema:` accepts a single `.sql` file **or a migration directory**; `queries:` holds your annotated queries. sqlc **never runs migrations**; it only parses them to learn the schema. It auto-detects atlas / dbmate / golang-migrate / goose / sql-migrate / tern formats and **ignores down migrations**. Numeric filenames must be zero-padded (`001_`, `010_`) because parsing is lexicographic.
- **Verified parsing:** plain `001_init.sql` + `002_notes.sql` with **no tool markers at all** parsed correctly in order (the `ALTER TABLE` column appeared in the struct); golang-migrate `000001_init.up.sql` parsed and its `.down.sql` was ignored. So a hand-rolled runner (§4) needs no special markers for sqlc.
- **Can generated code be committed so Docker doesn't need sqlc? Yes.** That is the intended workflow — `sqlc diff` exists precisely to compare `sqlc generate` output against what is on disk, i.e. generated files live in the repo. The `sqlc` binary is a dev/CI tool only; `docker build` just compiles the committed `.go` files.
- **Go 1.26 / generics: no issue.** Output is plain pre-generics Go (no type parameters). Verified: sqlc v1.31.1 output + `database/sql` + `modernc.org/sqlite` v1.59.0 compiled and ran on go1.26.7 (`id=1 count=1 listed=1`, nullable pointer `nil`).

## 4. Schema migrations — comparison

| Option | Weight | Embeddable | CGO-free w/ modernc | Fit (single-binary, single-user) |
|---|---|---|---|---|
| (a) hand-rolled `embed.FS` + `PRAGMA user_version` | ~60 LOC, **zero deps** | native | yes | **recommended** |
| (b) `goose` v3.28.0 | 1 module | `goose.NewProvider(goose.DialectSQLite3, db, embedFS)` takes any `fs.FS` | yes | best upgrade path |
| (c) `golang-migrate` v4.20.1 | 1 module + CLI | `iofs` source driver | **yes** — `database/sqlite` imports `modernc.org/sqlite`; `database/sqlite3` uses mattn/CGO | fine, more moving parts |
| (d) `atlas` v1.3.0 | CLI/Docker + module | pluggable | via driver | heaviest; declarative/team-oriented — overkill |

**Recommendation: (a).** No dependency, no version-table drift, and sqlc already parses plain `.sql` files (§3).

```go
//go:embed migrations/*.sql
var migrationsFS embed.FS

func Migrate(ctx context.Context, db *sql.DB) error {
	entries, err := fs.Glob(migrationsFS, "migrations/*.sql")
	if err != nil { return err }
	sort.Strings(entries) // zero-padded names -> lexicographic == numeric
	var cur int
	if err := db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&cur); err != nil { return err }
	for _, name := range entries {
		n, err := strconv.Atoi(strings.SplitN(path.Base(name), "_", 2)[0])
		if err != nil || n <= cur { continue }
		sqlBytes, err := migrationsFS.ReadFile(name)
		if err != nil { return err }
		tx, err := db.BeginTx(ctx, nil)
		if err != nil { return err }
		if _, err := tx.ExecContext(ctx, string(sqlBytes)); err != nil {
			tx.Rollback(); return fmt.Errorf("%s: %w", name, err)
		}
		// PRAGMA user_version does not accept a bound parameter -> interpolate a validated int.
		if _, err := tx.ExecContext(ctx, fmt.Sprintf(`PRAGMA user_version = %d`, n)); err != nil {
			tx.Rollback(); return err
		}
		if err := tx.Commit(); err != nil { return err }
		cur = n
	}
	return nil
}
```
Caveats (design-level, not executed): `PRAGMA user_version = ?` is not bindable — interpolate a validated integer; several PRAGMAs are no-ops inside a transaction (this one works); keep **one file = one version**; SQLite `ALTER TABLE` is limited, so sqlc-generated code must track whatever the migration history produces; back up before migrating.

**Goose if you outgrow it:** `provider, _ := goose.NewProvider(goose.DialectSQLite3, db, migrationsFS)` (accepts an `embed.FS` directly), `goose.WithTableName(...)` to rename `goose_db_version`, migrations marked `-- +goose Up` / `-- +goose Down`. Legacy API: `goose.SetBaseFS(embedMigrations)` + `goose.SetDialect("sqlite3")` + `goose.Up(db, "migrations")`.

## 5. `go:embed` — serving the SPA

```go
//go:embed all:web          // "all:" is required if the dir has _* or .* files
var webFS embed.FS

sub, _ := fs.Sub(webFS, "web")             // strip the "web" prefix
fileServer := http.FileServer(http.FS(sub))
index, _ := fs.ReadFile(sub, "index.html") // read once at startup for the fallback
```
- `fs.Sub` on a missing dir only fails at runtime — panic at startup is fine (fail fast).
- **MIME types** come from `mime.TypeByExtension` via `http.ServeContent`; verified `.css` → `text/css; charset=utf-8`. Unknown extensions may fall back to sniffing, so **emit hashed filenames from the Svelte build** (`app.[hash].js`) — that is what makes immutable caching safe.
- **Caching:** immutable only for content-hashed asset paths; `index.html` must be `no-cache` or the SPA never updates. `/assets/*` → `Cache-Control: public, max-age=31536000, immutable`; `index.html` + SPA fallback → `Cache-Control: no-cache`.
- Build tag note: `//go:embed` **fails the build** if the directory is missing or empty, so commit the built SPA into the embed dir before `go build` (or gate the pattern behind `//go:build !dev` *and* ship a dev stub).

## 6. Image handling in the Go stdlib

| Format | Stdlib decode | Encode |
|---|---|---|
| PNG | `image/png` ✔ | ✔ |
| JPEG | `image/jpeg` ✔ | ✔ |
| GIF | `image/gif` ✔ | ✔ |
| BMP / TIFF | `golang.org/x/image/bmp`, `/tiff` | partial |
| **WebP** | **not stdlib** — `golang.org/x/image/webp` v0.46.0, **decode only** | ✘ |
| **ICO** | **not in stdlib, not in `x/image`** (verified: `x/image` top level = bmp, ccitt, colornames, draw, font, math, riff, tiff, vector, vp8, vp8l, webp — no `ico`) | ✘ |

For ICO use `github.com/biessek/golang-ico` (blank-import to register, then `image.Decode`) or `github.com/fyne-io/image/ico` (v0.1.1). Neither is in the Go security-reviewed set — treat ICO as the risky path.

**Validating/transcoding a downloaded favicon:** `http.DetectContentType(head)` sniffs **only the first 512 bytes**; it knows PNG/JPEG/GIF/WebP/BMP but **not ICO**. Treat it as a cheap pre-filter, **not** authorization — the real validation is `image.DecodeConfig` (dimensions without a full decode) then a full `image.Decode`, which proves the bytes are a decodable image. Enforce a byte cap *before* decoding and cap pixels/dimensions too (a tiny file can declare a huge canvas — decompression bomb). Transcode to PNG with `png.Encode` for a uniform on-disk cache.

**SSRF — the main security risk here.** A naive `http.Get(userURL)` reaches `127.0.0.1`, `169.254.169.254` (cloud metadata) and RFC1918 hosts. Standard mitigations:
- **`net.Dialer.Control`** — the robust one: validate the *actual* IP about to be dialed, which closes DNS-rebinding/TOCTOU gaps that pre-flight hostname checks leave open.
```go
dialer := &net.Dialer{Timeout: 5 * time.Second, Control: func(network, address string, c syscall.RawConn) error {
	host, _, _ := net.SplitHostPort(address)
	ip := net.ParseIP(host)
	if ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() {
		return fmt.Errorf("blocked address %s", address)
	}
	return nil
}}
```
- **`CheckRedirect`** with a small hop limit (`http.ErrUseLastResponse`) so a redirect cannot bounce you to an internal host after the check.
- Restrict schemes to `http`/`https`, allow-list ports (80/443), reject userinfo, optionally resolve-and-pin DNS. Prefer fetching from the site's own origin (`/favicon.ico`, `<link rel="icon">`) over an arbitrary user URL.

## 7. Docker

```dockerfile
FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# CGO_ENABLED=0 is what makes the static/distroless target work; modernc needs no CGO.
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/nav ./cmd/nav

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/nav /nav
VOLUME ["/data"]
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/nav"]
```
- **Non-root:** distroless `:nonroot` already sets uid 65532. Make `/data` writable by it (`chown 65532:65532` on a host bind mount, or use a named volume).
- **Read-only rootfs:** `docker run --read-only --tmpfs /tmp -v nav-data:/data`. The binary comes from the image layer; **the SQLite DB *and* the icon cache must live on a mount** — nothing else needs to write.
- **Alpine alternative:** `alpine:3` + `apk add --no-cache ca-certificates tzdata`, create a user. You need CA certs to fetch favicons (distroless `static` bundles them). Build with `-tags netgo` so DNS needs no libc.
- **SQLite volume & WAL caveats:** put the DB on a Docker **named volume** or a **local bind mount** on a real local filesystem. **Never put a WAL-mode SQLite DB on NFS/CIFS/SMB or a network-backed volume** — WAL relies on shared-memory (`-shm`) mmap and POSIX file locking, which network filesystems implement unreliably, producing corruption or `SQLITE_BUSY`/`disk I/O error`. WAL also creates `-wal` and `-shm` sidecars: **back up all three, or use `VACUUM INTO` / `.backup` for a consistent copy.** Icon cache: same volume or another, but off the read-only rootfs.
- ⚠️ Base-image tag names are **unverified** — Docker Hub's tag API and the distroless/alpine release endpoints returned empty from this sandbox. Confirm `golang:1.26-bookworm` and `gcr.io/distroless/static-debian12:nonroot` exist, and **pin by digest**.

## 8. HTTP client for fetching remote favicons

```go
var httpClient = &http.Client{
	Timeout: 10 * time.Second,                       // total, incl. body read
	Transport: &http.Transport{
		DialContext:           safeDialer.DialContext, // §6 SSRF guard
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 5 * time.Second,
		MaxIdleConns:          20,
		MaxIdleConnsPerHost:   2,
		ForceAttemptHTTP2:     true,
	},
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 { return errors.New("too many redirects") }
		return nil // the Control-based guard re-validates the redirect target on dial
	},
}

func fetchIcon(ctx context.Context, raw string) ([]byte, error) {
	const maxBytes = 2 << 20 // 2 MiB
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil { return nil, err }
	resp, err := httpClient.Do(req)
	if err != nil { return nil, err }
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK { return nil, fmt.Errorf("status %d", resp.StatusCode) }
	// Cap AFTER the status check so we can still report the status.
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil { return nil, err }
	if len(body) > maxBytes { return nil, errors.New("icon too large") }
	return body, nil
}
```
- **Context** on every request, derived from the inbound HTTP request context so a client disconnect cancels the outbound fetch.
- **Size cap:** `io.LimitReader`. `http.MaxBytesReader` is for *serving*/server-side request bodies, **not** for capping a client download.
- **Concurrency limiting:** a buffered-channel semaphore (4–8 concurrent fetches) or `golang.org/x/sync/semaphore`, plus a low `MaxIdleConnsPerHost` so one popular host cannot exhaust the pool. A global semaphore of ~4 is plenty here.
- Serve from the **on-disk cache** on subsequent requests; never fetch on the hot path of page load without a timeout.

## 9. Uncertain / not verified

- §4 migration code, §6 image/SSRF code, §7 Dockerfile, §8 client: doc/source-grounded, **not executed**. §1/§2/§3/§5 were executed on go1.26.7.
- Docker base-image tag names and distroless contents — registry APIs unreachable from the sandbox.
- `modernc.org/sqlite`: whether a `_txlock=immediate` DSN option exists (mattn has it) — **not in the documented DSN key list**, so do not assume it; `SetMaxOpenConns(1)` is the portable way to serialize writes.
- sqlc SQLite is officially **Beta** — expect edge-case gaps independent of Go version.
- No third-party corroboration of the version table (`web_search` down); all numbers come from `proxy.golang.org`, the GitHub Releases API, `go.dev/VERSION`, or live execution.

## Sources

**Context7 library IDs:** `/go-chi/docs` · `/websites/sqlc_dev_en` · `/websites/pkg_go_dev_modernc_org_sqlite` · `/pressly/goose` (`/golang-migrate/migrate`, `/ariga/atlas` resolved for versions only)

**URLs:** [chi middleware](https://github.com/go-chi/docs/blob/master/pages/middleware.md) · [chi quickstart](https://github.com/go-chi/docs/blob/master/quickstart.md) · [realip.go deprecation notice](https://github.com/go-chi/chi/blob/master/middleware/realip.go) · [sqlc config](https://docs.sqlc.dev/en/latest/reference/config.html) · [sqlc SQLite tutorial](https://docs.sqlc.dev/en/latest/tutorials/getting-started-sqlite.html) · [sqlc annotations](https://docs.sqlc.dev/en/latest/reference/query-annotations.html) · [sqlc overrides](https://docs.sqlc.dev/en/latest/howto/overrides.html) · [sqlc DDL/migrations](https://docs.sqlc.dev/en/latest/howto/ddl.html) · [sqlc language support (SQLite = Beta)](https://docs.sqlc.dev/en/latest/reference/language-support.html) · [sqlc CI/CD](https://docs.sqlc.dev/en/latest/howto/ci-cd.html) · [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite) · [modernc benchmarks](https://pkg.go.dev/modernc.org/sqlite/benchmark) · [goose](https://github.com/pressly/goose) · [golang-migrate modernc sqlite driver](https://github.com/golang-migrate/migrate/blob/master/database/sqlite/sqlite.go) · [golang.org/x/image](https://pkg.go.dev/golang.org/x/image)

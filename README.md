# Gradeium

Gradeium is a long-lived, self-hosted media tracker for Games, Movies, and TV Shows. The repository contains the complete Phase 5 pre-1.0 product on top of its secure Go, React, PostgreSQL, OpenID Connect, and Docker foundation.

Games use IGDB, Movies use TMDB, and TV Shows use TVDB with a strictly verified TVDB-to-TMDB community-rating bridge. Each domain has provider search/Add, separate Library and Backlog views, personal statuses and ratings, full detail pages, provider artwork selection, and metadata refresh. TV also includes season/episode progress with Specials excluded from regular progress. The dashboard summarizes the currently persisted library, while portable backups, safe restore, automatic retention, and ratings CSV export keep user-owned data recoverable. Jellyfin, imports, custom lists, and user-management UI remain intentionally out of scope.

## Prerequisites

- Docker Engine with Docker Compose v2 for the complete local stack.
- Go 1.26 for native backend development.
- Node.js 24 and npm for native frontend development.
- sqlc 1.31.1 only when regenerating typed database queries.

## Start with Docker Compose

Copy the safe example environment file and replace its local development password placeholder:

```powershell
Copy-Item .env.example .env
docker compose up --build --detach
docker compose ps
```

On macOS or Linux, use `cp .env.example .env` for the first command. Open [http://localhost:8080](http://localhost:8080).

A fresh installation follows this sequence:

1. Complete the one-time installation initialization.
2. Enter the generic OIDC issuer URL, client ID, client secret, and public Gradeium URL.
3. Save and test discovery before enabling authentication.
4. Enable authentication, which permanently closes the unauthenticated configuration bootstrap.
5. Sign in. The first successfully verified issuer/subject identity becomes the initial administrator.
6. Open Admin Settings → Integrations and configure/test IGDB, TMDB, and TVDB as needed.

Perform the unauthenticated OIDC bootstrap only from a trusted network. There is no local-password account or hidden authentication reset endpoint.

Useful endpoints are:

- `GET /api/healthz` - process liveness; independent of PostgreSQL and the OIDC provider.
- `GET /api/readyz` - readiness; checks PostgreSQL with a short timeout, not provider reachability.
- `GET /api/setup/status` - safe first-run completion state.
- `GET /api/auth/status` - safe activation/session state needed by the entry screen.

PostgreSQL migrations run automatically before HTTP startup and fail closed. Inspect startup and migration logs with:

```powershell
docker compose logs gradeium postgres
```

Stop the stack without deleting persistent data:

```powershell
docker compose down
```

Add `--volumes` only when you explicitly intend to delete all local PostgreSQL, config, and backup volume data.

## Generic OIDC and Pocket ID

Gradeium uses standards-based OpenID Connect Authorization Code flow. Pocket ID is the primary tested provider, but no Pocket ID-specific API or issuer behavior is built into the runtime.

In the provider, register a confidential OIDC client with:

- the client ID and client secret shown by the provider;
- scopes `openid`, `profile`, and `email`;
- the exact callback URI shown by Gradeium.

The callback format is:

```text
{authentication.public_url}/api/auth/callback
```

For example, a public URL of `https://gradeium.example.com` requires:

```text
https://gradeium.example.com/api/auth/callback
```

Enter the provider's exact issuer value. Discovery must report that same issuer, and Gradeium strictly verifies the ID-token signature, issuer, client-ID audience, expiry, nonce, and stable `sub` claim. Email is never used as durable identity.

The configured public URL is the browser-facing origin through the reverse proxy. Use HTTPS in normal deployments, preserve the original host/scheme at the proxy, and register the exact displayed callback URI. Plain HTTP is accepted only for loopback development URLs such as `http://localhost:8080`.

OIDC settings live in Admin Settings, not `.env` or Compose. The client secret is encrypted through the Phase 2 secret store and is never returned after saving. Post-activation edits are tested before they replace the last-known-good active configuration, so an invalid draft does not break current login.

## Media provider configuration

An administrator configures all media providers in Gradeium under **Settings → Integrations**:

- IGDB uses a Twitch application client ID and client secret.
- TMDB uses an API Read Access Token.
- TVDB uses a v4 API key and an optional subscriber PIN when the key requires one.

Save credentials, then use **Test connection**. Saved secret fields become configured/not-configured state and are never returned to the browser. Replacing credentials invalidates the previous connection-test result until the new values are tested. Provider credentials are encrypted with the same master-key-backed secret store used by OIDC; there are deliberately no IGDB, TMDB, or TVDB environment variables.

Provider downtime does not prevent an authenticated user from opening already-stored Library, Backlog, or detail data, and it does not affect liveness/readiness. Searches, new Adds, and manual metadata refreshes return bounded, user-safe errors until the affected provider recovers.

## Core media behavior

Every item has a Gradeium-owned UUIDv7 identity while provider IDs remain unique external references. Canonical provider metadata is stored separately for Games, Movies, and TV Shows, and personal state is stored separately per user.

- Library statuses are In Progress, On Hold, Abandoned, and Completed; Backlog is a separate view.
- Ratings use 1.0–10.0 in exact 0.1 increments, with one optional private reason. Backlog cannot be rated.
- Moving a rated item to Backlog requires confirmation and removes its rating and reason.
- Artwork choices are limited to provider-supplied Cover/Poster, Backdrop, and Logo candidates. Pins survive metadata refresh; an unavailable pin falls back visibly to the provider default.
- TV progress is episode-based in TVDB default aired order. Specials are tracked separately and never affect the regular-episode percentage. Status changes never alter progress automatically.
- Removing an item deletes that user’s status, rating, reason, artwork pins, and—on TV—episode progress. Reusable canonical metadata may remain.

## Dashboard

The authenticated Dashboard is a read-only view of current, locally persisted Gradeium state. Its All/Games/Movies/TV scopes show totals, current in-progress items, personal-rating and status distributions, highest-rated items, and regular-episode TV progress. It deliberately does not invent viewing history or depend on live metadata providers, so it remains useful during provider outages.

## Portable backups and restore

Gradeium writes application-managed backups to the persistent `/backups` mount as gzip-compressed, versioned JSON. The canonical Phase 5 format is `gradeium-backup` version `1`; filenames use UTC timestamps and each completed file is validated and checksummed before atomic publication.

Portable backups contain Gradeium user identities, canonical Games/Movies/TV metadata, provider identity mappings, personal statuses and ratings/reasons, TV episode progress (including Specials), artwork pins, and user Library preferences. They never contain OIDC or provider secrets in plaintext or encrypted form, session material, CSRF tokens, database credentials, or the master key. `/config/master.key` and external-service credentials therefore remain a separate operator recovery responsibility.

Administrators can create, list, validate, download, restore, and delete recognized backups from **Settings → Backups**. Every restore validates the entire input, creates a pre-restore safety backup, and applies portable application state in one PostgreSQL transaction. Restore reconciles users using their stable OIDC issuer/subject identity, does not import administrator privileges, and never replaces the installation's current authentication, sessions, or encrypted credentials. A failed validation or transaction leaves the current database state unchanged.

Automatic backups are enabled by default every 3 days and retain the latest 30 automatic backups. Schedule and retention state live in PostgreSQL; an overdue installation runs one backup after startup. Retention never removes manual or pre-restore backups. Backup failures are reported in Settings without changing liveness or database readiness.

For disaster recovery, retain the portable backup files together with a separately protected copy of `/config/master.key` and a secure record of external-service credentials. Restore the Gradeium backup through a currently authenticated administrator; re-enter secrets through Admin Settings when moving to a new installation. Each user can also download a UTF-8 CSV of their current rated items from **Settings → Library**; administrators have the same export action in **Settings → Backups**. CSV export is intentionally one-way; import remains out of scope.

## Session and authorization security

Gradeium sessions are server-side PostgreSQL rows. The browser receives a cryptographically random opaque cookie; PostgreSQL stores only its SHA-256 hash. Cookies are `HttpOnly`, `SameSite=Lax`, restricted to `Path=/`, finite-lived, and `Secure` whenever the configured public URL uses HTTPS.

Unsafe authenticated API requests also require a session-bound CSRF token and reject mismatched browser origins. Logout revokes the current PostgreSQL session before clearing the cookie. Expired and revoked rows are rejected and removed through bounded cleanup.

Admin authorization trusts only the PostgreSQL user record. `/api/admin/**` returns 401 without a valid session and 403 for an authenticated non-admin. The first verified OIDC identity is bound atomically as the sole bootstrap administrator; concurrent later logins cannot auto-promote another user.

An OIDC outage does not affect liveness, readiness, or existing local sessions. Existing sessions continue until their normal expiry, while new login attempts may fail until the provider recovers. There is intentionally no local-password bypass. If every session expires while the OIDC provider is unavailable, restore provider availability; do not disable authentication in the database.

## Master key and disaster recovery

On the first successful boot, Gradeium creates `/config/master.key` from cryptographically secure random bytes. The file is versioned, created atomically, restricted to the application user on the Linux production runtime, and stored only in the persistent `/config` mount. It is never loaded from an environment variable or stored in PostgreSQL.

Secret settings use AES-256-GCM with a fresh nonce for every write. PostgreSQL contains only algorithm versions, nonces, authenticated ciphertext, and a one-way master-key fingerprint. APIs expose configured/not-configured state and never return saved secret values.

Treat PostgreSQL data and `/config` as one recovery set:

- Back up both at the same time.
- Restoring a database without its matching config key makes encrypted settings unrecoverable.
- Restoring a config key that does not match the database is rejected at startup.
- If an established installation loses `master.key`, startup fails instead of silently generating a replacement.

Deleting only PostgreSQL discards setup/settings/authentication data while leaving the old key behind; a new database may deliberately register that retained key. Deleting only `/config` while retaining the established database is rejected. For an unambiguous disposable-development reset, delete both together:

```powershell
docker compose down --volumes
```

This permanently deletes the local development database and key. The next startup is a new installation.

## Native development

Start PostgreSQL from Compose:

```powershell
docker compose up --detach postgres
```

Run the backend from `backend/`. The Vite development server can provide the frontend separately, so missing compiled frontend assets only place the Go server in API-only mode:

```powershell
$env:GRADEIUM_DATABASE_URL = "postgres://gradeium:YOUR_URL_ENCODED_PASSWORD@localhost:5432/gradeium?sslmode=disable"
$env:GRADEIUM_WEB_DIR = "../frontend/dist"
go run ./cmd/gradeium
```

Run the frontend from `frontend/`; Vite proxies `/api` to `http://localhost:8080`:

```powershell
npm ci
npm run dev
```

## Quality checks

Backend, from `backend/`:

```powershell
gofmt -w .
go test ./...
go test -race ./...
go vet ./...
```

Set `GRADEIUM_TEST_DATABASE_URL` to a PostgreSQL 18 URL to include isolated migration/repository integration tests. In addition to the existing authentication, security, and media coverage, tests cover the Phase 4-to-Phase 5 migration, dashboard isolation and aggregates, CSV escaping, portable backup validation and atomic publication, transactional restore and rollback, pre-restore safety backups, secret/session preservation, scheduler persistence, overdue execution, retention, and backup-operation serialization. CI supplies PostgreSQL 18.4.

Frontend, from `frontend/`:

```powershell
npm ci
npm run lint
npm run typecheck
npm run build
```

Regenerate typed query code after editing `backend/queries` or migrations:

```powershell
sqlc generate
git diff --exit-code -- internal/database/sqlc
```

Production image and Compose configuration, from the repository root:

```powershell
docker build --tag gradeium:phase5 .
docker compose config --quiet
docker compose up --build --detach
```

## Production image

The multi-stage image builds the Vite frontend and Go backend separately. The final Debian-family runtime contains the Gradeium binary, compiled web assets, CA certificates, and timezone data; it runs as UID/GID `10001`, has no Node or Go toolchain requirement, and exposes one HTTP port. The application image is read-only while `/config`, `/backups`, and PostgreSQL data remain persistent volumes.

## Project documentation

Read [`AGENTS.md`](./AGENTS.md) and every document in [`docs/`](./docs/) before implementation work. Phase sequencing and scope are defined in [`docs/IMPLEMENTATION_PLAN.md`](./docs/IMPLEMENTATION_PLAN.md), with the current milestone requirements in [`docs/CODEX_PHASE_5.md`](./docs/CODEX_PHASE_5.md). Gradeium remains pre-1.0 until the dedicated Phase 6 hardening and release work is complete.

## License

Gradeium is licensed under the GNU Affero General Public License v3.0. See [`LICENSE`](./LICENSE).

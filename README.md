# Gradeium

Gradeium is a long-lived, self-hosted media tracker for Games, Movies, and TV Shows. The repository currently contains the Phase 3 runtime, secure settings, and generic OpenID Connect authentication foundation: a Go API and static React frontend, PostgreSQL migrations, encrypted application secrets, one-time OIDC bootstrap, server-side sessions, CSRF protection, and a production-shaped Docker Compose stack.

Media domains, ratings/statuses, metadata providers, Jellyfin, user-management UI, and backup execution are intentionally not implemented yet.

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

Set `GRADEIUM_TEST_DATABASE_URL` to a PostgreSQL 18 URL to include isolated migration/repository integration tests. They cover Phase 2-to-Phase 3 migration, first-admin concurrency, issuer-qualified identity uniqueness, hash-only sessions, expiry/revocation, and restart persistence. CI always supplies PostgreSQL 18.4.

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
docker build --tag gradeium:phase3 .
docker compose config --quiet
docker compose up --build --detach
```

## Production image

The multi-stage image builds the Vite frontend and Go backend separately. The final Debian-family runtime contains the Gradeium binary, compiled web assets, CA certificates, and timezone data; it runs as UID/GID `10001`, has no Node or Go toolchain requirement, and exposes one HTTP port. The application image is read-only while `/config`, `/backups`, and PostgreSQL data remain persistent volumes.

## Project documentation

Read [`AGENTS.md`](./AGENTS.md) and every document in [`docs/`](./docs/) before implementation work. Phase sequencing and scope are defined in [`docs/IMPLEMENTATION_PLAN.md`](./docs/IMPLEMENTATION_PLAN.md), with the current requirements in [`docs/CODEX_PHASE_3.md`](./docs/CODEX_PHASE_3.md).

## License

Gradeium is licensed under the GNU Affero General Public License v3.0. See [`LICENSE`](./LICENSE).

# Gradeium

Gradeium is a long-lived, self-hosted media tracker for Games, Movies, and TV Shows. The repository currently contains the Phase 2 secure core-data and settings foundation: a Go API and static React frontend, PostgreSQL migrations, one-time first-run state, encrypted settings storage, and a production-shaped Docker Compose stack.

Media domains, ratings, metadata providers, OIDC login, Jellyfin, and backup execution are intentionally not implemented yet.

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

On macOS or Linux, use `cp .env.example .env` for the first command. Open [http://localhost:8080](http://localhost:8080). A fresh installation shows a minimal setup page that performs the one-time initialization and then opens Settings. It does not create a password or configure an external login or integration.

Useful endpoints are:

- `GET /api/healthz` - process liveness; independent of PostgreSQL.
- `GET /api/readyz` - readiness; checks PostgreSQL with a short timeout.
- `GET /api/setup/status` - safe first-run completion state.

PostgreSQL migrations run automatically before HTTP startup and fail closed. Inspect startup and migration logs with:

```powershell
docker compose logs gradeium postgres
```

Stop the stack without deleting persistent data:

```powershell
docker compose down
```

Add `--volumes` only when you explicitly intend to delete all local PostgreSQL, config, and backup volume data.

## Master key and disaster recovery

On the first successful boot, Gradeium creates `/config/master.key` from cryptographically secure random bytes. The file is versioned, created atomically, restricted to the application user on the Linux production runtime, and stored only in the persistent `/config` mount. It is never loaded from an environment variable or stored in PostgreSQL.

Secret settings use AES-256-GCM with a fresh nonce for every write. PostgreSQL contains only the algorithm version, nonce, authenticated ciphertext, and a one-way master-key fingerprint. APIs expose configured/not-configured state and never return saved secret values.

Treat the PostgreSQL data and `/config` mount as one recovery set:

- Back up both at the same time.
- Restoring a database without its matching config key makes encrypted settings unrecoverable.
- Restoring a config key that does not match the database is rejected at startup.
- If an established installation loses `master.key`, startup fails with a recovery error instead of generating a new key or corrupting stored settings.

Recovery requires restoring a matching PostgreSQL and `/config` pair. There is intentionally no secret-reset bypass in Phase 2.

Deleting only PostgreSQL discards setup/settings data while leaving the old key behind; a new database can deliberately register that retained key. Deleting only `/config` while keeping the established database is rejected. For an unambiguous clean development reset, delete both together.

For a complete local development reset, stop the stack and explicitly remove its disposable named volumes:

```powershell
docker compose down --volumes
```

This permanently deletes the local development database and key. The next startup is a new installation.

## Phase 2 security boundary

Phase 2 provides a clean admin-authorization middleware insertion point but does not implement OIDC, sessions, or a local password. Its setup flag is installation state, not a user identity; the later authentication phase will create or bind the initial administrator to an OIDC identity. The one-time setup transition is race-safe and cannot bootstrap twice, but the Admin Settings foundation is not a substitute for real authentication. Do not expose this phase to an untrusted network; that auth phase will replace the transparent middleware with real admin authorization.

No provider, OIDC, Jellyfin, backup schedule, or application preference belongs in `.env`. Only infrastructure required before the application can read its own settings is configured there.

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
go vet ./...
```

Set `GRADEIUM_TEST_DATABASE_URL` to a PostgreSQL 18 URL to include the isolated migration/repository integration test. CI always runs it against PostgreSQL 18.4.

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
docker build --tag gradeium:phase2 .
docker compose config --quiet
docker compose up --build --detach
```

## Production image

The multi-stage image builds the Vite frontend and Go backend separately. The final Debian-family runtime contains the Gradeium binary, compiled web assets, CA certificates, and timezone data; it runs as UID/GID `10001`, has no Node or Go toolchain requirement, and exposes one HTTP port. The application image is read-only while `/config`, `/backups`, and PostgreSQL data remain persistent volumes.

## Project documentation

Read [`AGENTS.md`](./AGENTS.md) and every document in [`docs/`](./docs/) before implementation work. Phase sequencing and scope are defined in [`docs/IMPLEMENTATION_PLAN.md`](./docs/IMPLEMENTATION_PLAN.md), with the current requirements in [`docs/CODEX_PHASE_2.md`](./docs/CODEX_PHASE_2.md).

## License

Gradeium is licensed under the GNU Affero General Public License v3.0. See [`LICENSE`](./LICENSE).

# Gradeium

Gradeium is a long-lived, self-hosted media tracker for Games, Movies, and TV Shows. The repository currently contains the Phase 1 runtime foundation: a Go API and static frontend server, a React/Vite application shell, PostgreSQL migrations, and a production-shaped Docker Compose stack.

Media domains, metadata providers, ratings, OIDC, Admin Settings, backups, and Jellyfin are intentionally not implemented in Phase 1.

## Prerequisites

- Docker Engine with Docker Compose v2 for the complete local stack.
- Go 1.26 for native backend development.
- Node.js 24 and npm for native frontend development.

## Start with Docker Compose

Copy the safe example environment file and replace its local development password placeholder:

```powershell
Copy-Item .env.example .env
docker compose up --build --detach
docker compose ps
```

On macOS or Linux, use `cp .env.example .env` for the first command. The stack creates named volumes for PostgreSQL data, `/config`, and `/backups`. PostgreSQL migrations run automatically before Gradeium starts serving traffic and startup fails if migration execution fails.

Open [http://localhost:8080](http://localhost:8080). Useful endpoints are:

- `GET /api/healthz` — process liveness; it does not depend on PostgreSQL.
- `GET /api/readyz` — application readiness; it checks PostgreSQL with a short timeout.

Inspect startup and migration logs with:

```powershell
docker compose logs gradeium postgres
```

Stop the stack without deleting persistent data:

```powershell
docker compose down
```

Add `--volumes` only when you explicitly intend to delete local PostgreSQL and Gradeium volume data.

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

Frontend, from `frontend/`:

```powershell
npm ci
npm run lint
npm run typecheck
npm run build
```

Production image and Compose configuration, from the repository root:

```powershell
docker build --tag gradeium:phase1 .
docker compose config --quiet
docker compose up --build --detach
```

## Production image

The multi-stage image builds the Vite frontend and Go backend separately. The final Debian-family runtime contains the Gradeium binary, compiled web assets, CA certificates, and timezone data; it runs as UID/GID `10001`, has no Node or Go toolchain requirement, and exposes one HTTP port. Docker uses liveness for the image health check while Compose uses database-aware readiness.

Only bootstrap infrastructure is configured through environment variables. Provider credentials and OIDC configuration must not be added to `.env`; those will be managed inside Gradeium in later phases.

## Project documentation

Read [`AGENTS.md`](./AGENTS.md) and the documents in [`docs/`](./docs/) before implementation work. Phase sequencing and scope are defined in [`docs/IMPLEMENTATION_PLAN.md`](./docs/IMPLEMENTATION_PLAN.md), with the current foundation requirements in [`docs/CODEX_PHASE_1.md`](./docs/CODEX_PHASE_1.md).

## License

Gradeium is licensed under the GNU Affero General Public License v3.0. See [`LICENSE`](./LICENSE).

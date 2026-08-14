# Codex Phase 1 — Runtime and Repository Foundation

This is the first implementation task for Gradeium. Read `AGENTS.md` and every file under `docs/` before making changes. If anything in this task conflicts with those documents, the product/architecture documents are authoritative unless this file explicitly narrows the scope.

## Objective

Create a production-quality foundation for Gradeium that can run continuously in Docker on Unraid. This phase establishes the repository structure, Go backend, React/TypeScript/Vite/shadcn frontend, PostgreSQL access and migrations, Docker image, health/readiness behavior, configuration validation, logging, graceful shutdown, CI, and basic tests.

Do **not** implement Games, Movies, TV Shows, provider APIs, OIDC/Pocket ID, Admin Settings, backups, Jellyfin, ratings, statuses, or real application data in this phase. Those belong to later vertical slices.

## Required stack

- Backend: Go 1.26, standard `net/http`, `chi` router.
- Database: PostgreSQL 18, `pgx`/`pgxpool`.
- SQL: explicit SQL and `sqlc`-ready structure. Do not introduce a general-purpose ORM.
- Migrations: versioned SQL migrations in the repository using a small, established Go migration library/tool. Prefer `goose` unless there is a concrete incompatibility.
- Frontend: React + TypeScript + Vite.
- UI: shadcn/ui, Tailwind CSS, Lucide icons.
- Production: one Gradeium application container that serves the built frontend and API, plus one PostgreSQL container.
- JavaScript package management: npm with a committed lockfile unless the existing repository already establishes another package manager.

Use current stable dependency releases that support the required toolchain. Pin dependencies via `go.mod`/`go.sum` and `package-lock.json`; do not depend on floating production versions.

## Repository structure

Establish a clear structure along these lines (minor improvements are fine if justified):

```text
backend/
  cmd/gradeium/
  internal/
    app/
    config/
    database/
    httpserver/
  migrations/
  queries/
  go.mod
  go.sum

frontend/
  src/
    app/
    components/
      ui/
      shared/
    features/
      games/
      movies/
      tv/
    lib/
  package.json
  package-lock.json
  vite.config.ts

Dockerfile
compose.yaml
.env.example
.github/workflows/ci.yml
```

The feature directories for Games/Movies/TV may exist as empty architectural boundaries, but do not implement domain functionality.

## Backend foundation

Implement a small, maintainable Go application with:

- `chi` routing.
- Structured logging using Go `log/slog`; JSON logs in production-friendly format.
- Request IDs and request logging.
- Panic recovery that logs the failure and returns a safe generic HTTP response.
- Server timeouts appropriate for an internet-facing/self-hosted web application (`ReadHeaderTimeout`, read/write/idle timeouts); do not leave them at unsafe zero defaults.
- Graceful shutdown on SIGTERM/SIGINT so Docker/Unraid restarts do not abruptly terminate requests.
- PostgreSQL connection pooling through `pgxpool` with conservative defaults suitable for a small self-hosted installation.
- Context timeouts for database health checks and other blocking operations.
- Clear package boundaries. Avoid god packages, global mutable state, and framework-style magic.

### Health endpoints

Provide separate liveness/readiness endpoints:

- `GET /api/healthz`
  - returns 200 when the process is alive and able to serve HTTP.
  - must not fail merely because PostgreSQL is temporarily unavailable.

- `GET /api/readyz`
  - verifies the application is ready to serve normal requests.
  - checks PostgreSQL connectivity with a short timeout.
  - returns a non-2xx status when dependencies required for normal operation are unavailable.

Responses should be small JSON objects suitable for Docker health checks. Do not expose credentials, stack traces, connection strings, or other secrets.

## Database and migrations

Create the PostgreSQL foundation without adding Gradeium media tables yet.

Requirements:

- Versioned migration directory committed to Git.
- A minimal initial migration proving the migration system works. It may create only infrastructure/schema metadata needed by the application; do not prematurely design the media domain here.
- Migration failures must stop startup cleanly and visibly rather than allowing the app to run against a mismatched schema.
- Concurrent starts must not corrupt or race migrations. Use the migration tool's locking facilities or an appropriate PostgreSQL advisory-lock strategy.
- Database errors must be logged clearly without logging passwords/credentials.

The application may automatically apply **committed, versioned** migrations at startup if this is implemented deterministically and fails closed. This is not permission for runtime schema generation or ORM auto-sync.

## Configuration

Keep external configuration deliberately small.

Phase 1 may use environment variables only for infrastructure/bootstrap values that are required before Gradeium can load its own settings, such as:

- database connection information;
- listen address/port if necessary;
- `/config` and `/backups` paths if necessary;
- development/logging mode.

Do **not** add environment variables for IGDB, TMDB, TVDB, OIDC/Pocket ID, Jellyfin, or future integrations. Per the project architecture, those will be entered from Gradeium's Admin Settings UI in later phases.

Configuration must be validated at startup. Missing/invalid required infrastructure values should produce a concise actionable error and exit non-zero.

Never commit secrets. Provide a safe `.env.example` containing placeholders only.

## Persistent paths

Prepare the application and Compose definition for these persistent mounts:

- `/config` — Gradeium-owned bootstrap/security state in later phases.
- `/backups` — portable application backups in later phases.
- PostgreSQL's data directory — persistent database state.

Phase 1 does not need to implement backups or credential encryption, but the container filesystem/layout must not make those later requirements awkward.

## Frontend foundation

Initialize React + TypeScript + Vite and shadcn/ui correctly.

Requirements:

- shadcn-oriented clean visual foundation as defined in `docs/DESIGN.md`.
- Tailwind configured using the current supported shadcn approach.
- Lucide for icons.
- Responsive application shell.
- A restrained sidebar/navigation shell with entries for Dashboard, Games, Movies, TV Shows, and Settings is acceptable, but these are placeholders only.
- No fake ratings, fake charts, fake media cards, stock dashboard numbers, decorative gradients, glassmorphism, or invented data.
- Placeholder pages should be intentionally minimal and clearly unfinished rather than pretending functionality exists.
- Loading/error/empty-state primitives may be introduced where useful, but avoid premature abstraction.
- Accessibility basics: semantic controls, visible keyboard focus, proper labels, sensible contrast.

The production Go server must serve the compiled frontend and support SPA history fallback without intercepting `/api/*` routes.

During development, Vite may run separately with a proxy to the Go backend.

## Docker and Unraid

Create a reliable multi-stage `Dockerfile`:

1. Node build stage builds the React/Vite frontend.
2. Go build stage compiles the backend.
3. Final runtime stage contains only what production needs.

Reliability/security requirements:

- Prefer a stable minimal Debian-family runtime over Alpine if it avoids musl/compatibility surprises.
- Run the Gradeium process as a non-root user.
- Do not ship source trees, `node_modules`, Go compiler, or Vite tooling in the final runtime image.
- Ensure CA certificates are available for future HTTPS provider calls.
- Support clean SIGTERM handling.
- Add a Docker `HEALTHCHECK` using the liveness/readiness design appropriately.
- Container startup should fail if migrations fail.

Create a `compose.yaml` suitable for local testing and straightforward adaptation to Unraid:

- `gradeium` service.
- `postgres` service using PostgreSQL 18.
- persistent named/local volumes for development.
- health checks and dependency readiness.
- `restart: unless-stopped` for production-like behavior.
- no unnecessary Redis, queue, search, worker, proxy, or other containers.

Do not bake PostgreSQL data into the app container.

## Security headers and HTTP behavior

Add a small, understandable middleware layer for generally safe headers where they do not break the SPA, e.g. MIME sniffing protection and a reasonable referrer policy. Do not invent a complex CSP before the actual frontend/integration needs are known.

Do not enable permissive CORS by default; production frontend and API are same-origin.

## CI and quality gates

Create GitHub Actions CI that runs on pull requests and pushes to `main`.

At minimum verify:

Backend:
- `go test ./...`
- `go vet ./...`
- formatting is clean (`gofmt` check or equivalent)

Frontend:
- `npm ci`
- lint
- TypeScript typecheck
- production build

Container:
- production Docker image builds successfully.

Prefer separate jobs with dependency caching where reasonable, but keep CI understandable and maintainable.

## Tests

Add meaningful foundation tests rather than placeholder assertions.

At minimum:

- backend tests for liveness handler;
- readiness behavior for healthy/unhealthy dependency state using an injectable readiness checker rather than requiring a live database for every unit test;
- configuration validation tests for at least required/missing values;
- SPA/API routing behavior where practical;
- frontend should at least pass typecheck/lint/build; add a small unit test only if a test framework is introduced for a real reason.

Do not add large test frameworks merely to inflate coverage.

## Documentation

Update `README.md` with concise development instructions:

- prerequisites;
- local startup using Compose;
- frontend/backend development commands;
- test/lint/build commands;
- service URLs/health endpoints;
- production image overview;
- reminder that media providers and OIDC are intentionally not configured in Phase 1.

If implementation choices materially refine architecture, update the relevant docs, but do not silently contradict them.

## Non-goals / forbidden scope for Phase 1

Do not implement any of the following:

- IGDB, TMDB, or TVDB clients.
- Games, Movies, TV Shows, ratings, statuses, artwork or episode progress.
- OIDC/Pocket ID or user accounts.
- Admin Settings or the first-run setup UI.
- credential encryption/master-key behavior.
- backup creation/restore.
- Jellyfin.
- dashboards with real or fake statistics.
- custom lists/favorites.
- Redis, message queues, Elasticsearch, GraphQL, microservices, Kubernetes.

If you notice something necessary for one of these later phases, document the extension point rather than implementing the feature now.

## Acceptance criteria

Phase 1 is complete only when all of the following are true:

1. A fresh clone can be brought up with documented Compose commands.
2. PostgreSQL becomes healthy and Gradeium starts successfully after migrations.
3. The Gradeium container reaches healthy status.
4. `/api/healthz` and `/api/readyz` behave distinctly and correctly.
5. The browser loads the compiled React/shadcn application through the Go server in production mode.
6. Refreshing a client-side route does not produce a 404.
7. Stopping/restarting the container triggers graceful application shutdown and preserves PostgreSQL data.
8. The production container runs non-root and contains no Node development runtime/toolchain requirement.
9. CI passes backend tests/vet/format checks, frontend lint/typecheck/build, and Docker build.
10. No provider, OIDC, Jellyfin, media-domain, backup, or fake dashboard functionality has slipped into the phase.
11. No secrets are committed and provider credentials are not represented as environment variables.
12. Repository structure follows the modular boundaries in the project documentation.

## Codex execution instructions

Work in a dedicated branch and deliver the result as a pull request rather than pushing directly to `main`.

Before finishing:

1. Run all documented backend quality checks.
2. Run all documented frontend quality checks.
3. Build the production Docker image.
4. Bring the Compose stack up and verify health/readiness if the execution environment permits Docker.
5. Inspect logs for startup/migration errors.
6. Summarize exactly what was implemented, what commands passed, and any verification that could not be run because of execution-environment limitations.

Do not claim checks passed unless they were actually executed.

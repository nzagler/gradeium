# Gradeium Architecture

## Goals
Gradeium should be lightweight, stable, understandable, and maintainable for long-term self-hosting on Unraid.

Prefer a small number of boring components with clear boundaries.

## Runtime topology
Production target:

1. `gradeium` application container
   - compiled Go backend;
   - serves REST API;
   - serves built React/Vite static frontend;
   - runs lightweight internal schedulers for metadata refresh and backups;
   - handles OIDC callbacks and future integration webhooks.
2. `postgres` container
   - PostgreSQL persistent database.

No other runtime services are required for v1.

Do not add Redis, message brokers, object storage, Elasticsearch, or separate worker containers without a demonstrated requirement.

## Frontend
- React
- TypeScript
- Vite
- shadcn/ui
- Tailwind CSS
- Lucide icons

The application is authenticated and self-hosted; SEO/SSR is not a product goal. Build the frontend to static assets and serve them from the Go application container.

### Frontend organization
Suggested shape:

```text
frontend/src/
  app/
  features/
    games/
    movies/
    tv/
    dashboard/
    settings/
    backups/
    auth/
  components/
    ui/
    shared/
  lib/
  api/
```

Feature/domain modules should own their own domain-specific components, queries/hooks, validation, and view models.

## Backend
- Go
- standard `net/http`
- `chi` router
- `pgx`
- `sqlc`
- explicit SQL migrations

Suggested shape:

```text
backend/
  cmd/gradeium/
  internal/
    app/
    auth/
    config/
    crypto/
    database/
    entities/
    games/
    movies/
    tv/
    dashboard/
    backups/
    settings/
    integrations/
      igdb/
      tmdb/
      tvdb/
      jellyfin/
    httpapi/
    jobs/
```

Games, Movies, and TV must remain separate domain modules.

## HTTP API
Use a straightforward JSON REST API under `/api`.

Potential route groups:
- `/api/auth/*`
- `/api/games/*`
- `/api/movies/*`
- `/api/tv/*`
- `/api/dashboard/*`
- `/api/settings/*`
- `/api/admin/*`
- `/api/backups/*`
- `/api/integrations/*`
- `/api/health`
- `/api/ready`

Do not expose provider API response structures directly.

Return Gradeium-owned response DTOs.

## Provider boundaries
Each provider gets a dedicated adapter/client.

### IGDB
Responsibilities:
- game search;
- game metadata;
- cover/artwork/screenshots;
- companies/developers/publishers;
- game type and relationships;
- DLC/expansion relationships;
- community user rating.

### TMDB
Responsibilities:
- movie search and movie metadata;
- movie artwork/cast/crew/collections/trailers;
- movie community rating;
- strict TVDB external-ID lookup and reverse verification for TV community-rating association;
- TV community rating after verified mapping.

TMDB must not become the authoritative TV metadata source.

### TVDB
Responsibilities:
- TV search and series metadata;
- seasons and episodes;
- series artwork;
- network/company/cast/character metadata where used.

### Jellyfin
Future integration boundary only in the initial architecture.
Core Gradeium logic must not depend on Jellyfin being configured or reachable.

## Persistence
PostgreSQL is authoritative for Gradeium data.

Provider metadata is cached locally so library browsing works independently of provider uptime.

User-owned data and provider metadata should be separated conceptually and in schema where practical.

## Global entity identity
Use a small shared entity registry to guarantee a UUID belongs to one domain.

Conceptual model:

```text
entities
  id UUIDv7 PRIMARY KEY
  type GAME | MOVIE | TV_SHOW
```

Domain tables reference the entity ID 1:1:

```text
games.entity_id -> entities.id
movies.entity_id -> entities.id
tv_shows.entity_id -> entities.id
```

Do not store all media metadata in `entities`.

This registry also provides a clean future basis for mixed-media custom lists.

## User-owned state
Personal state should be separate from provider/domain metadata.

Conceptually:
- `user_games`
- `user_movies`
- `user_tv_shows`
- `user_episode_progress`

This allows eventual multi-user support without duplicating provider metadata.

## Authentication
Authentication is generic OpenID Connect.
Pocket ID is the primary supported/tested provider.

Do not hard-code Pocket ID-specific login behavior into core auth.

### Bootstrap problem
Gradeium needs a secure way to reach Admin Settings before external OIDC is configured.

The implementation must provide a protected first-run bootstrap path. A recommended model is:

1. On a fresh database, Gradeium enters `setup_required` state.
2. The UI routes unauthenticated users only to a one-time setup wizard.
3. The setup wizard establishes the initial local administrator/bootstrap identity or another secure one-time admin mechanism.
4. The administrator configures OIDC/Pocket ID in the UI.
5. Gradeium tests the OIDC configuration before activation.
6. Once external OIDC is working, the bootstrap mechanism is disabled or tightly restricted.

The exact mechanism may be refined during implementation, but there must never be a permanently open unauthenticated Admin Settings route.

## Admin-managed integration configuration
Provider/integration configuration belongs in PostgreSQL through the Admin Settings UI.

Examples:
- IGDB/Twitch client ID and client secret;
- TMDB API token/key;
- TVDB API key and any provider-required credentials;
- generic OIDC issuer/discovery URL, client ID, client secret, scopes where configurable;
- Jellyfin base URL/API key in future;
- backup interval/retention;
- future integrations.

Do not require users to edit application environment variables for these after the app boots.

### Secret encryption at rest
Sensitive integration values must not be stored plaintext in PostgreSQL.

Use authenticated encryption with a strong, standard primitive available in Go (for example AES-256-GCM or XChaCha20-Poly1305 through a well-established package).

A data-encryption master key must exist outside the database.

Preferred long-lived self-hosted approach:
- Gradeium has a persistent `/config` mount;
- if no master key exists on first boot, Gradeium generates a cryptographically secure key;
- stores it in `/config` with restrictive filesystem permissions;
- uses it to encrypt/decrypt integration secrets;
- never stores that master key in PostgreSQL.

This minimizes manual bootstrap configuration while keeping database dumps alone insufficient to expose provider secrets.

The `/config` key file must be included in disaster-recovery guidance. Do not include raw decrypted secrets in portable user-data backups.

### Secret API behavior
- Accept secret when creating/replacing configuration.
- Never return the saved secret value to the frontend.
- Return only state such as `configured: true`, masked metadata where safe, and last test status.
- Allow explicit replacement and removal.
- Never log secret values.

## Metadata refresh
Store `metadata_refreshed_at` or equivalent timestamps.

Refresh provider metadata asynchronously from ordinary page rendering whenever possible.

Requirements:
- stale metadata must not block reading local library content;
- provider failure records an integration/refresh error but preserves existing metadata;
- manually pinned artwork survives refresh;
- use provider rate limits responsibly;
- avoid refresh storms after restart.

A small internal scheduler is sufficient for v1.

## Backups
Portable Gradeium backups run inside the app process via a lightweight scheduler.

The scheduler must be persistent-state-aware rather than relying on `sleep(72h)` semantics.

Conceptual flow:
1. Read backup schedule and `last_successful_backup_at` from PostgreSQL.
2. If a backup is due or overdue, enqueue/run one bounded backup job.
3. Write to a temporary file.
4. Validate output.
5. Calculate checksum.
6. Atomically rename to final backup name.
7. Persist successful backup metadata.
8. Apply retention policy.

Mount `/backups` to persistent Unraid storage.

Restore must create a pre-restore safety backup before mutating current personal state.

## Reliability
### HTTP server
- explicit read/write/idle/header timeouts;
- bounded request sizes on admin/backup upload endpoints;
- graceful shutdown on SIGTERM/SIGINT;
- structured logs;
- request correlation/request IDs where useful.

### Database
- sensible connection-pool limits;
- readiness fails when required DB connectivity is unavailable;
- liveness should not depend on transient external providers;
- use transactions for multi-record state changes;
- enforce uniqueness and rating/status constraints in DB where practical.

### External providers
- context timeouts on every request;
- bounded safe retries for transient failures;
- exponential backoff/jitter where applicable;
- provider-specific rate-limit awareness;
- never retry non-idempotent operations blindly;
- no external provider outage should crash the process.

### Background jobs
- no unbounded goroutine spawning;
- avoid running duplicate copies of the same scheduled job concurrently;
- record success/failure timestamps and diagnostic message without secrets;
- jobs must not monopolize DB connections or block interactive HTTP traffic.

## Docker
Use a multi-stage image:
1. Node build stage builds React/Vite frontend.
2. Go build stage compiles backend.
3. Minimal runtime image contains only the Go binary, frontend assets, required CA certificates/timezone data, and minimal runtime necessities.

Run as non-root where practical.

Expose one application port.

Provide Docker healthcheck against the liveness endpoint.

### Persistent mounts
Suggested:
- `/config` — master encryption key and small app-local bootstrap files;
- `/backups` — portable backups;
- PostgreSQL's own persistent data volume/directory.

The Gradeium application image itself must be disposable.

## Unraid
The project should eventually provide clear Unraid deployment documentation and an Unraid-friendly container configuration/template if appropriate.

Expected persistent host locations may resemble:

```text
/mnt/user/appdata/gradeium/config
/mnt/user/appdata/gradeium/backups
/mnt/user/appdata/gradeium/postgres
```

Exact host paths remain user-configurable.

## Versioning and upgrades
- Publish immutable versioned container tags.
- `latest` may exist as convenience but must not be the only tag.
- Keep schema migrations forward-managed and explicit.
- The app must surface its application version in an About/System area and logs.
- Upgrades should fail clearly rather than silently continuing on incompatible schema state.

## Testing priorities
High-value automated tests include:
- rating range/status rules;
- Backlog transition behavior;
- provider duplicate prevention;
- strict TVDB↔TMDB cross-ID verification;
- specials excluded from TV progress;
- bulk episode progress operations;
- artwork pin preservation through refresh;
- backup serialization/validation/restore;
- encrypted-secret persistence and redaction;
- provider failure does not break local reads.

# Gradeium — Codex Instructions

## Mission
Gradeium is a long-lived, self-hosted media tracker for Games, Movies, and TV Shows. It prioritizes reliability, clean UX, explicit data ownership, and a restrained shadcn-oriented design.

## Core stack
- Backend: Go (current stable release supported by CI), standard `net/http` plus `chi`.
- Frontend: React + TypeScript + Vite.
- UI: shadcn/ui + Tailwind CSS + Lucide icons.
- Database: PostgreSQL.
- DB access: `pgx` + `sqlc`.
- Migrations: explicit versioned SQL migrations.
- Auth: generic OpenID Connect; Pocket ID is the primary supported provider.
- Deployment: one Gradeium application container serving the built frontend + one PostgreSQL container.
- Production target: Docker on Unraid.

## Architecture rules
- Treat Games, Movies, and TV Shows as separate feature domains on both backend and frontend.
- Do not build a generic all-purpose `MediaItem` model containing all domain metadata.
- Shared infrastructure may be shared: users, auth, settings, backup machinery, global entity identity, common UI primitives, logging, health checks.
- Provider adapters must be isolated from domain/business logic.
- Provider DTOs must not leak through the application. Map them to Gradeium-owned types.
- All persisted media entities use Gradeium-owned UUIDv7 identifiers. Provider IDs are external references only.
- Prefer explicit SQL and clear constraints over ORM-like magic.
- Avoid premature infrastructure: no Redis, Kafka, RabbitMQ, Elasticsearch, GraphQL, microservices, Kubernetes, or separate workers unless a concrete requirement later justifies them.

## Reliability rules
- Gradeium is intended to run continuously for years.
- External provider failures must never prevent access to already stored local library data.
- Every outbound network call must use sensible timeouts and bounded retries only where safe.
- Implement graceful shutdown.
- Provide liveness and readiness endpoints.
- Database migrations must be explicit and reviewed.
- Production containers must run as non-root where practical.
- Avoid hidden background behavior that can block interactive requests.
- Prefer simple, deterministic behavior over cleverness.

## Product rules
- Global statuses: `BACKLOG`, `IN_PROGRESS`, `ON_HOLD`, `ABANDONED`, `COMPLETED`.
- The same names are used for Games, Movies, and TV Shows.
- Backlog is a separate view and cannot be rated.
- Ratings are 1.0–10.0 in 0.1 increments, stored as integers 10–100. `NULL` means unrated.
- A rating can have one optional `rating_reason`.
- Moving an item to Backlog removes its rating and reason after confirmation.
- No rating history, completion history, rewatch/replay history, completion dates, or played-platform tracking.
- Removing an item permanently removes the user's associated personal data after confirmation.
- Default Library sort is My Rating high-to-low; unrated items sort after rated items. The default sort is configurable in Settings.
- Rating must also be possible directly from Library views.
- Existing provider-search results already in Gradeium show `Already in Library` or `Already in Backlog` and never show an Add action.

## Metadata sources
- Games: IGDB.
- Movies: TMDB.
- TV Shows: TVDB.
- TV community rating: TMDB only after strict TVDB↔TMDB cross-ID verification. Never fuzzy-match titles.
- Games community rating: IGDB user rating.
- Movies community rating: TMDB user rating.
- Community ratings are secondary to the user's own rating.

## Games rules
- Remakes and remasters are separate ratable games.
- Content requiring the base game to play is nested under the base game's information page and is not independently ratable.
- Independently playable releases may be separate games.
- Editions, packs, updates, DLC, and non-standalone expansions must not become duplicate library entries.
- No age ratings in the v1 UI.

## TV rules
- Only the whole show is ratable. Seasons and episodes are never ratable.
- Track watched progress per episode.
- Season 0 / Specials may be tracked but never count toward overall regular-episode progress.
- Support marking one episode, a whole season, all episodes through a selected episode, and the whole series watched/unwatched where appropriate.
- Episode progress survives status changes, including moving to Backlog.
- Use TVDB's normal/default aired ordering as canonical in v1.

## Artwork rules
- Artwork is provider-owned; users choose among images supplied by that metadata provider.
- No arbitrary external image URLs or user uploads in v1.
- Manage Poster/Cover, Backdrop, and Logo independently when the provider offers them.
- Provider default/preferred artwork is selected initially.
- A manually selected artwork choice is pinned and must survive automatic metadata refreshes.

## Configuration and Admin Settings
- External-service credentials must be configured in Gradeium itself, not by editing environment files.
- Provide a protected first-run setup experience and an Admin Settings area.
- Admin Settings must support configuration/test/status for IGDB, TMDB, TVDB, generic OIDC/Pocket ID, Jellyfin (when implemented), backup schedule/retention, and future integrations.
- Store secrets encrypted at rest. Never return secret values to the browser after saving; show configured/not-configured state and allow replacement/removal.
- Minimize bootstrap configuration required before the UI can start. Infrastructure bootstrap configuration is the only acceptable exception to UI-managed settings.

## Backups
- Portable personal-data backups are a v1 requirement.
- Default automatic schedule: every 3 days, enabled, keep latest 30.
- Settings must allow changing interval and retention.
- Backup user-owned data: statuses, ratings, rating reasons, TV episode progress, artwork selections, user settings, and future user-created lists.
- Use a versioned portable format; gzip compression is appropriate.
- Backup writes must be atomic and validated before becoming visible as valid backups.
- Provide Create Now, Download, Restore, Delete, and automatic retention.
- Create a safety backup immediately before restore.
- Also provide a simple CSV export of ratings/library state.

## UI / UX
- Design language: modern, restrained, shadcn-oriented, content-first, artwork-led, responsive.
- Do not add decorative or fake UI to fill space.
- Every visible element must communicate useful information or provide a useful action.
- Avoid unnecessary gradients, glassmorphism, oversized hero sections, badge clutter, fake trends, and excessive animation.
- Keep cards clean. Detail pages can be richer.
- Use proper loading, empty, error, keyboard, focus, and mobile states.
- Preserve Library filter/sort state in URLs where practical and preserve scroll position when returning from a detail page.

## Implementation discipline
- Implement incrementally in vertical slices.
- Do not attempt to build the whole application in one task.
- Foundation first, then authentication/database, then Games end-to-end, Movies, TV, Dashboard, Backups/Settings, and polish.
- Before each task, inspect existing architecture and relevant docs under `docs/`.
- Add tests for non-trivial business rules and integration mappings.
- Run formatting, lint/type checks, backend tests, frontend tests where present, and production builds before declaring work complete.
- Do not weaken requirements silently. If a requirement conflicts with implementation reality, document the tradeoff and request a product decision.
# Gradeium Implementation Plan

The goal is to keep Codex tasks small enough to review, test, and correct before assumptions spread across the project.

Do not ask Codex to `build Gradeium` in one task.

## Phase 0 — Specification foundation
Status: current.

Deliverables:
- `AGENTS.md`;
- Product specification;
- Architecture specification;
- Design specification;
- Data-model specification;
- Implementation plan.

No application code in this phase.

## Phase 1 — Repository and runtime foundation
Goal: a boring, production-shaped skeleton that boots in Docker before implementing product features.

Deliverables:
- Go backend module and server entry point;
- `chi` routing;
- React + TypeScript + Vite frontend;
- shadcn/ui and Tailwind baseline;
- production multi-stage Dockerfile;
- `compose.yaml` with Gradeium + PostgreSQL;
- persistent `/config` and `/backups` mounts;
- PostgreSQL connectivity through `pgx`;
- `sqlc` configuration;
- migration tooling and first system migration;
- structured logging;
- graceful shutdown;
- `/api/health` and `/api/ready`;
- frontend application shell/sidebar placeholder;
- configuration validation;
- generated-at-first-run master encryption key in `/config` with tests;
- lint/format/test/build commands;
- CI workflow for backend tests, frontend checks, and production builds;
- `.env.example` only for unavoidable infrastructure/bootstrap values, not provider credentials.

Explicitly do not implement:
- Games;
- Movies;
- TV;
- provider APIs;
- OIDC;
- backups beyond mounts/foundation;
- dashboard.

Acceptance criteria:
- `docker compose up --build` boots from a clean checkout;
- DB migrations run in a controlled manner;
- readiness becomes healthy when PostgreSQL is usable;
- frontend loads from the Go application in production image;
- container handles SIGTERM cleanly;
- no provider secrets are expected in environment variables.

## Phase 2 — First-run bootstrap + Admin Settings framework
Goal: all user-supplied service configuration can later happen from Gradeium's UI.

Deliverables:
- fresh-install `setup_required` state;
- protected one-time bootstrap admin flow;
- Settings/Admin shell;
- Integrations page/cards framework;
- typed integration configuration persistence;
- authenticated encryption/decryption service using `/config` master key;
- redacted secret API behavior;
- generic `Test Connection` status model;
- setup completion state;
- lock down setup endpoints after completion;
- audit-safe structured logs without secrets.

Acceptance criteria:
- a clean install can reach a secure setup UI without editing provider `.env` values;
- secrets are ciphertext in PostgreSQL;
- secret values are never returned by GET APIs;
- restart preserves encrypted settings because `/config` key persists;
- removing `/config` key causes a clear recovery error rather than corrupt behavior.

## Phase 3 — Generic OIDC / Pocket ID
Goal: replace bootstrap access with normal OIDC authentication.

Deliverables:
- Admin Settings form for:
  - issuer/discovery URL;
  - client ID;
  - client secret;
  - optional scopes/settings where needed;
- validation and Test Connection/metadata discovery;
- generic OIDC authorization-code flow;
- Pocket ID documented/tested as primary provider;
- initial administrator maps to OIDC subject safely;
- session handling;
- logout;
- admin authorization middleware;
- bootstrap mechanism disabled/restricted once OIDC is confirmed.

Acceptance criteria:
- OIDC can be configured entirely in Admin Settings;
- Pocket ID login works;
- no client secret is sent to the browser after saving;
- changing a bad OIDC setting cannot permanently lock an administrator out without a documented recovery route.

## Phase 4 — Core media/user schema and shared library behavior
Goal: establish shared invariants before a provider vertical slice.

Deliverables:
- global entity UUIDv7 registry;
- users;
- status model;
- user settings including default Library sort;
- common rating/rating-reason constraints;
- shared provider artwork-selection concepts;
- common delete/status/rating domain helpers;
- API conventions/errors/pagination/search response structure;
- reusable frontend Library shell, rating editor, status controls, confirmation dialogs.

Acceptance criteria:
- Backlog rating invariant enforced/tested;
- moving to Backlog clears rating/reason transactionally;
- default sort setting is persisted;
- internal provider IDs never become user-facing primary identifiers.

## Phase 5 — Games complete vertical slice
Goal: make Games genuinely usable before copying architecture into other domains.

Deliverables:
### Admin
- IGDB configuration in Admin Settings;
- credential validation/Test Connection.

### Provider
- IGDB client with timeouts/rate handling;
- English search;
- provider-to-Gradeium mapping;
- game-type normalization;
- editions/DLC/expansion filtering rules;
- community user rating;
- artwork/screenshots;
- related releases/franchise data used by UI.

### Persistence
- games/provider metadata cache;
- user game state;
- artwork alternatives/selections;
- required relationships/indexes.

### UX
- Games Library;
- Games Backlog;
- local search/filter/sort;
- provider Search/Add;
- duplicate detection;
- rating directly in Library;
- Game detail page;
- artwork manager;
- metadata refresh;
- error/loading/empty/mobile states.

Acceptance criteria:
- existing local Games remain usable while IGDB is unavailable;
- duplicate provider IDs cannot be added twice;
- required-base-game DLC does not become an independently ratable entry;
- remakes/remasters can be separate Games;
- manual artwork remains pinned through refresh;
- default Library sort is My Rating descending unless user setting changes it.

## Phase 6 — Movies
Goal: implement Movies using the proven architecture without genericizing Games/Movies into one domain.

Deliverables:
- TMDB Admin Settings integration + Test Connection;
- movie search/metadata/cache;
- Movie Library/Backlog/Search/Add;
- Movie detail page;
- cast/key crew;
- collections;
- trailers;
- artwork management;
- TMDB community rating;
- rating in Library;
- filters/sorts/errors/loading/mobile.

Acceptance criteria mirror the Games reliability/duplicate/artwork behavior where applicable.

## Phase 7 — TV Shows
Goal: implement TVDB-authoritative TV tracking plus strict TMDB community-rating mapping.

Deliverables:
### Admin
- TVDB integration configuration + test.
- Reuse configured TMDB for rating bridge.

### Provider/data
- TVDB show search/metadata;
- seasons/episodes in default aired ordering;
- specials classification;
- artwork/cast/network data needed by UI;
- strict TMDB lookup by TVDB external ID;
- reverse external-ID verification;
- verified mapping persistence;
- TMDB community score only after verification.

### UX
- TV Library/Backlog/Search/Add;
- TV detail page;
- overall regular-episode progress;
- next unwatched episode;
- collapsible seasons;
- episode watched controls;
- mark season watched/unwatched;
- mark all through selected episode watched;
- whole-series bulk action;
- Specials section excluded from overall progress;
- artwork manager;
- rating in Library.

Acceptance criteria:
- no season/episode rating fields/actions;
- fuzzy TVDB↔TMDB matching does not exist;
- specials never influence regular progress percentage;
- episode progress survives status changes/backlog transition;
- deleting user's TV item removes its episode progress.

## Phase 8 — Backups and data portability
Goal: protect the personal data Gradeium exists to preserve.

Deliverables:
- Settings → Backups;
- automatic backup scheduler;
- default enabled/every 3 days/retain 30;
- configurable interval/retention;
- versioned portable backup format;
- gzip output;
- checksum;
- atomic temp→validated→rename creation;
- persistent backup inventory;
- Create Now;
- Download;
- Delete;
- Restore;
- automatic pre-restore backup;
- overdue backup runs safely after downtime/restart;
- CSV export of ratings/library state.

Acceptance criteria:
- interrupted backup never appears as valid completed backup;
- restore validates format/checksum before mutation;
- portable backup excludes decrypted integration secrets;
- restore tests cover statuses/ratings/reasons/artwork selections/TV progress/settings.

## Phase 9 — Dashboard
Goal: useful current-state overview without invented activity metrics.

Deliverables:
- totals by domain;
- In Progress items;
- average user ratings;
- rating distribution;
- status distribution;
- TV progress summaries;
- highest-rated items;
- Backlog counts;
- All/Games/Movies/TV filtering where useful;
- clean responsive shadcn-oriented charts/cards.

No completion-over-time charts unless history tracking becomes a future explicit product decision.

## Phase 10 — Reliability and release hardening
Goal: prepare first long-running Unraid deployment.

Deliverables:
- provider outage/failure tests;
- DB restart/reconnect behavior;
- scheduler duplicate-job protections;
- rate-limit behavior;
- request limits;
- security headers/cookie review;
- secret-redaction audit;
- dependency/license review;
- Docker non-root verification;
- Unraid deployment docs;
- backup/restore disaster-recovery guide including `/config` master key;
- version/build info page;
- immutable Docker version tags/release pipeline;
- upgrade/migration documentation.

## Future phases
Not v1 blockers unless reprioritized:
- Jellyfin integration;
- user-defined mixed-media lists;
- favorites;
- external imports;
- per-season artwork selection;
- alternate TV episode orders;
- multi-user UX refinements;
- full PostgreSQL disaster-recovery backup automation beyond portable Gradeium backups.

## Jellyfin future integration guidance
When scheduled:
- add configuration entirely through Admin Settings;
- support base URL and API key/token stored encrypted;
- Test Connection before enabling;
- isolate in `integrations/jellyfin`;
- use timeouts and tolerate Jellyfin downtime;
- consider official Jellyfin webhook plugin/API capabilities for events;
- never make Jellyfin a dependency for opening or using the Gradeium library.

# Codex Phase 6 — Theme Completion, 1.0 Hardening, and Release Readiness

## Purpose

Phase 6 is the final planned pre-1.0 implementation milestone for Gradeium.

Do not use this phase to add broad new product functionality. The product surface from Phases 1–5 is already the intended 1.0 feature set. Phase 6 exists to:

1. complete the missing application theme behavior;
2. harden the complete product as one integrated system;
3. fix defects discovered during realistic use and failure testing;
4. make installation, upgrade, recovery, and long-running self-hosted operation trustworthy;
5. leave `main` ready to be released as Gradeium 1.0 after review.

Jellyfin is explicitly post-1.0 and must not be implemented or scaffolded here.

---

## Locked product boundaries

Preserve all established Gradeium rules and Phase 1–5 behavior.

### Included 1.0 product

- generic OIDC authentication, with Pocket ID compatibility through standards-based OIDC;
- PostgreSQL-backed sessions, CSRF/origin protection, real admin authorization;
- encrypted provider/OIDC secrets backed by `/config/master.key`;
- Games through IGDB;
- Movies through TMDB;
- TV Shows through TVDB, with strictly verified TVDB→TMDB community-score mapping;
- separate Games, Movies, and TV domain models;
- Library and Backlog;
- manual statuses;
- 1.0–10.0 personal ratings in 0.1 steps and optional private rating reason;
- TV season/episode progress, with Specials excluded from regular progress percentages;
- provider artwork selection/pinning;
- Dashboard based only on current persisted Gradeium state;
- portable application backups, transactional restore, scheduling/retention, and ratings CSV export;
- General, Authentication, Integrations, Library, Backup, and System settings.

### Explicitly not part of 1.0

Do not implement or scaffold:

- Jellyfin;
- custom lists;
- favorites;
- imports;
- recommendations;
- social features;
- public profiles;
- review feeds;
- user-management UI;
- local-password authentication;
- LDAP/SAML;
- public APIs/API keys;
- replay/rewatch counters;
- user-facing watch diary/history;
- user-facing episode watched-at dates;
- mobile apps;
- additional metadata providers;
- queues, Redis, sidecars, or additional runtime services.

Internal timestamps already persisted by earlier phases may remain internal. Do not expose a watch-history UI in 1.0.

---

# 1. Appearance / theme completion

This is the only intentional product-surface addition in Phase 6.

## Required behavior

Gradeium must no longer be light-only.

Preferred implementation:

- `Dark`
- `Light`
- `System`

with **Dark as the default for new users**.

Use the existing React/Tailwind/shadcn architecture and the standard class-based dark-mode pattern. Do not create separate independently styled dark and light applications.

If supporting all three choices would require disproportionate architectural complexity, a polished **dark-only 1.0** is acceptable. Light-only is not acceptable.

Unless a real technical blocker exists, implement Dark/Light/System.

## Persistence

Theme preference is per user and must survive:

- navigation;
- reload;
- logout/login;
- app restart;
- backup/restore where user preferences are portable.

Extend the existing user preference storage rather than creating a parallel settings mechanism.

For pre-existing users with no stored theme preference, resolve to **Dark**.

## Initial paint / no flash

Apply the stored/default theme before the main application visibly renders.

Avoid an obvious white flash when a dark/default-dark Gradeium installation is loaded. A small inline bootstrap script in the application shell is acceptable if necessary, but it must be CSP-compatible and must not introduce unsafe-eval or broadly weaken CSP.

For `System`, react to `prefers-color-scheme` changes while the application is open.

## Coverage

Verify theme behavior across the whole product, including:

- first-run setup;
- login/authentication states;
- application shell/sidebar/mobile navigation;
- Dashboard and charts;
- Games Library/Backlog/Add/detail/artwork;
- Movies Library/Backlog/Add/detail/artwork/trailer;
- TV Library/Backlog/Add/detail/seasons/episodes/progress/artwork;
- General, Authentication, Integrations, Library, Backups, and System settings;
- dialogs, sheets, menus, popovers, selects, inputs, buttons, tables/lists, cards, badges, toasts;
- skeletons/loading states;
- empty states;
- error states;
- confirmations;
- focus/hover/disabled states.

Dark mode should be restrained and neutral, with media artwork providing most visual character. Do not add gradients, neon glow, glassmorphism, decorative blobs, or gratuitous animation.

Maintain accessible contrast and visible keyboard focus.

---

# 2. Whole-product regression pass

Treat Gradeium as one complete application rather than testing only newly changed files.

Exercise all core user journeys from a fresh install and from an upgraded Phase 5 database.

At minimum verify:

1. setup → OIDC configuration → activation → first-admin login;
2. logout/login and session persistence/revocation;
3. admin and non-admin authorization boundaries;
4. configure/test/replace provider credentials;
5. provider secret redaction;
6. search/add Games, Movies, and TV;
7. duplicate-add behavior;
8. move between Backlog and normal statuses;
9. rating/unrating and rating reasons;
10. rated → Backlog confirmation and rating deletion;
11. local Library/Backlog access while providers are down;
12. sorting, filtering, local search, grid/list preference;
13. metadata refresh and artwork pin preservation/fallback;
14. movie collections and trailer behavior;
15. TV individual episode, season, mark-through, all-regular, and Specials progress;
16. remove flows;
17. Dashboard All/Games/Movies/TV scopes;
18. manual backup create/list/download/delete;
19. uploaded restore and stored-backup restore;
20. pre-restore safety backup;
21. automatic schedule/retention;
22. ratings CSV export;
23. all Settings pages;
24. dark/light/system appearance behavior as implemented.

Fix defects found during this pass. Do not merely document them unless the fix would require a deliberate post-1.0 product decision.

---

# 3. Migration and upgrade hardening

Gradeium must reliably support both a clean 1.0 installation and upgrade from the repository state that existed immediately before Phase 6.

Verify with PostgreSQL 18:

- empty database → all migrations → current schema;
- Phase 5 schema/data → Phase 6 migration(s) → current schema;
- existing users, OIDC identities, sessions where schema-compatible, settings, encrypted provider state, media, ratings, artwork pins, TV progress, dashboard state, backup metadata, and schedules remain valid;
- no migration requires live metadata providers;
- migrations are deterministic and fail closed;
- downgrade SQL remains internally coherent where Goose down migrations exist, but do not sacrifice safe forward migration merely to support production downgrades.

If theme persistence needs a schema change, add one explicit migration. Do not rewrite older migrations already shipped through the project history.

Run sqlc generation with the pinned version and require a clean generated diff.

---

# 4. Authentication and security regression

Do not redesign authentication unless a real defect is found.

Re-verify:

- unauthenticated admin APIs return 401;
- authenticated non-admin access to admin APIs returns 403;
- unsafe authenticated actions reject missing CSRF token;
- wrong/cross-origin requests are rejected;
- OIDC state/nonce/PKCE and callback replay protections still work;
- first-admin claim remains race-safe;
- session tokens remain opaque and hash-only in PostgreSQL;
- logout revokes sessions;
- expired/revoked sessions fail closed;
- cookies retain the intended HttpOnly/SameSite/Secure properties;
- secret APIs never return saved secret values;
- application logs do not contain provider/OIDC secrets, raw session tokens, CSRF tokens, or master-key contents;
- CSP changes required for theme initialization do not broadly weaken the policy;
- provider artwork/trailers continue to work under the final CSP;
- health/readiness endpoints expose no sensitive information.

Perform targeted malformed-input/body-size checks on important mutation endpoints, especially authentication settings, provider settings, media state, backup upload, and restore.

---

# 5. Backup and disaster-recovery hardening

Backups are a core 1.0 reliability guarantee. Re-test them aggressively.

## Portable backup invariants

A portable backup must never contain:

- plaintext secrets;
- encrypted secret ciphertext/nonces;
- OIDC client secret;
- IGDB/TMDB/TVDB credentials;
- sessions or session hashes;
- CSRF material;
- database password/connection string;
- `/config/master.key` or master-key bytes;
- imported administrator privilege.

It may contain the current portable state already defined in Phase 5, including internal TV `watched_at` metadata.

## Failure cases

Verify:

- malformed gzip;
- malformed JSON;
- unsupported format/version;
- unknown fields;
- truncated files;
- trailing compressed data;
- decompression bomb/size limits;
- impossible foreign references;
- duplicate provider IDs;
- invalid statuses/ratings/preferences;
- missing referenced artwork/episodes;
- interrupted/failed restore leaves current state unchanged;
- pre-restore backup failure prevents restore;
- concurrent create/delete/restore/scheduler operations are serialized safely;
- failed backup creation leaves no published partial backup;
- stale temporary files do not accumulate during normal operation;
- retention removes only eligible automatic backups;
- manual and pre-restore backups are never deleted by automatic retention;
- overdue-on-startup creates at most the intended backup rather than a catch-up storm.

## Master-key recovery

Re-test the established guarantee:

- losing/replacing the matching `/config/master.key` while encrypted state exists causes clear fail-closed startup;
- restoring the correct key restores operation;
- no Phase 6 work weakens the PostgreSQL + `/config` recovery pairing.

---

# 6. Provider failure and network hardening

Run deterministic provider tests without live credentials.

For IGDB, TMDB, TVDB, and OIDC verify representative:

- DNS/connect failure;
- timeout;
- HTTP error;
- malformed/oversized response;
- authentication rejection where applicable.

Requirements:

- provider failures never crash the process;
- existing local Libraries, details, Dashboard, Settings, and backups remain usable where they do not require that provider;
- liveness remains independent of provider reachability;
- readiness remains a local PostgreSQL readiness signal, not a provider-health aggregate;
- user-facing errors are bounded and do not expose upstream secrets/internal response bodies;
- retry behavior is bounded and does not create unbounded goroutines or request storms.

Re-verify strict TVDB→TMDB mapping: no fuzzy fallback may appear during hardening.

---

# 7. PostgreSQL outage and concurrency hardening

Verify:

- process liveness remains 200 during a temporary PostgreSQL outage;
- readiness becomes 503;
- readiness returns to 200 after PostgreSQL recovery without restarting Gradeium;
- application requests fail cleanly rather than hanging indefinitely;
- pool limits and request timeouts remain bounded;
- graceful shutdown works while the database is healthy and while a request/provider call is in flight;
- backup scheduler shuts down without leaking goroutines;
- race detector remains clean.

Exercise concurrency-sensitive paths including:

- duplicate media adds;
- first-admin binding;
- authentication configuration revisions;
- rating/status updates where relevant;
- backup operations;
- restore serialization;
- scheduler/manual backup overlap.

---

# 8. Performance and resource sanity

Do not perform speculative optimization. Find obvious 1.0-scale problems and fix only justified ones.

Create deterministic realistic fixture data large enough to expose N+1/unbounded behavior, for example approximately:

- 1,000 total media items across domains;
- dozens of TV shows;
- several thousand TV episodes;
- ratings/statuses/artwork across the fixture.

Check representative operations:

- Dashboard;
- each Library/Backlog list;
- local filtering/search/sort behavior;
- TV detail/progress;
- backup snapshot creation;
- restore validation/application.

Look for:

- accidental per-row provider calls;
- egregious N+1 database query patterns;
- unbounded memory growth;
- giant response payloads where avoidable;
- runaway goroutines;
- obvious slow queries missing an index.

The goal is a responsive personal self-hosted application, not enterprise-scale benchmarking. Do not add caches/services merely to improve synthetic benchmark numbers.

---

# 9. Docker / Unraid production verification

Gradeium 1.0 is expected to run continuously on Docker/Unraid.

Re-verify the production image and Compose stack:

- frontend built with the pinned production Node image;
- Go backend built separately;
- no Node or Go toolchain in final runtime image;
- non-root UID/GID 10001;
- read-only root filesystem;
- writable mounts limited to intended persistent locations, including `/config` and `/backups`;
- `no-new-privileges` remains enabled;
- CA certificates and timezone data remain available;
- PostgreSQL 18 remains the database;
- `restart: unless-stopped` remains appropriate;
- healthcheck is liveness-oriented;
- database dependency/readiness startup behavior works;
- graceful stop succeeds within the existing shutdown budget;
- app container remains disposable while database/config/backups persist;
- restart preserves all intended state.

Check the image for accidental provider credentials, test fixtures, source-only secrets, development databases, or unnecessary generated artifacts.

Do not add a reverse proxy, Cloudflare, Tailscale, or Unraid-specific runtime dependency to the application stack.

---

# 10. Accessibility, responsive, and UX consistency pass

Do a production browser walkthrough at minimum:

- desktop width;
- 390×844 mobile viewport.

Also sanity-check an intermediate/tablet width where practical.

Verify:

- keyboard navigation for primary flows;
- visible focus;
- labels on controls;
- dialogs/sheets can be closed and do not trap the page incorrectly;
- important actions are reachable without hover;
- no horizontal overflow on normal pages;
- long titles/provider errors/filenames do not destroy layouts;
- touch targets are reasonable;
- loading/empty/error states are intentional;
- no fake statistics or filler UI is introduced;
- no duplicate information is added merely to fill space;
- browser console has zero unexpected errors/warnings in the tested production flows.

Pay special attention to dark mode contrast and charts.

---

# 11. Documentation and release readiness

Update documentation to describe the completed 1.0 behavior accurately.

At minimum ensure README/operator docs cover:

- what Gradeium 1.0 does;
- supported providers;
- OIDC setup and callback URL;
- provider credential setup through Admin Settings;
- dark/default appearance and theme choices;
- Docker Compose startup;
- persistent `/config`, PostgreSQL, and `/backups` responsibilities;
- portable backup vs full disaster-recovery responsibilities;
- master-key loss/recovery behavior;
- provider outage behavior;
- upgrade/migration expectations;
- security boundary and lack of local-password recovery bypass;
- Jellyfin clearly listed as post-1.0/not included.

Remove stale wording that calls currently implemented functionality future work.

## Version/build preparation

Prepare the codebase so a production build can identify itself as `1.0.0` through the existing build-info mechanism, while preserving a sensible development default.

Do **not** create a Git tag, GitHub Release, or merge the Phase 6 PR. Release publication happens only after final review and merge.

If appropriate, add a concise changelog/release notes document for 1.0, but avoid duplicating the entire README.

---

# 12. Required verification

Run all applicable checks and report exactly what actually ran.

Minimum expected verification:

## Backend

- `gofmt -l .`
- `go mod tidy`
- `go mod verify`
- `go test -count=1 ./...`
- `go test -count=1 -race ./...`
- `go vet ./...`
- PostgreSQL 18 integration tests, including fresh and Phase 5→6 migration paths
- pinned sqlc 1.31.1 generation and clean diff

## Frontend

- clean dependency install (`npm ci`)
- `npm run lint`
- `npm run typecheck`
- `npm run build`
- any frontend tests that exist by Phase 6; do not invent a large testing framework solely for this milestone

## Production

- production Docker build
- `docker compose config --quiet`
- clean Compose startup with PostgreSQL 18
- health/readiness behavior
- non-root/read-only/no-new-privileges inspection
- restart persistence
- graceful shutdown
- PostgreSQL outage/recovery
- master-key failure/recovery
- provider outage isolation
- backup/restore/scheduler/retention regression
- secret/session material scans

## Browser

Perform realistic production-mode browser verification at desktop and 390×844 at minimum, covering theme behavior and all major 1.0 flows.

The browser console must not contain unexpected errors or warnings.

## GitHub Actions

The final pushed Phase 6 head must have green CI before the PR is considered complete.

---

# 13. Internal execution checkpoints

This is one milestone, one branch, and one draft PR. Do not stop for intermediate approval.

Suggested internal order:

1. theme persistence + dark/light/system implementation;
2. migration/sqlc changes;
3. targeted theme/browser verification;
4. backend/security/provider regression;
5. backup/disaster recovery hardening;
6. realistic-data performance/resource pass;
7. Docker/Compose/Unraid-shaped verification;
8. full responsive/accessibility browser walkthrough;
9. docs/release-readiness cleanup;
10. final complete test/race/build/CI pass.

Fix issues discovered at any later checkpoint rather than merely listing them.

---

# 14. Delivery

Work on one dedicated branch, preferably:

`codex/phase-6-1.0-hardening`

When complete:

1. review the entire diff against `AGENTS.md` and every file under `docs/`;
2. ensure no post-1.0 functionality slipped in;
3. report exactly which checks actually ran and their results;
4. commit with a clear message;
5. push the branch;
6. open one **draft PR** against `main` with `Closes #12` (or the actual Phase 6 issue number if different);
7. do not merge;
8. do not tag or publish a release.

The desired outcome is a repository that needs no further planned feature implementation before Gradeium 1.0—only final review, merge, and release publication.
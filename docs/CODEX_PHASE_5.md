# Codex Phase 5 — Dashboard, Backups, and Settings Completion

Phase 5 completes the remaining user-facing functionality required before Gradeium 1.0 hardening. It builds on the merged Phase 1–4 runtime, security, authentication, provider, and full media product foundations.

Before changing anything, read `AGENTS.md` and every file under `docs/`. Existing product rules remain authoritative unless this phase explicitly narrows implementation scope.

## Goal

Deliver the complete pre-1.0 product experience around the existing Games, Movies, and TV domains:

- useful personal dashboard analytics;
- portable, safe, user-owned backups and restore;
- scheduled automatic backup execution and retention;
- manual ratings export;
- completion of General/System/Backup/user preference settings;
- final shared UX polish needed before the dedicated 1.0 hardening phase.

This is one dense milestone and one PR. Work through the internal checkpoints below without stopping for intermediate approval.

## Non-goals

Do **not** implement:

- Jellyfin or any Jellyfin scaffolding/runtime code;
- custom lists;
- favorites;
- Steam, Letterboxd, Trakt, IMDb, CSV import, or other imports;
- recommendations;
- social features;
- user-management UI;
- local-password authentication;
- public APIs/API keys;
- media-history/diary UI;
- replay/rewatch counters;
- provider changes unrelated to bugs found while implementing this phase;
- release tagging/version 1.0.0 itself.

Jellyfin remains explicitly post-1.0.

---

# 1. Dashboard

Implement the Dashboard as a read-only summary of current Gradeium state. It must remain useful without inventing history Gradeium does not otherwise model.

## Required dashboard content

Support an `All / Games / Movies / TV` scope control.

Include:

1. **Totals**
   - tracked totals by media domain;
   - Library and Backlog counts where useful.

2. **Currently In Progress**
   - concise artwork-first row/cards;
   - domain-aware metadata;
   - for TV include current regular-episode progress and next unwatched episode when available;
   - clicking opens the existing local Gradeium detail route.

3. **Average personal rating**
   - overall and/or per-domain according to selected scope;
   - exclude unrated items and Backlog.

4. **Rating distribution**
   - useful compact chart based on current personal ratings;
   - no fake precision or meaningless animation.

5. **Status distribution**
   - include Backlog plus normal statuses;
   - compact chart, not a management interface.

6. **Highest Rated**
   - personal rating first;
   - sensible stable tie-breaking;
   - no community-rating substitution when personal rating is absent.

7. **TV progress summary**
   - only when TV is in scope;
   - regular episodes drive percentages;
   - Specials remain separate and do not affect regular progress.

Keep the dashboard restrained. Use at most a few meaningful charts. Do not add time-based charts such as “watched this month” or “ratings over time” even if internal timestamps exist; Gradeium 1.0 does not expose a diary/history model.

## Dashboard backend

Prefer efficient aggregate/query-specific SQL rather than loading every complete detail object into Go and aggregating there.

Dashboard reads must:

- be user-scoped;
- require authentication;
- never depend on live metadata providers;
- remain available during IGDB/TMDB/TVDB outages;
- use already persisted Gradeium metadata only.

---

# 2. Portable backups

Implement first-class application-managed backups under the existing persistent `/backups` mount.

## Canonical backup format

Use a versioned gzip-compressed JSON format with this logical envelope:

```json
{
  "format": "gradeium-backup",
  "version": 1,
  "createdAt": "...",
  "applicationVersion": "...",
  "users": [],
  "games": [],
  "movies": [],
  "tvShows": [],
  "episodeProgress": [],
  "settings": []
}
```

The exact schema may add clearly versioned fields required for correctness, but do not make a raw PostgreSQL dump the primary portable backup format.

## Backup contents

A backup must contain enough Gradeium-owned state to reconstruct the user experience without requiring the old PostgreSQL database:

- users and stable Gradeium user identity required for restoration;
- Games/Movies/TV provider IDs and Gradeium IDs where appropriate;
- personal statuses;
- ratings and private rating reasons;
- TV watched-episode progress;
- provider artwork pin selections;
- relevant user preferences/settings;
- provider mapping IDs required to preserve identity (for example verified TVDB↔TMDB mapping where useful);
- enough canonical metadata to make restored Library/Backlog usable immediately while providers are offline.

Do not treat provider refresh as a prerequisite for successful restore.

## Secrets

Portable backups must **never contain plaintext secrets**.

Do not export:

- OIDC client secret;
- IGDB client secret;
- TMDB access token;
- TVDB key/PIN;
- session tokens;
- CSRF tokens;
- master key;
- encrypted secret ciphertext as a substitute for a proper recovery design.

Non-secret integration configuration may be included only when safe and useful.

Document clearly that `/config/master.key` and external-service credentials are outside the portable backup format.

## File creation safety

Backup creation must be atomic:

1. write to a temporary file inside `/backups`;
2. fsync/close as appropriate;
3. validate the generated backup by reading/parsing it;
4. calculate/store a checksum;
5. atomically rename to the final file;
6. never expose a partially written backup as valid.

Use deterministic, filesystem-safe names containing an unambiguous UTC timestamp.

Prevent path traversal and arbitrary file access. Backup APIs operate only on files Gradeium recognizes inside the configured backup directory.

## Backup metadata/browser

Provide a Backup page/section showing:

- filename;
- creation time;
- size;
- format version;
- checksum/validity status where useful;
- whether created manually, automatically, or as pre-restore safety backup if tracked.

Actions:

- Create backup now;
- Download backup;
- Restore backup;
- Delete backup with confirmation.

Do not expose arbitrary server filesystem paths.

---

# 3. Restore

Restore is security- and data-sensitive. Make it transactionally safe and deliberately explicit.

## Before restoration

- validate gzip and JSON structure;
- verify `format == "gradeium-backup"`;
- accept only supported version(s);
- enforce practical decompressed-size and item-count limits;
- reject malformed UUIDs/provider IDs/status/rating values;
- reject duplicate/conflicting identity records;
- reject unknown dangerous structure rather than attempting best-effort mutation;
- do not execute or interpret arbitrary content.

## Pre-restore safety backup

Before any destructive restore, automatically create a valid **pre-restore backup** of the current state.

If the safety backup cannot be created successfully, abort restore without modifying user data.

## Restore semantics

Restore should reconstruct Gradeium application-owned media state in PostgreSQL transactionally.

Requirements:

- no half-restored installation;
- preserve referential integrity;
- preserve user/media UUID relationships as appropriate;
- preserve ratings/statuses/reasons/progress/artwork pins;
- restore TV progress correctly including Specials;
- restored items are available immediately from local state;
- canonical/provider IDs remain unique;
- duplicate/conflict handling is deterministic and documented.

Authentication and encrypted credentials must not be silently overwritten from a portable backup.

Do not create a hidden authentication bypass. The current signed-in admin remains responsible for restore.

A restore endpoint must require authenticated admin authorization, CSRF, same-origin protection, and explicit confirmation information sufficient to prevent accidental invocation.

## Failed restore

On validation or transaction failure:

- current database state remains unchanged;
- return a safe user-visible error;
- logs may contain diagnostic structure but never secrets or imported sensitive values.

---

# 4. Automatic backup scheduler

Implement scheduled backups in the existing Go process. Do not add Redis, queues, cron sidecars, or a second application service.

## Settings

Admin Settings → Backups must support:

- automatic backups enabled/disabled;
- interval;
- retention count;
- last successful automatic backup;
- next expected backup;
- last error/status when useful;
- Create backup now.

Default configuration:

- automatic backups enabled unless existing product docs explicitly require otherwise;
- interval: every 3 days;
- retention: 30 backups.

Supported intervals:

- daily;
- every 3 days;
- weekly;
- every 2 weeks;
- monthly;
- a bounded custom interval if cleanly implementable without ambiguity.

Persist scheduler configuration/state in PostgreSQL.

## Scheduler behavior

- scheduler starts with the Gradeium app;
- no unbounded goroutines;
- use one bounded scheduler loop/timer;
- graceful shutdown stops it cleanly;
- if the app was offline when a backup became due, run one overdue backup after startup rather than trying to replay every missed occurrence;
- concurrent manual/automatic/pre-restore backups must serialize safely;
- avoid overlapping backup/restore operations;
- backup failure must never terminate the application;
- liveness/readiness must not depend on backup success;
- backup errors are recorded safely for the UI/logs.

## Retention

Retention applies to automatic backups. Preserve manual and pre-restore backups unless the UI explicitly says otherwise.

Delete oldest eligible automatic backups only after a new automatic backup has completed successfully.

Never delete the last known-good automatic backup because a newer backup failed.

---

# 5. Manual CSV rating export

Provide a simple user-owned CSV export for ratings.

Include one row per tracked rated item with useful portable columns such as:

- domain;
- Gradeium ID;
- provider ID;
- title;
- year;
- status;
- personal rating on the displayed 1.0–10.0 scale;
- rating reason.

Do not include secrets or unnecessary internal metadata.

Use standards-compliant CSV escaping and UTF-8.

This is export only. CSV import remains out of scope.

---

# 6. Settings completion

Finish the existing settings experience without inventing configuration merely to fill space.

## General

Keep/change the existing instance-name setting as appropriate.

Add the existing user-facing Library preferences to Settings if not already fully surfaced:

- default Library sort;
- preferred default view (`grid`/`list`).

The global default sort options are:

- My Rating high → low;
- My Rating low → high;
- Community Rating high → low;
- Title A → Z;
- Title Z → A;
- Release Date newest → oldest;
- Release Date oldest → newest;
- Date Added newest → oldest;
- Date Added oldest → newest.

Per-page URL state may override defaults. Existing view state behavior must remain coherent.

## Authentication

Do not redesign OIDC. Preserve the Phase 3 Authentication page and its last-known-good behavior.

## Integrations

Preserve Phase 4 IGDB/TMDB/TVDB cards and connection-state behavior. Fix only defects encountered during this phase.

## Backups

Implement the full backup settings/browser described above.

## System

Provide only useful operational facts already available safely, for example:

- setup complete;
- authentication activated;
- persistent master key available;
- database/backup runtime status where safe;
- application version/build information if already available through a reliable mechanism.

Do not expose filesystem secrets, environment values, database credentials, tokens, or raw master-key details.

---

# 7. Shared frontend/product polish

Bring the pre-1.0 UI to a coherent state across Dashboard, media domains, and Settings.

Required:

- responsive desktop and narrow-mobile behavior;
- loading/skeleton states where network latency is expected;
- clear empty states;
- clear retryable error states;
- keyboard-accessible controls;
- visible focus states;
- appropriate accessible names/labels;
- no console errors/warnings introduced by this phase;
- restrained shadcn visual language already established by Gradeium;
- no gradients/glassmorphism/decorative filler;
- no giant dashboard hero;
- no fake stats;
- no meaningless badges or animations.

Preserve useful URL-aware Library state and scroll behavior.

Do not perform a broad visual redesign unless required to fix inconsistency or accessibility.

---

# 8. Security requirements

Preserve all Phase 1–4 guarantees.

All state-changing backup/admin endpoints require:

- authenticated session;
- admin authorization where administration is required;
- CSRF token;
- same-origin enforcement.

Downloads must not allow path traversal.

Uploads/restores must be bounded against decompression bombs and excessive item counts.

Backup filenames and API parameters are identifiers, not arbitrary filesystem paths.

Logs must not contain:

- session cookies;
- CSRF tokens;
- OIDC/provider secrets;
- master key;
- imported secret-like fields;
- complete backup payloads.

Maintain non-root runtime, read-only application root filesystem, writable persistent `/backups`, `no-new-privileges`, HTTP timeouts, graceful shutdown, and PostgreSQL readiness behavior.

---

# 9. Persistence and migrations

Use explicit versioned migration(s).

Add only the state needed for dashboard preferences/backup scheduling/backup metadata.

Use sqlc for stable query sets where it improves consistency with the current codebase. Avoid introducing an ORM.

Existing data from Phase 4 must migrate safely without resets.

Test both:

- fresh installation through all migrations;
- upgrade from the Phase 4 schema/data state.

---

# 10. Internal implementation checkpoints

Work through these without opening intermediate PRs or waiting for approval:

### Checkpoint A — persistence and backup core
- migrations;
- backup format/types;
- snapshot/export queries;
- atomic file writer;
- parser/validator;
- transactional restore;
- operation serialization.

### Checkpoint B — scheduler and admin backup APIs
- persisted schedule;
- overdue-on-startup behavior;
- retention;
- manual create/list/download/delete/restore;
- CSRF/admin enforcement.

### Checkpoint C — dashboard backend and CSV export
- aggregate queries;
- domain scope;
- outage-independent reads;
- CSV ratings export.

### Checkpoint D — frontend
- Dashboard;
- Backup settings/browser;
- restore confirmation/error UX;
- General/user preferences completion;
- System page completion.

### Checkpoint E — full verification and defect fixing
- test all existing media/auth/security behavior again;
- fix regressions rather than just reporting them.

---

# 11. Required verification

Actually run and report what ran.

## Backend

At minimum:

```text
gofmt -l .
go mod tidy
go mod verify
go test -count=1 ./...
go test -count=1 -race ./...
go vet ./...
```

Run PostgreSQL 18 integration tests covering at least:

- Phase 4 → Phase 5 migration;
- fresh migrations;
- dashboard user isolation and aggregates;
- backup creation and validation;
- backup deterministic structural correctness;
- atomic file publication;
- malformed/truncated/oversized backup rejection;
- restore success for Games/Movies/TV;
- ratings/reasons/status restoration;
- TV regular/Specials progress restoration;
- artwork pin restoration;
- transaction rollback on restore failure;
- automatic pre-restore backup;
- scheduler persistence;
- overdue startup behavior;
- retention;
- concurrent backup serialization;
- current auth/session/provider-secret state not being replaced by portable restore;
- CSV escaping/export correctness.

Run sqlc generation/diff verification using the pinned project version/method.

## Frontend

At minimum:

```text
npm ci
npm run lint
npm run typecheck
npm run build
```

If frontend tests exist, run them.

## Docker/Compose

Verify:

- production Docker build;
- `docker compose config --quiet`;
- clean PostgreSQL 18 startup through all migrations;
- `/backups` is writable by UID/GID 10001 while root filesystem remains read-only;
- `no-new-privileges` remains enabled;
- `/healthz` remains process liveness;
- `/readyz` remains PostgreSQL readiness;
- PostgreSQL outage still yields liveness 200/readiness 503 and recovers;
- backup failures do not affect health/readiness;
- graceful shutdown while scheduler is idle and around backup execution;
- `/config/master.key` persistence/fail-closed behavior remains unchanged.

## Browser walkthrough

Use a production-shaped stack with deterministic/local fixtures as needed.

Verify desktop and approximately 390×844 mobile:

- Dashboard All/Games/Movies/TV scopes;
- empty dashboard and populated dashboard;
- dashboard links to existing detail pages;
- rating/status charts render correctly without console errors;
- backup Create → list → download;
- backup restore confirmation;
- successful restore;
- malformed restore safe failure;
- pre-restore safety backup visible;
- scheduler settings persistence;
- default Library sort/view settings actually affect new Library visits;
- CSV ratings download;
- non-admin cannot perform admin backup operations;
- unauthenticated access denied as appropriate;
- missing CSRF and cross-origin state changes rejected;
- existing Games/Movies/TV/OIDC/provider settings still work;
- browser console clean.

Do not require live third-party credentials for deterministic CI. Live credentials may be used only as optional manual verification and must never enter source, logs, fixtures, or PR content.

---

# 12. Documentation

Update README/operator docs to explain:

- Dashboard purpose;
- backup location `/backups`;
- canonical portable backup format/version;
- what is and is not included;
- secrets/master key are not contained in portable backups;
- automatic schedule and retention defaults;
- restore safety backup;
- disaster-recovery expectations;
- CSV ratings export;
- remaining pre-1.0 status.

Do not claim Jellyfin support.

---

# 13. Delivery

Work on one dedicated branch, preferably:

```text
codex/phase-5-dashboard-backups
```

When complete:

1. review the entire diff against `AGENTS.md` and all docs;
2. run the required verification and fix discovered defects;
3. report exactly what checks ran and their results;
4. commit clearly;
5. push the branch;
6. open one **draft PR** against `main` containing `Closes #10` (or the actual Phase 5 issue number if it differs);
7. do not merge the PR.

Phase 6 will be dedicated 1.0 hardening/release work, not another major feature milestone.

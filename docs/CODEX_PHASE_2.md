# Codex Phase 2 — Core data, first-run setup, and secure Admin Settings foundation

## Goal

Implement Gradeium's secure application-configuration foundation before any media-provider or authentication integration is added.

This phase establishes the database primitives, UUIDv7 identities, application settings storage, first-run setup state, persistent master-key handling, encrypted-at-rest secrets, and an Admin Settings shell/API that later phases can reuse for OIDC, IGDB, TMDB, TVDB, Jellyfin, and backups.

Read `AGENTS.md` and every document under `docs/` before changing code. Those documents remain authoritative.

## Hard scope boundary

Implement only the items in this document.

Do not implement:
- Games, Movies, or TV domain tables beyond any tiny identity primitive explicitly required here
- ratings, statuses, artwork, episode progress, custom lists, or dashboard statistics
- IGDB, TMDB, TVDB, Jellyfin, or any provider client
- OIDC/Pocket ID login itself
- backup creation/restore behavior
- provider credential forms specific to a real service
- fake provider data or placeholder secrets

The result of Phase 2 should be a reusable secure settings substrate and first-run/admin shell, not a partially implemented integration layer.

---

## 1. Database migrations

Add explicit versioned SQL migrations for the following foundation.

### `entities`

Create a tiny globally unique entity registry:
- `id UUID PRIMARY KEY`
- `type` constrained to future Gradeium entity kinds (`game`, `movie`, `tv_show`)
- `created_at TIMESTAMPTZ NOT NULL DEFAULT now()`

Requirements:
- IDs must be UUIDv7.
- Prefer PostgreSQL 18 native `uuidv7()` generation for database-created rows where practical.
- Do not add media-specific metadata columns here.
- Do not create a generic giant media table.

### `users`

Create a minimal users/admin foundation suitable for future generic OIDC:
- internal UUIDv7 primary key
- stable external subject field nullable for now because OIDC is not implemented yet
- display name nullable
- email nullable
- `is_admin BOOLEAN NOT NULL DEFAULT false`
- `created_at`
- `updated_at`

Do not implement login/session tables unless genuinely necessary for this phase. Do not invent a local-password authentication system.

### `app_settings`

Create a typed/general application settings store suitable for non-secret application configuration.

It must support:
- unique stable setting key
- value storage in a format that can evolve cleanly, preferably JSONB
- timestamps

Application code must expose a typed service/repository boundary so random handlers do not issue ad hoc setting SQL.

### `secret_settings`

Create a separate store for encrypted secrets rather than mixing plaintext secrets into `app_settings`.

Store only:
- stable secret key
- encryption version / algorithm version metadata
- nonce/IV as needed
- ciphertext
- timestamps

Never store plaintext copies.

### `setup_state`

Represent whether first-run bootstrap has been completed.

This may be a dedicated singleton table or a clearly defined setting, but it must be explicit, transaction-safe, and easy to query.

A fresh database must report setup as incomplete.

---

## 2. Persistent master encryption key

Use the existing persistent `/config` directory.

On startup:
1. Resolve the master-key path inside `/config` using a fixed documented filename such as `/config/master.key`.
2. If it does not exist, generate cryptographically secure random key material.
3. Create the file atomically with restrictive permissions appropriate for the runtime user.
4. If it exists, load and validate it.
5. If it is malformed/unreadable, fail startup clearly rather than silently generating a replacement and making existing secrets undecryptable.

Requirements:
- never log key material
- never expose key material over HTTP
- never store the master key in PostgreSQL
- never add it to `.env`
- never commit it
- generation must be race-safe/atomic
- document that `/config` must be persisted and backed up together with database backups because losing the key makes encrypted settings unrecoverable

Use a modern authenticated-encryption primitive from the Go standard library where possible. AES-256-GCM is acceptable.

Create a small, well-tested internal crypto/secrets package rather than scattering encryption calls around handlers.

Ciphertext format must be versioned so future key/algorithm migrations are possible.

---

## 3. Secret-settings service

Implement a reusable backend service with operations conceptually equivalent to:
- set/replace secret by stable key
- test whether a secret is configured
- decrypt/read secret for trusted server-side application code
- delete secret

Security invariants:
- no API response may contain stored plaintext secret values
- logs must never include plaintext or ciphertext secret payloads
- error messages must not leak secrets
- replacement must be transactional/safe
- encryption must use a fresh nonce for each write
- secret keys/names may be exposed; secret values may not

Tests must verify:
- encrypt/decrypt round trip
- same plaintext written twice results in different ciphertext/nonces
- tampering causes authenticated decryption failure
- malformed key file fails safely
- stored API representations do not expose secret values

---

## 4. First-run setup API

Add minimal setup endpoints under `/api/setup`.

At minimum:
- `GET /api/setup/status`
  - returns whether initial setup is complete
- a completion/bootstrap endpoint that can create the initial local Gradeium admin identity record and mark setup complete

Important: OIDC is not implemented yet.

The first-run mechanism should therefore create only the minimum application-side administrator/bootstrap state needed for later OIDC binding. Do not invent username/password authentication.

Setup completion must be idempotent/safe against double submission and must not allow a second bootstrap once setup is completed.

Document clearly that Phase 3 or a later auth phase will bind the initial administrator to an OIDC identity.

If a safer architecture is to defer creation of the user row and only mark bootstrap/application initialization complete, prefer that over inventing temporary credentials. The key requirement is that later phases can determine first-run vs configured state safely.

---

## 5. Admin Settings backend API

Create a secure, provider-agnostic settings API foundation.

Because OIDC/auth is not implemented yet, do not pretend that these endpoints are fully production-authorized. Structure the routing/middleware so an admin-authorization middleware can be inserted cleanly in the next auth phase.

Add only generic configuration endpoints needed to exercise the storage layer, for example:
- settings metadata/list endpoint that returns safe non-secret values
- save/update non-secret application setting
- secret status metadata (`configured: true/false`) without secret value
- save/replace generic secret by allowed key
- delete generic secret by allowed key

Do not allow arbitrary user-controlled keys to become an unrestricted secret database. Define a central registry/schema of allowed application-setting and secret-setting keys, even if Phase 2 contains only reserved/future keys needed to prove the mechanism.

Prefer explicit key definitions with types, validation, labels/descriptions, and sensitivity metadata so future integrations can register their settings consistently.

Never return a stored secret value after save. Responses should say only that it is configured/updated.

---

## 6. Frontend first-run experience

Use the existing React/Vite/shadcn design system.

Implement a restrained first-run setup page/shell that appears when `GET /api/setup/status` reports incomplete.

Phase 2 UI should explain that Gradeium is being initialized and prepare the administrator/configuration foundation, but must not ask for provider credentials or OIDC details yet.

Requirements:
- clean shadcn-style layout
- no marketing hero
- no fake stats
- clear progress/state
- successful completion routes into the normal application shell/Admin Settings
- useful failure/retry state

Do not create a local password form.

---

## 7. Admin Settings frontend shell

Create a real `/settings` / Admin Settings shell that future phases can extend.

Suggested navigation/sections:
- General
- Authentication (coming in later phase)
- Integrations (coming in later phase)
- Backups (coming in later phase)
- System

For Phase 2:
- General may expose only genuinely useful implemented settings
- System should show safe read-only status such as setup state and whether the encryption key is available
- future sections should be restrained empty/coming-later states, not fake forms

Do not expose raw internal secret/ciphertext information.

The UI architecture should make adding provider-specific settings cards later straightforward.

---

## 8. Settings registry/design

Create one central backend definition for Gradeium-owned configuration keys.

Each definition should be able to carry concepts such as:
- stable key
- type (`string`, `boolean`, `integer`, etc.)
- secret/non-secret
- validation constraints
- default when appropriate
- admin-facing label/description where useful

Do not over-engineer a plugin framework. Keep this small and explicit.

The goal is to prevent future code from introducing one-off environment variables or inconsistent credential storage.

---

## 9. Infrastructure/configuration rules

Keep the existing bootstrap env surface infrastructure-only.

Do not add env vars for:
- IGDB
- TMDB
- TVDB
- OIDC/Pocket ID
- Jellyfin
- backup schedules
- application preferences

The `/config` mount is now meaningful because it holds the encryption master key.

Compose and Docker behavior must preserve:
- non-root runtime
- read-only root filesystem
- writable `/config` and `/backups`
- graceful shutdown
- current liveness/readiness semantics

Ensure master-key creation works with the read-only container filesystem because `/config` is the intended writable mount.

---

## 10. Reliability and concurrency

Phase 2 must remain reliability-first.

Requirements:
- startup key initialization safe under restart/crash conditions
- database writes use appropriate transactions
- setup completion race-safe
- concurrent secret replacement cannot produce partial records
- all DB/API operations have bounded request/context timeouts where appropriate
- no secret-dependent failure should crash unrelated requests after startup
- startup should fail if the master key cannot be safely initialized/loaded

Do not add Redis, queues, or new infrastructure services.

---

## 11. API/error behavior

Continue the Phase 1 API conventions:
- JSON errors for `/api/*`
- no SPA fallback for unknown API paths
- request IDs/logging
- safe panic recovery

Validation errors should be structured and human-readable without dumping internals.

Do not return database errors or crypto internals directly to the browser.

---

## 12. Testing

Add meaningful tests, including at least:

Backend unit/integration coverage:
- UUIDv7/entity migration behavior where practical
- fresh setup state is incomplete
- setup completion is safe/idempotent and cannot bootstrap twice
- settings validation
- non-secret setting persistence
- master-key create/load
- malformed master-key behavior
- secret encryption/decryption
- fresh nonce per write
- tamper detection
- secret delete
- APIs never return plaintext secret
- readiness/liveness behavior remains intact

Frontend:
- lint
- typecheck
- build
- test setup/settings state logic if a test framework is introduced; do not introduce a huge frontend test dependency solely to satisfy this line

Production/runtime verification:
- clean Docker build
- Compose startup with a fresh `/config`
- master key appears only in `/config` with safe ownership/permissions
- restart reuses the same key
- encrypted secret remains decryptable after restart
- deleting/replacing container without deleting `/config`/Postgres preserves behavior
- unknown API route remains JSON 404

---

## 13. Documentation

Update README/development docs with:
- first-run behavior
- `/config` master-key persistence warning
- why provider secrets are not environment variables
- how to reset a disposable development instance safely
- clear warning that deleting the database while keeping `/config`, or `/config` while keeping encrypted DB secrets, may require deliberate recovery/reset behavior

Do not tell users to manually place provider credentials anywhere yet.

---

## Acceptance criteria

Phase 2 is complete only if all of the following are true:

- [ ] Versioned migrations create the Phase 2 foundation tables.
- [ ] Internal IDs use UUIDv7 semantics.
- [ ] Fresh install has explicit incomplete setup state.
- [ ] `/config` master key is generated securely and atomically on first startup.
- [ ] Existing valid master key is reused on restart.
- [ ] Invalid/missing-after-use key situations fail safely rather than silently rotating.
- [ ] Secret values are encrypted at rest with authenticated encryption.
- [ ] No plaintext application secret is stored in DB, logs, env examples, or API responses.
- [ ] Secret storage is reusable and provider-agnostic.
- [ ] A central allowed-settings registry exists.
- [ ] First-run setup API and frontend flow exist without local-password auth.
- [ ] Admin Settings shell exists and is ready for later Authentication/Integrations/Backups phases.
- [ ] No real provider/OIDC/Jellyfin integration was implemented.
- [ ] Phase 1 health/readiness/runtime behavior remains correct.
- [ ] Backend formatting/tests/vet pass.
- [ ] Frontend lint/typecheck/build pass.
- [ ] Production Docker image builds.
- [ ] Compose runtime verification passes with persisted `/config` and PostgreSQL data.
- [ ] GitHub Actions pass.

## Delivery

Work on a dedicated branch, preferably `codex/phase-2-secure-settings`.

When complete:
1. Review the entire diff against `AGENTS.md`, all repository docs, and this document.
2. Run every available verification listed above.
3. Fix discovered issues.
4. Commit clearly.
5. Push the branch.
6. Open a **draft pull request** against `main` that closes the Phase 2 issue.
7. In the PR body, list exactly what was implemented and exactly which checks were actually run.
8. Do not merge the PR.

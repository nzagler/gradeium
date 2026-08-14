# Codex Phase 3 — Generic OIDC Authentication and Real Admin Authorization

## Goal
Replace Phase 2's intentionally transparent Admin Settings authorization boundary with production-grade generic OpenID Connect authentication, using Pocket ID as the first supported/tested provider, while preserving Gradeium's reliability and Admin-Settings-first configuration model.

This phase must make Gradeium safe to expose behind a normal reverse proxy after OIDC is configured. It must not implement media domains, metadata providers, Jellyfin, backups, or unrelated product functionality.

## Read first
Before changing anything, read:
- `AGENTS.md`
- every file under `docs/`
- the current implementation on `main`

Repository documentation is the source of truth. If this file conflicts with a more general document, follow the more specific Phase 3 requirement unless it would violate an explicit security invariant.

## Scope

### 1. Generic OIDC configuration in Gradeium UI
OIDC configuration belongs in Gradeium's own web UI, not provider-specific environment variables.

Add typed settings/secrets for at least:
- `authentication.issuer_url` — public non-secret setting
- `authentication.client_id` — public non-secret setting
- `authentication.client_secret` — encrypted secret setting
- `authentication.public_url` — public external Gradeium base URL used to construct callback URLs
- `authentication.enabled` — explicit activation state, or an equivalent durable state machine

Keep the central settings registry as the only allowlist for accepted keys.

The Admin Settings → Authentication page must allow the operator to:
- enter issuer URL
- enter client ID
- enter/replace client secret
- enter Gradeium public/base URL
- test/validate the OIDC configuration before activation
- activate authentication only after validation succeeds
- see configured / not configured / validation error state
- replace configuration later while already authenticated as an admin

Never display a stored client secret after save. Show only configured/not-configured state and a replace/remove action.

No OIDC setting belongs in `.env`.

### 2. Bootstrap security model
Phase 2 has no identity, so there must be a narrowly defined one-time bootstrap window for configuring OIDC.

Required model:
- Before authentication has ever been activated, the Authentication configuration surface may be used without a Gradeium session after Phase 2 setup is complete.
- This unauthenticated bootstrap capability must be limited to the minimum endpoints required to configure, validate, and activate OIDC.
- General/System Admin Settings must not remain broadly unauthenticated once Phase 3 is complete.
- Activation must be a durable, race-safe transition.
- Once OIDC is activated, unauthenticated bootstrap configuration must become inaccessible and must not reopen merely because the provider is temporarily unreachable or the client secret is removed.
- Reconfiguration after activation requires an authenticated Gradeium admin.

Document clearly that a brand-new installation must be configured from a trusted network until OIDC is activated. Do not add a hidden reset/backdoor endpoint.

### 3. OIDC protocol requirements
Implement standards-based generic OIDC using Authorization Code flow.

Required protections:
- discovery from the configured issuer's `/.well-known/openid-configuration`
- strict issuer verification
- authorization code flow
- PKCE S256
- cryptographically random `state`
- cryptographically random `nonce`
- ID token signature verification using provider JWKS
- validate issuer, audience/client ID, expiry, nonce, and other standard claims
- require a stable `sub` claim
- use exact configured redirect URI derived from `authentication.public_url`
- reject insecure or malformed callback/state data
- do not accept arbitrary redirect URLs from query parameters

Pocket ID must work as the first supported provider, but implementation must remain generic OIDC and must not hard-code Pocket ID-specific issuer behavior.

Use a mature Go OIDC/OAuth2 library rather than implementing JWT/JWK/OAuth cryptography manually. Keep dependencies minimal and well established.

### 4. OIDC provider HTTP reliability
Provider calls must follow Gradeium reliability rules:
- explicit connect/request timeouts
- bounded response bodies where applicable
- no infinite retries
- safe retries only for idempotent discovery/JWKS requests if implemented
- provider outage must not crash the process
- existing authenticated sessions should not depend on a live provider request for every Gradeium API call
- clean user-facing errors; no raw upstream HTTP dumps or secrets

Do not log authorization codes, access tokens, ID tokens, refresh tokens, client secrets, session tokens, state values, nonce values, or CSRF secrets.

### 5. User identity and first-admin binding
Use the Phase 2 `users` table as the Gradeium identity record.

Identity rules:
- external identity key is at minimum issuer + subject, not email
- never use email as the durable account identifier
- update display name/email metadata opportunistically from verified OIDC claims
- do not change a user's Gradeium identity because their email/display name changes

Initial administrator:
- after OIDC activation, the first successful verified OIDC login must atomically bind/create the initial Gradeium user as `is_admin = true`
- exactly one concurrent first-login request may win initial-admin bootstrap
- later users must never accidentally become admin because of a race
- once an admin exists, normal login must not auto-promote additional users

The schema may be migrated as needed to make issuer + subject uniqueness explicit and correct.

### 6. Sessions
Use server-side Gradeium sessions persisted in PostgreSQL.

Requirements:
- opaque cryptographically random session token in the browser
- store only a cryptographic hash of the session token in PostgreSQL, not the raw bearer token
- session rows linked to Gradeium user ID
- expiry timestamp and creation/update metadata
- session rotation after successful login
- logout invalidates the current session server-side
- expired/revoked sessions are rejected
- session lookup must be local PostgreSQL work and not require OIDC provider availability
- bounded cleanup strategy for expired sessions; no unbounded forever-growing table

Cookie requirements:
- `HttpOnly`
- `Secure` when Gradeium's configured public URL is HTTPS
- `SameSite=Lax` unless a stronger setting is proven compatible with the OIDC redirect flow
- `Path=/`
- do not set a broad `Domain` unless explicitly necessary
- sensible finite Max-Age/Expires consistent with server-side expiry

Do not store access/ID/refresh tokens in browser localStorage or sessionStorage.

Avoid persisting provider tokens at all unless strictly required for OIDC login. Gradeium currently has no need to call user APIs after login, so tokens should normally be discarded after identity verification.

### 7. CSRF protection
State-changing authenticated Gradeium API requests need explicit CSRF protection in addition to SameSite cookies.

Implement a session-bound CSRF token or equally strong same-origin mechanism.

Expected behavior:
- authenticated unsafe methods (`POST`, `PUT`, `PATCH`, `DELETE`) under normal application/admin APIs require a valid CSRF signal
- token is unguessable and tied to the session
- frontend sends it automatically
- logout is protected
- OIDC callback itself is protected by OIDC `state`/nonce and should not be broken by the generic API CSRF mechanism
- reject cross-origin requests with a clean 403 where appropriate

Do not rely solely on CORS as CSRF protection.

### 8. Authorization
Replace `Phase2AdminAuthorization` with real authorization.

Required access model:
- public: health/readiness, minimal setup/auth status required to render the correct entry screen, OIDC login start/callback, and the strictly bounded pre-activation auth bootstrap endpoints
- authenticated user: session/user identity endpoint and future application shell
- admin only: `/api/admin/**` and all Admin Settings after activation

Admin middleware must:
- require a valid unexpired session
- load the Gradeium user
- require `is_admin = true`
- use generic 401 for unauthenticated, 403 for authenticated non-admin
- never trust a client-supplied user/admin flag

### 9. Auth API
Exact route naming may follow existing conventions, but provide a coherent API equivalent to:
- `GET /api/auth/status` — setup/auth configured/activated/session state safe for UI
- `GET /api/auth/login` or equivalent start endpoint
- `GET /api/auth/callback`
- `POST /api/auth/logout`
- `GET /api/auth/session` or `/api/me` — current Gradeium identity, admin flag, CSRF token if appropriate
- narrowly scoped pre-activation OIDC configuration/validation/activation endpoints

Redirect behavior:
- unauthenticated browser access to the SPA may render a Gradeium login screen rather than HTTP-redirecting every asset/API request
- after login, return to a fixed/safely validated same-origin Gradeium path
- never create an open redirect

### 10. Frontend UX
Keep the restrained shadcn-oriented design.

Fresh install flow after Phase 2 setup:
1. Setup completes.
2. User is taken to Authentication setup.
3. Page explains that Gradeium uses generic OIDC and Pocket ID is supported.
4. Operator enters issuer URL, client ID, client secret, and public Gradeium URL.
5. `Test configuration` performs real discovery/config validation without activating auth.
6. `Enable authentication` is available only after a successful validation of the current configuration.
7. After activation, show `Sign in with OIDC`.
8. First successful identity becomes the initial administrator.

After activation:
- unauthenticated users see a clean login screen
- authenticated admin sees normal shell and Settings
- Authentication settings show issuer/client ID/public URL and secret configured state, never plaintext secret
- provide sign out action
- show current user identity unobtrusively in the shell/settings area
- mobile layout must work

Do not invent profile management, avatars, role administration UI, invites, groups, or multi-user management in this phase.

### 11. OIDC configuration validation
`Test configuration` must at minimum:
- parse and normalize configured URLs safely
- require HTTPS for issuer/public URL in production-style use; localhost HTTP may be allowed explicitly for development/tests
- perform OIDC discovery
- verify discovered issuer exactly matches configured issuer semantics
- verify authorization/token/JWKS endpoints are valid HTTPS URLs (localhost dev exception where necessary)
- verify client ID is non-empty
- verify client secret is configured if the selected client authentication method requires it
- determine and display the exact redirect URI that must be registered in the OIDC provider

Do not require an actual interactive login merely to test discovery; successful end-to-end login is verified separately.

### 12. Secret/storage invariants
Preserve every Phase 2 secret guarantee:
- OIDC client secret goes through encrypted `secret_settings`
- master key stays only in `/config`
- API never returns saved secret plaintext
- logs never contain secret plaintext
- missing/mismatched master key still fails safely
- changing OIDC settings does not silently rotate the master key

If old reserved key names need migration, do it explicitly and safely.

### 13. Database/migrations/sqlc
Use versioned migrations and sqlc for new persistent queries.

Likely additions:
- issuer-qualified external identity uniqueness
- session table
- auth activation/bootstrap state if not represented cleanly in typed settings
- timestamps/indexes needed for session expiry/lookup

Migration invariants:
- no destructive reset of existing Phase 2 data
- migrations safe on an existing Phase 2 database
- rerun/restart safe
- generated sqlc output checked in and CI-verified

### 14. Tests
Add meaningful automated coverage.

Backend unit tests should cover at least:
- auth configuration validation
- state/nonce/PKCE generation and verification boundaries
- safe return-path validation/open-redirect rejection
- session token generation/hash lookup/expiry/revocation
- CSRF validation
- admin middleware 401/403/success behavior
- first-admin binding race behavior at service/repository level
- secret redaction and log safety around OIDC errors

Integration tests against PostgreSQL 18 should cover at least:
- migrations from Phase 2 schema to Phase 3
- user issuer+subject uniqueness
- first-admin concurrency
- session persistence/hash-only storage
- session expiration/revocation
- restart persistence

OIDC protocol tests:
- use a local deterministic test OIDC issuer/server in Go tests or a purpose-built test fixture
- verify discovery, auth callback, state, nonce, PKCE, signature/JWKS verification, audience/issuer rejection, expired token rejection
- verify end-to-end first login creates admin and a Gradeium session
- verify provider outage after login does not invalidate an otherwise valid local session

Frontend:
- lint/typecheck/build remain mandatory
- add frontend tests only if Phase 3 introduces a reasonable existing test framework; do not add a heavy framework solely for superficial tests

### 15. Docker/Compose verification
Preserve Phase 1/2 runtime guarantees:
- non-root UID/GID 10001
- read-only root filesystem
- persistent `/config` and `/backups`
- PostgreSQL persistence
- graceful shutdown
- `/api/healthz` remains liveness-only
- `/api/readyz` remains DB-aware and must not depend on OIDC provider reachability
- provider outage must not mark the entire running process unhealthy

No OIDC credentials in Compose or `.env.example`.

### 16. Documentation
Update README/docs with:
- generic OIDC setup flow
- Pocket ID setup example/guidance without hard-coding it into runtime behavior
- exact callback URI format
- reverse-proxy/public URL considerations
- HTTPS expectations
- first-admin behavior
- session/cookie security model at a concise operator level
- recovery procedure if OIDC provider is down (existing sessions continue until expiry; new logins may fail)
- warning that there is no local-password bypass/backdoor

Do not document insecure ways to disable auth after activation.

## Strict non-goals
Do NOT implement in Phase 3:
- Games, Movies, or TV Shows
- ratings/statuses/library/backlog
- IGDB/TMDB/TVDB
- Jellyfin
- backup creation/restore/scheduler
- email/password or local-password login
- WebAuthn/passkeys inside Gradeium
- LDAP
- SAML
- user invitation/role-management UI
- provider-specific Pocket ID APIs beyond standard OIDC
- social login providers as special cases
- OAuth access-token based Gradeium API authentication
- API keys

## Acceptance criteria
Phase 3 is complete when all of the following are true:

1. A Phase 2 installation migrates cleanly to Phase 3 without losing settings/key state.
2. OIDC issuer, client ID, client secret, and public URL are configured from Gradeium's web UI, not `.env`.
3. Client secret is encrypted at rest and never returned/logged.
4. OIDC configuration can be tested before activation.
5. Auth activation is durable/race-safe and closes the unauthenticated bootstrap configuration path permanently.
6. Generic OIDC Authorization Code + PKCE + state + nonce works against the test issuer and Pocket ID-compatible configuration.
7. ID tokens are cryptographically and semantically verified.
8. First successful verified login atomically becomes the initial Gradeium admin.
9. Sessions are server-side, PostgreSQL-backed, opaque in the browser, hash-only in DB, expiring, revocable, and do not need live OIDC access per request.
10. Authenticated state-changing APIs have explicit CSRF protection.
11. `/api/admin/**` is 401 without a session, 403 for non-admin, and allowed for an admin.
12. Logging out revokes the current session.
13. OIDC provider outage does not crash Gradeium or make liveness/readiness fail solely because of the provider.
14. Existing Phase 1/2 reliability/security tests continue to pass.
15. Backend tests, race-sensitive tests, PostgreSQL integration tests, frontend lint/typecheck/build, Docker build, Compose health, and required manual browser flows are actually run and reported.
16. No media/provider/Jellyfin/backup functionality is added.
17. A draft PR against `main` closes the Phase 3 GitHub issue and remains unmerged for review.

## Suggested branch
`codex/phase-3-oidc-auth`

## Delivery requirements
When finished:
1. Review the entire diff against `AGENTS.md`, every file under `docs/`, and this specification.
2. Fix issues found during self-review.
3. Run all available required checks and report exactly what actually ran.
4. Keep the working tree clean.
5. Commit clearly.
6. Push the dedicated branch.
7. Open a draft PR against `main` with `Closes #<phase-3-issue>`.
8. Do not merge the PR.

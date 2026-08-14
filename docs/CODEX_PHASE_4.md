# Codex Phase 4 — Full Media Product

## Goal

Implement Gradeium's complete core media-tracking product in one milestone: Games, Movies, and TV Shows end-to-end, using the secure runtime, Admin Settings, and OIDC foundation from Phases 1–3.

This phase intentionally combines the three media domains so Gradeium becomes genuinely usable after one implementation milestone. Keep the domains structurally separate in code and data, but share infrastructure only where it is truly generic (provider HTTP reliability, common rating/status primitives, artwork selection plumbing, pagination helpers, etc.).

## Mandatory reading

Before changing anything:

- read `AGENTS.md`
- read every document under `docs/`
- treat `PRODUCT_SPEC.md`, `ARCHITECTURE.md`, `DESIGN.md`, `DATA_MODEL.md`, and `IMPLEMENTATION_PLAN.md` as product/architecture source of truth
- preserve every Phase 1–3 reliability/security guarantee

Do not weaken authentication, CSRF, secret storage, database migration discipline, container hardening, readiness/liveness, graceful shutdown, or provider-outage isolation.

---

# Scope

## 1. Provider configuration in Admin Settings

Add real provider configuration and connection testing for:

### IGDB / Twitch
- client ID
- client secret
- credentials entered only through Gradeium Admin Settings
- client secret stored through existing encrypted secret storage
- connection test
- redacted API responses
- clear states: not configured / connected / error / disabled where applicable

### TMDB
- API credential required by the current supported TMDB API
- configured only through Gradeium Admin Settings
- secret values encrypted and never returned
- connection test

### TVDB
- current TVDB API credential(s) required by the supported API
- configured only through Admin Settings
- secrets encrypted and redacted
- connection test

No provider credential may be introduced as an environment variable or Compose requirement.

Provider HTTP clients must use explicit timeouts, bounded responses, useful user-safe errors, no credential logging, and no dependency on provider availability for `/api/healthz` or `/api/readyz`.

## 2. Core identity and domain data model

Use Gradeium UUIDv7 entity IDs from the existing `entities` registry.

Do not create one giant generic media table.

Implement separate canonical domain tables and separate user-state tables, including at minimum:

### Games
- canonical `games`
- provider identity: unique IGDB ID
- canonical metadata required by the product spec
- provider artwork references/candidates required for selection
- nested/related game relationships required by normalization rules
- `user_games`

### Movies
- canonical `movies`
- provider identity: unique TMDB ID
- canonical metadata required by the product spec
- collection membership where TMDB provides it
- artwork references/candidates
- `user_movies`

### TV
- canonical `tv_shows`
- unique TVDB ID
- optional unique verified TMDB ID only through the strict mapping algorithm below
- seasons
- episodes
- artwork references/candidates
- `user_tv_shows`
- per-user episode progress

Canonical provider data must remain separate from user-owned state so future multi-user support remains possible.

## 3. Shared personal-state rules

Statuses for all three domains:

- `backlog`
- `in_progress`
- `on_hold`
- `abandoned`
- `completed`

Rating rules:

- 1.0–10.0
- 0.1 increments
- store internally as integer 10–100
- nullable when unrated
- optional private rating reason text
- Backlog cannot have a rating
- moving a rated item to Backlog must require explicit confirmation and delete rating + reason
- TV episode progress must survive moves to/from Backlog

Do not add rating history, status history, watch/play dates, completion dates, replay/rewatch counters, platform-played state, diary features, reviews/social features, or favorites.

## 4. Provider search and Add flows

Implement dedicated Add flows:

- Games → IGDB
- Movies → TMDB
- TV Shows → TVDB

Requirements:

- debounced search
- sensible minimum query length
- bounded/paginated results; prefer explicit Load More over uncontrolled infinite scrolling
- clean loading, empty, provider-error, and retry states
- provider failures never crash the app
- no raw upstream errors shown to users
- duplicate detection by provider ID, never by title
- already-added result becomes `Already in Library` or `Already in Backlog` and links to local UUID route
- prompt for initial status before creating personal state
- Backlog may be the first/default option
- do not create a local tracked record until the user explicitly confirms Add
- successful add gives a restrained toast and optional View action

Disambiguation metadata:

- Games: cover, title, year, developer, game type where useful
- Movies: poster, title, year, director where available
- TV: poster, title, first-air year, network/service where available

## 5. IGDB game normalization

Use current IGDB game-type/relationship data, not deprecated categories.

Normalize according to the product specification:

Standalone ratable entries:
- base games
- independently playable titles
- standalone expansions
- remakes
- remasters

Nested/non-ratable additional content:
- DLC requiring base game
- expansions requiring base game
- cosmetic/bonus packs
- updates/content packs
- editions such as Deluxe/Gold/Collector's when they are not independent games

Ports/platform releases generally collapse into one title. Platforms are metadata only; do not add a personal `platform played` field.

Nested additional content must not get its own personal rating/status/Add state.

## 6. Strict TVDB → TMDB mapping

TVDB is authoritative for TV metadata. TMDB is used only for the secondary community rating when safely mapped.

For each TV show:

1. identify by exact TVDB ID
2. query TMDB's external-ID lookup using that TVDB ID
3. require a TV candidate
4. reverse-verify that candidate's external IDs include the exact same TVDB ID
5. only then persist the TMDB show ID

Requirements:

- no automatic title/year fuzzy matching
- no title-search fallback
- missing or ambiguous mapping means community score is omitted
- TVDB ID unique
- verified TMDB ID unique when present
- cache verified mapping

## 7. Community ratings

Exactly one secondary public/community rating per domain:

- Games: IGDB user `rating` + `rating_count`, normalize 0–100 → 1.0–10.0
- Movies: TMDB `vote_average` + `vote_count`
- TV: verified TMDB `vote_average` + `vote_count`

Personal rating must remain visually dominant.

Community score:

- read-only
- source-labelled
- muted/secondary
- vote count available in tooltip/popover or detail metadata where useful
- sortable from Library
- not duplicated all over cards

Do not add IMDb scraping, critic scores, Rotten Tomatoes, Metacritic, TVDB generic `score`, or multiple community-score sources.

## 8. Library and Backlog

Routes must follow the existing UUID architecture and product spec.

Navigation:

- Games → Library / Backlog
- Movies → Library / Backlog
- TV Shows → Library / Backlog

Normal Library excludes Backlog and includes:

- In Progress
- On Hold
- Abandoned
- Completed

Backlog gets a separate page.

### Library functionality

Implement:

- local text search
- status filtering
- rated/unrated filtering where useful
- year filtering
- genre filtering
- responsive artwork-first grid
- optional compact list view
- URL-aware filters/sorts
- remembered per-user/view preference where practical within existing Settings architecture
- preserve scroll when opening detail and returning where feasible

Sorting options required:

- My Rating high → low (default)
- My Rating low → high
- Community Rating high → low
- Title A → Z
- Title Z → A
- Release date newest → oldest
- Release date oldest → newest
- Date Added newest → oldest
- Date Added oldest → newest

Under My Rating sorts, unrated entries sort after rated entries.

Backlog must not show meaningless rating placeholders or rating sorting.

## 9. Cards and inline actions

Cards should stay restrained and useful.

### Games card
- cover
- title
- year
- status
- personal rating or `Rate`

### Movies card
- poster
- title
- year
- runtime
- status
- personal rating or `Rate`

### TV card
- poster
- title
- year
- status
- personal rating or `Rate`
- for In Progress: compact episode progress/bar
- Backlog: no meaningless 0/N progress treatment

Personal rating is an independent click target:

- rated → e.g. `★ 8.7`
- unrated non-Backlog → `☆ Rate`
- Backlog → no Rate action

Use one reusable rating editor behavior supporting 0.1 increments and optional reason.

Overflow menu may contain status changes, artwork access where appropriate, and remove.

## 10. Game detail page

Implement the locked Game v1 detail design:

Hero:
- restrained backdrop
- cover
- English title
- year
- developer
- 2–3 genres
- status
- personal rating
- secondary IGDB community score
- only useful Remake/Remaster badge

Overview:
- IGDB `summary`
- do not prioritize `storyline`

Info:
- first proper release date
- developer/publisher
- genres
- game modes
- platforms as metadata only
- franchise
- useful mode labels such as Single-player / Online co-op / Local co-op / Online multiplayer

Screenshots:
- approximately 3–5 useful screenshots
- lightbox
- mobile-friendly horizontal presentation
- no giant carousel

Additional content:
- nested DLC/expansions/etc according to normalization
- labels/info where useful
- no independent rating/status/Add controls for nested content

Related releases:
- Original / Remake / Remaster where relationships are explicit
- each separately tracked when it is an independent title
- no generic Similar Games carousel

Franchise:
- small row/list
- show local state/rating when a related title is already tracked

External links low priority:
- official site
- Steam
- GOG
- Epic where provider data supports them safely
- Wikipedia where explicitly available/reliable

Exclude product-spec non-goals such as age rating clutter, critic score, keywords, engine, provider timestamps, visible UUID, popularity/hype, generic similarity feed.

## 11. Movie detail page

Implement the locked Movie v1 detail design.

Hero:
- TMDB backdrop + poster
- English title
- year
- runtime
- director
- 2–3 genres
- status
- personal rating
- secondary TMDB community score
- trailer action where available

Overview:
- English TMDB overview

Info:
- release date
- runtime formatted naturally
- director
- genres
- primary production companies

Cast:
- approximately 8–10 principal cast
- no separate People domain

Key crew:
- director
- screenplay/writing
- cinematography
- music where cleanly available
- no exhaustive crew dump

Collection:
- use TMDB `belongs_to_collection`
- show collection entries cleanly
- tracked entries show local state
- untracked entries show Not in Library + Add/open provider action

Trailer:
- small action/lightbox, not a giant embedded hero

External links low priority:
- official
- IMDb where TMDB provides ID
- TMDB

Do not add Where to Watch, recommendations, public reviews, critic scores, budget/revenue clutter, popularity, keyword dumps, language/country noise, or giant image galleries.

## 12. TV detail page and episode progress

TVDB is authoritative.

Hero:
- TVDB backdrop/poster
- English title
- first-air year
- main network/service
- 2–3 genres
- personal status
- personal rating
- verified TMDB community score when mapping exists

Progress:
- prominent but compact regular-episode watched count + percentage
- In Progress shows next unwatched regular episode, e.g. `S2 E4 · Title`
- Season 0 / Specials excluded from overall percentage
- Specials individually trackable
- status is always manual; episode progress never auto-changes status

Overview:
- English TVDB overview

Info:
- first aired
- provider series status distinct from personal status
- main/original network
- genres
- regular season count
- regular episode count

Seasons:
- accordion/collapsible presentation
- season number/year
- watched count/total
- compact progress bar
- Specials shown separately and clearly state that they do not affect overall progress
- avoid dumping every episode expanded at once

Episode rows:
- watched checkbox/control
- episode number
- title
- runtime where known
- air date where useful
- still optional
- overview collapsed or secondary
- no episode ratings

Bulk progress actions:
- mark season watched
- mark season unwatched
- mark all through selected episode
- mark all regular episodes watched

Specials track independently, e.g. `2/7`.

Cast:
- approximately 8–10 principal TVDB cast

Creators/key people only when provider data is clean and useful.

Do not add regional streaming availability, generic recommendations, episode public ratings, reviews, awards, note-taking, watch dates, rewatch counters, or progress-driven status changes.

Canonical episode ordering is TVDB's normal/default aired order for v1.

## 13. Artwork manager

Provider-only artwork customization.

For Games, Movies, and TV Shows support slots where provider supports them:

- Poster/Cover
- Backdrop
- Logo

Rules:

- no local file upload in v1
- no arbitrary image URLs
- provider preferred/high-ranked artwork becomes default
- user may independently pin a specific provider artwork choice per slot
- metadata refresh must preserve valid pins
- if pinned artwork disappears upstream, fall back to provider default and surface a restrained unavailable state where useful
- always request/prefer English metadata/title-bearing artwork where possible; fall back cleanly when unavailable
- TV season posters may use provider defaults but no manual season-artwork editor in v1

Use a restrained dialog/sheet with Poster / Backdrop / Logo tabs.

## 14. Metadata refresh and caching

Implement provider metadata caching and refresh behavior appropriate for a long-running self-hosted service.

Requirements:

- refresh provider-owned canonical metadata without overwriting user-owned state
- preserve personal status/rating/reason/progress/artwork pins
- preserve verified TVDB↔TMDB mapping unless revalidation explicitly proves it invalid
- community ratings refresh
- provider outages do not make existing local Library/detail pages unusable
- no interactive request should block indefinitely on background refresh work
- manual `Refresh metadata` action may be exposed where useful
- bounded concurrency; no unbounded goroutine spawning

## 15. Remove behavior

Removing a tracked item permanently removes the user's personal record for that domain.

Remove:
- status
- personal rating
- rating reason
- artwork pins
- TV episode progress for that user/show

Canonical provider cache records may remain if safely reusable, but no orphaned personal data.

Require destructive confirmation. TV confirmation must explicitly mention episode progress.

No trash/archive/history layer.

## 16. Authentication and authorization

All media tracking is user-owned.

Require an authenticated Gradeium session for Library, Backlog, Add, rating, progress, artwork selection, remove, metadata refresh, and other personal actions.

Provider configuration remains admin-only.

Preserve existing CSRF/origin requirements for unsafe authenticated requests.

Never authorize via UI state alone.

## 17. UX and responsive design

Follow `docs/DESIGN.md` strictly.

Required:

- clean desktop sidebar and mobile equivalent
- restrained shadcn design
- no giant hero banners
- no gradients/blobs/glassmorphism
- no fake statistics
- no decorative cards just to fill space
- artwork supplies most visual character
- keyboard/focus accessibility
- good empty/loading/error states
- mobile Add flow may use a full-screen sheet/dialog
- mobile filters in Sheet
- avoid accidental card-open when using rating/overflow controls
- maintain useful target sizes

Do not let this milestone's large scope become an excuse for inconsistent one-off UI patterns. Reuse deliberate shared primitives while keeping domain-specific pages/domain logic separate.

---

# Persistence and migrations

Use explicit forward migrations from the current Phase 3 schema.

Requirements:

- PostgreSQL 18
- UUIDv7 entity IDs
- provider IDs uniquely constrained
- sensible foreign keys and indexes
- no destructive reset migration
- sqlc-generated code checked into the repository if sqlc remains the established query strategy
- generated-code verification remains in CI
- migration from a real Phase 3 database must work

Large metadata payloads should be normalized enough to support reliable querying/sorting and avoid turning the database into an unstructured provider-response dump. It is acceptable to retain narrowly scoped raw/provider payload fragments only where they clearly reduce fragility, but canonical product fields must be first-class.

---

# Provider credential / secret security

All provider secrets must use the Phase 2 encrypted secret service.

Must verify:

- plaintext absent from database rows
- plaintext absent from API responses
- plaintext absent from logs
- replace/remove semantics work
- existing key-loss/mismatch behavior remains fail-closed

Do not copy secrets into new unrelated tables.

---

# Testing and verification

Because this is intentionally a dense milestone, verification must be correspondingly strong.

## Backend

Run at minimum:

```text
gofmt -l .
go mod tidy
go mod verify
go test -count=1 ./...
go test -count=1 -race ./...
go vet ./...
```

With PostgreSQL 18 integration coverage for:

- Phase 3 → Phase 4 migration
- UUID/provider uniqueness
- each domain's canonical + user-state persistence
- Backlog/rating invariants
- rating precision
- duplicate prevention
- TV episode progress and Specials exclusion
- strict TVDB↔TMDB mapping persistence rules
- artwork pins
- remove semantics
- metadata refresh preserving user state
- concurrent adds/updates where races matter

## Provider protocol tests

Use deterministic mock/test servers and fixtures where practical.

Cover:

- provider auth/config validation
- bounded timeout/error behavior
- credential redaction
- IGDB normalization edge cases
- movie collection parsing
- TVDB→TMDB exact mapping success/failure/reverse-verification cases
- community rating normalization
- artwork fallback/pin preservation
- malformed/oversized upstream responses

Do not make CI depend on live third-party APIs.

## Frontend

Run:

```text
npm ci
npm run lint
npm run typecheck
npm run build
```

Add meaningful frontend tests if the repository's current setup supports them without disproportionate scaffolding. At minimum verify domain state/formatting utilities and high-risk interactions where practical.

## Docker / Compose

Run:

```text
docker build --tag gradeium:phase4 .
docker compose config --quiet
```

Verify a production-shaped Compose deployment:

- migrates Phase 3 database to Phase 4
- fresh install works
- non-root/read-only/no-new-privileges preserved
- `/api/healthz` and `/api/readyz` behavior preserved
- provider outage does not affect liveness/readiness
- PostgreSQL outage behavior still correct
- restart persistence for users, provider settings, media, ratings, progress, and artwork pins
- graceful shutdown
- master-key recovery behavior unchanged

## Browser / end-to-end verification

Perform a real production UI walkthrough with mock/local provider endpoints or controlled fixtures sufficient to exercise the product without relying on public providers.

Verify at minimum:

### Games
- configure/test IGDB
- search/add game
- duplicate state
- Library ↔ Backlog transitions
- rate/edit reason
- filters/sorts
- detail page
- DLC/related-release normalization
- artwork selection
- remove

### Movies
- configure/test TMDB
- search/add movie
- rate/status
- collection display
- trailer interaction where fixture exists
- artwork selection
- remove

### TV
- configure/test TVDB + TMDB
- search/add series
- strict verified mapping
- season/episode progress
- Specials exclusion
- next-episode display
- bulk progress actions
- status independence from progress
- rating
- artwork
- remove warning/progress deletion

### Cross-cutting
- mobile layout around 390×844
- keyboard/focus basics
- no browser console errors
- unauthenticated access protection
- non-admin provider-settings protection where a test identity can cover it
- CSRF rejection for unsafe API calls
- provider outage: local libraries/details still usable
- restart persistence

---

# Strict non-goals for Phase 4

Do **not** implement:

- Dashboard analytics
- automatic backup creation
- backup restore
- CSV export
- backup scheduling/retention
- Jellyfin
- Steam/Letterboxd/Trakt/import integrations
- custom lists
- Favorites
- social/public profiles
- user-management UI
- invitations/groups
- local-password auth
- SAML/LDAP
- API keys
- notifications
- mobile apps
- AI/LLM features
- Where to Watch
- recommendations/similar-media feeds beyond explicitly specified franchise/collection/related-release relationships

Jellyfin is explicitly post-1.0 and must not be introduced as scaffolding or runtime code in this phase.

---

# Implementation strategy

This is one milestone/branch/PR, but Codex should work through internal checkpoints without stopping for approval:

1. schema/migrations + provider settings registry
2. provider clients + deterministic provider tests
3. Games end-to-end
4. Movies end-to-end
5. TV end-to-end
6. shared Library/Backlog/rating/artwork UX refinement
7. refresh/error/offline behavior
8. full backend/frontend/race/Docker/Compose/browser verification
9. full diff review against every project doc

Fix defects discovered during later checkpoints even if they belong to earlier checkpoints.

Do not open intermediate PRs.

---

# Delivery

When complete:

1. review the entire diff against `AGENTS.md` and every file under `docs/`
2. confirm no Phase 5/6 or post-1.0 scope slipped in
3. run all available required verification and report exactly what actually ran
4. do not claim a check passed unless it ran
5. commit clearly
6. push the dedicated branch, preferably `codex/phase-4-full-media`
7. open a **draft PR** against `main`
8. PR body must include `Closes #8`
9. do not merge the PR

The milestone is complete only when all three media domains are genuinely usable end-to-end, not when only schemas or provider clients exist.
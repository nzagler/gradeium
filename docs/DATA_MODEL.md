# Gradeium Data Model Specification

This document defines the intended relational shape and invariants. Exact column names/types may be refined during implementation, but the behavioral guarantees should remain.

## Principles
- PostgreSQL is authoritative.
- Gradeium-owned UUIDv7 IDs are primary identities.
- Provider IDs are unique external references.
- Shared entity identity is minimal; domain metadata remains separate.
- User-owned state is separate from provider metadata.
- Important invariants should be enforced in PostgreSQL where practical, not only in UI code.

## Entity registry

```text
entities
- id UUID PRIMARY KEY DEFAULT uuidv7()
- type ENUM/CHECK: GAME | MOVIE | TV_SHOW
- created_at timestamptz
```

Purpose:
- guarantee global media-item identity;
- support generic cross-domain references later (for example custom lists);
- do not place domain metadata here.

## Users

```text
users
- id UUID PRIMARY KEY DEFAULT uuidv7()
- external_subject text NULL (unused Phase 2 compatibility column)
- oidc_issuer text NULL
- oidc_subject text NULL
- display_name text NULL
- email text NULL
- is_admin boolean NOT NULL DEFAULT false
- created_at timestamptz
- updated_at timestamptz
```

`oidc_issuer` + `oidc_subject` form the unique stable identity once OIDC is active. Email and display name are mutable verified metadata, never account identity. Both OIDC identity columns are nullable only for safe migration from the Phase 2 foundation.

## Authentication state and sessions

```text
authentication_state
- singleton boolean PRIMARY KEY
- configuration_revision bigint
- validated_revision bigint NULL
- activated boolean
- activated_at timestamptz NULL
- active_revision bigint NULL
- active_issuer_url text NULL
- active_client_id text NULL
- active_public_url text NULL
```

The active fields are a last-known-good validated snapshot. Editing a draft setting cannot alter login behavior until that exact revision validates and is atomically published. Activation never transitions back to false.

```text
sessions
- id UUIDv7 PRIMARY KEY
- user_id UUID REFERENCES users(id) ON DELETE CASCADE
- token_hash bytea UNIQUE
- expires_at timestamptz
- revoked_at timestamptz NULL
- created_at / updated_at
```

The raw session bearer token exists only in the HTTP-only browser cookie. Login state/nonce/PKCE material is short-lived, one-time, and encrypted in the `oidc_login_flows` table until callback consumption.

## Games

```text
games
- entity_id UUID PRIMARY KEY REFERENCES entities(id) ON DELETE CASCADE
- igdb_id bigint UNIQUE NOT NULL
- english_title text NOT NULL
- original_title text NULL
- summary text NULL
- first_release_date date/timestamptz NULL
- release_year int NULL
- community_rating numeric/integer-cache NULL
- community_rating_count int NULL
- metadata_refreshed_at timestamptz NULL
- provider_payload_version int NULL
```

Additional normalized/cache tables may hold:
- companies and game-company relationships;
- genres;
- platforms;
- game modes;
- screenshots;
- provider artwork alternatives;
- franchises/collections;
- explicit related releases;
- nested additional-content relationships.

Do not model required-base-game DLC as independently user-ratable Gradeium library entities.

## Movies

```text
movies
- entity_id UUID PRIMARY KEY REFERENCES entities(id) ON DELETE CASCADE
- tmdb_id bigint UNIQUE NOT NULL
- english_title text NOT NULL
- original_title text NULL
- overview text NULL
- release_date date NULL
- release_year int NULL
- runtime_minutes int NULL
- community_rating numeric/integer-cache NULL
- community_rating_count int NULL
- metadata_refreshed_at timestamptz NULL
```

Additional normalized/cache tables may hold:
- genres;
- production companies;
- cast;
- key crew;
- artwork alternatives;
- trailers/videos;
- collection membership;
- external links/IDs.

## TV Shows

```text
tv_shows
- entity_id UUID PRIMARY KEY REFERENCES entities(id) ON DELETE CASCADE
- tvdb_id bigint UNIQUE NOT NULL
- verified_tmdb_id bigint UNIQUE NULL
- english_title text NOT NULL
- original_title text NULL
- overview text NULL
- first_aired date NULL
- release_year int NULL
- provider_status text NULL
- network_name text NULL
- community_rating numeric/integer-cache NULL
- community_rating_count int NULL
- tmdb_mapping_verified_at timestamptz NULL
- metadata_refreshed_at timestamptz NULL
- community_rating_refreshed_at timestamptz NULL
```

`verified_tmdb_id` is optional and may only be populated after strict external-ID verification.

No fuzzy title matching is permitted for automatic mapping.

## TV Seasons

```text
tv_seasons
- id UUID PRIMARY KEY DEFAULT uuidv7()
- tv_show_id UUID NOT NULL REFERENCES tv_shows(entity_id) ON DELETE CASCADE
- tvdb_season_id bigint UNIQUE NOT NULL
- season_number int NOT NULL
- name text NULL
- is_specials boolean NOT NULL DEFAULT false
- air_date date NULL
- episode_count int NULL
- artwork_reference ... NULL
```

Uniqueness should prevent duplicate canonical seasons for the same show/order.

`is_specials` should reflect canonical Season 0 / Specials handling and is used to exclude specials from overall regular progress.

## TV Episodes

```text
tv_episodes
- id UUID PRIMARY KEY DEFAULT uuidv7()
- tv_show_id UUID NOT NULL REFERENCES tv_shows(entity_id) ON DELETE CASCADE
- season_id UUID NOT NULL REFERENCES tv_seasons(id) ON DELETE CASCADE
- tvdb_episode_id bigint UNIQUE NOT NULL
- season_number int NOT NULL
- episode_number int NOT NULL
- english_title text NOT NULL
- overview text NULL
- air_date date NULL
- runtime_minutes int NULL
- still_reference ... NULL
- is_special boolean NOT NULL DEFAULT false
```

No user rating columns belong on season or episode tables.

## Status
Use one shared status domain/enum/check:

```text
BACKLOG
IN_PROGRESS
ON_HOLD
ABANDONED
COMPLETED
```

UI labels use exactly:
- Backlog
- In Progress
- On Hold
- Abandoned
- Completed

## User Game State

```text
user_games
- user_id UUID REFERENCES users(id) ON DELETE CASCADE
- game_id UUID REFERENCES games(entity_id) ON DELETE CASCADE
- status status_type NOT NULL
- rating smallint NULL
- rating_reason text NULL
- date_added timestamptz NOT NULL DEFAULT now()
- selected_cover_provider_image_id text NULL
- selected_backdrop_provider_image_id text NULL
- selected_logo_provider_image_id text NULL
- PRIMARY KEY (user_id, game_id)
```

## User Movie State

```text
user_movies
- user_id UUID REFERENCES users(id) ON DELETE CASCADE
- movie_id UUID REFERENCES movies(entity_id) ON DELETE CASCADE
- status status_type NOT NULL
- rating smallint NULL
- rating_reason text NULL
- date_added timestamptz NOT NULL DEFAULT now()
- selected_poster_provider_image_id text NULL
- selected_backdrop_provider_image_id text NULL
- selected_logo_provider_image_id text NULL
- PRIMARY KEY (user_id, movie_id)
```

## User TV State

```text
user_tv_shows
- user_id UUID REFERENCES users(id) ON DELETE CASCADE
- tv_show_id UUID REFERENCES tv_shows(entity_id) ON DELETE CASCADE
- status status_type NOT NULL
- rating smallint NULL
- rating_reason text NULL
- date_added timestamptz NOT NULL DEFAULT now()
- selected_poster_provider_image_id text NULL
- selected_backdrop_provider_image_id text NULL
- selected_logo_provider_image_id text NULL
- PRIMARY KEY (user_id, tv_show_id)
```

## Rating constraints
Ratings are stored as integers:
- 10 = 1.0
- 11 = 1.1
- ...
- 100 = 10.0
- NULL = unrated

DB-level constraint should enforce:

```text
rating IS NULL OR (rating BETWEEN 10 AND 100)
```

Backlog invariant:

```text
status = BACKLOG => rating IS NULL AND rating_reason IS NULL
```

Use a DB CHECK where possible. If the chosen PostgreSQL type/table arrangement makes the complete invariant awkward, enforce it transactionally in domain code and cover it with tests.

`rating_reason` may be NULL even when rating is present.

A reason without a rating should normally be invalid.

## Episode Progress

```text
user_episode_progress
- user_id UUID REFERENCES users(id) ON DELETE CASCADE
- episode_id UUID REFERENCES tv_episodes(id) ON DELETE CASCADE
- watched boolean NOT NULL DEFAULT true
- PRIMARY KEY (user_id, episode_id)
```

Alternative implementation may store only watched rows and interpret absence as unwatched. Prefer whichever produces simpler, safer queries and bulk mutations.

Progress rules:
- specials are trackable;
- specials are excluded from regular-series progress calculations;
- changing TV status never deletes episode progress;
- deleting the TV item for that user deletes the user's episode progress for that show.

Bulk operations should execute transactionally.

## Provider Artwork
The implementation may use a generic provider-artwork table with domain relationships or domain-specific tables.

Required information per provider image should include enough to:
- identify the exact provider image stably;
- know image type (poster/cover/backdrop/logo/season poster/etc.);
- know language when supplied;
- know ranking/default/provider-preference information where supplied;
- construct or retrieve the image URL without trusting arbitrary user URLs.

Manual selections in user state store provider image identity, not arbitrary remote URLs.

If a selected provider image disappears from the refreshed provider set, do not silently choose a new manual selection. The domain service should fall back safely to current provider default for display and surface the pinned selection as unavailable where UX warrants.

## Settings
Separate non-secret and secret configuration where useful.

Conceptual tables:

```text
app_settings
- key text PRIMARY KEY
- value jsonb
- updated_at timestamptz
```

and/or strongly typed tables for important settings such as:

```text
user_settings
- user_id UUID PRIMARY KEY
- default_library_sort ...
- preferred_view ...
```

Admin/integration configuration should be typed enough to validate safely.

## Integration Configuration
Conceptual model:

```text
integrations
- id UUID PRIMARY KEY DEFAULT uuidv7()
- type text UNIQUE NOT NULL
- enabled boolean NOT NULL
- non_secret_config jsonb NOT NULL
- encrypted_secret_blob bytea NULL
- encryption_version int NOT NULL
- last_tested_at timestamptz NULL
- last_test_status text NULL
- last_test_message text NULL
- updated_at timestamptz
```

A more normalized per-provider schema is acceptable if it improves safety and clarity.

Never store decrypted secrets in ordinary config JSON.

## Bootstrap / Setup State
Use explicit state to distinguish a new installation from an established installation.

Conceptual:

```text
system_state
- setup_completed boolean
- setup_completed_at timestamptz NULL
- schema/application metadata...
```

The first-run setup route must cease being generally accessible after completion.

## Backup Settings

```text
backup_settings
- enabled boolean
- interval specification
- retention_count int
- last_successful_backup_at timestamptz NULL
- next_due_at timestamptz NULL / derivable
```

Backup inventory metadata may be recorded in DB:

```text
backups
- id UUID PRIMARY KEY
- filename text UNIQUE
- created_at timestamptz
- size_bytes bigint
- sha256 text
- format_version int
- application_version text
- kind automatic | manual | pre_restore
- valid boolean
```

The backup file itself resides on `/backups` persistent storage.

## Portable Backup Format
Use a versioned Gradeium-owned format rather than a raw SQL dump as the primary in-app backup.

Conceptual root:

```json
{
  "format": "gradeium-backup",
  "formatVersion": 1,
  "createdAt": "...",
  "applicationVersion": "...",
  "users": [],
  "games": [],
  "movies": [],
  "tvShows": [],
  "episodeProgress": [],
  "settings": [],
  "lists": []
}
```

Do not include decrypted provider secrets in portable personal-data backups.

The backup should contain enough provider IDs and local identities to restore the user's library/state and rehydrate provider metadata later.

## Future Custom Lists
Prepare for:

```text
lists
- id UUID
- user_id UUID
- name text
- ...

list_items
- list_id UUID
- entity_id UUID REFERENCES entities(id)
- position/order optional
```

This is why the global `entities` registry exists.

Do not implement custom lists in initial v1 unless explicitly scheduled later.

## Indexes
At minimum consider indexes supporting:
- provider unique IDs;
- user+status library queries;
- user+rating sorting;
- title/search helpers where local filtering becomes server-side;
- TV show season/episode ordering;
- episode progress by user/show;
- metadata refresh due queries;
- backup due queries.

Do not create speculative indexes without measuring/query justification, but ensure core Library queries are indexed from the start.

## Deletion semantics
### Remove from user's library
Delete the relevant `user_*` state row and dependent user-owned progress/state. Provider metadata entity may remain cached if useful for other users or future re-add, subject to future cleanup policy.

### Provider metadata cleanup
No aggressive provider-cache garbage collector is required for initial v1.

### Remove a user
Cascade personal state appropriately without destroying shared provider metadata needed by other users.

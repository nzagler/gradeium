# Gradeium Product Specification

## Product goal
Gradeium is a self-hosted personal media tracker and rating application for Games, Movies, and TV Shows. It is designed primarily for one user but must not paint the data model into a single-user-only corner.

The product should feel calm, modern, fast, and polished. It is not a social network, review platform, streaming guide, or activity diary.

## Media domains
Games, Movies, and TV Shows are separate first-class domains with separate navigation, feature modules, pages, and provider integrations.

Shared concepts may be reused where appropriate, but the product must not feel like one generic media list with different icons.

## Common statuses
Every media type uses exactly these labels:

- Backlog
- In Progress
- On Hold
- Abandoned
- Completed

Backlog has its own dedicated view and is excluded from the normal Library view.

## Rating model
- Default user rating scale: 0.0–10.0.
- Increment: 0.1.
- 5.0 represents the midpoint; below 5.0 is negative, above 5.0 is positive.
- Ratings are optional for all statuses except Backlog, where ratings are forbidden.
- A rating may have one optional free-text reason explaining why that score was chosen.
- No rating history.
- No completion requirement before rating.
- Moving a rated item back to Backlog requires confirmation and deletes the current rating and rating reason.

## No activity diary
Gradeium intentionally does not track:
- completion dates;
- watch/play start dates;
- replay/rewatch counts;
- prior statuses;
- prior ratings;
- prior rating reasons;
- played platform;
- watch history timeline.

`date_added` may be stored for library administration and sorting.

## Community ratings
Gradeium also displays one secondary community/user score:
- Games: IGDB user rating + rating count.
- Movies: TMDB user vote average + vote count.
- TV Shows: TMDB user vote average + vote count, only after a strict verified TVDB↔TMDB mapping.

Community ratings must be visually secondary to the Gradeium user's own rating.

Do not display critic-score aggregations in v1.

## Metadata language
All provider metadata should request/prefer English.

If no English value exists, fall back to the provider's original/default value rather than presenting an empty field.

## Metadata refresh
Provider metadata refreshes automatically on a freshness policy defined by the implementation.

Manual user artwork choices must never be overwritten by automatic refreshes.

Existing locally stored library content must remain usable if an external metadata provider is unavailable.

## Identity
Every tracked Game, Movie, and TV Show receives a globally unique Gradeium-owned UUIDv7.

Provider IDs are external references and must never be the application's primary identity.

Provider IDs must be unique within the appropriate domain.

## Search and add
Library search and provider search are distinct interactions:
- Library search filters already-added local items.
- Add searches the domain's metadata provider.

Provider search results should include enough information to distinguish similarly named items.

If an item already exists locally, provider search shows `Already in Library` or `Already in Backlog`, does not show an Add action, and opens the existing Gradeium item when selected.

Nothing is persisted locally merely because it appeared in search results. A local entity is created only when the user chooses to add it and selects an initial status.

## Library
The normal Library contains:
- In Progress;
- On Hold;
- Abandoned;
- Completed.

Default view: responsive artwork card grid.
Optional secondary view: list/table.

### Default sorting
Default Library sort is My Rating high-to-low.
- Unrated items appear after rated items.
- The default sort is editable in Settings.
- Runtime/manual sort selection can differ from the configured default.

Supported baseline sorting:
- My Rating high-to-low / low-to-high;
- Community Rating high-to-low;
- Title A–Z / Z–A;
- Release Date newest / oldest;
- Date Added newest / oldest.

Supported baseline filters:
- Status;
- Rated / Unrated / rating range;
- Release year;
- Genre.

Filter/sort/search state should be represented in the URL where practical so browser Back/Forward and refresh behave predictably.

### Rating from Library
Users must be able to create/edit their rating and rating reason directly from the Library without opening the detail page.

The card itself still opens the detail page. Backlog cards have no rating action.

## Backlog
Backlog is a dedicated page per media domain, not merely a Library filter.

Backlog items:
- cannot be rated;
- should not show meaningless rating placeholders;
- should remain visually clean.

## Removing items
Removing an item permanently deletes that user's associated personal data after a destructive confirmation.

For TV Shows this includes episode progress.

No Archive/Trash/Restore feature is required in v1.

## Games
Metadata provider: IGDB.

### Inclusion model
- Remakes: separate ratable Games.
- Remasters: separate ratable Games.
- Independently playable releases: may be separate ratable Games.
- Content that requires the base game to play: nested under the parent Game and never independently ratable.
- Do not create separate library entries for normal DLC, required-base-game expansions, editions, packs, updates, or bonus content.

### Game detail page
Primary content:
- Backdrop and cover;
- English title;
- release year/date;
- developer;
- publisher;
- genres;
- game modes;
- supported platforms as provider metadata;
- user's status;
- user's rating + optional reason;
- IGDB community rating;
- overview;
- screenshots;
- nested additional content;
- related original/remake/remaster releases when explicit;
- franchise/collection when useful;
- understated external links.

Do not show in v1:
- age rating;
- public reviews;
- critic rating;
- provider popularity/hype;
- keywords;
- engine;
- language support;
- provider timestamps;
- generic recommendation carousels.

## Movies
Metadata provider: TMDB.

### Movie detail page
Primary content:
- Backdrop and poster;
- English title;
- release year/date;
- runtime;
- director;
- genres;
- primary production companies;
- user's status;
- user's rating + optional reason;
- TMDB community rating;
- overview;
- principal cast + characters;
- restrained key crew information where useful;
- explicit TMDB collection/series membership;
- trailer action when a suitable trailer exists;
- understated external links.

Do not show in v1:
- TMDB popularity;
- budget/revenue;
- age certification;
- public reviews;
- keywords;
- current streaming availability;
- generic recommendations.

## TV Shows
Metadata provider: TVDB.
Community rating provider: TMDB after verified mapping only.

### TVDB↔TMDB mapping
- Never match by title, year, artwork, or other fuzzy metadata.
- Resolve TMDB using the exact TVDB external ID.
- Verify the reverse external-ID relationship before persisting the mapping.
- A missing or ambiguous mapping means no community score is shown.
- Once verified, persist the TMDB ID and refresh rating data from that exact ID.
- TVDB remains authoritative for title, descriptions, seasons, episodes, artwork, cast, network, and other TV metadata.

### TV detail page
Primary content:
- Backdrop and poster;
- English title;
- first-air year/date;
- network/service;
- genres;
- provider show status;
- user's status;
- user's rating + optional reason;
- verified TMDB community rating;
- overview;
- overall regular-episode progress;
- next unwatched regular episode where meaningful;
- season and episode tracking;
- principal cast;
- useful key people only when provider data is good;
- understated external links.

### Episode progress
- Only the whole TV Show is ratable.
- Seasons are never ratable.
- Episodes are never ratable.
- Track watched state per episode.
- Season 0 / Specials can be tracked but never count toward overall progress.
- Use TVDB's normal/default aired ordering in v1.
- Support marking a single episode watched/unwatched.
- Support marking a season watched/unwatched.
- Support marking all episodes through a selected episode watched.
- Support whole-series watched/unwatched where appropriate.
- Episode progress survives status changes, including moving to Backlog.
- Status remains user-controlled rather than being force-changed by episode progress.

## Artwork management
Artwork behavior should feel similar in spirit to Jellyfin image management while staying simpler.

Per tracked item, manage independently when available:
- Poster / Cover;
- Backdrop;
- Logo.

Rules:
- Only images from that item's metadata provider are selectable.
- No arbitrary URL entry in v1.
- No user uploads in v1.
- Provider default/preferred artwork is initially selected.
- User-selected images are pinned independently and survive metadata refreshes.
- If an artwork type has no useful options, do not render an empty management section.
- Season posters may be displayed from TVDB but do not need manual per-season customization in v1.

## Dashboard
Provide one cross-domain Dashboard with useful current-state analytics rather than fake activity/history metrics.

Potential v1 widgets:
- Total Games / Movies / TV Shows;
- Currently In Progress;
- Average user rating per domain;
- User rating distribution;
- Status distribution;
- TV progress summaries;
- Highest-rated items;
- Backlog counts.

Dashboard stats should support All / Games / Movies / TV filtering where it improves clarity.

Do not invent trends such as `+12% this month` because Gradeium intentionally does not store completion/activity history.

## Custom lists
Not required for v1, but the data architecture should make future user-defined mixed-media lists possible.

No hard-coded Favorites feature in v1.

## Imports
No external library/import integrations are required in v1. Add manually through provider search.

Future imports may be added later.

## Jellyfin
Not required for initial v1 implementation, but architecture must leave room for a future Jellyfin integration through Jellyfin APIs and/or webhooks.

Do not tightly couple core Gradeium behavior to Jellyfin.

## Backups
Portable automatic backups are a v1 requirement.

Default:
- enabled;
- every 3 days;
- retain latest 30.

Settings allow changing interval and retention.

Portable backups contain user-owned data rather than re-downloadable provider metadata:
- statuses;
- ratings;
- rating reasons;
- episode progress;
- artwork selections;
- user/admin settings where safe;
- future user-created lists.

Provide:
- automatic scheduled backup;
- Create Backup Now;
- list backup files;
- Download;
- Restore;
- Delete;
- retention cleanup;
- pre-restore safety backup;
- atomic validated backup creation;
- versioned format;
- gzip compression;
- CSV export for human-readable ratings/library data.

## Administration and setup
Gradeium should be configured through its own UI wherever possible.

### First-run experience
A protected first-run setup flow should guide the administrator through creating/establishing the initial Gradeium administrator and configuring integrations.

### Admin Settings
Provide dedicated integration cards/pages for:
- IGDB;
- TMDB;
- TVDB;
- Authentication / generic OIDC / Pocket ID;
- backups;
- Jellyfin when implemented;
- future integrations.

Each integration should expose useful states/actions such as:
- Not configured / Configured / Error;
- Test connection;
- Replace credentials;
- Remove/disable integration;
- provider-specific non-secret configuration.

Secrets must be stored encrypted at rest and never echoed back to the browser after saving.

External-service credentials should not require editing `.env` files or Docker templates after Gradeium has started.

Only infrastructure configuration that is fundamentally required before the web application can boot is exempt from this rule, and that bootstrap surface should be minimized.

## UX philosophy
- shadcn-oriented visual language;
- neutral and restrained shell;
- artwork provides visual personality;
- useful information first;
- few clicks for common actions;
- responsive desktop/mobile behavior;
- proper skeleton loading states;
- graceful empty/error states;
- keyboard/focus accessibility;
- no decorative filler;
- no oversized marketing-style heroes;
- no badge soup;
- no fake statistics;
- no excessive animation.

Rule of thumb: every visible element must either communicate useful information or provide a useful action.

# Gradeium Design Specification

## Design direction
Gradeium should look and feel modern, clean, calm, and intentional.

The overall visual language should be oriented around shadcn/ui: restrained surfaces, strong typography, subtle borders, sensible spacing, familiar controls, and minimal decorative noise.

Artwork from the media itself provides most of the visual personality.

## Principles
- Content first.
- Useful actions should be easy to reach.
- Do not add UI merely to fill space.
- Avoid badge soup.
- Avoid oversized hero areas that waste vertical space.
- Avoid strong gradients, glassmorphism, or decorative animations unless they directly improve comprehension.
- Use subtle motion for state transitions, dialogs, sheets, hover feedback, and progress changes only.
- Keep mobile behavior intentional rather than simply stacking the desktop layout.
- Empty, loading, and error states must be polished and concise.

## Navigation
Desktop should use a persistent, compact sidebar.

Suggested hierarchy:

```text
Dashboard

Games
  Library
  Backlog

Movies
  Library
  Backlog

TV Shows
  Library
  Backlog

Settings
```

Do not add separate navigation destinations for Completed, In Progress, On Hold, or Abandoned. Those are filters within Library.

On mobile, use a compact navigation treatment appropriate to the viewport. Do not force a desktop sidebar into a narrow screen.

## Shared Library views
### Header
Each media domain should have a concise page header with:
- domain title;
- local library search;
- Add action;
- sort/filter controls;
- grid/list view toggle where appropriate.

### Library contents
Library excludes Backlog and includes:
- In Progress;
- On Hold;
- Abandoned;
- Completed.

Default sort: My Rating high-to-low.

### Grid cards
Cards should answer primarily:
- What is it?
- What year/relevant secondary fact identifies it?
- What is my status?
- What is my rating?

Do not place descriptions, large metadata lists, or unnecessary badges on cards.

Rated items expose the user's rating as an interactive control. Unrated non-Backlog items expose a concise `Rate` action. Rating opens the same rating editor used on detail pages.

The optional rating reason is editable there but is not printed on normal cards.

### List view
List view is optional but useful for larger libraries.

Baseline columns:
- artwork thumbnail;
- title;
- release year;
- status;
- user's rating.

TV may additionally show regular-episode progress.

Status and rating may be directly editable in list view if interaction remains clear and accessible.

### Backlog
Backlog is its own page.

Do not show disabled rating controls or meaningless `N/A` placeholders.

Because the entire page already represents Backlog, repeating a Backlog badge on every card is optional and should be decided based on visual redundancy.

## Add/Search flow
Provider search is separate from local library search.

Use a large dialog, sheet, or dedicated responsive search experience rather than an admin-style form.

Search results should include enough metadata to distinguish similarly named records.

### Game search result
- cover;
- English title;
- release year;
- developer;
- game type label only where useful (for example Remake/Remaster).

### Movie search result
- poster;
- English title;
- release year;
- director where available.

### TV search result
- poster;
- English title;
- first-air year;
- network/service where available.

Existing items show `Already in Library` or `Already in Backlog`, are clickable to the local detail page, and have no Add button.

Adding a new result asks only for the initial status. Do not open a large metadata/edit form.

## Rating editor
Use one shared rating interaction across all domains.

Requirements:
- exact 0.1 increments;
- direct numeric interaction plus convenient +/- or similarly precise controls;
- optional rating-reason textarea;
- clear save/cancel behavior;
- Backlog has no active rating editor.

The user's rating is visually more prominent than the community rating.

## Community rating
Detail pages display one secondary provider/community score and rating count when available.

It should be smaller/more muted than the user's rating and labeled with its source (IGDB or TMDB).

Community rating does not need to appear on standard library cards by default.

## Detail page visual structure
All detail pages share a broad visual rhythm but remain domain-specific.

### Hero
- restrained backdrop;
- readable gradient/overlay only as needed for contrast;
- poster/cover;
- title;
- concise key metadata;
- user's status;
- user's rating;
- secondary community rating.

The hero must not consume most of the first viewport.

### Overflow actions
Use a discreet overflow menu for less-common actions:
- Manage artwork;
- Refresh metadata;
- Remove from library.

Do not show a row of large administrative buttons.

## Game detail page
Suggested hierarchy:
1. Hero
2. Overview
3. Game Information
4. Screenshots
5. Additional Content
6. Related Releases
7. Franchise/Collection
8. External Links

### Hero key data
- cover;
- backdrop;
- English title;
- release year;
- developer;
- concise genres;
- status;
- user's rating;
- IGDB community rating.

### Game information
Useful fields:
- release date;
- developer;
- publisher;
- genres;
- game modes;
- platforms;
- franchise where useful.

### Screenshots
Show a restrained gallery (roughly 3–5 initially visible on desktop) with lightbox/detail behavior.

### Additional content
Required-base-game DLC/expansions/bonus content appear as secondary informational entries under the parent game.

They do not expose Gradeium rating/status/Add controls.

### Related releases
Explicit originals/remakes/remasters may appear as real linked Gradeium/provider entities.

## Movie detail page
Suggested hierarchy:
1. Hero
2. Overview
3. Movie Information
4. Cast
5. Key Crew
6. Collection
7. External Links

### Hero key data
- poster;
- backdrop;
- English title;
- year;
- runtime;
- director;
- concise genres;
- status;
- user's rating;
- TMDB community rating;
- Trailer action only when a suitable trailer exists.

### Cast
Show a limited principal cast row/grid (roughly 8–10) with:
- profile image;
- actor name;
- character name.

Do not turn People into a fourth major library domain in v1.

### Key crew
Keep this restrained to roles that materially help a movie page, for example:
- Director;
- Screenplay;
- Cinematography;
- Music.

### Collection
Explicit TMDB collection membership may show sibling movies with local Gradeium state/rating where available.

## TV detail page
Suggested hierarchy:
1. Hero
2. User Progress
3. Overview
4. Show Information
5. Seasons & Episodes
6. Cast
7. Key People where genuinely useful
8. External Links

### Hero key data
- poster;
- backdrop;
- English title;
- first-air year;
- network/service;
- concise genres;
- user's status;
- user's rating;
- verified TMDB community rating.

### Progress
Show regular-episode progress compactly near the top.

Examples:
- `13 of 19 episodes`;
- progress bar;
- next unwatched regular episode when meaningful.

Specials are tracked but excluded from overall progress.

### Seasons
Use collapsible/accordion-like season sections.

Collapsed season card should show:
- season number/name;
- year where useful;
- watched count / total;
- progress bar.

Specials clearly indicate that they do not affect overall progress.

### Episode rows
Useful fields only:
- watched checkbox/control;
- episode number;
- English title;
- runtime if available;
- air date in secondary text where useful.

Episode descriptions/stills may appear in an expanded row/sheet rather than making every row oversized.

Bulk actions:
- mark season watched/unwatched;
- mark through selected episode watched;
- whole-series watched/unwatched in a secondary menu.

## Artwork management
Artwork management should be inspired by Jellyfin's practical image-selection model while matching Gradeium's restrained design.

Use tabs or equivalent sections for available types:
- Poster/Cover;
- Backdrop;
- Logo.

For each type:
- clearly identify current selection;
- show provider-supplied alternatives in a clean image grid;
- selection is immediate or explicitly saveable, but behavior must be consistent;
- language preference is English where artwork contains text;
- provider default is used until user pins an alternative.

Do not render an empty tab/type if the provider offers no useful alternatives.

## Dashboard
The Dashboard should use shadcn-style statistic cards and a small number of useful charts.

Potential layout:
- total Games / Movies / TV Shows;
- currently In Progress strip/grid;
- average user rating by domain;
- user rating distribution chart;
- status distribution chart;
- TV progress summaries;
- highest-rated items;
- Backlog counts.

Allow All / Games / Movies / TV filtering where useful.

Charts must use only data Gradeium actually stores. Do not display fake trends.

## Settings
Settings should be organized into clear sections rather than one huge form.

Suggested areas:
- General
- Library defaults
- Integrations
- Authentication
- Backups
- System/About

Admin-only sections should be clearly distinguished when multi-user support exists.

### Integrations
Use provider cards with:
- provider logo/name where permitted/appropriate;
- configured/not configured/error status;
- brief purpose;
- Configure/Edit action;
- Test Connection action;
- last successful test/refresh where useful;
- never reveal stored secret values.

## Empty states
Keep empty states concise and non-cutesy.

Examples:
- `No movies yet. Add your first movie to start your library.`
- `Your backlog is empty.`

Use one primary action where appropriate.

Avoid marketing-style copy such as `Embark on your cinematic journey`.

## Errors
External provider errors should be scoped to the affected operation.

Example:
- existing library continues to render;
- Add Game may show `IGDB could not be reached. Try again.`

Never expose raw stack traces or provider JSON to normal users.

## Loading
Use skeletons that preserve layout dimensions.

Avoid large global spinners for ordinary page/data loading.

## Responsive behavior
### Desktop
- persistent sidebar;
- responsive multi-column artwork grid;
- popovers/dialogs where appropriate.

### Mobile
- two-column poster/cover grid where practical;
- filters in a Sheet or full-width surface;
- Add/provider search may use full-screen dialog/sheet behavior;
- season/episode controls remain comfortably tappable.

## Accessibility
- keyboard-navigable dialogs/menus/search;
- visible focus states;
- correct labels and descriptions;
- meaningful button text/accessible names;
- sensible contrast over artwork;
- no interaction available only on hover.

## Motion
Motion should be subtle and functional:
- dialog/sheet transitions;
- hover/focus feedback;
- progress changes;
- optional gentle card repositioning after rating changes re-sort the grid.

Do not use motion as decoration.
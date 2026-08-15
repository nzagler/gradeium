# Changelog

## 1.1.0 (upcoming)

- Added encrypted Jellyfin API-key configuration, library discovery/mapping, and manual add-only Movie/TV imports using TMDB and TVDB canonical provider IDs.
- Added per-user 1–10, 0–5, -5–+5, and 0–100 personal rating scales while preserving canonical 0–100 ratings, sorting, backups, and existing values.
- Replaced browser number controls with a controlled, keyboard- and mobile-friendly rolling-digit rating editor.

## 1.0.1

- Fixed TVDB artwork paths and legacy TVDB image URLs so they resolve through the HTTPS TVDB artwork CDN.
- Added an authenticated global quick-add menu for Games, TV Shows, and Movies.

## 1.0.0 release candidate

- Completed Games, Movies, and TV Shows libraries, backlogs, details, ratings, artwork, metadata refresh, and TV episode progress.
- Added the local-state Dashboard, ratings CSV export, and versioned portable JSON.gz backups with transactional restore, automatic pre-restore safety backups, scheduling, and retention.
- Added generic OIDC authentication, PostgreSQL-backed sessions, CSRF and origin protection, first-admin binding, encrypted provider settings, and first-run setup.
- Added per-user Dark, Light, and System themes. Dark is the fallback for new and existing users without a stored preference and is applied before first render.
- Hardened PostgreSQL 18 migrations and recovery, provider outage isolation, backup disaster recovery, Docker/Compose runtime behavior, accessibility, responsive layouts, and production build information.

Other post-1.0 ideas such as custom lists, social/history features, public APIs, and user-management UI are intentionally not included.

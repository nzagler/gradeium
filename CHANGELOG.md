# Changelog

## 1.0.0 release candidate

- Completed Games, Movies, and TV Shows libraries, backlogs, details, ratings, artwork, metadata refresh, and TV episode progress.
- Added the local-state Dashboard, ratings CSV export, and versioned portable JSON.gz backups with transactional restore, automatic pre-restore safety backups, scheduling, and retention.
- Added generic OIDC authentication, PostgreSQL-backed sessions, CSRF and origin protection, first-admin binding, encrypted provider settings, and first-run setup.
- Added per-user Dark, Light, and System themes. Dark is the fallback for new and existing users without a stored preference and is applied before first render.
- Hardened PostgreSQL 18 migrations and recovery, provider outage isolation, backup disaster recovery, Docker/Compose runtime behavior, accessibility, responsive layouts, and production build information.

Post-1.0 ideas such as Jellyfin, imports, custom lists, social/history features, public APIs, and user-management UI are intentionally not included.

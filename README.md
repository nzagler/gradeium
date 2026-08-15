# Gradeium

Gradeium is a self-hosted personal media tracker for **Games, Movies, and TV Shows**. It keeps your ratings, statuses, progress, and library data under your control while using external providers only for metadata.

## Features

- **Games:** IGDB
- **Movies:** TMDB
- **TV Shows:** TVDB
- Library and Backlog views with personal statuses and ratings
- Configurable rating scales: **0–10, 0–5, -5 to +5, and 0–100**
- Provider artwork selection, metadata refresh, and manual metadata rematching
- TV season and episode progress tracking
- Manual, add-only **Jellyfin import/resync** for Movies and TV Shows
- Dashboard, ratings CSV export, portable backups, and safe restore
- Generic **OpenID Connect** authentication; Pocket ID is the primary tested provider
- Docker deployment with PostgreSQL

## Quick start

Requirements: **Docker Engine + Docker Compose v2**.

```bash
cp .env.example .env
docker compose up --build -d
```

Open `http://localhost:8080` and complete the first-run setup.

For production, use HTTPS and configure the exact public Gradeium URL in OIDC. The callback URI is:

```text
https://your-gradeium-domain.example/api/auth/callback
```

## Integrations

Configure providers under **Settings → Integrations**:

- **IGDB:** Twitch client ID and client secret
- **TMDB:** API Read Access Token
- **TVDB:** API key and optional subscriber PIN
- **Jellyfin:** server URL, API key, and library mappings

Secrets are encrypted and are not returned by the API after saving.

## Metadata maintenance

Under **Settings → Library → Metadata maintenance**, Games, Movies, and TV Shows can each be refreshed in bulk. Individual items can also be rematched to a different IGDB, TMDB, or TVDB entry without deleting the Gradeium item or its personal state.

## Data and backups

Persist PostgreSQL, `/config`, and `/backups`.

`/config/master.key` protects encrypted settings and must be backed up separately. Portable Gradeium backups intentionally do **not** contain provider/OIDC secrets or the master key.

## AI-assisted development

Gradeium was created with extensive assistance from **OpenAI Codex and ChatGPT**. AI was used throughout implementation, refactoring, debugging, testing, review, and documentation, with project direction, architecture, product decisions, and final approval handled by the maintainer.

## License

Gradeium is licensed under the **GNU Affero General Public License v3.0 (AGPL-3.0)**. See [`LICENSE`](./LICENSE).

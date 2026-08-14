# Gradeium

Gradeium is a self-hosted media tracker and rating application for Games, Movies, and TV Shows.

The project is designed for long-running Docker deployment on Unraid, with a lightweight Go backend, React/TypeScript frontend, PostgreSQL persistence, shadcn-oriented UI, generic OIDC authentication, provider-backed metadata, and portable automatic backups.

## Planned stack

- Go backend (`net/http` + `chi`)
- React + TypeScript + Vite
- shadcn/ui + Tailwind CSS
- PostgreSQL
- `pgx` + `sqlc`
- Generic OIDC, with Pocket ID as the primary supported provider
- Docker / Unraid

## Metadata providers

- Games: IGDB
- Movies: TMDB
- TV Shows: TVDB
- TV community score: TMDB only after strict TVDB↔TMDB external-ID verification

## Project status

Gradeium is currently in the specification/foundation stage. Application code should be implemented incrementally according to the repository documents rather than generated as one monolithic task.

Start with:

- [`AGENTS.md`](./AGENTS.md) — persistent implementation instructions for Codex/agents
- [`docs/PRODUCT_SPEC.md`](./docs/PRODUCT_SPEC.md) — product behavior and feature requirements
- [`docs/ARCHITECTURE.md`](./docs/ARCHITECTURE.md) — runtime, security, integration, and reliability architecture
- [`docs/DESIGN.md`](./docs/DESIGN.md) — UI/UX and page design requirements
- [`docs/DATA_MODEL.md`](./docs/DATA_MODEL.md) — relational model and invariants
- [`docs/IMPLEMENTATION_PLAN.md`](./docs/IMPLEMENTATION_PLAN.md) — phased development plan

## Configuration philosophy

External-service credentials are intended to be configured from Gradeium's own protected Admin Settings UI rather than by editing `.env` files. This includes metadata providers, OIDC/Pocket ID, backups, and future Jellyfin integration.

Only the smallest unavoidable infrastructure bootstrap surface should exist outside the application UI.

## License

Gradeium is licensed under the GNU Affero General Public License v3.0. See [`LICENSE`](./LICENSE).
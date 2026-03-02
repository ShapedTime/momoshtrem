# Contributing

Thanks for your interest in contributing! Whether it's a bug report, feature suggestion, documentation improvement, or code change — all contributions are welcome.

## Development Setup

### Prerequisites

- [Go 1.25+](https://go.dev/dl/)
- [Node.js 20+](https://nodejs.org/)
- [Docker](https://docs.docker.com/get-docker/) and Docker Compose

### Full Stack (Docker)

The fastest way to get everything running:

```bash
cp .env.example .env        # Fill in required variables
cp Caddyfile.example Caddyfile
docker compose up -d
```

### Frontend Development

```bash
cd PrettyTVCatalog
npm install
npm run dev
```

The dev server runs at `http://localhost:3000`. It expects momoshtrem and Jackett to be running (via Docker or locally).

### Backend Development

```bash
cd momoshtrem
go build -o momoshtrem ./cmd/momoshtrem
./momoshtrem
```

The backend needs a PostgreSQL database and a TMDB API key. See `.env.example` for required environment variables.

## Code Quality

Each component has its own code quality guide:

- **Go (momoshtrem):** [`docs/momoshtrem/CODE_QUALITY.md`](docs/momoshtrem/CODE_QUALITY.md) — interfaces, concurrency patterns, error handling
- **TypeScript (PrettyTVCatalog):** [`docs/prettytvcatalog/CODE_QUALITY.md`](docs/prettytvcatalog/CODE_QUALITY.md) — architecture, strict mode, API patterns

Key principles:

- API routes are thin orchestration — business logic lives in `lib/api/` or `internal/`
- TypeScript strict mode, no `any`, use type guards
- Go interfaces for testability, structured error handling
- Custom error classes: `APIError`, `NotFoundError`, `ValidationError`

## Commit Messages

Use [Conventional Commits](https://www.conventionalcommits.org/):

```
feat: add subtitle language filter
fix: handle missing episode metadata gracefully
refactor: extract torrent assignment logic into service
chore: update Go dependencies
docs: add WebDAV setup instructions
```

## Pull Request Process

1. Fork the repository and create a feature branch from `main`
2. Make your changes, following the code quality guides linked above
3. Test your changes — run the full stack with `docker compose up -d` and verify behavior
4. Write a clear PR description explaining **what** changed and **why**
5. Link any related issues

## Reporting Bugs

When filing a bug report, please include:

- Steps to reproduce the issue
- Expected vs actual behavior
- Relevant logs (`docker compose logs <service>`)
- Your environment (OS, Docker version, browser)

## Feature Requests

Feature requests are welcome! When proposing a feature:

- Describe the **use case** rather than a specific solution
- Explain how it fits into the existing workflow
- Note if you'd be interested in implementing it yourself

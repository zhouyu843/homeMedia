# HomeMedia Project Guidelines

## Communication
- Always reply to the user in Chinese unless the user explicitly asks for another language.
- Prefer concise, engineering-focused answers. Do not give long theory explanations unless asked.

## Workflow
- Before making a non-trivial change, read README.md first to align with the current architecture and commands.
- After changing behavior, commands, setup, routes, configuration, or project structure, update README.md in the same task.
- Use Docker Compose to run project commands whenever possible. Do not assume the host machine has a usable Go toolchain.
- Prefer changes that reduce long-term maintenance cost and clarify ownership boundaries, even if they are not the smallest possible patch.
- Frontend page work should default to React + TypeScript SPA patterns, not SSR templates or isolated islands.

## Build And Test
- Use TDD pragmatically: write or update tests first for critical behavior changes, then implement.
- Prioritize tests around login, auth status, upload, list, detail, trash, download, file-type validation, missing-file handling, and path safety.
- Prefer running commands through the app container, for example `docker compose run --rm app go test ./...`.
- For frontend changes, use containerized commands from `web/frontend` through Docker Compose.
- Do not assume `gofmt` is available in the current app container image; verify tool availability before prescribing containerized formatting steps.

## Architecture
- Keep the project as a small monolith. Do not introduce microservices, CQRS, event sourcing, or heavy abstraction.
- Apply light DDD only where it improves clarity: keep core business rules in the media domain, not scattered across handlers.
- Treat PostgreSQL as metadata storage only. Store original media files in the mounted local directory.
- Keep boundaries clear between HTTP, domain logic, repository, local file storage, and frontend API client.
- Prefer Gin, PostgreSQL, local storage, and Docker Compose unless there is a strong reason to change.
- Keep Go as the single runtime entrypoint. Frontend assets are built from `web/frontend` and served by Gin from `web/static`.
- Keep auth/session/CSRF authority on the Go side. React consumes those capabilities through explicit `/api/` endpoints.
- Prefer explicit `/api/` JSON endpoints for frontend interactions. Avoid mixing long-term HTML rendering concerns into handler logic.

## Conventions
- Favor maintainable decisions over MVP-only shortcuts when the two conflict.
- Do not add AI features, search, sharing, object storage, background workers, or complex auth unless explicitly requested.
- When adding abstractions, require a real current need, not a hypothetical future use case.
- Keep README.md, docker-compose.yml, migrations, and environment examples aligned with implementation.
- Preserve safe path handling for local file access and download behavior.
- Keep frontend API calls in a dedicated client layer instead of embedding request details directly in UI components.
- Remove obsolete SSR templates, transitional glue code, and stale build artifacts when a migration makes them unnecessary.

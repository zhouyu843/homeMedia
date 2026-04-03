# HomeMedia Project Guidelines

## Communication
- Always reply to the user in Chinese unless the user explicitly asks for another language.
- Prefer concise, engineering-focused answers. Do not give long theory explanations unless asked.

## Workflow
- Before making a non-trivial change, read README.md first to align with the current project usage and scope.
- After changing behavior, commands, setup, routes, configuration, or project structure, update README.md in the same task.
- Use Docker Compose to run project commands whenever possible. Do not assume the host machine has a usable Go toolchain.
- Prefer small, end-to-end deliverables that keep the MVP working at each step.

## Build And Test
- Use TDD pragmatically: write or update tests first for critical behavior changes, then implement.
- Prioritize tests around upload, list, detail, download, file-type validation, missing-file handling, and path safety.
- Prefer running commands through the app container, for example `docker compose run --rm app go test ./...`.
- If formatting is needed, use the containerized toolchain, for example `docker compose run --rm app sh -c 'gofmt -w ./cmd ./internal'`.

## Architecture
- Keep the project as a small monolith. Do not introduce microservices, CQRS, event sourcing, or heavy abstraction.
- Apply light DDD only where it improves clarity: keep core business rules in the media domain, not scattered across handlers.
- Treat PostgreSQL as metadata storage only. Store original media files in the mounted local directory.
- Keep boundaries clear between HTTP, domain logic, repository, and local file storage, but avoid academic layering.
- Prefer Gin, PostgreSQL, local storage, and Docker Compose unless there is a strong reason to change.

## Conventions
- Favor MVP-first decisions. Do not add AI features, search, sharing, object storage, background workers, or complex auth unless explicitly requested.
- When adding abstractions, require a real current need, not a hypothetical future use case.
- Keep README.md, docker-compose.yml, migrations, and environment examples aligned with implementation.
- Preserve safe path handling for local file access and download behavior.

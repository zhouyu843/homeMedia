test:
	docker compose run --rm app go test ./...

fmt:
	docker compose run --rm app sh -c 'gofmt -w ./cmd ./internal'

up:
	docker compose up --build

down:
	docker compose down

migrate:
	docker compose run --rm migrate

frontend-install:
	docker compose run --rm app sh -lc 'cd web/frontend && npm install'

frontend-build:
	docker compose run --rm app sh -lc 'cd web/frontend && npm run build'

frontend-dev:
	docker compose run --rm --service-ports app sh -lc 'cd web/frontend && npm run dev'

frontend-test:
	docker compose run --rm app sh -lc 'cd web/frontend && npm run test'

backfill-video-thumbnails:
	docker compose run --rm app go run ./cmd/backfill-thumbnails -media-types=video

.PHONY: e2e
e2e:
	docker compose --profile e2e run --rm e2e

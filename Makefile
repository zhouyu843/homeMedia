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

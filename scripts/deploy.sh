#!/usr/bin/env sh

set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
PROJECT_ROOT=$(dirname "$SCRIPT_DIR")
ENV_FILE=.env.production
COMPOSE_FILE=docker-compose.prod.yml

cd "$PROJECT_ROOT"

on_exit() {
	status=$?
	if [ "$status" -eq 0 ]; then
		return
	fi

	echo "deployment failed" >&2
	echo "compose status:" >&2
	docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" ps >&2 || true
	echo "recent app logs:" >&2
	docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" logs --tail=100 app >&2 || true
	echo "recent postgres logs:" >&2
	docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" logs --tail=50 postgres >&2 || true
	echo "hint: fix the error above and rerun ./scripts/deploy.sh" >&2
}

trap on_exit EXIT

if [ ! -f "$ENV_FILE" ]; then
	echo "missing env file: $ENV_FILE" >&2
	echo "create it from .env.production.example first" >&2
	exit 1
fi

if ! command -v docker >/dev/null 2>&1; then
	echo "docker is required" >&2
	exit 1
fi

if ! docker compose version >/dev/null 2>&1; then
	echo "docker compose is required" >&2
	exit 1
fi

if ! command -v git >/dev/null 2>&1; then
	echo "warning: git is not installed; skipping git pull" >&2
else
	if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
		if [ -n "$(git status --porcelain)" ]; then
			echo "warning: git working tree is not clean; skipping git pull" >&2
		else
			echo "[1/5] pulling latest code"
			git pull --ff-only
		fi
	else
		echo "warning: current directory is not a git work tree; skipping git pull" >&2
	fi
fi

mkdir -p data/uploads

echo "[2/5] starting postgres"
docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" up -d postgres

echo "[3/5] applying migrations"
docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" run --rm migrate

echo "[4/5] building and starting app"
docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" up -d --build app

if ! docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" ps --status running --services app | grep -qx app; then
	echo "app container is not running after startup" >&2
	exit 1
fi

echo "[5/5] current status"
docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" ps

trap - EXIT

echo "deploy finished successfully"
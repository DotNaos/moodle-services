#!/usr/bin/env bash
set -euo pipefail

IMAGE="${MOODLE_DOCKER_IMAGE:-ghcr.io/dotnaos/moodle-services:latest}"
DEPLOY_DIR="${MOODLE_DEPLOY_DIR:-${HOME}/moodle-services}"
COMPOSE_FILE="${MOODLE_COMPOSE_FILE:-docker-compose.yml}"
HEALTH_URL="${MOODLE_HEALTH_URL:-http://127.0.0.1:8080/healthz}"

if [[ ! -d "${DEPLOY_DIR}" ]]; then
  echo "Deploy directory not found: ${DEPLOY_DIR}" >&2
  exit 1
fi

if [[ ! -f "${DEPLOY_DIR}/${COMPOSE_FILE}" ]]; then
  echo "Compose file not found: ${DEPLOY_DIR}/${COMPOSE_FILE}" >&2
  exit 1
fi

cd "${DEPLOY_DIR}"

if [[ -n "${GHCR_TOKEN:-}" && -n "${GHCR_USERNAME:-}" ]]; then
  echo "Logging in to ghcr.io..."
  echo "${GHCR_TOKEN}" | docker login ghcr.io -u "${GHCR_USERNAME}" --password-stdin >/dev/null
fi

echo "Pulling ${IMAGE}..."
docker pull "${IMAGE}"

echo "Recreating moodle-services..."
docker compose -f "${COMPOSE_FILE}" pull
docker compose -f "${COMPOSE_FILE}" up -d --remove-orphans

echo "Waiting for ${HEALTH_URL}..."
for _ in $(seq 1 30); do
  if curl -fsS "${HEALTH_URL}" >/dev/null; then
    curl -fsS "${HEALTH_URL}"
    echo
    echo "Deploy OK (${IMAGE})"
    exit 0
  fi
  sleep 2
done

echo "Health check failed after deploy." >&2
docker compose -f "${COMPOSE_FILE}" ps >&2
exit 1

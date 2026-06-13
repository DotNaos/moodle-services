#!/usr/bin/env bash
set -euo pipefail

IMAGE="${MOODLE_DOCKER_IMAGE:-ghcr.io/dotnaos/moodle-services:latest}"
DEPLOY_DIR="${MOODLE_DEPLOY_DIR:-${HOME}/moodle-services}"
COMPOSE_FILE="${MOODLE_COMPOSE_FILE:-docker-compose.yml}"
HEALTH_URL="${MOODLE_HEALTH_URL:-http://127.0.0.1:8080/healthz}"
MIGRATIONS_DIR="${MOODLE_MIGRATIONS_DIR:-${DEPLOY_DIR}/migrations}"
POSTGRES_SERVICE="${MOODLE_POSTGRES_SERVICE:-postgres}"

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

if [[ -d "${MIGRATIONS_DIR}" ]]; then
  echo "Applying database migrations from ${MIGRATIONS_DIR}..."
  shopt -s nullglob
  migrations=("${MIGRATIONS_DIR}"/*.sql)
  if [[ "${#migrations[@]}" -eq 0 ]]; then
    echo "No migration files found."
  else
    POSTGRES_USER="${MOODLE_POSTGRES_USER:-$(docker compose -f "${COMPOSE_FILE}" exec -T "${POSTGRES_SERVICE}" printenv POSTGRES_USER 2>/dev/null || true)}"
    POSTGRES_DB="${MOODLE_POSTGRES_DB:-$(docker compose -f "${COMPOSE_FILE}" exec -T "${POSTGRES_SERVICE}" printenv POSTGRES_DB 2>/dev/null || true)}"
    POSTGRES_USER="${POSTGRES_USER:-postgres}"
    POSTGRES_DB="${POSTGRES_DB:-${POSTGRES_USER}}"
    for migration in "${migrations[@]}"; do
      echo "Applying $(basename "${migration}")..."
      docker compose -f "${COMPOSE_FILE}" exec -T "${POSTGRES_SERVICE}" \
        psql -v ON_ERROR_STOP=1 -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" < "${migration}"
    done
  fi
else
  echo "Migration directory not found: ${MIGRATIONS_DIR}. Skipping database migrations."
fi

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

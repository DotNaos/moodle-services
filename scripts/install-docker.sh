#!/usr/bin/env bash
set -euo pipefail

IMAGE="${MOODLE_DOCKER_IMAGE:-ghcr.io/dotnaos/moodle-services:latest}"
PAYLOAD=""
BIN_DIR="${HOME}/.local/bin"
DATA_DIR="${HOME}/.moodle"
LEGACY_DIR="${HOME}/.moodle-cli"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --payload)
      PAYLOAD="${2:-}"
      shift 2
      ;;
    --image)
      IMAGE="${2:-}"
      shift 2
      ;;
    --bin-dir)
      BIN_DIR="${2:-}"
      shift 2
      ;;
    *)
      echo "Unknown argument: $1" >&2
      exit 2
      ;;
  esac
done

if [[ -z "${PAYLOAD}" ]]; then
  echo "--payload is required" >&2
  exit 2
fi

if ! command -v docker >/dev/null 2>&1; then
  echo "Docker is required but was not found in PATH." >&2
  exit 1
fi

if ! docker info >/dev/null 2>&1; then
  echo "Docker is installed, but the Docker daemon is not reachable." >&2
  exit 1
fi

mkdir -p "${DATA_DIR}" "${BIN_DIR}"

if [[ -d "${LEGACY_DIR}" ]] && [[ -z "$(find "${DATA_DIR}" -mindepth 1 -maxdepth 1 2>/dev/null)" ]]; then
  echo "Legacy ${LEGACY_DIR} data exists. Leaving it untouched; run 'moodle config migrate-home' after install if you want to copy it."
fi

echo "Pulling ${IMAGE}..."
docker pull "${IMAGE}" >/dev/null

echo "Applying bootstrap payload..."
docker run --rm \
  -v "${DATA_DIR}:/data" \
  -e MOODLE_HOME=/data \
  "${IMAGE}" bootstrap apply --payload "${PAYLOAD}" >/dev/null

cat > "${BIN_DIR}/moodle" <<'WRAPPER'
#!/usr/bin/env bash
set -euo pipefail

IMAGE="${MOODLE_DOCKER_IMAGE:-__MOODLE_DOCKER_IMAGE__}"
DATA_DIR="${MOODLE_HOME_HOST:-${HOME}/.moodle}"
mkdir -p "${DATA_DIR}"

if [[ -t 0 && -t 1 ]]; then
  docker_args=(
    --rm -it
    -v "${DATA_DIR}:/data"
    -e MOODLE_HOME=/data
    -e MOODLE_DOCKER_CONTAINER_DATA_DIR=/data
    -e MOODLE_DOCKER_HOST_DATA_DIR="${DATA_DIR}"
    -e MOODLE_OCR_CACHE_DIR=/data/ocr/cache
    -e TERM="${TERM:-xterm-256color}"
  )
  if [[ -S /var/run/docker.sock ]]; then
    docker_args+=(-v /var/run/docker.sock:/var/run/docker.sock)
  fi
  exec docker run \
    "${docker_args[@]}" \
    "${IMAGE}" "$@"
fi

docker_args=(
  --rm
  -v "${DATA_DIR}:/data"
  -e MOODLE_HOME=/data
  -e MOODLE_DOCKER_CONTAINER_DATA_DIR=/data
  -e MOODLE_DOCKER_HOST_DATA_DIR="${DATA_DIR}"
  -e MOODLE_OCR_CACHE_DIR=/data/ocr/cache
  -e TERM="${TERM:-xterm-256color}"
)
if [[ -S /var/run/docker.sock ]]; then
  docker_args+=(-v /var/run/docker.sock:/var/run/docker.sock)
fi

exec docker run \
  "${docker_args[@]}" \
  "${IMAGE}" "$@"
WRAPPER
escaped_image="${IMAGE//\\/\\\\}"
escaped_image="${escaped_image//&/\\&}"
escaped_image="${escaped_image//|/\\|}"
sed -i.bak "s|__MOODLE_DOCKER_IMAGE__|${escaped_image}|g" "${BIN_DIR}/moodle"
rm -f "${BIN_DIR}/moodle.bak"
chmod 0755 "${BIN_DIR}/moodle"

add_line_once() {
  local file="$1"
  local line="$2"
  mkdir -p "$(dirname "${file}")"
  touch "${file}"
  if ! grep -Fqx "${line}" "${file}"; then
    printf '\n%s\n' "${line}" >> "${file}"
  fi
}

path_line='export PATH="$HOME/.local/bin:$PATH"'
add_line_once "${HOME}/.profile" "${path_line}"
[[ -n "${BASH_VERSION:-}" || -f "${HOME}/.bashrc" ]] && add_line_once "${HOME}/.bashrc" "${path_line}"
[[ -n "${ZSH_VERSION:-}" || -f "${HOME}/.zshrc" ]] && add_line_once "${HOME}/.zshrc" "${path_line}"

install_completion() {
  mkdir -p "${HOME}/.local/share/bash-completion/completions"
  if "${BIN_DIR}/moodle" completion bash > "${HOME}/.local/share/bash-completion/completions/moodle" 2>/dev/null; then
    :
  fi

  mkdir -p "${HOME}/.local/share/zsh/site-functions"
  if "${BIN_DIR}/moodle" completion zsh > "${HOME}/.local/share/zsh/site-functions/_moodle" 2>/dev/null; then
    add_line_once "${HOME}/.zshrc" 'fpath=("$HOME/.local/share/zsh/site-functions" $fpath)'
    add_line_once "${HOME}/.zshrc" 'autoload -Uz compinit && compinit'
  fi

  if command -v fish >/dev/null 2>&1 || [[ -d "${HOME}/.config/fish" ]]; then
    mkdir -p "${HOME}/.config/fish/completions"
    "${BIN_DIR}/moodle" completion fish > "${HOME}/.config/fish/completions/moodle.fish" 2>/dev/null || true
  fi
}

echo "Installing shell completion..."
install_completion

echo "Verifying Moodle access..."
"${BIN_DIR}/moodle" --json list courses >/dev/null

echo "Moodle Services is installed."
echo "Wrapper: ${BIN_DIR}/moodle"
echo "Data: ${DATA_DIR}"
echo "Open a new shell if 'moodle' is not yet in PATH."

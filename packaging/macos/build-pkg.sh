#!/usr/bin/env bash
set -euo pipefail

archive=""
version=""
output=""
signing_identity=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --archive)
      archive="$2"
      shift 2
      ;;
    --version)
      version="$2"
      shift 2
      ;;
    --output)
      output="$2"
      shift 2
      ;;
    --signing-identity)
      signing_identity="$2"
      shift 2
      ;;
    *)
      echo "Unknown argument: $1" >&2
      exit 1
      ;;
  esac
done

if [[ -z "${archive}" || -z "${version}" || -z "${output}" ]]; then
  echo "Usage: $0 --archive <path> --version <tag> --output <path>" >&2
  exit 1
fi

work_dir="$(mktemp -d)"
payload_dir="${work_dir}/payload/usr/local/bin"
trap 'rm -rf "${work_dir}"' EXIT

mkdir -p "${payload_dir}"
COPYFILE_DISABLE=1 tar -xzf "${archive}" -C "${work_dir}"
install -m 0755 "${work_dir}/moodle" "${payload_dir}/moodle"
find "${work_dir}/payload" -name '._*' -delete
xattr -cr "${work_dir}/payload"

mkdir -p "$(dirname "${output}")"
rm -f "${output}"

pkgbuild_args=(
  --root "${work_dir}/payload"
  --identifier "com.dotnaos.moodle-services"
  --version "${version#v}"
  --install-location "/"
)

if [[ -n "${signing_identity}" ]]; then
  pkgbuild_args+=(--sign "${signing_identity}")
fi

pkgbuild \
  "${pkgbuild_args[@]}" \
  "${output}"

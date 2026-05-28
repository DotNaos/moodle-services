#!/usr/bin/env bash
set -euo pipefail

version=""
app=""
output=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version)
      version="$2"
      shift 2
      ;;
    --app)
      app="$2"
      shift 2
      ;;
    --output)
      output="$2"
      shift 2
      ;;
    *)
      echo "Unknown argument: $1" >&2
      exit 1
      ;;
  esac
done

if [[ -z "${version}" || -z "${app}" || -z "${output}" ]]; then
  echo "Usage: $0 --version <tag> --app <path> --output <path>" >&2
  exit 1
fi

work_dir="$(mktemp -d)"
stage_dir="${work_dir}/moodle-services"
rw_dmg="${work_dir}/moodle-services-temp.dmg"
attach_plist="${work_dir}/attach.plist"
device=""
cleanup() {
  if [[ -n "${device}" ]]; then
    hdiutil detach "${device}" >/dev/null 2>&1 || true
  fi
  rm -rf "${work_dir}"
}
trap cleanup EXIT

detach_device() {
  local target_device="$1"

  for _ in {1..10}; do
    if hdiutil detach "${target_device}" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done

  hdiutil detach -force "${target_device}" >/dev/null
}

mkdir -p "${stage_dir}"
cp -R "${app}" "${stage_dir}/moodle-services.app"
ln -s /Applications "${stage_dir}/Applications"

mkdir -p "$(dirname "${output}")"
rm -f "${output}"

volume_name="moodle-services ${version}"

hdiutil create \
  -volname "${volume_name}" \
  -srcfolder "${stage_dir}" \
  -fs HFS+ \
  -format UDRW \
  "${rw_dmg}" >/dev/null

hdiutil attach -readwrite -noverify -noautoopen -plist "${rw_dmg}" > "${attach_plist}"

attach_details="$(
  python3 - <<'PY' "${attach_plist}"
import pathlib
import plistlib
import sys

data = plistlib.loads(pathlib.Path(sys.argv[1]).read_bytes())
device = ""
mount_point = ""

for entity in data.get("system-entities", []):
    dev_entry = entity.get("dev-entry", "")
    if dev_entry and not device:
        device = dev_entry

    current_mount = entity.get("mount-point", "")
    if current_mount:
        mount_point = current_mount

print(device)
print(mount_point)
PY
)"

device="$(printf '%s\n' "${attach_details}" | sed -n '1p')"
mount_dir="$(printf '%s\n' "${attach_details}" | sed -n '2p')"

if [[ -n "${device}" && -n "${mount_dir}" ]] && command -v osascript >/dev/null 2>&1; then
  osascript <<EOF || true
tell application "Finder"
  tell disk "${volume_name}"
    open
    delay 1
    set current view of container window to icon view
    set toolbar visible of container window to false
    set statusbar visible of container window to false
    set bounds of container window to {160, 120, 860, 500}
    set icon size of icon view options of container window to 128
    set arrangement of icon view options of container window to not arranged
    close
    open
    delay 1
    set position of item "moodle-services" of container window to {210, 190}
    set position of item "Applications" of container window to {520, 190}
    close
    open
    update without registering applications
    delay 1
    close
  end tell
end tell
EOF
fi

if [[ -n "${device}" ]]; then
  detached_device="${device}"
  detach_device "${device}"
  device=""
  for _ in {1..20}; do
    if ! hdiutil info | grep -Fq "${detached_device}"; then
      break
    fi
    sleep 1
  done
  sync
  sleep 1
fi

hdiutil convert "${rw_dmg}" -format UDZO -o "${output}" >/dev/null

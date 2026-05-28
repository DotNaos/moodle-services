#!/usr/bin/env bash
set -euo pipefail

archive=""
version=""
output=""

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
app_dir="${work_dir}/moodle-services.app"
contents_dir="${app_dir}/Contents"
macos_dir="${contents_dir}/MacOS"
resources_dir="${contents_dir}/Resources"
binary_dir="${resources_dir}/bin"
script_dir="$(cd "$(dirname "$0")" && pwd)"
source_icon="${script_dir}/../../assets/moodle_cli_icon.png"
trap 'rm -rf "${work_dir}"' EXIT

mkdir -p "${macos_dir}" "${binary_dir}"
COPYFILE_DISABLE=1 tar -xzf "${archive}" -C "${work_dir}"
install -m 0755 "${work_dir}/moodle" "${binary_dir}/moodle"

if [[ ! -f "${source_icon}" ]]; then
  echo "Missing app icon at ${source_icon}" >&2
  exit 1
fi

iconset_dir="${work_dir}/moodle-services.iconset"
mkdir -p "${iconset_dir}"
for size in 16 32 64 128 256 512; do
  sips -z "${size}" "${size}" "${source_icon}" --out "${iconset_dir}/icon_${size}x${size}.png" >/dev/null
done
sips -z 32 32 "${source_icon}" --out "${iconset_dir}/icon_16x16@2x.png" >/dev/null
sips -z 64 64 "${source_icon}" --out "${iconset_dir}/icon_32x32@2x.png" >/dev/null
sips -z 256 256 "${source_icon}" --out "${iconset_dir}/icon_128x128@2x.png" >/dev/null
sips -z 512 512 "${source_icon}" --out "${iconset_dir}/icon_256x256@2x.png" >/dev/null
sips -z 1024 1024 "${source_icon}" --out "${iconset_dir}/icon_512x512@2x.png" >/dev/null
iconutil -c icns "${iconset_dir}" -o "${resources_dir}/moodle-services.icns"

cat > "${contents_dir}/Info.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleDevelopmentRegion</key>
  <string>en</string>
  <key>CFBundleDisplayName</key>
  <string>moodle-services</string>
  <key>CFBundleExecutable</key>
  <string>moodle-services</string>
  <key>CFBundleIdentifier</key>
  <string>com.dotnaos.moodle-services</string>
  <key>CFBundleInfoDictionaryVersion</key>
  <string>6.0</string>
  <key>CFBundleIconFile</key>
  <string>moodle-services.icns</string>
  <key>CFBundleName</key>
  <string>moodle-services</string>
  <key>CFBundlePackageType</key>
  <string>APPL</string>
  <key>CFBundleShortVersionString</key>
  <string>${version#v}</string>
  <key>CFBundleVersion</key>
  <string>${version#v}</string>
  <key>LSMinimumSystemVersion</key>
  <string>12.0</string>
  <key>NSHighResolutionCapable</key>
  <true/>
</dict>
</plist>
EOF

cat > "${macos_dir}/moodle-services" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

app_contents="$(cd "$(dirname "$0")/.." && pwd)"
app_bundle="$(dirname "${app_contents}")"
cli_bin="${app_contents}/Resources/bin/moodle"
user_bin="${HOME}/.local/bin"
link_path="${user_bin}/moodle"

if [[ "${app_bundle}" == /Volumes/* ]]; then
  /usr/bin/osascript <<'APPLESCRIPT'
display dialog "Drag moodle-services.app into Applications first, then open it to install the command-line tool." buttons {"OK"} default button "OK" with title "moodle-services"
APPLESCRIPT
  exit 1
fi

mkdir -p "${user_bin}"
ln -sfn "${cli_bin}" "${link_path}"

if [[ "${MOODLE_CLI_NO_TERMINAL:-}" == "1" ]]; then
  export PATH="${user_bin}:${PATH}"
  exec "${link_path}" version
fi

/usr/bin/osascript <<'APPLESCRIPT'
set launchCommand to "export PATH=\"$HOME/.local/bin:$PATH\"; clear; echo \"moodle-services is ready.\"; echo \"The command is linked at ~/.local/bin/moodle.\"; echo; \"$HOME/.local/bin/moodle\" version; echo; echo \"You can now run moodle in this terminal.\"; exec \"$SHELL\" -l"
tell application "Terminal"
  activate
  do script launchCommand
end tell
APPLESCRIPT
EOF

chmod 0755 "${macos_dir}/moodle-services"
find "${app_dir}" -name '._*' -delete
xattr -cr "${app_dir}"

mkdir -p "$(dirname "${output}")"
rm -rf "${output}"
cp -R "${app_dir}" "${output}"

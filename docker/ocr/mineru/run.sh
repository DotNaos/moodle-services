#!/usr/bin/env bash
set -euo pipefail

input="${1:?input PDF required}"
output="${2:?output directory required}"
work="$output/artifacts/mineru"

mkdir -p "$output/images" "$output/artifacts" "$output/logs" "$work"

formula_flag=True
if [[ "${OCR_FORMULA:-0}" != "1" ]]; then formula_flag=False; fi

if command -v mineru >/dev/null 2>&1; then
  mineru -p "$input" -o "$work" -b pipeline -m auto -f "$formula_flag"
elif command -v magic-pdf >/dev/null 2>&1; then
  magic-pdf -p "$input" -o "$work"
else
  echo "MinerU CLI not found in image" >&2
  exit 127
fi

md="$(find "$work" -type f -name '*.md' | head -n 1 || true)"
json="$(find "$work" -type f -name '*.json' | head -n 1 || true)"
html="$(find "$work" -type f -name '*.html' | head -n 1 || true)"

if [[ -n "$md" ]]; then cp "$md" "$output/output.md"; fi
if [[ -n "$json" ]]; then cp "$json" "$output/output.json"; fi
if [[ -n "$html" ]]; then cp "$html" "$output/output.html"; fi
cp "$output/output.md" "$output/text.txt" 2>/dev/null || true
find "$work" -type f \( -name '*.png' -o -name '*.jpg' -o -name '*.jpeg' -o -name '*.webp' \) -exec cp {} "$output/images/" \; 2>/dev/null || true

#!/usr/bin/env bash
set -euo pipefail

input="${1:?input PDF required}"
output="${2:?output directory required}"

mkdir -p "$output/images" "$output/artifacts" "$output/logs"
work="$output/artifacts/docling"
mkdir -p "$work"

rapidocr_models="/usr/local/lib/python3.11/site-packages/rapidocr/models"
if [[ -d /cache && -w /cache && -d "$rapidocr_models" && ! -L "$rapidocr_models" ]]; then
  mkdir -p /cache/rapidocr/models
  cp -an "$rapidocr_models/." /cache/rapidocr/models/ 2>/dev/null || true
  rm -rf "$rapidocr_models"
  ln -s /cache/rapidocr/models "$rapidocr_models"
fi

common_args=("$input" --image-export-mode referenced --output "$work")

if ! docling "${common_args[@]}" --to md --to html --to json; then
  docling "$input" --to md --output "$work"
fi

md="$(find "$work" -type f -name '*.md' | head -n 1 || true)"
html="$(find "$work" -type f -name '*.html' | head -n 1 || true)"
json="$(find "$work" -type f -name '*.json' | head -n 1 || true)"

if [[ -n "$md" ]]; then cp "$md" "$output/output.md"; fi
if [[ -n "$html" ]]; then cp "$html" "$output/output.html"; fi
if [[ -n "$json" ]]; then cp "$json" "$output/output.json"; fi
cp "$output/output.md" "$output/text.txt" 2>/dev/null || true
find "$work" -type f \( -name '*.png' -o -name '*.jpg' -o -name '*.jpeg' -o -name '*.webp' \) -exec cp {} "$output/images/" \; 2>/dev/null || true

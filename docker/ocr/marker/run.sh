#!/usr/bin/env bash
set -euo pipefail

input="${1:?input PDF required}"
output="${2:?output directory required}"
work="$output/artifacts/marker"

mkdir -p "$output/images" "$output/artifacts" "$output/logs" "$work"

marker_single "$input" \
  --output_dir "$work" \
  --output_format markdown \
  --disable_image_extraction \
  --disable_multiprocessing

md="$(find "$work" -type f -name '*.md' | head -n 1 || true)"
html="$(find "$work" -type f -name '*.html' | head -n 1 || true)"
json="$(find "$work" -type f -name '*.json' | head -n 1 || true)"

if [[ -n "$md" ]]; then cp "$md" "$output/output.md"; fi
if [[ -n "$html" ]]; then cp "$html" "$output/output.html"; fi
if [[ -n "$json" ]]; then cp "$json" "$output/output.json"; fi
cp "$output/output.md" "$output/text.txt" 2>/dev/null || true
find "$work" -type f \( -name '*.png' -o -name '*.jpg' -o -name '*.jpeg' -o -name '*.webp' \) -exec cp {} "$output/images/" \; 2>/dev/null || true

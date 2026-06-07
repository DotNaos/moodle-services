#!/usr/bin/env bash
set -euo pipefail

input="${1:?input PDF required}"
output="${2:?output directory required}"
workspace="$output/artifacts/olmocr-workspace"

mkdir -p "$output/images" "$output/artifacts" "$output/logs" "$workspace"

args=("$workspace" --markdown --pdfs "$input")
if [[ -n "${OLMOCR_SERVER:-}" ]]; then
  args+=("--server" "$OLMOCR_SERVER")
fi
if [[ -n "${OLMOCR_MODEL:-}" ]]; then
  args+=("--model" "$OLMOCR_MODEL")
fi
if [[ -n "${OLMOCR_API_KEY:-}" ]]; then
  args+=("--api_key" "$OLMOCR_API_KEY")
fi

olmocr "${args[@]}"

md="$(find "$workspace/markdown" "$workspace" -type f -name '*.md' 2>/dev/null | head -n 1 || true)"
if [[ -n "$md" ]]; then cp "$md" "$output/output.md"; fi
cp "$output/output.md" "$output/text.txt" 2>/dev/null || true

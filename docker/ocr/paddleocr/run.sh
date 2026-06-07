#!/usr/bin/env bash
set -euo pipefail

input="${1:?input PDF required}"
output="${2:?output directory required}"
work="$output/artifacts/paddleocr"

mkdir -p "$output/images" "$output/artifacts" "$output/logs" "$work"

formula_flag=True
if [[ "${OCR_FORMULA:-0}" != "1" ]]; then formula_flag=False; fi

python - "$input" "$work" "$formula_flag" <<'PY'
import json
import sys
from pathlib import Path

from paddleocr import PPStructureV3

input_pdf = sys.argv[1]
work = Path(sys.argv[2])
formula = sys.argv[3].lower() == "true"

pipeline = PPStructureV3(use_formula_recognition=formula)
results = pipeline.predict(input_pdf)

markdown_pages = []
json_pages = []
for index, result in enumerate(results, start=1):
    page_dir = work / f"page-{index:04d}"
    page_dir.mkdir(parents=True, exist_ok=True)
    if hasattr(result, "save_to_markdown"):
        result.save_to_markdown(str(page_dir))
    if hasattr(result, "save_to_json"):
        result.save_to_json(str(page_dir))
    if hasattr(result, "save_to_img"):
        result.save_to_img(str(page_dir))
    if isinstance(result, dict):
        json_pages.append(result)
    elif hasattr(result, "json"):
        json_pages.append(result.json)
    else:
        json_pages.append(str(result))

for path in sorted(work.rglob("*.md")):
    markdown_pages.append(path.read_text(encoding="utf-8", errors="replace"))

(work / "output.md").write_text("\n\n".join(markdown_pages), encoding="utf-8")
(work / "output.json").write_text(json.dumps(json_pages, ensure_ascii=False, indent=2, default=str), encoding="utf-8")
PY

md="$(find "$work" -type f -name '*.md' | head -n 1 || true)"
json="$(find "$work" -type f -name '*.json' | head -n 1 || true)"
html="$(find "$work" -type f -name '*.html' | head -n 1 || true)"

if [[ -n "$md" ]]; then cp "$md" "$output/output.md"; fi
if [[ -n "$json" ]]; then cp "$json" "$output/output.json"; fi
if [[ -n "$html" ]]; then cp "$html" "$output/output.html"; fi
cp "$output/output.md" "$output/text.txt" 2>/dev/null || true
find "$work" -type f \( -name '*.png' -o -name '*.jpg' -o -name '*.jpeg' -o -name '*.webp' \) -exec cp {} "$output/images/" \; 2>/dev/null || true

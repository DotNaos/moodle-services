# Study Pipeline Trace Implementation Plan

This is the implementation checklist for the traceable study pipeline.

The goal is to turn Moodle course material into clean script and task views
without losing traceability. Every generated result must be explainable from
the original Moodle resource down to page, block, pipeline run, and Codex
change.

## Working Rules

- Work in small vertical slices.
- Keep one active implementation goal per slice.
- Deploy to the VPS after each useful backend or frontend slice.
- Store shared course artifacts once, keyed by Moodle resource and file hash.
- Keep user-specific Codex work separate from shared course artifacts.
- Do not rely on repository-local `study-bundles` for production data.
- Do not hard-code MinIO. Use a storage abstraction with local filesystem first.

## Target Architecture

```text
Shared Course Store
├─ Moodle inventory
├─ raw files
├─ file hashes
├─ rendered pages
├─ extraction runs
├─ document blocks
├─ trace events
├─ task/solution pairing
└─ published course views

User Workspace Store
├─ Codex sessions
├─ prompts
├─ chat history
├─ user drafts
├─ personal task progress
├─ accepted/rejected suggestions
└─ per-user generated outputs
```

Codex containers read shared artifacts as input, but write to user workspace
runs first.

```text
Shared extracted blocks
        │ read-only
        ▼
Per-user Codex container
        │ writes
        ▼
User draft / Codex run
        │ optional publish
        ▼
Shared published course view
```

## Pipeline States

```text
State 1                 State 2                         State 3
Raw Course Input        Extracted Renderable Structure   Curated Final View
────────────────        ──────────────────────────────   ──────────────────
Moodle resources  ───►  pages + document blocks    ───►  script/tasks view
course inventory        headings, paragraphs, images     Codex-cleaned output
task groups             formulas, tables, lists          source-traced changes
```

## PDF Element Accountability

Every meaningful element detected in a PDF must receive an explicit final
processing decision. This applies to text, images, figures, tables, formulas,
charts, diagrams, captions, headers, footers, and any other detected page
content.

Allowed final outcomes are:

- `used_in_output`: the element is represented in the final website output.
- `ignored`: the element was intentionally discarded, such as a decorative
  template logo.
- `unsupported`: the element was detected but the current renderer cannot
  faithfully represent it yet.
- `failed`: processing was attempted and failed.
- `needs_review`: the element has not yet received a final decision, so the
  curated run must not be considered complete.

The curated/Codex step must answer these questions for every detected element:

- What kind of element is it?
- Where did it come from in the PDF?
- Was it represented in the final output?
- If yes, where?
- If no, why was it intentionally ignored?
- If processing failed, what failed?

Extraction alone is not enough. Page images, extracted blocks, embedded images,
the generated task/script output, and the rendered preview must be tied together
by a curation checklist and an element-accountability manifest.

A curated run may only be considered successful when:

- rendered page evidence is available,
- extracted elements were reviewed,
- every detected element has one explicit final outcome,
- the layout was reconstructed or intentionally simplified,
- a rendered website preview exists, and
- source mapping from PDF element to output is complete.

If any element remains `needs_review` or `failed`, the run is recorded as not
publishable and the frontend must show the concrete unresolved elements instead
of a generic missing-output warning.

## Server Storage Model

Use Postgres for metadata and status. Use a blob/artifact store for large files.
Start with local filesystem storage under a configured VPS path.

```text
/srv/moodle-study/
├─ objects/
│  └─ sha256/<prefix>/<file-hash>/<artifact>
├─ runs/
│  └─ <run-id>/
├─ users/
│  └─ <user-id>/
└─ published/
   └─ <course-id>/
```

The filesystem layout is an implementation detail behind an `ArtifactStore`
interface. Later storage backends must not change the pipeline data model.

## Scheduling Model

Each pipeline step is a run. Runs are append-only. Re-running OCR or block
detection creates a new run instead of overwriting the previous result.

```text
Resource / file hash
├─ run_001 fetch_file
├─ run_010 render_pages
├─ run_020 extract_text / pdftotext
├─ run_021 extract_text / docling
├─ run_022 extract_text / other_ocr
├─ run_030 extract_images
├─ run_040 detect_blocks
└─ run_050 codex_curate
```

Downstream runs record which upstream run they used.

```text
run_040 detect_blocks
├─ input_page_render_run: run_010
├─ input_text_run: run_021
├─ input_image_run: run_030
└─ status: done
```

If a user switches the active OCR result, downstream results become stale but
are not deleted.

```text
User selects docling run_021
├─ block detection: stale
├─ Codex curation: stale
└─ published view: stale
```

## Phase 1: Backend Inventory Slice

Goal: store and expose a real course inventory with task/solution pairing.

- [x] Define `CourseInventory` response shape.
- [x] Define `ResourceNode` metadata.
- [x] Define `TaskGroup` with `sheet`, `solution`, `pairing_status`, and `pairing_reason`.
- [x] Classify Moodle resources into buckets:
  - [x] lecture material
  - [x] task groups
  - [x] references
  - [x] interactions
  - [x] unknown
- [x] Pair task sheets and solutions by normalized title and sheet number.
- [x] Preserve unknown or ambiguous resources instead of dropping them.
- [x] Store inventory state server-side.
- [x] Add API endpoint for inventory inspection.
- [x] Add tests with a real-shaped course fixture.
- [x] Verify with a real Moodle course such as HPC `22584`.
- [ ] Deploy `moodle-services` to the VPS.
- [ ] Verify the live API from the VPS.

Local verification result for HPC `22584`:

- total resources: `29`
- lecture material: `6`
- task groups: `12`
- paired task groups: `11`
- missing solution groups: `1` (`Aufgabenblatt 09`)
- ambiguous task groups: `0`

Expected first API shape:

```text
GET /courses/{courseID}/study-pipeline/inventory

CourseInventory
├─ lecture_material
├─ task_groups
│  ├─ sheet
│  ├─ solution
│  ├─ pairing_status
│  └─ pairing_reason
├─ references
├─ interactions
└─ unknown
```

## Phase 2: Backend Extracted Structure Slice

Goal: create renderable PDF document structure before Codex runs.

- [x] Define `PDFDocument`.
- [x] Define `PDFPage`.
- [x] Define `DocumentBlock`.
- [x] Define block `type` values:
  - [x] heading
  - [x] paragraph
  - [x] list
  - [x] table
  - [x] image
  - [x] formula
  - [x] code
  - [x] page_header
  - [x] page_footer
  - [x] caption
  - [x] unknown
- [x] Define block `label` values for semantic meaning.
- [x] Render page previews.
- [x] Extract text with first baseline engine.
- [x] Extract images/assets.
- [x] Build blocks from page outputs.
- [x] Store extraction runs as append-only artifacts.
- [x] Expose extracted document structure via API.
- [x] Add diagnostics:
  - [x] pages missing text
  - [x] visual-only pages
  - [x] extracted images
  - [x] unused images
  - [x] unknown blocks
- [x] Verify one task sheet and one solution PDF end to end.
- [ ] Deploy `moodle-services` to the VPS.

Local verification result for HPC `22584`:

- run id: `baseline-20260612T114007Z`
- total documents: `29`
- total pages: `417`
- total blocks: `1830`
- page preview assets: `417`
- embedded image assets: `641`
- task sheet checked: `Aufgabenblatt 01`
- solution checked: `Aufgabenblatt 01 -- Lösung`

## Phase 3: Frontend Inventory Inspector

Goal: show the first mapping from Moodle resources into course buckets and task
groups.

- [x] Add course inventory screen in `moodle-clients`.
- [x] Show pipeline status per course.
- [x] Show lecture material bucket.
- [x] Show task groups with sheet and solution attached.
- [x] Show missing solution states.
- [x] Show references, interactions, and unknown resources.
- [x] Show pairing reason and confidence.
- [x] Add action to trigger inventory refresh.
- [x] Test locally against the local API/mock flow.
- [ ] Test locally against the VPS API.
- [ ] Deploy frontend.

Local verification result:

- `bun test apps/web/components/task-study-panel.test.ts`: passed
- `bun run web:build`: passed
- visual fallback check: opened `/dev/mock`, selected a course without a prepared task view, and verified the `Kurs-Mapping` panel renders without overlap

## Phase 4: Frontend PDF / Block Inspector

Goal: let the user inspect what was recognized inside a PDF.

- [x] Add PDF inspector view.
- [x] Show page preview on the left.
- [x] Show recognized document structure on the right.
- [x] Render block types website-like.
- [x] Show extraction engine and run metadata.
- [x] Show page/block diagnostics.
- [x] Highlight weak, missing, unknown, and unused blocks.
- [ ] Add OCR run comparison UI when multiple runs exist.
- [ ] Allow selecting an active run for downstream steps.
- [ ] Mark downstream steps stale after selection changes.

Local verification result:

- `bun test apps/web/components/task-study-panel.test.ts`: passed
- `bun run web:build`: passed
- visual fallback check: opened `/dev/mock`, loaded a mocked extracted-document response, verified run metadata, page tabs, PDF canvas rendering, block rendering, and missing/unknown/unused diagnostics
- layout note: the inspector uses the real container width; it stacks in the current narrow center column and switches to PDF-left / structure-right when the inspector has enough horizontal space

## Phase 5: Backend Codex Trace Slice

Goal: make Codex curation traceable and user-specific.

- [ ] Ensure Codex containers are isolated per user.
- [ ] Mount only the user's workspace for writes.
- [ ] Provide shared extracted artifacts read-only.
- [ ] Store Codex curation as user draft first.
- [ ] Define `TraceEvent`.
- [ ] Track block actions:
  - [ ] kept
  - [ ] rewritten
  - [ ] split
  - [ ] merged
  - [ ] moved
  - [ ] dropped
  - [ ] unused_needs_review
- [ ] Require dropped content to include a reason.
- [ ] Require generated content to include source block links or mark it as user/generated material.
- [ ] Add stale detection when upstream extraction changes.
- [ ] Add tests for deleted image/footer/logo behavior.
- [ ] Deploy `moodle-services` to the VPS.

## Phase 6: Frontend Before / After Trace View

Goal: show what Codex changed.

- [ ] Show extracted document structure before Codex.
- [ ] Show curated final view after Codex.
- [ ] Show block-level mappings between before and after.
- [ ] Show dropped blocks with reasons.
- [ ] Show rewritten or merged blocks.
- [ ] Warn if a block is not mapped and not explicitly dropped.
- [ ] Warn if Codex introduced content without source links.
- [ ] Allow user to mark a dropped block as allowed.
- [ ] Allow user to send affected content back through a rerun.
- [ ] Publish accepted final view.

## Phase 7: Deployment And Operational Checks

- [ ] Configure VPS storage root.
- [ ] Configure backup path for artifact store.
- [ ] Add artifact cleanup policy for failed temporary runs.
- [ ] Add retention policy for user drafts.
- [ ] Add metrics/logs for long-running extraction and Codex runs.
- [ ] Verify multiple users can share extracted artifacts.
- [ ] Verify Codex usage stays isolated per user.
- [ ] Verify rerunning OCR does not duplicate raw PDFs.
- [ ] Verify published view survives app redeploys.

## Completion Definition

The plan is complete when:

- A course can be fetched and organized into visible buckets.
- Task sheets and solutions are paired with explicit reasons.
- PDFs become renderable extracted document structures.
- OCR/extraction runs can be rerun and compared.
- Codex curation is user-specific and traceable.
- The frontend shows inventory, extracted blocks, before/after curation, and diagnostics.
- Shared artifacts are stored once and reused across users.
- Published output is stored server-side and survives deployment.

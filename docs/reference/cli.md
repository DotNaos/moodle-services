# CLI commands

Use this page when you want the exact command for a common task.

## Core commands

- `moodle` opens the interactive view.
- `moodle login` creates or refreshes the saved session.
- `moodle mobile qr login` creates and saves a reusable Moodle mobile token.
- `moodle bootstrap server --copy` creates a one-command Docker server install command and copies it to the clipboard.
- `moodle config migrate-home` copies old `~/.moodle-cli` data into `~/.moodle` without deleting the old folder.
- `moodle serve --addr :8080` starts the local JSON API and serves the built-in Scalar reference at `/docs`.
- `moodle update` installs the latest stable release.
- `moodle skill` prints the bundled agent skill (use `--install` to install it to codex/opencode/claude-code/gemini-cli).
- `moodle logs` tails the CLI logs (`--error` for error log, `--lines` to change the tail size, `--follow=false` to exit after printing).

## List data

```sh
moodle --json list courses
moodle --yaml list files <course-id|name|current|0>
```

Global machine-readable output is available on all non-interactive commands:

```sh
moodle --json <command>
moodle --yaml <command>
moodle --yml <command>
```

## Open in your browser

```sh
moodle open course <course-id|name|current|0>
moodle open current current
moodle open resource <course-id|name|current|0> <resource-id|name|current|0>
```

## Print course content

```sh
moodle print <course-id|name|index|0>
moodle print course-page <course-id|name|current|0>
moodle print course <course-id|name|current|0> <resource-id|name|current|0> --pdf-vision
moodle print <course-id|name|current|0> <resource-id|name|current|0> --engine pdftotext --out ./out/pdftotext
moodle print <course-id|name|current|0> <resource-id|name|current|0> --engine docling --out ./out/docling
moodle print <course-id|name|current|0> <resource-id|name|current|0> --engine all --out ./out/ocr-comparison --keep-artifacts
```

`--pdf-vision` uses the local Codex runtime through `codex app-server` for PDF
pages. It does not call the OpenAI API directly. Use
`--pdf-vision-model <model>` to compare models, `--pdf-vision-max-pages <n>` to
limit a test run to the first pages, and `--pdf-vision-codex-command <command>`
or `MOODLE_CODEX_APP_SERVER_COMMAND` to override the app-server command.

PDF text/OCR engines are `pdftotext`, `docling`, `marker`, `paddleocr`, `mineru`, `olmocr`, and `all`. `pdftotext` runs locally and is useful for PDFs that already contain selectable text. The other engines are Docker-backed document parsers/OCR engines. Without `--engine`, `moodle print` uses the existing text extraction path.

## Download files

```sh
moodle download file <course-id|name|current|0> <resource-id|name|current|0> --output-dir <path>
moodle export course <course-id|name|current|0> --output-dir <path>
```

## Export FHGR Moodle

```sh
moodle export fhgr --workspace /Users/oli/school --upload
moodle export fhgr --workspace /Users/oli/school --semester FS26 --upload
moodle export fhgr --workspace /Users/oli/school --semester FS26 --archive-output /tmp/fhgr-moodle-archive
moodle export fhgr --workspace /Users/oli/school --semester FS26 --archive-output /tmp/fhgr-goodnotes --archive-profile goodnotes
```

The command reads `school.yaml`, always processes `current_term`, and only backfills older semesters when `export.index.yaml` does not already show a completed export. Use `--archive-output` to write a local ZIP archive with sanitized offline file paths. Use `--archive-profile goodnotes` for a PDF-only archive grouped as `<semester>/<course>/<section>/<Moodle activity name>.pdf`; general course information sections are omitted.

## Shell completion

```sh
autoload -Uz compinit && compinit
source <(moodle completion zsh)
```

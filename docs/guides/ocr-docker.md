# Docker OCR

Use this when you want selectable PDF text extraction or Docker-backed PDF parsing for Moodle PDFs.

The lightweight `pdftotext` engine runs on the host and needs Poppler's `pdftotext` binary. The heavier document/OCR engines do not install Python, OCR, or ML packages on the host; they call Docker only when those engines are requested.

## Build provider images

From the repository root:

```sh
docker build -t moodle-ocr-docling:local docker/ocr/docling
docker build -t moodle-ocr-marker:local docker/ocr/marker
docker build -t moodle-ocr-paddleocr:local docker/ocr/paddleocr
docker build -t moodle-ocr-mineru:local docker/ocr/mineru
docker build -t moodle-ocr-olmocr:local docker/ocr/olmocr
```

On Apple Silicon, use this if a Python image does not work natively:

```sh
export OCR_DOCKER_PLATFORM=linux/amd64
```

## Run one engine

```sh
moodle print <course> <resource> --engine pdftotext --out ./out/pdftotext
moodle print <course> <resource> --engine docling --out ./out/docling
moodle print <course> <resource> --engine marker --out ./out/marker
moodle print <course> <resource> --engine paddleocr --out ./out/paddleocr
moodle print <course> <resource> --engine mineru --out ./out/mineru
```

The normal print command is unchanged when `--engine` is not set.

`pdftotext` is not OCR. It is a fast baseline for PDFs that already contain text and it is included in `--engine all`.

## Compare engines

```sh
moodle print <course> <resource> --engine all --out ./out/ocr-comparison --keep-artifacts
```

This writes one folder per enabled provider and a `comparison.md` index with status, duration, warnings, output files, Markdown character count, and image count.

`olmocr` is not included in `--engine all` by default. Include it with GPU mode:

```sh
moodle print <course> <resource> --engine all --gpu --out ./out/ocr-comparison
```

Or run it explicitly:

```sh
moodle print <course> <resource> --engine olmocr --gpu --out ./out/olmocr
```

## API

Start the API with a long enough request timeout:

```sh
moodle serve --addr :8080 --request-timeout 30m
```

Call OCR through the course/resource endpoint:

```sh
curl "http://127.0.0.1:8080/api/courses/<course>/resources/<resource>/ocr?engine=docling&timeout=900"
curl "http://127.0.0.1:8080/api/courses/<course>/resources/<resource>/ocr?engine=all&timeout=1800&keepArtifacts=true"
```

## VPS notes

If the Moodle API itself runs in Docker and should launch OCR provider containers, mount the host Docker socket into the Moodle API container:

```sh
-v /var/run/docker.sock:/var/run/docker.sock
```

The OCR provider images must also exist on that Docker host. Build them on the VPS or push/pull tagged images from a registry.

Use a persistent output directory for API runs when you want to inspect artifacts later:

```sh
curl "http://127.0.0.1:8080/api/courses/<course>/resources/<resource>/ocr?engine=all&out=/data/ocr&keepArtifacts=true"
```

Provider containers also receive a shared cache at `/cache` when `MOODLE_HOME` or `MOODLE_OCR_CACHE_DIR` is set. This helps avoid repeated model downloads for engines that respect Hugging Face, XDG, or ModelScope cache settings.

## olmOCR and GPU

The `olmocr` provider is included as an optional engine. It can run on a CUDA laptop or GPU server with Docker GPU support:

```sh
moodle print <course> <resource> --engine olmocr --gpu --out ./out/olmocr
```

For remote inference, pass the server settings into the Moodle process. The Docker runtime forwards them into the provider container:

```sh
export OLMOCR_SERVER=http://gpu-server:8000/v1
export OLMOCR_MODEL=allenai/olmOCR-2-7B-1025-FP8
moodle print <course> <resource> --engine olmocr --out ./out/olmocr
```

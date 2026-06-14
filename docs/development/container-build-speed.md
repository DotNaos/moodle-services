# Container Build Speed

The service image is intentionally split into two layers:

1. `ghcr.io/dotnaos/moodle-services-runtime-base`
   - Debian runtime packages
   - Playwright browser cache
   - Poppler and Tesseract
   - Docker CLI for pipeline helper containers

2. `ghcr.io/dotnaos/moodle-services`
   - The compiled `moodle` Go binary

The runtime base image is content-tagged from:

- `docker/runtime-base/Dockerfile`
- `go.mod`
- `go.sum`

Regular backend changes should only rebuild the service image. The heavy runtime
base image is rebuilt only when the dependency inputs above change or when the
base image tag is missing in GHCR.

The release workflow keeps the VPS deploy path fast by publishing the service
image for `linux/amd64` first:

- `ghcr.io/dotnaos/moodle-services:<tag>-amd64`
- `ghcr.io/dotnaos/moodle-services:latest-amd64`

The VPS deploy workflow uses `latest-amd64` by default. Multi-arch images are
kept outside the critical deploy path and can be published manually with the
`Container Image Multi-Arch` workflow when an ARM image is needed.

For local verification:

```bash
docker buildx build --load \
  --file docker/runtime-base/Dockerfile \
  --tag moodle-services-runtime-base:test .

docker buildx build --load \
  --build-arg RUNTIME_BASE_IMAGE=moodle-services-runtime-base:test \
  --tag moodle-services:test .
```

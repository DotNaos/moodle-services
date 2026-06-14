FROM golang:1.24-bookworm AS playwright

WORKDIR /workspace
COPY go.mod go.sum ./
RUN go mod download
RUN go run github.com/playwright-community/playwright-go/cmd/playwright install chromium

FROM debian:bookworm-slim
ARG DOCKER_CLI_VERSION=28.0.4

RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl \
    poppler-utils \
    tesseract-ocr \
    tesseract-ocr-deu \
    tesseract-ocr-eng \
    libnss3 \
    libnspr4 \
    libatk1.0-0 \
    libatk-bridge2.0-0 \
    libcups2 \
    libdrm2 \
    libdbus-1-3 \
    libxcb1 \
    libxkbcommon0 \
    libx11-6 \
    libxcomposite1 \
    libxdamage1 \
    libxext6 \
    libxfixes3 \
    libxrandr2 \
    libgbm1 \
    libasound2 \
    libpango-1.0-0 \
    libcairo2 \
    && rm -rf /var/lib/apt/lists/* \
    && ARCH="$(dpkg --print-architecture)" \
    && case "$ARCH" in \
         amd64) DOCKER_ARCH=x86_64 ;; \
         arm64) DOCKER_ARCH=aarch64 ;; \
         *) echo "unsupported Docker CLI arch: $ARCH" >&2; exit 1 ;; \
       esac \
    && curl -fsSL "https://download.docker.com/linux/static/stable/${DOCKER_ARCH}/docker-${DOCKER_CLI_VERSION}.tgz" \
    | tar -xzf - -C /usr/local/bin --strip-components=1 docker/docker \
    && docker --version

COPY --from=playwright /root/.cache/ms-playwright-go /root/.cache/ms-playwright-go
COPY --from=playwright /root/.cache/ms-playwright /root/.cache/ms-playwright

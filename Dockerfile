FROM golang:1.24-bookworm AS builder

WORKDIR /workspace
COPY go.mod go.sum ./
RUN go mod download

COPY . .
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} go build -o /out/moodle ./cmd/moodle

# Download and install playwright driver and chromium (no large OS deps here, those are installed in the final image)
RUN go run github.com/playwright-community/playwright-go/cmd/playwright install chromium

FROM debian:bookworm-slim
ARG DOCKER_CLI_VERSION=28.0.4
# Install minimum system dependencies required by playwright/chromium and a current Docker CLI for Codex/OCR runners.
RUN apt-get update && apt-get install -y ca-certificates curl \
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

# Copy playwright driver and browser binaries
COPY --from=builder /root/.cache/ms-playwright-go /root/.cache/ms-playwright-go
COPY --from=builder /root/.cache/ms-playwright /root/.cache/ms-playwright

WORKDIR /app
ENV MOODLE_HOME=/data
VOLUME ["/data"]
COPY --from=builder /out/moodle /usr/local/bin/moodle
EXPOSE 8080
ENTRYPOINT ["moodle"]
CMD ["--help"]

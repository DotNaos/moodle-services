ARG RUNTIME_BASE_IMAGE=ghcr.io/dotnaos/moodle-services-runtime-base:bookworm

FROM golang:1.24-bookworm AS builder

WORKDIR /workspace
COPY go.mod go.sum ./
RUN go mod download

COPY . .
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} go build -o /out/moodle ./cmd/moodle

FROM ${RUNTIME_BASE_IMAGE} AS runtime

WORKDIR /app
ENV MOODLE_HOME=/data
VOLUME ["/data"]
COPY --from=builder /out/moodle /usr/local/bin/moodle
EXPOSE 8080
ENTRYPOINT ["moodle"]
CMD ["--help"]

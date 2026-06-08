FROM node:24-bookworm-slim

ARG CODEX_VERSION=0.130.0

RUN apt-get update \
  && apt-get install -y --no-install-recommends ca-certificates \
  && rm -rf /var/lib/apt/lists/* \
  && npm install -g @openai/codex@${CODEX_VERSION} \
  && useradd -m -u 10001 codex \
  && mkdir -p /home/codex/.codex \
  && chown -R codex:codex /home/codex/.codex

USER codex
WORKDIR /work
ENV HOME=/home/codex
ENV CODEX_HOME=/home/codex/.codex

CMD ["sh", "-lc", "codex --version"]

FROM python:3.11-slim

RUN apt-get update \
  && apt-get install -y --no-install-recommends libgl1 libglib2.0-0 poppler-utils \
  && rm -rf /var/lib/apt/lists/* \
  && pip install --no-cache-dir docling

COPY run.sh /usr/local/bin/moodle-ocr-docling
RUN chmod +x /usr/local/bin/moodle-ocr-docling

ENTRYPOINT ["moodle-ocr-docling"]

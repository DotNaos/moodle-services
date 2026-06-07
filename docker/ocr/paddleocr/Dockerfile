FROM python:3.11-slim

RUN apt-get update \
  && apt-get install -y --no-install-recommends libgl1 libglib2.0-0 poppler-utils \
  && rm -rf /var/lib/apt/lists/* \
  && pip install --no-cache-dir paddlepaddle paddleocr 'paddlex[ocr]'

COPY run.sh /usr/local/bin/moodle-ocr-paddleocr
RUN chmod +x /usr/local/bin/moodle-ocr-paddleocr

ENTRYPOINT ["moodle-ocr-paddleocr"]

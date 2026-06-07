FROM python:3.11-slim

RUN apt-get update \
  && apt-get install -y --no-install-recommends libgl1 libglib2.0-0 poppler-utils \
  && rm -rf /var/lib/apt/lists/* \
  && pip install --no-cache-dir --index-url https://download.pytorch.org/whl/cpu torch torchvision \
  && pip install --no-cache-dir mineru 'transformers<5' shapely six pyclipper

COPY run.sh /usr/local/bin/moodle-ocr-mineru
RUN chmod +x /usr/local/bin/moodle-ocr-mineru

ENTRYPOINT ["moodle-ocr-mineru"]

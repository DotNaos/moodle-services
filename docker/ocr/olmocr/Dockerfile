FROM alleninstituteforai/olmocr:latest

COPY run.sh /usr/local/bin/moodle-ocr-olmocr
RUN chmod +x /usr/local/bin/moodle-ocr-olmocr

ENTRYPOINT ["moodle-ocr-olmocr"]

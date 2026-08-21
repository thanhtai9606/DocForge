# CPU OCR sidecar (Tesseract + RapidOCR + poppler). Build from apps/worker:
#   docker build -f apps/worker/Dockerfile -t docforge-ocr apps/worker
FROM python:3.12-slim-bookworm

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        tesseract-ocr \
        tesseract-ocr-eng \
        tesseract-ocr-vie \
        poppler-utils \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY requirements.txt /app/requirements.txt
RUN pip install --no-cache-dir -r /app/requirements.txt
COPY ocr_server.py /app/ocr_server.py

ENV OCR_ADDR=0.0.0.0:8090 \
    OCR_ENGINE=auto \
    OCR_DPI=150 \
    OCR_MAX_CONCURRENT=4 \
    PYTHONUNBUFFERED=1

EXPOSE 8090
USER nobody
CMD ["python3", "/app/ocr_server.py"]

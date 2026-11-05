FROM python:3.11-slim AS assets
COPY download.py /download.py
RUN python /download.py

FROM python:3.11-slim
WORKDIR /app
ENV OMP_NUM_THREADS=1 OPENBLAS_NUM_THREADS=1
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt && useradd --system --uid 10001 hush
COPY --from=assets /opt/hush /opt/hush
COPY processor.py service.py test_processor.py test_transport.py ./
RUN python -m unittest test_processor test_transport -v
USER hush
EXPOSE 8003
HEALTHCHECK --interval=10s --timeout=3s CMD python -c "import urllib.request; urllib.request.urlopen('http://127.0.0.1:8003/health', timeout=2)"
CMD ["uvicorn", "service:app", "--host", "0.0.0.0", "--port", "8003", "--ws-max-size", "32768"]

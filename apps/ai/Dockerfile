FROM python:3.12-slim
WORKDIR /app
COPY apps/ai/requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Embedding model for hybrid retrieval is baked into the image at build time (CPU, ONNX, ~0.5 GB),
# so the running service needs no network access to Hugging Face. Only retrieval.py is copied first
# to keep this layer cached across other code changes.
ARG RAG_EMBEDDING_MODEL=intfloat/multilingual-e5-small
ENV FASTEMBED_CACHE_PATH=/models RAG_EMBEDDING_MODEL=${RAG_EMBEDDING_MODEL}
COPY apps/ai/app/retrieval.py /tmp/prefetch/retrieval.py
RUN cd /tmp/prefetch && python -c "from retrieval import FastEmbedder; import os; FastEmbedder(os.environ['RAG_EMBEDDING_MODEL'])" \
    && rm -rf /tmp/prefetch && chown -R 10001 /models

COPY apps/ai/app ./app
COPY data /data
ENV DATA_DIR=/data PYTHONDONTWRITEBYTECODE=1 PYTHONUNBUFFERED=1 HF_HUB_OFFLINE=1
USER 10001
EXPOSE 8001
CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8001"]

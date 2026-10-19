FROM postgres:17.11-bookworm
RUN apt-get update && apt-get install -y --no-install-recommends restic python3 ca-certificates \
    && rm -rf /var/lib/apt/lists/* && mkdir -p /work /repository /restore \
    && chown -R 10001:10001 /work /repository /restore
COPY deploy/backup.py /opt/backup.py
ENV RESTIC_CACHE_DIR=/tmp/restic HOME=/tmp
USER 10001:10001
ENTRYPOINT ["python3", "/opt/backup.py"]

FROM debian:trixie-slim
RUN apt-get update \
 && apt-get install -y --no-install-recommends restic python3-pyftpdlib curl ca-certificates \
 && rm -rf /var/lib/apt/lists/*
COPY check-backup-* /usr/local/bin/
CMD ["sleep", "infinity"]

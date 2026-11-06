#!/bin/sh
set -eu
if [ -e .env ]; then
  echo ".env already exists, refusing to overwrite" >&2
  exit 1
fi
if ! command -v openssl >/dev/null 2>&1; then
  echo "openssl is required" >&2
  exit 1
fi
API_TOKEN=$(openssl rand -hex 32)
ENCRYPTION_KEY=$(openssl rand -base64 32 | tr -d '\n')
STREAM_PASSWORD=$(openssl rand -hex 24)
cp .env.example .env
sed -i "s|^API_TOKEN=.*|API_TOKEN=${API_TOKEN}|" .env
sed -i "s|^ENCRYPTION_KEY=.*|ENCRYPTION_KEY=${ENCRYPTION_KEY}|" .env
sed -i "s|^REX_SOURCE_PASSWORD=.*|REX_SOURCE_PASSWORD=${STREAM_PASSWORD}|" .env
chmod 600 .env
echo "wrote .env with generated secrets"

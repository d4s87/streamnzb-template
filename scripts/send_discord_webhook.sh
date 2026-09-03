#!/usr/bin/env bash

payload_file="${1:-}"

if [ -z "$payload_file" ]; then
  echo "::error::Discord payload file argument is required."
  exit 2
fi

if [ ! -f "$payload_file" ]; then
  echo "::error::Discord payload file not found: $payload_file"
  exit 2
fi

if [ ! -s "$payload_file" ]; then
  echo "::error::Discord payload file is empty: $payload_file"
  exit 2
fi

if [ -z "${DISCORD_WEBHOOK:-}" ]; then
  echo "::warning::DISCORD_VIDHIN_WEBHOOK is not configured."
  exit 0
fi

if [ -z "${DISCORD_THREAD_ID:-}" ]; then
  echo "::error::DISCORD_THREAD_ID is not configured."
  exit 2
fi

curl \
  --fail-with-body \
  --silent \
  --show-error \
  -H "Content-Type: application/json" \
  --data-binary "@${payload_file}" \
  "${DISCORD_WEBHOOK}?thread_id=${DISCORD_THREAD_ID}&wait=true"
curl_rc=$?

exit "$curl_rc"

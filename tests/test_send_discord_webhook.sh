#!/usr/bin/env bash

root="$(cd "$(dirname "$0")/.." && pwd)"
script="$root/scripts/send_discord_webhook.sh"

tmpdir="$(mktemp -d)"
tmp_rc=$?

if [ "$tmp_rc" -ne 0 ]; then
  echo "ERROR: failed to create temp directory"
  exit "$tmp_rc"
fi

fake_bin="$tmpdir/bin"
payload="$tmpdir/payload.json"
empty_payload="$tmpdir/empty.json"
curl_log="$tmpdir/curl.log"

mkdir -p "$fake_bin"
mkdir_rc=$?

if [ "$mkdir_rc" -ne 0 ]; then
  echo "ERROR: failed to create fake bin directory"
  exit "$mkdir_rc"
fi

cat >"$fake_bin/curl" <<'CURL'
#!/usr/bin/env bash
printf '%s\n' "$@" >"$CURL_LOG"
exit "${FAKE_CURL_RC:-0}"
CURL
fake_rc=$?

if [ "$fake_rc" -ne 0 ]; then
  echo "ERROR: failed to create fake curl"
  exit "$fake_rc"
fi

chmod 755 "$fake_bin/curl"
chmod_rc=$?

if [ "$chmod_rc" -ne 0 ]; then
  echo "ERROR: failed to make fake curl executable"
  exit "$chmod_rc"
fi

printf '{"content":"test"}\n' >"$payload"
payload_rc=$?

if [ "$payload_rc" -ne 0 ]; then
  echo "ERROR: failed to create test payload"
  exit "$payload_rc"
fi

: >"$empty_payload"

PATH="$fake_bin:$PATH" \
CURL_LOG="$curl_log" \
"$script" >/dev/null 2>&1
missing_arg_rc=$?

if [ "$missing_arg_rc" -ne 2 ]; then
  echo "ERROR: missing payload argument returned $missing_arg_rc, expected 2"
  exit 1
fi

PATH="$fake_bin:$PATH" \
CURL_LOG="$curl_log" \
"$script" "$tmpdir/missing.json" >/dev/null 2>&1
missing_file_rc=$?

if [ "$missing_file_rc" -ne 2 ]; then
  echo "ERROR: missing payload file returned $missing_file_rc, expected 2"
  exit 1
fi

PATH="$fake_bin:$PATH" \
CURL_LOG="$curl_log" \
"$script" "$empty_payload" >/dev/null 2>&1
empty_file_rc=$?

if [ "$empty_file_rc" -ne 2 ]; then
  echo "ERROR: empty payload file returned $empty_file_rc, expected 2"
  exit 1
fi

PATH="$fake_bin:$PATH" \
CURL_LOG="$curl_log" \
DISCORD_THREAD_ID="1542856068135125002" \
"$script" "$payload" >/dev/null 2>&1
missing_webhook_rc=$?

if [ "$missing_webhook_rc" -ne 0 ]; then
  echo "ERROR: missing webhook returned $missing_webhook_rc, expected 0"
  exit 1
fi

PATH="$fake_bin:$PATH" \
CURL_LOG="$curl_log" \
DISCORD_WEBHOOK="https://discord.example/webhook" \
"$script" "$payload" >/dev/null 2>&1
missing_thread_rc=$?

if [ "$missing_thread_rc" -ne 2 ]; then
  echo "ERROR: missing thread returned $missing_thread_rc, expected 2"
  exit 1
fi

: >"$curl_log"

PATH="$fake_bin:$PATH" \
CURL_LOG="$curl_log" \
DISCORD_WEBHOOK="https://discord.example/webhook" \
DISCORD_THREAD_ID="1542856068135125002" \
FAKE_CURL_RC=0 \
"$script" "$payload" >/dev/null 2>&1
success_rc=$?

if [ "$success_rc" -ne 0 ]; then
  echo "ERROR: successful fake curl returned $success_rc"
  exit 1
fi

expected_url='https://discord.example/webhook?thread_id=1542856068135125002&wait=true'

for expected in \
  '--fail-with-body' \
  '--silent' \
  '--show-error' \
  'Content-Type: application/json' \
  '--data-binary' \
  "@$payload" \
  "$expected_url"
do
  grep -Fx -- "$expected" "$curl_log" >/dev/null
  grep_rc=$?

  if [ "$grep_rc" -ne 0 ]; then
    echo "ERROR: fake curl log missing expected argument: $expected"
    exit 1
  fi
done

PATH="$fake_bin:$PATH" \
CURL_LOG="$curl_log" \
DISCORD_WEBHOOK="https://discord.example/webhook" \
DISCORD_THREAD_ID="1542856068135125002" \
FAKE_CURL_RC=22 \
"$script" "$payload" >/dev/null 2>&1
curl_failure_rc=$?

if [ "$curl_failure_rc" -ne 22 ]; then
  echo "ERROR: curl failure returned $curl_failure_rc, expected 22"
  exit 1
fi

printf '%s\n' "PASS: shared Discord webhook sender tests"
exit 0

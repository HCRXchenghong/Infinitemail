#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
server_pid=""
tmp_dir=""

cleanup() {
  if [[ -n "$server_pid" ]] && kill -0 "$server_pid" 2>/dev/null; then
    kill "$server_pid" 2>/dev/null || true
    wait "$server_pid" 2>/dev/null || true
  fi
  if [[ -n "$tmp_dir" ]]; then
    rm -rf "$tmp_dir"
  fi
}
trap cleanup EXIT

section() {
  printf '\n== %s ==\n' "$1"
}

pick_port() {
  python3 - <<'PY'
import socket
s = socket.socket()
s.bind(("127.0.0.1", 0))
print(s.getsockname()[1])
s.close()
PY
}

wait_health() {
  local base="$1"
  for _ in $(seq 1 80); do
    if curl -fsS "$base/healthz" >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.25
  done
  echo "BFF did not become healthy: $base" >&2
  return 1
}

section "BFF tests"
(cd "$ROOT_DIR/bff" && go test ./...)

section "contract tests"
find "$ROOT_DIR/packages/contracts/src" -maxdepth 1 -name '*.test.mjs' -print | sort | while read -r test_file; do
  node "$test_file"
done

section "frontend builds"
(cd "$ROOT_DIR" && npm run build:all)

section "strict-mode no-mock guard"
tmp_dir="$(mktemp -d -t infinitemail-commercial.XXXXXX)"
port="$(pick_port)"
(
  cd "$ROOT_DIR/bff"
  HTTP_ADDR="127.0.0.1:$port" \
  DATA_PATH="$tmp_dir/state.json" \
  ATTACHMENT_DIR="$tmp_dir/attachments" \
  ADMIN_PASSWORD="StrictCheckAdmin2026!" \
  INFINITEMAIL_PRODUCTION_STRICT=true \
  REQUIRE_POSTGRES=true \
  REQUIRE_MAIL_DATA_PLANE=true \
  REQUIRE_MAIL_WEBHOOK=true \
  REQUIRE_REAL_SMS=true \
  REQUIRE_REAL_OAUTH=true \
  go run ./cmd/bff
) &
server_pid="$!"
wait_health "http://127.0.0.1:$port"

ready_file="$tmp_dir/ready.json"
ready_status="$(curl -sS -o "$ready_file" -w '%{http_code}' "http://127.0.0.1:$port/readyz" || true)"
if [[ "$ready_status" != "503" ]]; then
  echo "expected strict /readyz to reject incomplete real dependencies with HTTP 503, got $ready_status" >&2
  cat "$ready_file" >&2 || true
  exit 1
fi

node - "$ready_file" <<'NODE'
const fs = require("fs");
const payload = JSON.parse(fs.readFileSync(process.argv[2], "utf8"));
const checks = payload?.deployment?.checks || [];
const blocking = new Set(checks.filter((item) => item.status === "blocking").map((item) => item.id));
for (const id of ["store", "mailbox_credential_key", "mail_control", "mail_data_plane", "sms", "oauth"]) {
  if (!blocking.has(id)) {
    console.error(`strict guard did not block missing ${id}`);
    process.exit(1);
  }
}
if (payload.ready !== false || !payload.deployment || payload.deployment.ready !== false) {
  console.error("strict guard did not return an explicit not-ready deployment payload");
  process.exit(1);
}
NODE

section "commercial acceptance complete"
echo "InfiniteMail commercial no-mock checks passed"

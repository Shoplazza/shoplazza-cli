#!/usr/bin/env bash
# Roll the metadata origin back to an earlier revision's content.
#
# Usage:   meta-rollback.sh --to <revision> [min_cli_version]
# Env:     META_OSS_BUCKET / OSSUTIL — same as meta-publish.sh
#
# Clients only adopt a revision newer than what they have, so rollback means
# re-publishing the old content under a fresh generated_at.
set -euo pipefail

[ "${1:-}" = "--to" ] || { echo "usage: meta-rollback.sh --to <revision> [min_cli_version]" >&2; exit 1; }
TARGET="${2:?usage: meta-rollback.sh --to <revision> [min_cli_version]}"
MIN_CLI_VERSION="${3:-}"
: "${META_OSS_BUCKET:?set META_OSS_BUCKET, e.g. oss://bucket/shoplazza-cli/meta}"
OSSUTIL="${OSSUTIL:-ossutil}"

TARGET_KEY=$(echo "$TARGET" | tr -d ':')
WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT

"$OSSUTIL" cp "$META_OSS_BUCKET/specs/$TARGET_KEY.json.gz" "$WORK/old.json.gz"
gunzip "$WORK/old.json.gz"

# Restamp with now-UTC. If the revision being replaced carries a FUTURE
# generated_at (skewed generator clock), a now() stamp would sort older and
# be ignored by every client — refuse so the operator picks a later stamp.
python3 - "$WORK/old.json" <<'PY'
import datetime, json, sys
spec = json.load(open(sys.argv[1]))
now = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
spec["generated_at"] = now
json.dump(spec, open(sys.argv[1], "w"), separators=(",", ":"))
PY

CURRENT=$("$OSSUTIL" cat "$META_OSS_BUCKET/manifest.json" 2>/dev/null | python3 -c "import json,sys; print(json.load(sys.stdin).get('revision',''))" || echo "")
NEW=$(python3 -c "import json,sys; print(json.load(open(sys.argv[1]))['generated_at'])" "$WORK/old.json")
if [ -n "$CURRENT" ] && ! [[ "$NEW" > "$CURRENT" ]]; then
  echo "restamped revision $NEW does not sort after current $CURRENT (future-dated publish?) — aborting" >&2
  exit 1
fi

"$(dirname "$0")/meta-publish.sh" "$WORK/old.json" "$MIN_CLI_VERSION"

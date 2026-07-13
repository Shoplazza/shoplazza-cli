#!/usr/bin/env bash
# Roll the metadata origin back to an earlier revision's content.
#
# Usage:   meta-rollback.sh --to <revision> [min_cli_version]
# Env:     META_OSS_BUCKET / OSSUTIL — same as meta-publish.sh
#
# Clients only adopt a revision newer than what they have, so rollback means
# re-publishing the old content under a fresh generated_at (see the design's
# rollback section). This fetches the old spec, restamps it, and delegates to
# meta-publish.sh.
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

python3 - "$WORK/old.json" <<'PY'
import datetime, json, sys
spec = json.load(open(sys.argv[1]))
spec["generated_at"] = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
json.dump(spec, open(sys.argv[1], "w"), separators=(",", ":"))
PY

exec "$(dirname "$0")/meta-publish.sh" "$WORK/old.json" "$MIN_CLI_VERSION"

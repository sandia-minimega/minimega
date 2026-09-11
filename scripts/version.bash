#!/bin/bash

set -e

ROOT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )/.." && pwd )"

mkdir -p "$ROOT_DIR/internal/version"

# shellcheck source=VERSION
source "$ROOT_DIR/VERSION"
if ! REVISION="$(git -C "$ROOT_DIR" rev-parse HEAD 2>/dev/null)"; then
	echo "warning: Git metadata unavailable; using unknown revision" >&2
	REVISION="unknown"
fi
DATE="$(date +%F)"

cat > "$ROOT_DIR/internal/version/version.go" <<EOF
package version

var (
	Version  = "$VERSION"
	Revision = "$REVISION"
	Date     = "$DATE"
)
EOF

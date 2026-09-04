#!/bin/bash

ROOT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )/.." && pwd )"
MODE="${1:-build}"

set -e

case "$MODE" in
    build|serve)
        ;;
    *)
        echo "usage: $0 [build|serve]" >&2
        exit 2
        ;;
esac

"$ROOT_DIR/scripts/version.bash"

GOBIN="$ROOT_DIR/bin" GOFLAGS="${GOFLAGS:--mod=vendor}" \
    go install ./cmd/apigen ./cmd/minimega ./cmd/minirouter ./cmd/pyapigen

"$ROOT_DIR/scripts/doc.bash"
python3 "$ROOT_DIR/scripts/check-docs.py" "$ROOT_DIR/doc/content"

if [[ "$MODE" == "build" ]]; then
    python3 "$ROOT_DIR/scripts/zensical_build.py" "$MODE" --strict
else
    python3 "$ROOT_DIR/scripts/zensical_build.py" "$MODE"
fi

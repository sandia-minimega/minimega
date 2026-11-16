#!/bin/bash

# Collect the licenses of the Go modules that the programs link into
# LICENSES/go, which the packages and the container image install. The build
# fails if a module has a license that is not in ALLOWED_LICENSES.

set -e

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
ROOT_DIR="$( cd "$SCRIPT_DIR/.." && pwd )"

. "$SCRIPT_DIR/env.bash"

# the programs import the generated version package
"$SCRIPT_DIR/version.bash"

GO_LICENSES_VERSION="v1.6.0"
ALLOWED_LICENSES="Apache-2.0,BSD-2-Clause,BSD-3-Clause,GPL-3.0,ISC,MIT"
# minimega's own LICENSE is installed separately
IGNORE="github.com/sandia-minimega/minimega/v2"
OUT="$ROOT_DIR/LICENSES/go"

# keep in sync with the programs that build.bash builds for each OS
LINUX_PACKAGES="./cmd/..."
WINDOWS_PACKAGES="./cmd/miniccc ./cmd/protonuke"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

# install outside bin/ so that the packages do not include it
GOBIN="$TMP" go install "github.com/google/go-licenses@$GO_LICENSES_VERSION"
GO_LICENSES="$TMP/go-licenses"

cd "$ROOT_DIR"

echo "CHECK LICENSES"
$GO_LICENSES check $LINUX_PACKAGES --ignore "$IGNORE" --allowed_licenses "$ALLOWED_LICENSES"
GOOS=windows $GO_LICENSES check $WINDOWS_PACKAGES --ignore "$IGNORE" --allowed_licenses "$ALLOWED_LICENSES"

echo "SAVE LICENSES"
rm -rf "$OUT"
$GO_LICENSES save $LINUX_PACKAGES --ignore "$IGNORE" --save_path "$OUT"
GOOS=windows $GO_LICENSES save $WINDOWS_PACKAGES --ignore "$IGNORE" --save_path "$TMP/windows"
cp -rn "$TMP/windows/." "$OUT/"
echo

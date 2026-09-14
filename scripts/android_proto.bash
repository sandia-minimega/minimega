#!/bin/bash
# Generate Go protobuf and gRPC stubs from the trimmed emulator controller proto.
#
# Upstream source:
#   https://android.googlesource.com/platform/external/qemu/+/refs/heads/emu-master-dev/android/android-grpc/
#
# Prerequisites:
#   go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.5
#   go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1
#
# The trimmed proto lives alongside the generated code in internal/android/emupb/.
# It contains only the subset of RPCs/messages needed for screenshot streaming
# and input injection (from the full upstream emulator_controller.proto).

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
PROTO_DIR="$REPO_ROOT/internal/android/emupb"

for tool in protoc protoc-gen-go protoc-gen-go-grpc; do
    if ! command -v "$tool" &>/dev/null; then
        echo "ERROR: $tool not found in PATH" >&2
        exit 1
    fi
done

echo "Generating Go stubs from $PROTO_DIR/emulator_controller.proto ..."
cd "$PROTO_DIR"
protoc \
    --go_out=. --go_opt=paths=source_relative \
    --go-grpc_out=. --go-grpc_opt=paths=source_relative \
    emulator_controller.proto

echo "Done. Generated files:"
ls -1 "$PROTO_DIR"/*.pb.go

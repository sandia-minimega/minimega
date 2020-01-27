#!/bin/bash
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
$SCRIPT_DIR/misc/daemon/minimega.init install # puts symlinks in /etc/minimega/ and /etc/init.d/

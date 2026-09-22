#!/bin/bash

# Spawned by miniweb for the web console (MINIWEB_CONSOLE). miniweb runs the
# console binary with only `-attach`, so this wrapper adds the base directory
# that start-miniweb.sh exports. Without it a non-default MINIWEB_BASE would
# leave the console looking for the command socket in the wrong place.

exec /opt/minimega/bin/minimega -base="${MINIWEB_BASE:-/tmp/minimega}" -attach

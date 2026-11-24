#!/bin/bash

# Spawned by miniweb for the web console (MINIWEB_CONSOLE). miniweb runs the
# console binary with only `-attach`, so this wrapper adds the base directory
# that start-miniweb.sh exports. Without it a non-default MINIWEB_BASE would
# leave the console looking for the command socket in the wrong place. The
# console also inherits MM_LOGFILE from a shared /etc/default/minimega, so an
# empty -logfile keeps the client from opening the daemon's log file.

# A detached container has no TERM, and liner then disables line editing.
if [[ -z "${TERM:-}" || "${TERM}" == "dumb" ]]; then
  export TERM=xterm
fi

exec /opt/minimega/bin/minimega -base="${MINIWEB_BASE:-/tmp/minimega}" -logfile= -attach

#!/bin/bash

/usr/share/openvswitch/scripts/ovs-ctl start

: "${MINIWEB_ROOT:=/opt/minimega/web}"
: "${MINIWEB_HOST:=0.0.0.0}"
: "${MINIWEB_PORT:=9001}"

: "${MM_BASE:=/tmp/minimega}"
: "${MM_FILEPATH:=/tmp/minimega/files}"
: "${MM_BROADCAST:=255.255.255.255}"
: "${MM_PORT:=9000}"
: "${MM_DEGREE:=2}"
: "${MM_CONTEXT:=minimega}"
: "${MM_LOGLEVEL:=info}"
: "${MM_LOGFILE:=/var/log/minimega.log}"
: "${MM_RECOVER:=true}"
: "${MM_RECOVER:=false}"

[[ -f "/etc/default/minimega" ]] && source "/etc/default/minimega"

/opt/minimega/bin/miniweb -root=${MINIWEB_ROOT} -addr=${MINIWEB_HOST}:${MINIWEB_PORT} &

/opt/minimega/bin/minimega \
  -nostdin \
  -force=${MM_FORCE} \
  -recover=${MM_RECOVER} \
  -base=${MM_BASE} \
  -filepath=${MM_FILEPATH} \
  -broadcast=${MM_BROADCAST} \
  -port=${MM_PORT} \
  -degree=${MM_DEGREE} \
  -context=${MM_CONTEXT} \
  -level=${MM_LOGLEVEL} \
  -logfile=${MM_LOGFILE}

#!/bin/bash

/usr/share/openvswitch/scripts/ovs-ctl start

: "${MINIWEB_ROOT:=/opt/minimega/web}"
: "${MINIWEB_HOST:=0.0.0.0}"
: "${MINIWEB_PORT:=9001}"

: "${MM_BASE:=/tmp/minimega}"
: "${MM_FILEPATH:=/tmp/minimega/files}"
: "${MM_BROADCAST:=255.255.255.255}"
: "${MM_VLANRANGE:=101-4096}"
: "${MM_PORT:=9000}"
: "${MM_DEGREE:=2}"
: "${MM_CONTEXT:=minimega}"
: "${MM_LOGLEVEL:=info}"
: "${MM_LOGFILE:=/var/log/minimega.log}"
: "${MM_FORCE:=true}"
: "${MM_RECOVER:=false}"
: "${MM_CGROUP:=/sys/fs/cgroup}"
: "${MM_APPEND:=}"

[[ -f "/etc/default/minimega" ]] && source "/etc/default/minimega"

/opt/minimega/bin/miniweb -root=${MINIWEB_ROOT} -addr=${MINIWEB_HOST}:${MINIWEB_PORT} &

/opt/minimega/bin/minimega \
  -nostdin \
  -force=${MM_FORCE} \
  -recover=${MM_RECOVER} \
  -base=${MM_BASE} \
  -filepath=${MM_FILEPATH} \
  -broadcast=${MM_BROADCAST} \
  -vlanrange=${MM_VLANRANGE} \
  -port=${MM_PORT} \
  -degree=${MM_DEGREE} \
  -context=${MM_CONTEXT} \
  -level=${MM_LOGLEVEL} \
  -logfile=${MM_LOGFILE} \
  -cgroup=${MM_CGROUP} \
  ${MM_APPEND}

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

: "${OVS_HOST_IFACE:=}"

[[ -f "/etc/default/minimega" ]] && source "/etc/default/minimega"

# Use OVS_HOST_IFACE env variable to auto add a host Ethernet interface(s) to an
# OVS bridge. Note that the OVS bridge to add the interface(s) to must be
# specified. The format of the value is "<bridge>:<port>[,<port>,...]".
#
# Single Interface Example (where bridge name is "phenix"): OVS_HOST_IFACE=phenix:eth0
# Multi Interface Example (where bridge name is "phenix"): OVS_HOST_IFACE=phenix:eth0,eth1,eth2

if [[ -v "OVS_HOST_IFACE" ]]; then
  iface=(${OVS_HOST_IFACE//:/ })

  if [[ -n "${iface[0]}" ]]; then
    /usr/bin/ovs-vsctl --may-exist add-br ${iface[0]}
    ip link set dev ${iface[0]} up
  fi

  if [[ -n "${iface[1]}" ]]; then
    ports=(${iface[1]//,/ })

    for port in "${ports[@]}"; do
      /usr/bin/ovs-vsctl --may-exist add-port ${iface[0]} ${port}
    done
  fi
fi

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

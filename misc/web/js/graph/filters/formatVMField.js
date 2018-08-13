const formatted = {
  'append': 'Append',
  'bridge': 'Bridge',
  'cc_active': 'Active CC',
  'cdrom': 'CD-ROM',
  'console_port': 'Console Port',
  'disk': 'Disk',
  'fifo': 'FIFO',
  'filesystem': 'Filesystem',
  'host': 'Host',
  'hostname': 'Hostname',
  'id': 'ID',
  'init': 'init',
  'initrd': 'initrd',
  'ip': 'IPv4',
  'ip6': 'IPv6',
  'kernel': 'Kernel',
  'mac': 'MAC',
  'memory': 'Memory',
  'migrate': 'Migrate',
  'name': 'Name',
  'pid': 'PID',
  'preinit': 'preinit',
  'qos': 'QoS',
  'serial-ports': 'Serial Ports',
  'snapshot': 'Snapshot',
  'state': 'State',
  'tags': 'Tags',
  'tap': 'Tap',
  'type': 'Type',
  'uptime': 'Uptime',
  'uuid': 'UUID',
  'vcpus': 'VCPUs',
  'virtio-ports': 'Virtio Ports',
  'vlan': 'VLAN',
  'vnc_port': 'VNC Port',
  'volume': 'Volume',
};

// formatVMField is a custom Vue filter that translates
// machine-friendly VM field names to their human-friendly
// counterparts.
export const formatVMField = (value) => {
  if (!formatted[value]) {
    return value;
  }
  return formatted[value];
};

/*
The phenix application is a minimega experiment orchestration tool.

Building

Simply run `make bin/phenix`.

Using

The following output results from `bin/phenix --help`:

Example

  $> bin/phenix create data/topology.yml data/scenario.yml data/experiment.yml
  Topology/foo-bar-topo config created
  Scenario/foo-bar-scenario config created
  experiment app sink not found
  host app protonuke not found
  host app wireguard not found
  Experiment/foobar config created

... or ...

  $> bin/phenix list

  +------------+----------------------+------------------+---------------------------+
  |    KIND    |       VERSION        |       NAME       |          CREATED          |
  +------------+----------------------+------------------+---------------------------+
  | Topology   | phenix.sandia.gov/v1 | foo-bar-topo     | 2020-04-17T12:13:48-06:00 |
  | Scenario   | phenix.sandia.gov/v1 | foo-bar-scenario | 2020-04-17T12:13:48-06:00 |
  | Experiment | phenix.sandia.gov/v1 | foobar           | 2020-04-17T12:13:48-06:00 |
  +------------+----------------------+------------------+---------------------------+

... or ...

  $> bin/phenix get scenario/foo-bar-scenario
  apiVersion: phenix.sandia.gov/v1
  kind: Scenario
  metadata:
      name: foo-bar-scenario
      created: "2020-04-17T12:13:48-06:00"
      updated: "2020-04-17T12:13:48-06:00"
      annotations:
          topology: foo-bar-topo
  spec:
      apps:
          experiment:
            - metadata: {}
              name: sink
          host:
            - hosts:
                - hostname: turbine-01
                  metadata:
                      args: -logfile /var/log/protonuke.log -level debug -http -https
                          -smtp -ssh 192.168.100.100
              name: protonuke
            - hosts:
                - hostname: turbine-01
                  metadata:
                      infrastructure:
                          address: 10.255.255.1/24
                          listen_port: 51820
                          private_key: GLlxWJom8cQViGHojqOUShWIZG7IsSX8
                      peers:
                          allowed_ips: 10.255.255.10/32
                          public_key: +joyya2F9g72qbKBtPDn00mIevG1j1OqeN76ylFLsiE=
              name: wireguard

... or ...

  $> bin/phenix experiment start foobar
  namespace foobar
  ns queueing true

  disk snapshot bennu.qc2 0b02f5d75d22_foobar_turbine-01_snapshot
  clear vm config
  vm config vcpus 1
  vm config cpu Broadwell
  vm config memory 512
  vm config snapshot true
  vm config disk 0b02f5d75d22_foobar_turbine-01_snapshot
  vm config qemu-append -vga qxl
  vm config net ot MGMT
  vm launch kvm turbine-01

  disk snapshot bennu.qc2 0b02f5d75d22_foobar_turbine-02_snapshot
  clear vm config
  vm config vcpus 1
  vm config cpu Broadwell
  vm config memory 512
  vm config snapshot true
  vm config disk 0b02f5d75d22_foobar_turbine-02_snapshot
  vm config qemu-append -vga qxl
  vm config net ot MGMT
  vm launch kvm turbine-02

  $> bin/phenix experiment stop foobar

You can also edit configs in place via something like:

  $> bin/phenix edit experiment/foobar
*/
package main

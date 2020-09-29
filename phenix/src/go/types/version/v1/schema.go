package v1

var OpenAPI = []byte(`
openapi: "3.0.0"
info:
  title: phenix
  version: "1.0"
paths: {}
components:
  schemas:
    Image:
      type: object
      title: miniccc Image
      required:
      - release
      properties:
        release:
          type: string
          minLength: 1
    Topology:
      type: object
      title: Demo Topology
      required:
      - nodes
      properties:
        nodes:
          type: array
          title: Nodes
          items:
            $ref: "#/components/schemas/Node"
    Scenario:
      type: object
      required:
      - apps
      properties:
        apps:
          type: object
          properties:
            experiment:
              type: array
              items:
                type: object
                required:
                - name
                properties:
                  name:
                    type: string
                    minLength: 1
    Experiment:
      type: object
      required:
      - experimentName
      properties:
        experimentName:
          type: string
          minLength: 1
        baseDir:
          type: string
        vlans:
          type: object
          title: VLANs
          properties:
            aliases:
              type: object
              title: Aliases
              additionalProperties:
                type: integer
              example:
                MGMT: 200
            min:
              type: integer
            max:
              type: integer
        schedule:
          type: object
          title: Schedule
          additionalProperties:
            type: string
          example:
            ADServer: compute1
    Node:
      type: object
      title: Node
      required:
      - type
      - general
      - hardware
      properties:
        type:
          type: string
          title: Node Type
          enum:
          - Firewall
          - Printer
          - Router
          - Server
          - Switch
          - VirtualMachine
          default: VirtualMachine
          example: VirtualMachine
        general:
          type: object
          title: General Node Configuration
          required:
          - hostname
          properties:
            hostname:
              type: string
              title: Hostname
              minLength: 1
              example: ADServer
            description:
              type: string
              title: Description
              example: Active Directory Server
            vm_type:
              type: string
              title: VM (Emulation) Type
              enum:
              - kvm
              - container
              - ""
              default: kvm
              example: kvm
            snapshot:
              type: boolean
              title: Snapshot Mode
              default: false
              example: false
              nullable: true
            do_not_boot:
              type: boolean
              title: Do Not Boot VM
              default: false
              example: false
              nullable: true
        hardware:
          type: object
          title: Node Hardware Configuration
          required:
          - os_type
          - drives
          properties:
            cpu:
              type: string
              title: CPU Emulation
              enum:
              - Broadwell
              - Haswell
              - core2duo
              - pentium3
              - ""
              default: Broadwell
              example: Broadwell
            vcpus:
              type: integer
              title: VCPU Count
              default: 1
              example: 4
            memory:
              type: integer
              title: Memory
              default: 1024
              example: 8192
            os_type:
              type: string
              title: OS Type
              enum:
              - windows
              - linux
              - rhel
              - centos
              default: linux
              example: windows
            drives:
              type: array
              title: Drives
              items:
                type: object
                title: Drive
                required:
                - image
                properties:
                  image:
                    type: string
                    title: Image File Name
                    minLength: 1
                    example: ubuntu.qc2
                  interface:
                    type: string
                    title: Drive Interface
                    enum:
                    - ahci
                    - ide
                    - scsi
                    - sd
                    - mtd
                    - floppy
                    - pflash
                    - virtio
                    - ""
                    default: ide
                    example: ide
                  cache_mode:
                    type: string
                    title: Drive Cache Mode
                    enum:
                    - none
                    - writeback
                    - unsafe
                    - directsync
                    - writethrough
                    - ""
                    default: writeback
                    example: writeback
                  inject_partition:
                    type: integer
                    title: Disk Image Partition to Inject Files Into
                    default: 1
                    example: 2
                    nullable: true
        network:
          type: object
          title: Node Network Configuration
          required:
          - interfaces
          properties:
            interfaces:
              type: array
              title: Network Interfaces
              items:
                type: object
                title: Network Interface
                oneOf:
                - $ref: '#/components/schemas/static_iface'
                - $ref: '#/components/schemas/dhcp_iface'
                - $ref: '#/components/schemas/serial_iface'
            routes:
              type: array
              items:
                type: object
                title: Network Route
                required:
                - destination
                - next
                - cost
                properties:
                  destination:
                    type: string
                    title: Routing Destination
                    minLength: 1
                    example: 192.168.0.0/24
                  next:
                    type: string
                    title: Next Hop for Routing Destination
                    minLength: 1
                    example: 192.168.1.254
                  cost:
                    type: integer
                    title: Routing Cost (weight)
                    default: 1
                    example: 1
            ospf:
              type: object
              title: OSPF Routing Configuration
              required:
              - router_id
              - areas
              properties:
                router_id:
                  type: string
                  title: Router ID
                  minLength: 1
                  example: 0.0.0.1
                areas:
                  type: array
                  title: Routing Areas
                  items:
                    type: object
                    title: Routing Area
                    required:
                    - area_id
                    - area_networks
                    properties:
                      area_id:
                        type: integer
                        title: Area ID
                        example: 1
                        default: 1
                      area_networks:
                        type: array
                        title: Area Networks
                        items:
                          type: object
                          title: Area Network
                          required:
                          - network
                          properties:
                            network: 
                              type: string
                              title: Network
                              minLength: 1
                              example: 10.1.25.0/24
            rulesets:
              type: array
              title: Firewall Rulesets
              items:
                type: object
                title: Firewall Ruleset
                required:
                - name
                - default
                - rules
                properties:
                  name:
                    type: string
                    title: Ruleset Name
                    minLength: 1
                    example: OutToDMZ
                  description:
                    type: string
                    title: Ruleset Description
                    minLength: 1
                    example: From Corp to the DMZ network
                  default:
                    type: string
                    title: Default Firewall Action
                    enum:
                    - accept
                    - drop
                    - reject
                    example: drop
                  rules:
                    type: array
                    title: Firewall Rules
                    items:
                      type: object
                      title: Firewall Rule
                      required:
                      - id
                      - action
                      - protocol
                      properties:
                        id:
                          type: integer
                          title: Rule ID
                          example: 10
                        description:
                          type: string
                          title: Rule Description
                          example: Allow UDP 10.1.26.80 ==> 10.2.25.0/24:123
                        action:
                          type: string
                          title: Rule Action
                          enum:
                          - accept
                          - drop
                          - reject
                          example: accept
                        protocol:
                          type: string
                          title: Network Protocol
                          enum:
                          - tcp
                          - udp
                          - icmp
                          - esp
                          - ah
                          - all
                          default: tcp
                          example: tcp
                        source:
                          type: object
                          title: Source Address
                          required:
                          - address
                          properties:
                            address:
                              type: string
                              title: IP Address
                              minLength: 1
                              example: 10.1.24.60
                            port:
                              type: integer
                              title: Port Number
                              example: 3389
                        destination:
                          type: object
                          title: Destination Address
                          required:
                          - address
                          properties:
                            address:
                              type: string
                              title: IP Address
                              minLength: 1
                              example: 10.1.24.60
                            port:
                              type: integer
                              title: Port Number
                              example: 3389
        injections:
          type: array
          title: Node File Injections
          items:
            type: object
            title: Node File Injection
            required:
            - src
            - dst
            properties:
              src:
                type: string
                title: Location of Source File to Inject
                minLength: 1
                example: foo.xml
              dst:
                type: string
                title: Destination Location to Inject File To
                minLength: 1
                example: /etc/phenix/foo.xml
              description:
                type: string
                title: Description of file being injected
                example: phenix config file
              permissions:
                type: string
                title: Injected file permissions (UNIX style)
                example: 0664
    iface:
      type: object
      required:
      - name
      - vlan
      properties:
        name:
          type: string
          title: Name
          minLength: 1
          example: eth0
        vlan:
          type: string
          title: VLAN
          minLength: 1
          example: EXP-1
        autostart:
          type: boolean
          title: Auto Start Interface
          default: true
        mac:
          type: string
          title: Interface MAC Address
          example: 00:11:22:33:44:55:66
          pattern: '^([0-9a-fA-F]{2}[:-]){5}([0-9a-fA-F]){2}$'
        mtu:
          type: integer
          title: Interface MTU
          default: 1500
          example: 1500
        bridge:
          type: string
          title: OpenVSwitch Bridge
          default: phenix
    iface_address:
      type: object
      required:
      - address
      - mask
      properties:
        address:
          type: string
          format: ipv4
          title: IP Address
          minLength: 7
          example: 192.168.1.100
        mask:
          type: integer
          title: IP Address Netmask
          minimum: 0
          maximum: 32
          default: 24
          example: 24
        gateway:
          type: string
          format: ipv4
          title: Default Gateway
          minLength: 7
          example: 192.168.1.1
    iface_rulesets:
      type: object
      properties:
        ruleset_out:
          type: string
          title: Outbound Ruleset
          example: OutToInet
          pattern: '^[\w-]+$'
        ruleset_in:
          type: string
          title: Inbound Ruleset
          example: InFromInet
          pattern: '^[\w-]+$'
    static_iface:
      allOf:
      - $ref: '#/components/schemas/iface'
      - $ref: '#/components/schemas/iface_address'
      - $ref: '#/components/schemas/iface_rulesets'
      required:
      - type
      - proto
      properties:
        type:
          type: string
          title: Interface Type
          enum:
          - ethernet
          default: ethernet
          example: ethernet
        proto:
          type: string
          title: Interface Protocol
          enum:
          - static
          - ospf
          default: static
          example: static
    dhcp_iface:
      allOf:
      - $ref: '#/components/schemas/iface'
      - $ref: '#/components/schemas/iface_rulesets'
      required:
      - type
      - proto
      properties:
        type:
          type: string
          title: Interface Type
          enum:
          - ethernet
          default: ethernet
          example: ethernet
        proto:
          type: string
          title: Interface Protocol
          enum:
          - dhcp
          default: dhcp
          example: dhcp
    serial_iface:
      allOf:
      - $ref: '#/components/schemas/iface'
      - $ref: '#/components/schemas/iface_address'
      - $ref: '#/components/schemas/iface_rulesets'
      required:
      - type
      - proto
      - udp_port
      - baud_rate
      - device
      properties:
        type:
          type: string
          title: Interface Type
          enum:
          - serial
          default: serial
          example: serial
        proto:
          type: string
          title: Interface Protocol
          enum:
          - static
          default: static
          example: static
        udp_port:
          type: integer
          title: UDP Port
          minimum: 0
          maximum: 65535
          default: 8989
          example: 8989
        baud_rate:
          type: integer
          title: Serial Baud Rate
          enum:
          - 110
          - 300
          - 600
          - 1200
          - 2400
          - 4800
          - 9600
          - 14400
          - 19200
          - 38400
          - 57600
          - 115200
          - 128000
          - 256000
          default: 9600
          example: 9600
        device:
          type: string
          title: Serial Device
          minLength: 1
          default: /dev/ttyS0
          example: /dev/ttyS0
          pattern: '^[\w\/]+\w+$'
`)

// Copyright 2018-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS). 
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain 
// rights in this software.

package qemu

// kvm -cpu ?
const cpusOut = `Available CPUs:
x86              486
x86  Broadwell-noTSX  Intel Core Processor (Broadwell, no TSX)
x86        Broadwell  Intel Core Processor (Broadwell)
x86           Conroe  Intel Celeron_4x0 (Conroe/Merom Class Core 2)
x86             EPYC  AMD EPYC Processor
x86    Haswell-noTSX  Intel Core Processor (Haswell, no TSX)
x86          Haswell  Intel Core Processor (Haswell)
x86        IvyBridge  Intel Xeon E3-12xx v2 (Ivy Bridge)
x86          Nehalem  Intel Core i7 9xx (Nehalem Class Core i7)
x86       Opteron_G1  AMD Opteron 240 (Gen 1 Class Opteron)
x86       Opteron_G2  AMD Opteron 22xx (Gen 2 Class Opteron)
x86       Opteron_G3  AMD Opteron 23xx (Gen 3 Class Opteron)
x86       Opteron_G4  AMD Opteron 62xx class CPU
x86       Opteron_G5  AMD Opteron 63xx class CPU
x86           Penryn  Intel Core 2 Duo P9xxx (Penryn Class Core 2)
x86      SandyBridge  Intel Xeon E312xx (Sandy Bridge)
x86   Skylake-Client  Intel Core Processor (Skylake)
x86   Skylake-Server  Intel Xeon Processor (Skylake)
x86         Westmere  Westmere E56xx/L56xx/X56xx (Nehalem-C)
x86           athlon  QEMU Virtual CPU version 2.5+
x86         core2duo  Intel(R) Core(TM)2 Duo CPU     T7700  @ 2.40GHz
x86          coreduo  Genuine Intel(R) CPU           T2600  @ 2.16GHz
x86            kvm32  Common 32-bit KVM processor
x86            kvm64  Common KVM processor
x86             n270  Intel(R) Atom(TM) CPU N270   @ 1.60GHz
x86          pentium
x86         pentium2
x86         pentium3
x86           phenom  AMD Phenom(tm) 9550 Quad-Core Processor
x86           qemu32  QEMU Virtual CPU version 2.5+
x86           qemu64  QEMU Virtual CPU version 2.5+
x86             base  base CPU model type with no features enabled
x86             host  KVM processor with all supported host features (only available in KVM mode)
x86              max  Enables all features supported by the accelerator in the current host

Recognized CPUID flags:
  fpu vme de pse tsc msr pae mce cx8 apic sep mtrr pge mca cmov pat pse36 pn clflush ds acpi mmx fxsr sse sse2 ss ht tm ia64 pbe
  pni pclmulqdq dtes64 monitor ds-cpl vmx smx est tm2 ssse3 cid fma cx16 xtpr pdcm pcid dca sse4.1 sse4.2 x2apic movbe popcnt tsc-deadline aes xsave osxsave avx f16c rdrand hypervisor
  fsgsbase tsc-adjust bmi1 hle avx2 smep bmi2 erms invpcid rtm mpx avx512f avx512dq rdseed adx smap avx512ifma pcommit clflushopt clwb avx512pf avx512er avx512cd sha-ni avx512bw avx512vl
  avx512vbmi umip pku ospke avx512-vpopcntdq la57 rdpid
  avx512-4vnniw avx512-4fmaps
  syscall nx mmxext fxsr-opt pdpe1gb rdtscp lm 3dnowext 3dnow
  lahf-lm cmp-legacy svm extapic cr8legacy abm sse4a misalignsse 3dnowprefetch osvw ibs xop skinit wdt lwp fma4 tce nodeid-msr tbm topoext perfctr-core perfctr-nb
  invtsc
  xstore xstore-en xcrypt xcrypt-en ace2 ace2-en phe phe-en pmm pmm-en
  kvmclock kvm-nopiodelay kvm-mmu kvmclock kvm-asyncpf kvm-steal-time kvm-pv-eoi kvm-pv-unhalt kvm-pv-tlb-flush kvmclock-stable-bit



  npt lbrv svm-lock nrip-save tsc-scale vmcb-clean flushbyasid decodeassists pause-filter pfthreshold
  xsaveopt xsavec xgetbv1 xsaves
  arat


`

// qemu-system-aarch64 -M vexpress-a15 -cpu ?
const cpusOutARM = `Available CPUs:
  arm1026
  arm1136
  arm1136-r2
  arm1176
  arm11mpcore
  arm926
  arm946
  cortex-a15
  cortex-a53
  cortex-a57
  cortex-a7
  cortex-a8
  cortex-a9
  cortex-m3
  cortex-m4
  cortex-r5
  pxa250
  pxa255
  pxa260
  pxa261
  pxa262
  pxa270-a0
  pxa270-a1
  pxa270
  pxa270-b0
  pxa270-b1
  pxa270-c0
  pxa270-c5
  sa1100
  sa1110
  ti925t

`

// kvm -M ?
const machinesOut = `Supported machines are:
pc-i440fx-2.9        Standard PC (i440FX + PIIX, 1996)
pc-i440fx-2.8        Standard PC (i440FX + PIIX, 1996)
pc-i440fx-2.7        Standard PC (i440FX + PIIX, 1996)
pc-i440fx-2.6        Standard PC (i440FX + PIIX, 1996)
pc-i440fx-2.5        Standard PC (i440FX + PIIX, 1996)
pc-i440fx-2.4        Standard PC (i440FX + PIIX, 1996)
pc-i440fx-2.3        Standard PC (i440FX + PIIX, 1996)
pc-i440fx-2.2        Standard PC (i440FX + PIIX, 1996)
pc                   Standard PC (i440FX + PIIX, 1996) (alias of pc-i440fx-2.11)
pc-i440fx-2.11       Standard PC (i440FX + PIIX, 1996) (default)
pc-i440fx-2.10       Standard PC (i440FX + PIIX, 1996)
pc-i440fx-2.1        Standard PC (i440FX + PIIX, 1996)
pc-i440fx-2.0        Standard PC (i440FX + PIIX, 1996)
pc-i440fx-1.7        Standard PC (i440FX + PIIX, 1996)
pc-i440fx-1.6        Standard PC (i440FX + PIIX, 1996)
pc-i440fx-1.5        Standard PC (i440FX + PIIX, 1996)
pc-i440fx-1.4        Standard PC (i440FX + PIIX, 1996)
pc-1.3               Standard PC (i440FX + PIIX, 1996)
pc-1.2               Standard PC (i440FX + PIIX, 1996)
pc-1.1               Standard PC (i440FX + PIIX, 1996)
pc-1.0               Standard PC (i440FX + PIIX, 1996)
pc-0.15              Standard PC (i440FX + PIIX, 1996)
pc-0.14              Standard PC (i440FX + PIIX, 1996)
pc-0.13              Standard PC (i440FX + PIIX, 1996)
pc-0.12              Standard PC (i440FX + PIIX, 1996)
pc-0.11              Standard PC (i440FX + PIIX, 1996)
pc-0.10              Standard PC (i440FX + PIIX, 1996)
pc-q35-2.9           Standard PC (Q35 + ICH9, 2009)
pc-q35-2.8           Standard PC (Q35 + ICH9, 2009)
pc-q35-2.7           Standard PC (Q35 + ICH9, 2009)
pc-q35-2.6           Standard PC (Q35 + ICH9, 2009)
pc-q35-2.5           Standard PC (Q35 + ICH9, 2009)
pc-q35-2.4           Standard PC (Q35 + ICH9, 2009)
q35                  Standard PC (Q35 + ICH9, 2009) (alias of pc-q35-2.11)
pc-q35-2.11          Standard PC (Q35 + ICH9, 2009)
pc-q35-2.10          Standard PC (Q35 + ICH9, 2009)
isapc                ISA-only PC
none                 empty machine
xenfv                Xen Fully-virtualized PC
xenpv                Xen Para-virtualized PC`

// kvm -device ?
const deviceOut = `Controller/Bridge/Hub devices:
name "i82801b11-bridge", bus PCI
name "ioh3420", bus PCI, desc "Intel IOH device id 3420 PCIE Root Port"
name "pci-bridge", bus PCI, desc "Standard PCI Bridge"
name "pci-bridge-seat", bus PCI, desc "Standard PCI Bridge (multiseat)"
name "pcie-pci-bridge", bus PCI
name "pcie-root-port", bus PCI, desc "PCI Express Root Port"
name "pxb", bus PCI, desc "PCI Expander Bridge"
name "pxb-pcie", bus PCI, desc "PCI Express Expander Bridge"
name "usb-host", bus usb-bus
name "usb-hub", bus usb-bus
name "vfio-pci-igd-lpc-bridge", bus PCI, desc "VFIO dummy ISA/LPC bridge for IGD assignment"
name "x3130-upstream", bus PCI, desc "TI X3130 Upstream Port of PCI Express Switch"
name "xio3130-downstream", bus PCI, desc "TI X3130 Downstream Port of PCI Express Switch"

USB devices:
name "ich9-usb-ehci1", bus PCI
name "ich9-usb-ehci2", bus PCI
name "ich9-usb-uhci1", bus PCI
name "ich9-usb-uhci2", bus PCI
name "ich9-usb-uhci3", bus PCI
name "ich9-usb-uhci4", bus PCI
name "ich9-usb-uhci5", bus PCI
name "ich9-usb-uhci6", bus PCI
name "nec-usb-xhci", bus PCI
name "pci-ohci", bus PCI, desc "Apple USB Controller"
name "piix3-usb-uhci", bus PCI
name "piix4-usb-uhci", bus PCI
name "qemu-xhci", bus PCI
name "usb-ehci", bus PCI
name "vt82c686b-usb-uhci", bus PCI

Storage devices:
name "am53c974", bus PCI, desc "AMD Am53c974 PCscsi-PCI SCSI adapter"
name "dc390", bus PCI, desc "Tekram DC-390 SCSI adapter"
name "floppy", bus floppy-bus, desc "virtual floppy drive"
name "ich9-ahci", bus PCI, alias "ahci"
name "ide-cd", bus IDE, desc "virtual IDE CD-ROM"
name "ide-drive", bus IDE, desc "virtual IDE disk or CD-ROM (legacy)"
name "ide-hd", bus IDE, desc "virtual IDE disk"
name "isa-fdc", bus ISA
name "isa-ide", bus ISA
name "lsi53c810", bus PCI
name "lsi53c895a", bus PCI, alias "lsi"
name "megasas", bus PCI, desc "LSI MegaRAID SAS 1078"
name "megasas-gen2", bus PCI, desc "LSI MegaRAID SAS 2108"
name "nvme", bus PCI, desc "Non-Volatile Memory Express"
name "piix3-ide", bus PCI
name "piix3-ide-xen", bus PCI
name "piix4-ide", bus PCI
name "pvscsi", bus PCI
name "scsi-block", bus SCSI, desc "SCSI block device passthrough"
name "scsi-cd", bus SCSI, desc "virtual SCSI CD-ROM"
name "scsi-disk", bus SCSI, desc "virtual SCSI disk or CD-ROM (legacy)"
name "scsi-generic", bus SCSI, desc "pass through generic scsi device (/dev/sg*)"
name "scsi-hd", bus SCSI, desc "virtual SCSI disk"
name "sdhci-pci", bus PCI
name "usb-bot", bus usb-bus
name "usb-mtp", bus usb-bus, desc "USB Media Transfer Protocol device"
name "usb-storage", bus usb-bus
name "usb-uas", bus usb-bus
name "vhost-scsi", bus virtio-bus
name "vhost-scsi-pci", bus PCI
name "vhost-user-scsi", bus virtio-bus
name "vhost-user-scsi-pci", bus PCI
name "virtio-9p-device", bus virtio-bus
name "virtio-9p-pci", bus PCI, alias "virtio-9p"
name "virtio-blk-device", bus virtio-bus
name "virtio-blk-pci", bus PCI, alias "virtio-blk"
name "virtio-scsi-device", bus virtio-bus
name "virtio-scsi-pci", bus PCI, alias "virtio-scsi"

Network devices:
name "e1000", bus PCI, alias "e1000-82540em", desc "Intel Gigabit Ethernet"
name "e1000-82544gc", bus PCI, desc "Intel Gigabit Ethernet"
name "e1000-82545em", bus PCI, desc "Intel Gigabit Ethernet"
name "e1000e", bus PCI, desc "Intel 82574L GbE Controller"
name "i82550", bus PCI, desc "Intel i82550 Ethernet"
name "i82551", bus PCI, desc "Intel i82551 Ethernet"
name "i82557a", bus PCI, desc "Intel i82557A Ethernet"
name "i82557b", bus PCI, desc "Intel i82557B Ethernet"
name "i82557c", bus PCI, desc "Intel i82557C Ethernet"
name "i82558a", bus PCI, desc "Intel i82558A Ethernet"
name "i82558b", bus PCI, desc "Intel i82558B Ethernet"
name "i82559a", bus PCI, desc "Intel i82559A Ethernet"
name "i82559b", bus PCI, desc "Intel i82559B Ethernet"
name "i82559c", bus PCI, desc "Intel i82559C Ethernet"
name "i82559er", bus PCI, desc "Intel i82559ER Ethernet"
name "i82562", bus PCI, desc "Intel i82562 Ethernet"
name "i82801", bus PCI, desc "Intel i82801 Ethernet"
name "ne2k_isa", bus ISA
name "ne2k_pci", bus PCI
name "pcnet", bus PCI
name "rocker", bus PCI, desc "Rocker Switch"
name "rtl8139", bus PCI
name "usb-bt-dongle", bus usb-bus
name "usb-net", bus usb-bus
name "virtio-net-device", bus virtio-bus
name "virtio-net-pci", bus PCI, alias "virtio-net"
name "vmxnet3", bus PCI, desc "VMWare Paravirtualized Ethernet v3"

Input devices:
name "ccid-card-emulated", bus ccid-bus, desc "emulated smartcard"
name "ccid-card-passthru", bus ccid-bus, desc "passthrough smartcard"
name "ipoctal232", bus IndustryPack, desc "GE IP-Octal 232 8-channel RS-232 IndustryPack"
name "isa-parallel", bus ISA
name "isa-serial", bus ISA
name "pci-serial", bus PCI
name "pci-serial-2x", bus PCI
name "pci-serial-4x", bus PCI
name "tpci200", bus PCI, desc "TEWS TPCI200 IndustryPack carrier"
name "usb-braille", bus usb-bus
name "usb-ccid", bus usb-bus, desc "CCID Rev 1.1 smartcard reader"
name "usb-kbd", bus usb-bus
name "usb-mouse", bus usb-bus
name "usb-serial", bus usb-bus
name "usb-tablet", bus usb-bus
name "usb-wacom-tablet", bus usb-bus, desc "QEMU PenPartner Tablet"
name "virtconsole", bus virtio-serial-bus
name "virtio-input-host-device", bus virtio-bus
name "virtio-input-host-pci", bus PCI, alias "virtio-input-host"
name "virtio-keyboard-device", bus virtio-bus
name "virtio-keyboard-pci", bus PCI, alias "virtio-keyboard"
name "virtio-mouse-device", bus virtio-bus
name "virtio-mouse-pci", bus PCI, alias "virtio-mouse"
name "virtio-serial-device", bus virtio-bus
name "virtio-serial-pci", bus PCI, alias "virtio-serial"
name "virtio-tablet-device", bus virtio-bus
name "virtio-tablet-pci", bus PCI, alias "virtio-tablet"
name "virtserialport", bus virtio-serial-bus

Display devices:
name "cirrus-vga", bus PCI, desc "Cirrus CLGD 54xx VGA"
name "isa-cirrus-vga", bus ISA
name "isa-vga", bus ISA
name "qxl", bus PCI, desc "Spice QXL GPU (secondary)"
name "qxl-vga", bus PCI, desc "Spice QXL GPU (primary, vga compatible)"
name "secondary-vga", bus PCI
name "sga", bus ISA, desc "Serial Graphics Adapter"
name "VGA", bus PCI
name "virtio-gpu-device", bus virtio-bus
name "virtio-gpu-pci", bus PCI, alias "virtio-gpu"
name "virtio-vga", bus PCI
name "vmware-svga", bus PCI

Sound devices:
name "AC97", bus PCI, desc "Intel 82801AA AC97 Audio"
name "adlib", bus ISA, desc "Yamaha YM3812 (OPL2)"
name "cs4231a", bus ISA, desc "Crystal Semiconductor CS4231A"
name "ES1370", bus PCI, desc "ENSONIQ AudioPCI ES1370"
name "gus", bus ISA, desc "Gravis Ultrasound GF1"
name "hda-duplex", bus HDA, desc "HDA Audio Codec, duplex (line-out, line-in)"
name "hda-micro", bus HDA, desc "HDA Audio Codec, duplex (speaker, microphone)"
name "hda-output", bus HDA, desc "HDA Audio Codec, output-only (line-out)"
name "ich9-intel-hda", bus PCI, desc "Intel HD Audio Controller (ich9)"
name "intel-hda", bus PCI, desc "Intel HD Audio Controller (ich6)"
name "sb16", bus ISA, desc "Creative Sound Blaster 16"
name "usb-audio", bus usb-bus

Misc devices:
name "hyperv-testdev", bus ISA
name "i6300esb", bus PCI
name "ib700", bus ISA
name "isa-applesmc", bus ISA
name "isa-debug-exit", bus ISA
name "isa-debugcon", bus ISA
name "ivshmem", bus PCI, desc "Inter-VM shared memory (legacy)"
name "ivshmem-doorbell", bus PCI, desc "Inter-VM shared memory"
name "ivshmem-plain", bus PCI, desc "Inter-VM shared memory"
name "pc-testdev", bus ISA
name "pci-testdev", bus PCI, desc "PCI Test Device"
name "pvpanic", bus ISA
name "usb-redir", bus usb-bus
name "vfio-pci", bus PCI, desc "VFIO-based PCI device assignment"
name "vhost-vsock-device", bus virtio-bus
name "vhost-vsock-pci", bus PCI
name "virtio-balloon-device", bus virtio-bus
name "virtio-balloon-pci", bus PCI, alias "virtio-balloon"
name "virtio-crypto-device", bus virtio-bus
name "virtio-crypto-pci", bus PCI
name "virtio-rng-device", bus virtio-bus
name "virtio-rng-pci", bus PCI, alias "virtio-rng"
name "vmcoreinfo"
name "vmgenid"
name "xen-backend", bus xen-sysbus
name "xen-pci-passthrough", bus PCI, desc "Assign an host PCI device with Xen"
name "xen-platform", bus PCI, desc "XEN platform pci device"

CPU devices:
name "486-x86_64-cpu"
name "athlon-x86_64-cpu"
name "base-x86_64-cpu"
name "Broadwell-noTSX-x86_64-cpu"
name "Broadwell-x86_64-cpu"
name "Conroe-x86_64-cpu"
name "core2duo-x86_64-cpu"
name "coreduo-x86_64-cpu"
name "EPYC-x86_64-cpu"
name "Haswell-noTSX-x86_64-cpu"
name "Haswell-x86_64-cpu"
name "host-x86_64-cpu"
name "IvyBridge-x86_64-cpu"
name "kvm32-x86_64-cpu"
name "kvm64-x86_64-cpu"
name "max-x86_64-cpu"
name "n270-x86_64-cpu"
name "Nehalem-x86_64-cpu"
name "Opteron_G1-x86_64-cpu"
name "Opteron_G2-x86_64-cpu"
name "Opteron_G3-x86_64-cpu"
name "Opteron_G4-x86_64-cpu"
name "Opteron_G5-x86_64-cpu"
name "Penryn-x86_64-cpu"
name "pentium-x86_64-cpu"
name "pentium2-x86_64-cpu"
name "pentium3-x86_64-cpu"
name "phenom-x86_64-cpu"
name "qemu32-x86_64-cpu"
name "qemu64-x86_64-cpu"
name "SandyBridge-x86_64-cpu"
name "Skylake-Client-x86_64-cpu"
name "Skylake-Server-x86_64-cpu"
name "Westmere-x86_64-cpu"

Uncategorized devices:
name "amd-iommu", bus System
name "AMDVI-PCI", bus PCI
name "edu", bus PCI
name "i8042", bus ISA
name "igd-passthrough-isa-bridge", bus PCI, desc "ISA bridge faked to support IGD PT"
name "intel-iommu", bus System
name "ipmi-bmc-extern"
name "ipmi-bmc-sim"
name "isa-ipmi-bt", bus ISA
name "isa-ipmi-kcs", bus ISA
name "loader", desc "Generic Loader"
name "mptsas1068", bus PCI, desc "LSI SAS 1068"
name "nvdimm", desc "DIMM memory module"
name "pc-dimm", desc "DIMM memory module"
name "sd-card", bus sd-bus
name "tpm-tis", bus ISA
name "xen-pvdevice", bus PCI, desc "Xen PV Device"`

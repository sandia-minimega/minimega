# Chapter 18: Android VMs

In this chapter you add an Android emulator instance to the router sandwich
as a third kind of VM, watch minimega assign its console, ADB, and gRPC
ports, open it in the miniweb console, and give it an address on `net_left`
so the KVM clients can reach it. At the end `phone0` is running beside
`vm_left` and `vm_right`, controllable from the browser and from `adb`.

It assumes the sandwich from [Chapter 3](03-router-sandwich.md), miniweb
from [Chapter 5](05-miniweb.md), and the netspec from
[Chapter 6](06-networking.md). The Android SDK is a large download and an
AVD takes time to create, so do the [preparation](#prepare-the-host) before
class if you are teaching this chapter.

## Prepare the host

Android VMs run the official Android emulator, which is itself a QEMU
build, under KVM. Each host that will run one needs:

- the `kvm` kernel module, as for any KVM guest;
- an Android SDK with the `emulator` and `platform-tools` packages, which
  provide the `emulator` and `adb` binaries;
- an Android Virtual Device (AVD), the emulator's description of a phone:
  system image, screen, RAM.

The SDK's command line tools are available from
[developer.android.com](https://developer.android.com/studio#command-line-tools-only).
With them installed under `/opt/android-sdk`, a system image and an AVD are
created with `sdkmanager` and `avdmanager`:

```bash
$ export ANDROID_HOME=/opt/android-sdk
$ sdkmanager "platform-tools" "emulator" "system-images;android-35;google_apis;x86_64"
$ avdmanager create avd -n Pixel_9a -k "system-images;android-35;google_apis;x86_64"
```

Use `sdkmanager --list` to see which system images your SDK offers, and pick
a `google_apis` image rather than a `google_apis_playstore` one: the former
is a debuggable build that allows `adb root`, which you need to configure
the guest network below. The AVD lands under `~/.android/avd/` for the user
that created it; minimega runs as root, so either create it as root or point
`vm config android-avd-dir` at the directory that holds `Pixel_9a.avd`.

## Configure and launch

Only one field is required, `android-avd`. The rest tell minimega where the
SDK is and tune the emulator:

| Field | Purpose | Default |
|---|---|---|
| `android-avd` | AVD name to boot | required |
| `android-sdk` | SDK root on the host | none |
| `android-emulator`, `android-adb` | emulator and adb binaries, as names on `PATH` or absolute paths | `emulator`, `adb` |
| `android-avd-dir` | directory holding `<name>.avd` | the emulator's default |
| `android-no-window` | suppress the emulator's own window | `true` |
| `android-console-base-port`, `android-grpc-base-port` | preferred starting ports; `0` means start at the bottom of the range | `0` |
| `android-extra-args` | raw arguments appended to the emulator command line | none |
| `android-writable-system` | writable `/system` partition | `false` |

With the sandwich running in namespace `sandwich`:

```minimega
minimega$ namespace sandwich
minimega$ clear vm config
minimega$ vm config android-sdk /opt/android-sdk
minimega$ vm config android-avd Pixel_9a
minimega$ vm config networks net_left
minimega$ vm launch android phone0
minimega$ vm start phone0
```

`clear vm config` drops the KVM fields from Chapter 3 so the template holds
only what this VM needs; it would have worked without, because fields that
do not apply to the launched type are ignored. The emulator takes a minute
or more to boot the first time. minimega monitors it over QMP like a KVM VM
and reports the ports it reserved:

```minimega
minimega$ .columns name,state,type,android_avd,android_console_port,android_adb_port,android_serial,android_grpc_port .filter type=android vm info
host  | name   | state   | type    | android_avd | android_console_port | android_adb_port | android_serial | android_grpc_port
node1 | phone0 | RUNNING | android | Pixel_9a    | 5554                 | 5555             | emulator-5554  | 8554
```

Each Android VM takes one console/ADB port pair from 5554–5681 and one gRPC
port from 8554–8617, scanning forward from the base port until a free one is
found, so a host can run 64 Android VMs at once. `host androidvms` shows
how many are running, and the `android_serial` column is the name `adb`
uses:

```bash
$ adb -s emulator-5554 shell getprop ro.build.version.release
```

## The web console

miniweb serves a console for the VM at `/vm/phone0/connect/`, the same path
you used for VNC in Chapter 5. It streams the emulator's display as PNG
frames over a WebSocket, maps mouse and keyboard input, and adds what a
phone has and a PC does not: Back, Home, Recents, Power, and volume buttons,
a landscape toggle, and a GPS panel with presets and custom coordinates. All
of it goes through miniweb's gRPC connection to the emulator, so the browser
never needs to reach the emulator's ports itself.

Screenshots come from the emulator over gRPC, not from QEMU, so the
`vm screenshot` command does not support Android VMs yet and returns an
error. Use miniweb instead: `/vm/phone0/screenshot.png?size=300` serves a
scaled screenshot, which is what the tile view uses.

## Networking

`vm config networks net_left` gave the VM a tap on the sandwich's left
network. The emulator's QEMU does not support the default `e1000` device,
so minimega attaches the tap with `virtio-net-pci`; the guest sees it as an
extra interface, `eth1`, with no address. The emulator's own `eth0` is its
built-in user-mode network and has nothing to do with your experiment.

minimega only builds the host side. It does not run a DHCP client, set
routes, or touch the guest firewall, because an Android image has no
miniccc and no standard place to do that. Configure the interface over
`adb`:

```bash
$ adb -s emulator-5554 root
$ adb -s emulator-5554 shell ip link set eth1 up
$ adb -s emulator-5554 shell ip addr add 10.0.0.50/24 dev eth1
$ adb -s emulator-5554 shell ip addr show eth1
```

Whether `adb root` is allowed depends on the system image; Google Play
images refuse it. Once the address is there, `vm_left` can reach the phone
through `cc`:

```minimega
minimega$ cc filter name=vm_left
minimega$ cc exec ping -c 3 10.0.0.50
minimega$ cc responses all
minimega$ clear cc filter
```

`vm net connect` and `vm net disconnect` move or detach the host-side tap
without the guest noticing anything but a link change. `vm net add` hot-adds
a device that the Android guest may not enumerate by itself; run
`echo 1 > /sys/bus/pci/rescan` as root in the guest and configure the new
interface as above.

## What does not work

Android VMs cannot be saved: `vm save` refuses them, and `ns save` records
their configuration only, so a restored namespace relaunches the emulator
fresh, without your guest-side network setup. The web console streams video
only, without audio, clipboard, or touch events, at roughly 10 to 30 frames
per second depending on resolution. If the console shows *Connecting...*
forever, check that `vm info` shows a gRPC port and the `RUNNING` state; if
the emulator never reaches `RUNNING`, read `android-emulator.log` in the
VM's instance directory. [Android VMs](../../articles/android.md#troubleshooting)
lists more symptoms.

## The script

The script rebuilds the sandwich and adds `phone0`. Adjust the SDK path and
AVD name to yours.

```minimega title="18-android.mm"
--8<-- "training/miniclass/scripts/18-android.mm"
```

[Download this example](scripts/18-android.mm){ download="18-android.mm" }

## What you built

- An Android emulator VM launched and managed by minimega like a KVM guest,
  with auto-assigned console, ADB, and gRPC ports.
- A browser console for it in miniweb with phone buttons, rotation, and GPS.
- A guest interface on `net_left` configured over `adb` and reachable from
  `vm_left`.

## Where to read more

- [Android VMs](../../articles/android.md): prerequisites, port assignment,
  the console architecture and API routes, limitations, and troubleshooting.
- [Virtual machine types](../../articles/vmtypes.md#android-virtual-machines)
- [VM configuration reference](../../articles/vm-config-reference.md#android)
- [miniweb](../../articles/miniweb.md)
- Reference: [`vm launch`](../../reference/minimega.md#vm-launch),
  [`vm config android-avd`](../../reference/minimega.md#vm-config-android-avd),
  [`vm config android-sdk`](../../reference/minimega.md#vm-config-android-sdk),
  [`vm config android-console-base-port`](../../reference/minimega.md#vm-config-android-console-base-port),
  [`host`](../../reference/minimega.md#host).

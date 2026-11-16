# Chapter 18: Android VMs

In this chapter you add an Android emulator instance to the router sandwich as a
third kind of VM. You watch minimega assign its console, ADB, and gRPC ports,
and you open the VM in the miniweb console. You give the VM an address on
`net_left`, so that the KVM clients can reach it. At the end, `phone0` runs
beside `vm-left` and `vm-right`. You control it from the browser and from `adb`.

It assumes the sandwich from [Chapter 3](03-router-sandwich.md), miniweb from
[Chapter 5](05-miniweb.md), and the netspec from [Chapter 6](06-networking.md).
The Android SDK is a large download, and an Android Virtual Device (AVD) takes
time to create. If you teach this chapter, do the
[preparation](#prepare-the-host) before class.

## Prepare the host

Android VMs run the official Android emulator, which is itself a QEMU
build, under KVM. Each host that will run one needs:

- The `kvm` kernel module, as for any KVM guest.
- An Android SDK with the `emulator` and `platform-tools` packages. The packages
  provide the `emulator` and `adb` binaries.
- An Android Virtual Device (AVD), the emulator's description of a phone:
  system image, screen, RAM.

The command line tools of the SDK are available from
[developer.android.com](https://developer.android.com/studio#command-line-tools-only).
With the tools installed under `/opt/android-sdk`, you create a system image and
an AVD with `sdkmanager` and `avdmanager`:

```bash
$ export ANDROID_HOME=/opt/android-sdk
$ export PATH=$PATH:$ANDROID_HOME/cmdline-tools/latest/bin
$ sdkmanager "platform-tools" "emulator" "system-images;android-35;google_apis;x86_64"
$ avdmanager create avd -n Pixel_9a -k "system-images;android-35;google_apis;x86_64"
```

Use `sdkmanager --list` to see which system images your SDK offers. Pick a
`google_apis` image, not a `google_apis_playstore` image. The `google_apis`
image is a debuggable build that allows `adb root`. You need `adb root` to
configure the guest network below.

The AVD lands under `~/.android/avd/` for the user that created it. minimega
runs as root, so create the AVD as root, or point `vm config android-avd-dir` at
the directory that holds `Pixel_9a.avd`.

## Configure and launch

Only one field is required, `android-avd`. minimega finds the `emulator` and
`adb` binaries on the `PATH`, unless `android-emulator` and `android-adb` give
their paths. The SDK in this chapter is not on the `PATH`, so the example sets
both fields:

| Field | Purpose | Default |
|---|---|---|
| `android-avd` | AVD name to boot | required |
| `android-sdk` | SDK root on the host | none |
| `android-emulator`, `android-adb` | emulator and adb binaries, as names on `PATH` or absolute paths | `emulator`, `adb` |
| `android-avd-dir` | directory holding `<name>.avd` | the emulator's default |
| `android-no-window` | suppress the emulator's own window | `true` |
| `android-console-base-port`, `android-grpc-base-port` | preferred starting ports. `0` means start at the bottom of the range | `0` |
| `android-extra-args` | raw arguments appended to the emulator command line | none |
| `android-writable-system` | writable `/system` partition | `false` |

With the sandwich running in namespace `sandwich`:

```minimega
minimega$ namespace sandwich
minimega$ clear vm config
minimega$ vm config android-sdk /opt/android-sdk
minimega$ vm config android-emulator /opt/android-sdk/emulator/emulator
minimega$ vm config android-adb /opt/android-sdk/platform-tools/adb
minimega$ vm config android-avd Pixel_9a
minimega$ vm config net net_left
minimega$ vm launch android phone0
minimega$ vm start phone0
```

`clear vm config` drops the KVM fields from Chapter 3, so the template holds
only what this VM needs. The launch also works without this command, because
minimega ignores fields that do not apply to the launched type. The emulator
takes a minute or more to boot the first time. minimega monitors it over QMP
like a KVM VM and reports the ports that it reserved:

```minimega
minimega$ .columns name,state,type,android_avd,android_console_port,android_adb_port,android_serial,android_grpc_port .filter type=android vm info
host  | name   | state   | type    | android_avd | android_console_port | android_adb_port | android_serial | android_grpc_port
node1 | phone0 | RUNNING | android | Pixel_9a    | 5554                 | 5555             | emulator-5554  | 8554
```

Each Android VM takes one console/ADB port pair from 5554–5681 and one gRPC port
from 8554–8617. minimega scans forward from the base port until it finds a free
port. A host can therefore run 64 Android VMs at once. `host androidvms` shows
how many are running. The `android_serial` column is the name that `adb` uses.
The `android-adb` field sets the path only for minimega, so put `adb` on the
`PATH` of your shell first:

```bash
$ export PATH=$PATH:/opt/android-sdk/platform-tools
$ adb -s emulator-5554 shell getprop ro.build.version.release
```

## The web console

miniweb serves a console for the VM at `/vm/phone0/connect/`, the same path that
you used for VNC in Chapter 5. The console streams the display of the emulator
as PNG frames over a WebSocket and maps mouse and keyboard input. It also adds
what a phone has and a PC does not. These are Back, Home, Recents, Power, and
volume buttons, a landscape toggle, and a GPS panel with presets and custom
coordinates. All of it goes through the gRPC connection of miniweb to the
emulator. The browser therefore never needs to reach the ports of the emulator
itself.

Screenshots come from the emulator over gRPC, not from QEMU. The `vm screenshot`
command therefore does not support Android VMs yet and returns an error. Use
miniweb instead. `/vm/phone0/screenshot.png?size=300` serves a scaled
screenshot. The tile view uses this screenshot.

## Networking

`vm config net net_left` gave the VM a tap on the left network of the
sandwich. The QEMU of the emulator does not support the default `e1000` device,
so minimega attaches the tap with `virtio-net-pci`. The guest sees the tap as an
extra interface, `eth1`, with no address. The `eth0` of the emulator is its
built-in user-mode network. It has nothing to do with your experiment.

minimega builds only the host side. It does not run a DHCP client, set routes,
or touch the guest firewall. An Android image has no miniccc and no standard
place to do that. Configure the interface over `adb`:

```bash
$ adb -s emulator-5554 root
$ adb -s emulator-5554 shell ip link set eth1 up
$ adb -s emulator-5554 shell ip addr add 10.0.0.50/24 dev eth1
$ adb -s emulator-5554 shell ip addr show eth1
```

The system image decides whether `adb root` is allowed. Google Play images
refuse it. After you set the address, `vm-left` can reach the phone through
`cc`:

```minimega
minimega$ cc filter name=vm-left
minimega$ cc exec ping -c 3 10.0.0.50
minimega$ cc responses all
minimega$ clear cc filter
```

`vm net connect` and `vm net disconnect` move or detach the host-side tap. The
guest notices only a link change. `vm net add` hot-adds a device that the
Android guest can fail to enumerate by itself. In that case, run
`echo 1 > /sys/bus/pci/rescan` as root in the guest. Then configure the new
interface as above.

## What does not work

You cannot save Android VMs. `vm save` refuses them, and `ns save` records their
configuration only. A restored namespace therefore relaunches the emulator
fresh, without your guest-side network setup. The web console streams video
only, without audio, clipboard, or touch events. It streams at roughly 10 to 30
frames per second. The rate depends on resolution.

If the console shows *Connecting...* forever, make sure that `vm info` shows a
gRPC port and the `RUNNING` state. If the emulator never reaches `RUNNING`, read
`android-emulator.log` in the instance directory of the VM.
[Android VMs](../../articles/android.md#troubleshooting) lists more symptoms.

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
  `vm-left`.

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

# Chapter 18: Android VMs
# Rebuilds the router sandwich from chapter 3 and adds an Android emulator
# VM on net_left. Needs the chapter 2 images plus an Android SDK at
# /opt/android-sdk with an AVD named Pixel_9a (see chapter 18).
clear namespace sandwich
namespace sandwich

# The sandwich, as in chapter 3
vm config kernel miniccc.kernel
vm config initrd miniccc.initrd
vm config memory 2048
vm config networks net_left
vm launch kvm vm_left
vm config networks net_right
vm launch kvm vm_right
vm config kernel minirouter.kernel
vm config initrd minirouter.initrd
vm config networks net_left net_right
vm launch kvm router
router router interface 0 10.0.0.1/24
router router dhcp 10.0.0.0 range 10.0.0.2 10.0.0.254
router router dhcp 10.0.0.0 router 10.0.0.1
router router interface 1 10.0.1.1/24
router router dhcp 10.0.1.0 range 10.0.1.2 10.0.1.254
router router dhcp 10.0.1.0 router 10.0.1.1
router router commit
vm start router
shell sleep 10
vm start all

# The Android VM: only android-avd is required; android-sdk tells minimega
# where the emulator and adb binaries live
clear vm config
vm config android-sdk /opt/android-sdk
vm config android-avd Pixel_9a
vm config networks net_left
vm launch android phone0
vm start phone0

# The emulator takes a while to boot
shell sleep 60
.columns name,state,type,android_avd,android_console_port,android_adb_port,android_serial,android_grpc_port .filter type=android vm info
host androidvms
.columns name,type,state,vlan,ip vm info

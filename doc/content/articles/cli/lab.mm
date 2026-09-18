# lab.mm: a small lab that any host can reproduce.
# Set IMAGES first, for example: .env IMAGES /srv/images

# Base configuration shared by every VM.
vm config memory 1024
vm config vcpus 1

# The router gets a second interface toward the outside.
vm config disks $IMAGES/router.qc2
vm config networks LAN WAN
vm launch kvm router

# Two clients on the LAN.
vm config disks $IMAGES/client.qc2
vm config networks LAN
vm launch kvm client[0-1]

vm start all
echo lab launched, check state with vm info

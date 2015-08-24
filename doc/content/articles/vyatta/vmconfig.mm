# Use the Vyatta ISO
vm config cdrom vyatta.iso
# Two network interfaces
vm config net 100 200

# Tell QEMU to use that image as a floppy drive
vm config qemu-append -fda /tmp/minimega/files/router.img

# Launch your VM
vm launch kvm router noblock
vm start router

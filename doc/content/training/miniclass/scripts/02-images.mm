# 02-images.mm: boot one VM from each image pair built in chapter 2 and
# confirm that the miniccc agent inside each guest connects back to minimega.
# Relative image paths resolve inside the files directory, /tmp/minimega/files.
clear namespace sandwich
namespace sandwich

# a client from the miniccc image; 3072 MB because the image unpacks into RAM
vm config kernel miniccc.kernel
vm config initrd miniccc.initrd
vm config memory 3072
vm launch kvm client-test

# a router from the minirouter image
vm config kernel minirouter.kernel
vm config initrd minirouter.initrd
vm launch kvm router-test

vm start all
# wait until both agents have connected (about 90 s)
shell timeout 300 sh -c "until minimega -e .annotate false .columns cc_active,ip namespace sandwich vm info 2>/dev/null | grep -c 'true' | grep -qx 2; do sleep 3; done"
.annotate false .columns name,state,cc_active vm info

# tear the test VMs down again; chapter 3 builds the real experiment
vm kill all
vm flush

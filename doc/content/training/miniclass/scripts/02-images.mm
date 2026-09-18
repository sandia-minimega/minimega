# 02-images.mm: boot one VM from each image pair built in chapter 2 and
# confirm that the miniccc agent inside each guest connects back to minimega.
# Relative image paths resolve inside the files directory, /tmp/minimega/files.
clear namespace sandwich
namespace sandwich

# a client from the miniccc image
vm config kernel miniccc.kernel
vm config initrd miniccc.initrd
vm launch kvm client_test

# a router from the minirouter image
vm config kernel minirouter.kernel
vm config initrd minirouter.initrd
vm launch kvm router_test

vm start all
# give the guests time to boot and for miniccc to connect
shell sleep 30
.annotate false .columns name,state,cc_active vm info

# tear the test VMs down again; chapter 3 builds the real experiment
vm kill all
vm flush

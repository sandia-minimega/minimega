# create a TCP tunnel into a VM, even though it has no network!
cc tunnel foo 4444 127.0.0.1 9000
cc background nc -p 9000 -l -e /bin/sh

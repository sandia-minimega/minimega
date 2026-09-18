# forward tunnel: port 4444 on the host running VM foo is tunnelled to
# 127.0.0.1:9000 inside foo, even if foo has no experiment network
cc tunnel foo 4444 127.0.0.1 9000
cc background nc -p 9000 -l -e /bin/sh

# list and close forward tunnels; ids start at 1
cc tunnel list foo
cc tunnel close foo 1

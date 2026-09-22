# start a web server inside the VM, on port 80
cc filter name=webserver
cc send file:protonuke
cc background /tmp/miniccc/files/protonuke -serve -http

# forward tunnel: port 4444 on the host that runs webserver reaches
# 127.0.0.1:80 inside it, even though the VM has no route to that host
cc tunnel webserver 4444 127.0.0.1 80

# use the tunnel from that host: this fetches the page protonuke serves
shell curl -s http://127.0.0.1:4444/

# list and close forward tunnels; ids start at 1
cc tunnel list webserver
cc tunnel close webserver 1
clear cc filter

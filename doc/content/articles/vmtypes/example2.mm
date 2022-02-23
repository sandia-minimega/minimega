#Configure RKVM for vnc endpoint
vm config vnc_host 192.168.1.201
vm config vnc_port 5900

#launch 
vm launch rkvm foo 

#configure vnc endpoint using another VNC server
vm config vnc_host 192.168.1.202
vm config vnc_port 5900

#launch 
vm launch rkvm bar

#start 
vm start foo
vm start bar

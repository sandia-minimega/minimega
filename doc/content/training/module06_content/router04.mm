router foo dhcp 10.0.0.0 range 10.0.0.2 10.0.0.254
# You can also specify static IP assignments with a MAC/IP address pair:
router foo dhcp 10.0.0.0 static 00:11:22:33:44:55 10.0.0.100
# Additionally, you can specify the default gateway and nameserver:
router foo dhcp 10.0.0.0 router 10.0.0.254
router foo dhcp 10.0.0.0 dns 8.8.8.8

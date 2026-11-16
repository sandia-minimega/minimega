# Module 38 - Guacamole VNC Management

!!! warning "Historical miniclass"
    This lesson was recovered from the former Sandia minimega site. It is preserved for reference and may describe obsolete software, operating systems, commands, or external resources. Consult the [current documentation](../../index.md) before applying it.

[View the archived source URL](https://www.sandia.gov/minimega/module-38-guacamole-vnc-management/).

## Introduction

When you provide access to miniweb on port 9001 you provide a noVNC session to all the VMs.

Using Apache Guacamole we can provide users with access to only select VMs.

`iptables` can then be used to block access from all hosts besides the Guacamole and administrator IP addresses.

## Installing Guacamole

[www.chasewright.com/guacamole-with-mysql-on-ubuntu/](https://www.chasewright.com/guacamole-with-mysql-on-ubuntu/)

**WARNING:** It should be noted that serious vulnerabilities in the Guacamole software have been discovered in older versions prior to 2020; ensure that software is up-to-date before installing in production.

```
wget https://raw.githubusercontent.com/MysticRyuujin/guac-install/master/guac-install.sh
chmod +x guac-install.sh
apt-get update
apt-get -y install dos2unix
dos2unix guac-install.sh
./guac-install.sh
<type in a mysql password>
<type in a Guacamole db password>
```

### Starting VMs

```
vm kill all
vm flush
vm config cdrom /home/ubuntu/tinycore.iso
vm config memory 128
vm launch kvm lin[1-3]
vm start all
```

### Getting vnc_ports

```
$ .columns name,vnc_port vm info
host | name | vnc_port
m3   | lin1 | 36357
m3   | lin2 | 35437
m3   | lin3 | 41256
```

## Configuring user access

Be careful not to mix spaces with tabs when creating this file.

`nano /etc/guacamole/user-mapping.xml`

```
<user-mapping>
<authorize username="a" password="a">
 <connection name="lin1">
 <protocol>vnc</protocol>
 <param name="hostname">192.168.1.100</param>
 <param name="port">36357</param>
 </connection>
 <connection name="lin2">
 <protocol>vnc</protocol>
 <param name="hostname">192.168.1.100</param>
 <param name="port">35437</param>
 </connection>
</authorize>
<authorize username="b" password="b">
 <connection name="lin3">
 <protocol>vnc</protocol>
 <param name="hostname">192.168.1.100</param>
 <param name="port">41256</param>
 </connection>
</authorize>
</user-mapping>
```

When the file is saved its changes are immediately effective.

### Access the website from your browser

```
http://<guacamoleip>:8080/guacamole
```

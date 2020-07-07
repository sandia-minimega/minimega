#!/bin/bash

echo "\"node_map\":"
echo "{"

for i in `seq $2 $3`
do
	ping -c 1 ${1}${i} > /dev/null
	eth=`ssh ${1}${i} ip link show $4 | grep ether | awk '{print $2}' | awk -F: '{print $1$2 "." $3$4 "." $5$6}'`
	port=`ssh -T arista <<EOF
enable
show mac address-table | grep ${eth}
EOF

`

port=`echo $port | awk '{print $4}'`

echo "\"${1}${i}\": \"${port}\","

done

echo "},"


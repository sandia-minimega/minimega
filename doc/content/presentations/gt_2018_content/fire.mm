clear namespace wan

vm config filesystem /root/containerfs

# machines in the machine room
vm launch container server[1-10]

# sensors in the machine room
vm launch container sensor_room_[1-4]

# sensors in the hallway
vm launch container sensor_hall_[1-3]

# launch faulty sensors
vm launch container sensor_faulty_[1-2]

vm start all

# setup vias
pipe Troom via bin/normal -stddev 5.0
pipe Thall via bin/normal -stddev 5.0

# connect the hall to the room temperature
plumb Troom "bin/mult .66" Thall

# launch room sensors
cc filter hostname=sensor_room_1
cc exec stdin=Troom stdout=sensor_room_1 /sensor
cc filter hostname=sensor_room_2
cc exec stdin=Troom stdout=sensor_room_2 /sensor
cc filter hostname=sensor_room_3
cc exec stdin=Troom stdout=sensor_room_3 /sensor
cc filter hostname=sensor_room_4
cc exec stdin=Troom stdout=sensor_room_4 /sensor

# launch hall sensors
cc filter hostname=sensor_hall_1
cc exec stdin=Thall stdout=sensor_hall_1 /sensor
cc filter hostname=sensor_hall_2
cc exec stdin=Thall stdout=sensor_hall_2 /sensor
cc filter hostname=sensor_hall_3
cc exec stdin=Thall stdout=sensor_hall_3 /sensor

# connect critical thresholds
plumb Troom "bin/crit 100.0"

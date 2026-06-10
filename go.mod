module github.com/sandia-minimega/minimega/v2

go 1.24.0

require (
	github.com/Harvey-OS/ninep v0.0.0-20200724082702-d30a6d4f9789
	github.com/anthonynsimon/bild v0.14.0
	github.com/c9s/goprocinfo v0.0.0-20210130143923-c95fcf8c64a8
	github.com/dutchcoders/goftp v0.0.0-20170301105846-ed59a591ce14
	github.com/fsnotify/fsnotify v1.9.0
	github.com/goftp/server v0.0.0-20200708154336-f64f7c2d8a42
	github.com/google/gopacket v1.1.19
	github.com/jbuchbinder/gopnm v0.0.0-20251119211316-bb594e0d2e34
	github.com/kr/pty v1.1.8
	github.com/miekg/dns v1.1.72
	github.com/nfnt/resize v0.0.0-20180221191011-83c6a9932646
	github.com/peterh/liner v1.2.2
	github.com/stargrave/goircd v0.0.0-00010101000000-000000000000
	github.com/thoj/go-ircevent v0.0.0-20210723090443-73e444401d64
	github.com/twmb/murmur3 v1.1.8
	github.com/ziutek/telnet v0.1.0
	golang.org/x/crypto v0.48.0
	golang.org/x/net v0.50.0
	golang.org/x/sys v0.41.0
	golang.org/x/tools v0.42.0
)

require (
	github.com/clipperhouse/uax29/v2 v2.7.0 // indirect
	github.com/creack/pty v1.1.24 // indirect
	github.com/mattn/go-runewidth v0.0.20 // indirect
	golang.org/x/mod v0.33.0 // indirect
	golang.org/x/sync v0.19.0 // indirect
	golang.org/x/term v0.40.0 // indirect
)

replace github.com/Harvey-OS/ninep => github.com/jcrussell/ninep v0.0.0-20180619175724-35ad2879c0a3

replace github.com/dutchcoders/goftp => ./packages/github.com/dutchcoders/goftp

replace github.com/goftp/server => ./packages/github.com/goftp/server

replace github.com/stargrave/goircd => ./packages/github.com/stargrave/goircd

replace github.com/thoj/go-ircevent => ./packages/github.com/thoj/go-ircevent

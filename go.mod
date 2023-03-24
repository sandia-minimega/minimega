module github.com/sandia-minimega/minimega/v2

go 1.13

require (
	github.com/Harvey-OS/ninep v0.0.0-00010101000000-000000000000
	github.com/anthonynsimon/bild v0.10.1-0.20190311092716-e21126554192
	github.com/c9s/goprocinfo v0.0.0-20151025191153-19cb9f127a9c
	github.com/dutchcoders/goftp v0.0.0-00010101000000-000000000000
	github.com/fsnotify/fsnotify v1.5.5-0.20220826001856-69c24b069553
	github.com/goftp/server v0.0.0-00010101000000-000000000000
	github.com/google/gopacket v1.1.18-0.20190711070436-ce2e696dc0c9
	github.com/jbuchbinder/gopnm v0.0.0-20150223212718-5176c556b9ce
	github.com/kr/pty v0.0.0-20160716204620-ce7fa45920dc
	github.com/miekg/dns v1.1.25
	github.com/nfnt/resize v0.0.0-20160724205520-891127d8d1b5
	github.com/peterh/liner v1.0.1-0.20170317030525-88609521dc4b
	github.com/stargrave/goircd v0.0.0-00010101000000-000000000000
	github.com/thoj/go-ircevent v0.0.0-00010101000000-000000000000
	github.com/twmb/murmur3 v1.1.6
	github.com/ziutek/telnet v0.0.0-20150427115447-49d9be70897f
	golang.org/x/crypto v0.0.0-20210921155107-089bfa567519
	golang.org/x/net v0.0.0-20210226172049-e18ecbb05110
	golang.org/x/sys v0.0.0-20220825204002-c680a09ffe64
	golang.org/x/tools v0.0.0-20190907020128-2ca718005c18
)

replace github.com/Harvey-OS/ninep => github.com/jcrussell/ninep v0.0.0-20180619175724-35ad2879c0a3

replace github.com/dutchcoders/goftp => ./packages/github.com/dutchcoders/goftp

replace github.com/goftp/server => ./packages/github.com/goftp/server

replace github.com/stargrave/goircd => ./packages/github.com/stargrave/goircd

replace github.com/thoj/go-ircevent => ./packages/github.com/thoj/go-ircevent

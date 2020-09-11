module github.com/sandia-minimega/minimega

go 1.13

require (
	github.com/Harvey-OS/ninep v0.0.0-20200701230449-5148a8f50134
	github.com/anthonynsimon/bild v0.12.0
	github.com/c9s/goprocinfo v0.0.0-20200311234719-5750cbd54a3b
	github.com/dutchcoders/goftp v0.0.0-20170301105846-ed59a591ce14
	github.com/goftp/file-driver v0.0.0-20180502053751-5d604a0fc0c9 // indirect
	github.com/goftp/server v0.0.0-20190712054601-1149070ae46b
	github.com/google/gopacket v1.1.17
	github.com/jbuchbinder/gopnm v0.0.0-20150223212718-5176c556b9ce
	github.com/jlaffaye/ftp v0.0.0-20200602180915-5563613968bf // indirect
	github.com/kr/pty v1.1.8
	github.com/miekg/dns v1.1.30
	github.com/nfnt/resize v0.0.0-20180221191011-83c6a9932646
	github.com/peterh/liner v1.2.0
	github.com/ziutek/telnet v0.0.0-20180329124119-c3b780dc415b
	golang.org/x/crypto v0.0.0-20200622213623-75b288015ac9
	golang.org/x/net v0.0.0-20200707034311-ab3426394381
)

replace github.com/Harvey-OS/ninep => ./packages/github.com/Harvey-OS/ninep

replace github.com/anthonynsimon/bild => ./packages/github.com/anthonynsimon/bild

replace github.com/c9s/goprocinfo => ./packages/github.com/c9s/goprocinfo

replace github.com/dutchcoders/goftp => ./packages/github.com/dutchcoders/goftp

replace github.com/goftp/server => ./packages/github.com/goftp/server

replace github.com/google/gopacket => ./packages/github.com/google/gopacket

replace github.com/jbuchbinder/gopnm => ./packages/github.com/jbuchbinder/gopnm

replace github.com/kr/pty => ./packages/github.com/kr/pty

replace github.com/miekg/dns => ./packages/github.com/miekg/dns

replace github.com/nfnt/resize => ./packages/github.com/nfnt/resize

replace github.com/peterh/liner => ./packages/github.com/peterh/liner

replace github.com/ziutek/telnet => ./packages/github.com/ziutek/telnet


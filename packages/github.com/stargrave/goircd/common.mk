LDFLAGS = -X main.version=$(VERSION)

goircd: *.go
	go build -ldflags "$(LDFLAGS)"

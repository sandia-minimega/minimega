all: build

GOPATH := $(shell pwd)

clean:
	+@echo "cleaning minimega"
	+@rm -rf bin
	+@rm -rf pkg


build:
	+@echo "building minimega"
	+@export GOBIN=; export GOPATH=${GOPATH}; go install minimega

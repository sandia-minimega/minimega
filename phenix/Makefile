SHELL := /bin/bash

all:

clean:
	$(RM) bin/phenix
	$(MAKE) -C src/go clean
	$(MAKE) -C src/js clean

bin/phenix:
	$(MAKE) -C src/js dist/index.html
	cp -a src/js/dist/* src/go/web/public
	$(MAKE) -C src/go bin/phenix
	mkdir -p bin
	cp src/go/bin/phenix bin/phenix

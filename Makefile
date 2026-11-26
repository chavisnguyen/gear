BINARY := gear
PREFIX ?= $(HOME)/.local

.PHONY: all build install test clean help

all: build

build:
	mkdir -p bin
	go build -ldflags="-s -w" -o bin/$(BINARY) .

install: build
	mkdir -p $(PREFIX)/bin
	cp bin/$(BINARY) $(PREFIX)/bin/$(BINARY)

test:
	go test -v -count=1 ./...

clean:
	rm -rf bin dist $(BINARY)

help:
	@echo "Targets: build | install | test | clean"

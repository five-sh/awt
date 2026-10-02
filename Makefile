BINARY      := awt
INSTALL_DIR := $(HOME)/.local/bin
VERSION     := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

.PHONY: build install test test-tmux fmt fmt-fix vet clean check check-all

build:
	go build -ldflags "-X main.version=$(VERSION)" -o $(BINARY) .

install: build
	mkdir -p $(INSTALL_DIR)
	install -m 0755 $(BINARY) $(INSTALL_DIR)/$(BINARY)

test:
	go test ./...

# Integration tests too: they need a real tmux, and run on a private socket.
test-tmux:
	go test -tags tmux -count=1 ./...

fmt:
	@test -z "$$(gofmt -l .)" || (gofmt -l .; exit 1)

fmt-fix:
	gofmt -w .

vet:
	go vet ./...

clean:
	rm -f $(BINARY)

check: fmt vet test build

check-all: fmt vet test-tmux build

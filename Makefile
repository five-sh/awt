BINARY      := awt
INSTALL_DIR := $(HOME)/.local/bin

.PHONY: build install test fmt fmt-fix vet clean check

build:
	go build -o $(BINARY) .

install: build
	mkdir -p $(INSTALL_DIR)
	install -m 0755 $(BINARY) $(INSTALL_DIR)/$(BINARY)

test:
	go test ./...

fmt:
	@test -z "$$(gofmt -l .)" || (gofmt -l .; exit 1)

fmt-fix:
	gofmt -w .

vet:
	go vet ./...

clean:
	rm -f $(BINARY)

check: fmt vet test build

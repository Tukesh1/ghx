VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(shell git rev-parse --short HEAD 2>/dev/null)

.PHONY: build install test check demo snapshot clean

build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o ghx .

install:
	go install -trimpath -ldflags "$(LDFLAGS)" .

test:
	go test ./...

# vet + unit tests + render every view at 80x24, 120x36 and 160x48 and verify exact frame sizes.
check: build
	go vet ./...
	go test ./...
	./ghx --demo --check

demo: build
	./ghx --demo

# Print one frame, e.g. `make snapshot TAB=6 SIZE=160x48` (TAB 6 = repository insights).
TAB ?= 1
SIZE ?= 120x36
snapshot: build
	./ghx --demo --snapshot --tab $(TAB) --size $(SIZE)

clean:
	rm -rf ghx dist

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(shell git rev-parse --short HEAD 2>/dev/null)

.PHONY: build install run demo test race check lint fmt snapshot screenshots clean

build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o ghx .

install:
	go install -trimpath -ldflags "$(LDFLAGS)" .

run: build
	./ghx

demo: build
	./ghx --demo

test:
	go test ./...

race:
	go test -race ./...

# gofmt + vet + race tests + render every view at 80x24, 120x36 and 160x48 and verify exact frame sizes.
check: build
	@test -z "$$(gofmt -l .)" || (echo "gofmt needed:"; gofmt -l .; exit 1)
	go vet ./...
	go test -race ./...
	./ghx --demo --check

lint:
	golangci-lint run

fmt:
	gofmt -w .

# Print one frame, e.g. `make snapshot TAB=7 SIZE=160x48` (TAB 7 = repository insights).
TAB ?= 1
SIZE ?= 120x36
snapshot: build
	./ghx --demo --snapshot --tab $(TAB) --size $(SIZE)

# Regenerate the README screenshots from the demo dataset.
screenshots:
	go run ./tools/screenshot -out docs/screenshots

clean:
	rm -rf ghx dist

PREFIX     ?= /usr/local
BINDIR     := $(PREFIX)/bin
MANDIR     := $(PREFIX)/share/man/man1
BUILDDIR   := build
BIN        := $(BUILDDIR)/yoro

MODULE     := github.com/zackb/yoro
VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT     ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE       ?= $(shell date -u +%Y-%m-%d)

LDFLAGS := -s -w \
	-X $(MODULE)/internal/version.Version=$(VERSION) \
	-X $(MODULE)/internal/version.Commit=$(COMMIT) \
	-X $(MODULE)/internal/version.Date=$(DATE)

.PHONY: default
default: build

.PHONY: build
build:
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/yoro

.PHONY: run
run: build
	$(BIN)

.PHONY: test
test:
	go test ./...

.PHONY: lint
lint:
	gofmt -l .
	go vet ./...

.PHONY: fmt
fmt:
	gofmt -w .

.PHONY: install
install: build
	install -Dm755 $(BIN) $(DESTDIR)$(BINDIR)/yoro
	install -Dm644 man/yoro.1 $(DESTDIR)$(MANDIR)/yoro.1
	install -Dm644 LICENSE $(DESTDIR)$(PREFIX)/share/licenses/yoro/LICENSE

.PHONY: clean
clean:
	rm -rf $(BUILDDIR)

BIN := sinty-hostname
DAEMON := sinty-hostnamed
PREFIX ?= /usr

.PHONY: all build static test vet install clean

all: build

build:
	go build -o $(BIN) ./cmd/sinty-hostname
	go build -o $(DAEMON) ./cmd/sinty-hostnamed

static:
	CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o $(BIN) ./cmd/sinty-hostname
	CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o $(DAEMON) ./cmd/sinty-hostnamed

test:
	go test ./...

vet:
	go vet ./...

install: static
	install -Dm755 $(BIN) $(DESTDIR)$(PREFIX)/bin/$(BIN)
	install -Dm755 $(DAEMON) $(DESTDIR)$(PREFIX)/libexec/$(DAEMON)
	install -Dm644 packaging/org.freedesktop.hostname1.conf $(DESTDIR)$(PREFIX)/share/dbus-1/system.d/org.freedesktop.hostname1.conf
	install -Dm644 packaging/org.freedesktop.hostname1.service $(DESTDIR)$(PREFIX)/share/dbus-1/system-services/org.freedesktop.hostname1.service
	install -Dm644 packaging/sinty-hostnamed.service $(DESTDIR)$(PREFIX)/lib/systemd/system/sinty-hostnamed.service

clean:
	rm -f $(BIN) $(DAEMON)

PREFIX  ?= /usr/local
SBINDIR  = $(PREFIX)/sbin
RCDIR    = $(PREFIX)/etc/rc.d
CONFDIR  = $(PREFIX)/etc/fansd
MANDIR   = $(PREFIX)/man/man8

BINARY   = fansd

.PHONY: all build install uninstall clean fmt vet

all: build

build:
	# -buildvcs=false: allows building as root in a user-owned checkout,
	# where git's ownership check would otherwise fail VCS stamping
	go build -buildvcs=false -o $(BINARY) .

install: build
	install -d $(DESTDIR)$(SBINDIR) $(DESTDIR)$(RCDIR) \
	           $(DESTDIR)$(CONFDIR) $(DESTDIR)$(MANDIR)
	install -m 755 $(BINARY)       $(DESTDIR)$(SBINDIR)/$(BINARY)
	install -m 555 rc.d/fansd      $(DESTDIR)$(RCDIR)/$(BINARY)
	install -m 444 man/fansd.8     $(DESTDIR)$(MANDIR)/$(BINARY).8
	@if [ ! -f $(DESTDIR)$(CONFDIR)/fansd.toml ]; then \
	    install -m 640 fansd.toml.example $(DESTDIR)$(CONFDIR)/fansd.toml.example; \
	    echo ""; \
	    echo "No config found. Generate one with:"; \
	    echo "  $(SBINDIR)/$(BINARY) -init-config $(CONFDIR)/fansd.toml"; \
	fi

uninstall:
	rm -f $(DESTDIR)$(SBINDIR)/$(BINARY)
	rm -f $(DESTDIR)$(RCDIR)/$(BINARY)
	rm -f $(DESTDIR)$(MANDIR)/$(BINARY).8
	rm -f $(DESTDIR)$(CONFDIR)/fansd.toml.example
	@echo "Note: $(CONFDIR)/fansd.toml was not removed."

clean:
	rm -f $(BINARY)

fmt:
	gofmt -w .

vet:
	go vet ./...

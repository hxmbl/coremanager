.PHONY: build install uninstall clean

BINARY  := coremanager
ALIAS   := cm
PREFIX  ?= /usr/local
BINDIR  := $(PREFIX)/bin

build:
	go build -o $(BINARY) .

install: build
	mkdir -p $(DESTDIR)$(BINDIR)
	install -m 755 $(BINARY) $(DESTDIR)$(BINDIR)/$(BINARY)
	ln -sf $(BINARY) $(DESTDIR)$(BINDIR)/$(ALIAS)

uninstall:
	rm -f $(DESTDIR)$(BINDIR)/$(BINARY)
	rm -f $(DESTDIR)$(BINDIR)/$(ALIAS)

clean:
	rm -f $(BINARY)

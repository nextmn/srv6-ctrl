prefix = /usr/local
exec_prefix = $(prefix)
bindir = $(exec_prefix)/bin
BASHCOMPLETIONSDIR = $(exec_prefix)/share/bash-completion/completions


RM = rm -f
INSTALL = install -D

.PHONY: install uninstall update build clean default
default: build
build:
	go build
clean:
	go clean
reinstall: uninstall install
update:
	go get -u github.com/louisroyer/go-pfcp-networking@master
	go mod tidy
install:
	$(INSTALL) nextmn-srv6-ctrl $(DESTDIR)$(bindir)/nextmn-srv6-ctrl
	$(INSTALL) bash-completion/completions/nextmn-srv6-ctrl $(DESTDIR)$(BASHCOMPLETIONSDIR)/nextmn-srv6-ctrl
	@echo "================================="
	@echo ">> Now run the following command:"
	@echo "\tsource $(DESTDIR)$(BASHCOMPLETIONSDIR)/nextmn-srv6-ctrl"
	@echo "================================="
uninstall:
	$(RM) $(DESTDIR)$(bindir)/nextmn-srv6-ctrl
	$(RM) $(DESTDIR)$(BASHCOMPLETIONSDIR)/nextmn-srv6-ctrl

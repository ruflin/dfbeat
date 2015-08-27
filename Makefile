GODEP=$(GOPATH)/bin/godep
PREFIX?=/build

GOFILES = $(shell find . -type f -name '*.go')
dfbeat: $(GOFILES)
	# first make sure we have godep
	go get github.com/tools/godep
	$(GODEP) go build

.PHONY: getdeps
getdeps:
	go get -t -u -f


.PHONY: install_cfg
install_cfg:
	cp etc/dfbeat.yml $(PREFIX)/dfbeat-linux.yml
	cp etc/dfbeat.template.json $(PREFIX)/dfbeat.template.json
	# darwin
	cp etc/dfbeat.yml $(PREFIX)/dfbeat-darwin.yml
	# win
	cp etc/dfbeat.yml $(PREFIX)/dfbeat-win.yml

.PHONY: cover
cover:
	# gotestcover is needed to fetch coverage for multiple packages
	go get github.com/pierrre/gotestcover
	GOPATH=$(shell $(GODEP) path):$(GOPATH) $(GOPATH)/bin/gotestcover -coverprofile=profile.cov -covermode=count github.com/ruflin/dfbeat/...
	mkdir -p cover
	$(GODEP) go tool cover -html=profile.cov -o cover/coverage.html

.PHONY: clean
clean:
	-rm dfbeat

build:
	godep go build

run: build
	./dfbeat -c etc/dfbeat.dev.yml

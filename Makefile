BINARY  := wisee
VERSION ?= $(shell cat VERSION 2>/dev/null || echo dev)
LDFLAGS := -ldflags "-s -w -X main.version=$(VERSION)"

.PHONY: all build run demo test fmt vet check clean dist help

all: build

## build: compile the binary into bin/
build:
	go build $(LDFLAGS) -o bin/$(BINARY) .

## run: build and run with real scanning
run: build
	./bin/$(BINARY)

## demo: build and serve the sample network (no scanning)
demo: build
	./bin/$(BINARY) -demo

## test: run unit tests
test:
	go test ./...

fmt:
	gofmt -s -w .

vet:
	go vet ./...

## check: formatting, vet and tests (what CI runs)
check:
	@test -z "$$(gofmt -s -l .)" || { echo "gofmt needed:"; gofmt -s -l .; exit 1; }
	go vet ./...
	go test ./...

clean:
	rm -rf bin dist

## release build for one platform: make dist GOOS=linux GOARCH=amd64
dist:
	@mkdir -p dist
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) \
		go build $(LDFLAGS) -o dist/$(BINARY)-$(GOOS)-$(GOARCH)$(if $(filter windows,$(GOOS)),.exe,) .

help: ## show this help
	@grep -E '^## ' Makefile | sed 's/## //'

.PHONY: all build test

GIT_TAG := $(shell git describe --tags --exact-match 2>/dev/null)
GIT_BRANCH := $(shell git rev-parse --abbrev-ref HEAD 2>/dev/null)
GIT_SHA := $(shell git rev-parse --short=12 HEAD 2>/dev/null)
GIT_DATE := $(shell git show -s --format=%cd --date=format:%Y%m%d%H%M%S 2>/dev/null)
UNRELEASED_VERSION := $(if $(and $(GIT_SHA),$(GIT_DATE)),v0.0.0-$(GIT_DATE)-$(GIT_SHA),$(shell date -u +%Y%m%d%H%M%S)-dev)
VERSION := $(if $(GIT_TAG),$(GIT_TAG),$(if $(filter-out HEAD,$(GIT_BRANCH)),$(GIT_BRANCH),$(UNRELEASED_VERSION)))
LDFLAGS := -X github.com/cmmoran/swarmcp/cmd.Version=$(VERSION)

all: build

build: test
	go build -ldflags "$(LDFLAGS)" -o swarmcp .

test:
	go test ./...

install: build
	cp swarmcp $(HOME)/.local/bin

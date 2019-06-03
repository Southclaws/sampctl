-include .env
VERSION := $(shell git describe --tags --dirty --always)
LDFLAGS := -ldflags "-X main.version=$(VERSION) -X main.segmentKey=$(SEGMENT_KEY)"


# -
# Builds
# -

static:
	CGO_ENABLED=0 GOOS=linux go build -a $(LDFLAGS) -o sampctl .

fast:
	go build $(LDFLAGS) -o sampctl

install:
	go install $(LDFLAGS)

clean:
	-rm sampctl

# -
# Unit Tests
# -

test:
	go test -race -v ./src/versioning
	go test -race -v ./src/util
	go test -race -v ./src/download
	go test -race -v ./src/compiler
	go test -race -v ./src/runtime
	go test -race -v ./src/rook
	echo SUCCESS!


# -
# Release
# -

docs: fast
	./docgen.sh

dist:
	# for osx tar fix
	# https://github.com/goreleaser/goreleaser/issues/409
	PATH="/usr/local/opt/gnu-tar/libexec/gnubin:$(PATH)" \
	SEGMENT_KEY=$(SEGMENT_KEY) \
	GITHUB_TOKEN=$(GITHUB_TOKEN) \
	goreleaser \
		--snapshot \
		--rm-dist


# -
# Docker
# -

build:
	docker build -t southclaws/sampctl:$(VERSION) .

push: build
	docker push southclaws/sampctl:$(VERSION)


# -
# Test environments
# -

ubuntu:
	docker run \
		-it \
		-v$(shell pwd):/sampctl \
		ubuntu

centos:
	docker run \
		-it \
		-v$(shell pwd):/sampctl \
		centos

-include .env
VERSION := $(shell cat VERSION)
LDFLAGS := -ldflags "-X main.version=$(VERSION) -X main.segmentKey=$(SEGMENT_KEY)"

.PHONY: version


# -
# Builds
# -

fast:
	go build $(LDFLAGS) -o sampctl

static:
	CGO_ENABLED=0 GOOS=linux go build -a $(LDFLAGS) -o sampctl .

install:
	go install $(LDFLAGS)

clean:
	-rm sampctl


# -
# Linting
# -

lint-all:
	gometalinter \
		--deadline=10m \
		--vendor \
		--aggregate \
		--disable-all \
		--enable=interfacer \
		--enable=misspell \
		--enable=gas \
		--enable=gotype \
		--enable=megacheck \
		--enable=errcheck \
		--enable=safesql \
		--enable=vet \
		--enable=golint \
		--enable=structcheck \
		--enable=deadcode \
		--enable=vetshadow \
		--enable=unconvert \
		--enable=varcheck \
		./...

lint-fast:
	gometalinter \
		--vendor \
		--disable-all \
		--enable=gotype \
		--enable=vet \
		--enable=megacheck \
		./...

lint-revive:
	revive \
		--exclude vendor/... \
		--config=revive.toml


# -
# Unit Tests
# -

test:
	go test -race -v ./versioning
	go test -race -v ./util
	go test -race -v ./download
	go test -race -v ./compiler
	go test -race -v ./runtime
	go test -race -v ./rook
	echo SUCCESS!


# -
# Release
# -

version:
	git tag $(VERSION)
	git push
	git push origin $(VERSION)

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

build-dev: static
	docker build -t southclaws/sampctl:$(VERSION) -f Dockerfile.dev .

push: build
	docker push southclaws/sampctl:$(VERSION)

run:
	-docker rm sampctl-test
	docker run --name sampctl-test southclaws/sampctl:$(VERSION)

enter:
	docker run \
		-it \
		-v ~/.samp:/root/.samp \
		--entrypoint=bash \
		southclaws/sampctl:$(VERSION)

enter-mount:
	docker run \
		-v $(shell pwd)/testspace:/samp \
		-it --entrypoint=bash \
		--security-opt='seccomp=unconfined' \
		southclaws/sampctl:$(VERSION)


# -
# Test environments
# -

ubuntu-build:
	docker run \
		-it \
		-w /go/src/github.com/Southclaws/sampctl \
		-v$(shell pwd):/go/src/github.com/Southclaws/sampctl \
		golang

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

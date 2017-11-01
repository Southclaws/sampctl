VERSION := $(shell cat VERSION)
LDFLAGS := -ldflags "-X main.version=$(VERSION)"

.PHONY: version

test:
	go test -race -v -p 4 \
		./compiler \
		./download \
		./rook \
		./server \
		./settings \
		./util
fast:
	go build $(LDFLAGS) -o sampctl

static:
	CGO_ENABLED=0 GOOS=linux go build -a $(LDFLAGS) -o sampctl .

install:
	go install $(LDFLAGS)

version:
	git tag $(VERSION)
	git push
	git push origin $(VERSION)

clean:
	-rm sampctl

docs: fast
	./docgen.sh

# Docker

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
	docker run -it --entrypoint=bash southclaws/sampctl:$(VERSION)

enter-mount:
	docker run \
		-v $(shell pwd)/testspace:/samp \
		-it --entrypoint=bash \
		--security-opt='seccomp=unconfined' \
		southclaws/sampctl:$(VERSION)

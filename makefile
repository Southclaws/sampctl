VERSION := $(shell cat VERSION)
LDFLAGS := -ldflags "-X main.version=$(VERSION)"

.PHONY: version

fast:
	go build $(LDFLAGS) -o sampctl

install:
	go install $(LDFLAGS)

version:
	git tag $(VERSION)
	git push origin $(VERSION)

clean:
	-rm sampctl

# Docker

build:
	docker build -t southclaws/sampctl:$(VERSION) .

push: build
	docker push southclaws/sampctl:$(VERSION)
	
run:
	-docker rm sampctl-test
	docker run --name sampctl-test southclaws/sampctl:$(VERSION)

enter:
	docker run -it --entrypoint=bash southclaws/sampctl:$(VERSION)

enter-mount:
	docker run -v $(shell pwd)/testspace:/samp -it --entrypoint=bash southclaws/sampctl:$(VERSION)

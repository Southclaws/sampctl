VERSION := $(shell cat VERSION)

.PHONY: version

install:
	go install

fast:
	go build -o sampctl

version:
	git tag -a $(VERSION)
	git push origin $(VERSION)

static:
	CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o sampctl .

static_windows:
	CGO_ENABLED=0 GOOS=windows go build -a -installsuffix cgo -o sampctl.exe .

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

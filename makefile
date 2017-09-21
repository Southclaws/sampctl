VERSION := $(shell cat VERSION)


fast:
	go build -o sampctl

static:
	CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o sampctl .

build:
	docker build -t southclaws/sampctl .

run: build
	docker rm sampctl-test
	docker run --name sampctl-test southclaws/sampctl

enter: build
	docker run -it --entrypoint=bash southclaws/sampctl

clean:
	-rm sampctl

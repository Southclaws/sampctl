VERSION := $(shell cat VERSION)


install:
	go install

fast:
	go build -o sampctl

static:
	CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o sampctl .

build: static
	docker build -t southclaws/sampctl .

run:
	-docker rm sampctl-test
	docker run --name sampctl-test southclaws/sampctl

enter:
	docker run -it --entrypoint=bash southclaws/sampctl

clean:
	-rm sampctl

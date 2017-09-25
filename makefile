VERSION := $(shell cat VERSION)


install:
	go install

fast:
	go build -o sampctl

dist:
	gox -os="windows linux" -arch="386"

static:
	CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o sampctl .

static_windows:
	CGO_ENABLED=0 GOOS=windows go build -a -installsuffix cgo -o sampctl.exe .

clean:
	-rm sampctl

# Docker

build:
	docker build --no-cache -t southclaws/sampctl:$(VERSION) .

push: build
	docker push southclaws/sampctl:$(VERSION)
	
run:
	-docker rm sampctl-test
	docker run --name sampctl-test southclaws/sampctl:$(VERSION)

enter:
	docker run -it --entrypoint=bash southclaws/sampctl:$(VERSION)

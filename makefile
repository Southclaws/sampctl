VERSION := $(shell cat VERSION)


compile:
	go build -o sampctl

build:
	docker build -t southclaws/sampctl .

run:
	docker run --name sampctl-test southclaws/sampctl

enter:
	docker run -it --entrypoint=bash southclaws/sampctl

clean:
	-rm sampctl

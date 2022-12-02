VERSION 0.6

test:
    FROM golang:latest
    WORKDIR /app
    COPY . .
    RUN dpkg --add-architecture i386 && \
        apt update && \
        apt install -y g++-multilib
    RUN go get -v -t -d
    RUN --secret FULL_ACCESS_GITHUB_TOKEN go test --race -v ./...

release:
    FROM goreleaser/goreleaser:v1.12.3
    WORKDIR /app
    COPY . .
    RUN --secret GITHUB_TOKEN goreleaser release --rm-dist --skip-publish

release-push:
    FROM goreleaser/goreleaser:v1.12.3
    WORKDIR /app
    COPY . .
    RUN --secret GITHUB_TOKEN goreleaser release --rm-dist
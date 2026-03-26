VERSION 0.8

deps:
    FROM golang:1.25-bookworm
    WORKDIR /workspace
    
    RUN dpkg --add-architecture i386 && \
        apt-get update && \
        apt-get install -y g++-multilib git && \
        rm -rf /var/lib/apt/lists/*
    
    COPY go.mod go.sum ./
    RUN go mod download

    RUN go mod verify

quality-deps:
    FROM golang:1.25-bookworm
    WORKDIR /workspace

    RUN dpkg --add-architecture i386 && \
        apt-get update && \
        apt-get install -y g++-multilib git && \
        rm -rf /var/lib/apt/lists/*

    COPY go.mod go.sum ./
    COPY .golangci.yml ./

    RUN go mod download
    RUN GOBIN=/usr/local/bin go install tool

quality-src:
    FROM +quality-deps

    COPY --dir src ./
    COPY .git ./.git

quality-vet:
    FROM +quality-src

    RUN go vet ./...

quality-lint:
    FROM +quality-src

    ARG LINT_FLAGS=""

    RUN sh -eu -c 'golangci-lint run ${LINT_FLAGS}'

quality-staticcheck:
    FROM +quality-src

    RUN staticcheck ./...

quality-vuln:
    FROM +quality-src

    RUN govulncheck ./...

quality:
    BUILD +quality-vet
    BUILD +quality-lint

quality-full:
    BUILD +quality
    BUILD +quality-staticcheck
    BUILD +quality-vuln

src:
    FROM +deps
    
    COPY --dir src ./
    COPY --dir scripts ./
    COPY Taskfile.yml ./

test:
    FROM +src
    
    RUN go test -race -v -timeout=10m ./...

build:
    FROM +src
    
    ARG VERSION=$(git describe --tags --dirty --always 2>/dev/null || echo "dev")
    
    RUN CGO_ENABLED=0 GOOS=linux go build -a -ldflags "-X main.version=${VERSION}" -o sampctl ./src
    
    SAVE ARTIFACT sampctl /sampctl

build-windows:
    FROM +src
    
    ARG VERSION=$(git describe --tags --dirty --always 2>/dev/null || echo "dev")
    
    RUN CGO_ENABLED=0 GOOS=windows go build -a -ldflags "-X main.version=${VERSION}" -o sampctl.exe ./src
    
    SAVE ARTIFACT sampctl.exe /sampctl.exe

build-all:
    BUILD +build
    BUILD +build-windows

release:
    FROM +build
    
    COPY +build/sampctl ./sampctl
    
    RUN chmod +x ./sampctl
    
    SAVE ARTIFACT sampctl AS LOCAL ./sampctl

release-windows:
    FROM +build-windows
    
    COPY +build-windows/sampctl.exe ./sampctl.exe
    
    SAVE ARTIFACT sampctl.exe AS LOCAL ./sampctl.exe

release-all:
    BUILD +release
    BUILD +release-windows

all:
    BUILD +test
    BUILD +build

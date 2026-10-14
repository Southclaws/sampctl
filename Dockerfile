FROM golang:1.26.9-bookworm AS builder
WORKDIR /workspace
COPY go.mod go.sum ./
RUN go mod download
COPY src ./src
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -ldflags "-X main.version=${VERSION}" -o /sampctl ./src

FROM debian:bookworm AS runtime
RUN mkdir samp && \
    dpkg --add-architecture i386 && \
    apt update && \
    apt install -y g++-multilib git ca-certificates
WORKDIR /samp
ENTRYPOINT ["sampctl"]

FROM runtime AS development
COPY --from=builder /sampctl /bin/sampctl

FROM runtime AS release
ARG TARGETARCH
COPY linux/${TARGETARCH}/sampctl /bin/sampctl

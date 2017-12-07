FROM golang AS compile
WORKDIR /go/src/github.com/Southclaws/sampctl
RUN \
    wget https://github.com/golang/dep/releases/download/v0.3.2/dep-linux-amd64 -o /usr/bin/dep && \
    go get github.com/Southclaws/sampctl && \
    dep ensure && \
    make static
FROM ubuntu
COPY --from=compile /go/src/github.com/Southclaws/sampctl/sampctl /bin/sampctl
RUN mkdir samp && \
    dpkg --add-architecture i386 && \
    apt update && \
    apt install -y g++-multilib git
WORKDIR /samp
ENTRYPOINT ["sampctl"]

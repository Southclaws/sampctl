FROM golang AS compile
WORKDIR /go/src/github.com/Southclaws/sampctl
RUN go get github.com/golang/dep/cmd/dep && \
    go get github.com/Southclaws/sampctl && \
    dep ensure && \
    make static

FROM ubuntu
COPY --from=compile /go/src/github.com/Southclaws/sampctl/sampctl /bin/sampctl
RUN mkdir samp && \
    dpkg --add-architecture i386 && \
    apt update && \
    apt install -y g++-multilib
WORKDIR /samp
ENTRYPOINT ["sampctl"]

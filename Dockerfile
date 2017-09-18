FROM golang AS compile

RUN go get -u github.com/Southclaws/sampctl && \
    cd $GOPATH/src/github.com/Southclaws/sampctl && \
    go build -o sampctl

FROM debian:jessie

COPY --from=compile /go/src/github.com/Southclaws/sampctl/sampctl /usr/bin/sampctl

ENTRYPOINT ["sampctl"]

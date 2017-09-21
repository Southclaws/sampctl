FROM golang AS compile

RUN go get -u github.com/Southclaws/sampctl && \
    cd $GOPATH/src/github.com/Southclaws/sampctl && \
    make sampctl

FROM ubuntu

COPY --from=compile /go/src/github.com/Southclaws/sampctl/sampctl /sampctl

ENTRYPOINT ["/sampctl"]

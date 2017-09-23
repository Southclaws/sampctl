FROM golang AS compile
RUN go get -u github.com/Southclaws/sampctl && \
    cd $GOPATH/src/github.com/Southclaws/sampctl && \
    make static

FROM debian:jessie
COPY --from=compile /go/src/github.com/Southclaws/sampctl/sampctl /bin/sampctl
RUN mkdir samp
WORKDIR /samp
ENTRYPOINT ["sampctl"]

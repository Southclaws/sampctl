FROM golang AS compile
WORKDIR /go/src/github.com/Southclaws/sampctl
COPY vendor vendor
COPY . .
RUN make static

FROM debian:jessie
COPY --from=compile /go/src/github.com/Southclaws/sampctl/sampctl /bin/sampctl
RUN mkdir samp
WORKDIR /samp
ENTRYPOINT ["sampctl"]

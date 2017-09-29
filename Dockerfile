FROM golang AS compile
# just a builder so no need to optimise layers, also makes errors easier to read
RUN go get github.com/golang/dep/cmd/dep
RUN go get github.com/Southclaws/sampctl
WORKDIR /go/src/github.com/Southclaws/sampctl
RUN dep ensure
RUN make static

FROM ubuntu
COPY --from=compile /go/src/github.com/Southclaws/sampctl/sampctl /bin/sampctl
RUN mkdir samp && \
    dpkg --add-architecture i386 && \
    apt update && \
    apt install -y g++-multilib
WORKDIR /samp
ENTRYPOINT ["sampctl"]

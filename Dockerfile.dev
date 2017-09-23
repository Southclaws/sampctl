FROM debian:jessie

COPY sampctl /bin/sampctl
RUN mkdir samp
WORKDIR /samp

ENTRYPOINT ["sampctl"]

FROM ubuntu
COPY sampctl /bin/sampctl
RUN mkdir samp && \
    dpkg --add-architecture i386 && \
    apt update && \
    apt install -y g++-multilib git ca-certificates
WORKDIR /samp
ENTRYPOINT ["sampctl"]

FROM qmcgaw/basedevcontainer:debian

ENV SHELL /bin/zsh
SHELL ["/bin/zsh", "-c"]

# Install Ubuntu packages
RUN dpkg --add-architecture i386 && apt update && apt install -y curl g++-multilib git ca-certificates wget sudo zip gnupg2

# Setup shells
RUN echo "plugins=(vscode git colorize docker docker-compose)" >> /root/.zshrc-specific

# Install Task
RUN sh -c "$(curl --location https://taskfile.dev/install.sh)" -- -d -b /usr/local/bin && chmod +x /usr/local/bin/task

# Install Earthly
RUN sudo /bin/sh -c 'wget https://github.com/earthly/earthly/releases/latest/download/earthly-linux-amd64 -O /usr/local/bin/earthly && chmod +x /usr/local/bin/earthly && /usr/local/bin/earthly bootstrap --with-autocomplete'

# Install GoBrew
RUN curl -sLk https://raw.githubusercontent.com/kevincobain2000/gobrew/master/git.io.sh | sh
ENV PATH "/root/.gobrew/current/bin:/root/.gobrew/bin:$PATH"
ENV GOROOT "/root/.gobrew/current/go"

# Install Golang
RUN gobrew use latest
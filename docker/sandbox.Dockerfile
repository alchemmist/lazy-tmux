FROM ubuntu:26.04

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y \
    build-essential \
    libevent-dev \
    libncurses5-dev \
    bison \
    pkg-config \
    wget \
    zsh \
    git \
    vim \
    curl \
    tree \
    locales \
    ncurses-term \
    htop \
 && rm -rf /var/lib/apt/lists/* \
 && locale-gen en_US.UTF-8

ENV LANG=en_US.UTF-8
ENV LC_ALL=en_US.UTF-8

RUN RUNZSH=no CHSH=no \
    sh -c "$(curl -fsSL https://raw.githubusercontent.com/ohmyzsh/ohmyzsh/master/tools/install.sh)"

RUN cp /root/.oh-my-zsh/templates/zshrc.zsh-template /root/.zshrc

ENV SHELL=/usr/bin/zsh

COPY docker/install-tmux.sh .

# Which tmux release to build into the sandbox. Override per build, e.g.
#   make sandbox TMUX_VERSION=3.5a
#   podman build --build-arg TMUX_VERSION=3.4 -f docker/sandbox.Dockerfile .
ARG TMUX_VERSION=3.6a

RUN TMUX_VERSION=${TMUX_VERSION} ./install-tmux.sh

WORKDIR /root

RUN git clone https://github.com/nordtheme/tmux.git /root/.tmux/themes/nord-tmux

COPY docker/tmux.conf.example /root/.tmux.conf

RUN echo "source /root/.tmux/welcome.sh" >> /root/.zshrc

COPY docker/welcome.sh /root/.tmux/welcome.sh
RUN chmod +x /root/.tmux/welcome.sh

RUN curl -fsSL https://lazy-tmux.xyz/install.sh | sh

COPY docker/test-versions-inner.sh /test-versions-inner.sh
RUN chmod +x /test-versions-inner.sh

ENV TERM=xterm-256color

ENTRYPOINT ["zsh", "-i", "-c", "clear; tmux attach 2>/dev/null || tmux new; clear; exec zsh -i"]

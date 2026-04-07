FROM ubuntu:24.04

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

ARG TMUX_VERSION=3.6a

RUN cd /tmp && \
    wget https://github.com/tmux/tmux/releases/download/${TMUX_VERSION}/tmux-${TMUX_VERSION}.tar.gz && \
    tar -xzf tmux-${TMUX_VERSION}.tar.gz && \
    cd tmux-${TMUX_VERSION} && \
    ./configure && \
    make && \
    make install && \
    cd / && \
    rm -rf /tmp/tmux-${TMUX_VERSION}*

WORKDIR /root

RUN git clone https://github.com/nordtheme/tmux.git /root/.tmux/themes/nord-tmux

COPY tmux.conf.example /root/.tmux.conf

RUN echo "source /root/.tmux/welcome.sh" >> /root/.zshrc

COPY welcome.sh /root/.tmux/welcome.sh
RUN chmod +x /root/.tmux/welcome.sh

RUN curl -fsSL https://lazy-tmux.xyz/install.sh | sh

COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ENV TERM=xterm-256color

ENTRYPOINT ["tmux"]

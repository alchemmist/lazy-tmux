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

COPY install-tmux.sh .

RUN TMUX_VERSION=3.6a ./install-tmux.sh

WORKDIR /root

RUN git clone https://github.com/nordtheme/tmux.git /root/.tmux/themes/nord-tmux

COPY tmux.conf.example /root/.tmux.conf

RUN echo "source /root/.tmux/welcome.sh" >> /root/.zshrc

COPY welcome.sh /root/.tmux/welcome.sh
RUN chmod +x /root/.tmux/welcome.sh

RUN curl -fsSL https://lazy-tmux.xyz/install.sh | sh

ENV TERM=xterm-256color

ENTRYPOINT ["zsh", "-c", "tmux attach || tmux new; exec zsh"]

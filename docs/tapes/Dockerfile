FROM ghcr.io/charmbracelet/vhs:latest

RUN apt-get update && apt-get install -y --no-install-recommends tmux procps python3 \
    && rm -rf /var/lib/apt/lists/*

COPY lazy-tmux /usr/local/bin/lazy-tmux

# A prepared, quiet environment so every recording is clean and reproducible:
# a plain prompt, a hostname-free tmux status bar, a UTF-8 locale (so the
# picker's box-drawing frame renders), and a lazy-tmux config with scrollback on.
RUN printf 'PS1="demo:\\w\\$ "\n' > /root/.bash_profile \
    && cp /root/.bash_profile /root/.bashrc \
    && printf 'set -g status-right " %%H:%%M "\nset -sg escape-time 20\n' > /root/.tmux.conf \
    && printf 'data_dir="/root/data"\nrestore_timeout="3s"\n[scrollback]\nenabled=true\nlines=5000\n' > /root/cfg.toml

ENV TERM=xterm-256color LANG=C.UTF-8 LC_ALL=C.UTF-8 LAZY_TMUX_CONFIG=/root/cfg.toml
WORKDIR /root

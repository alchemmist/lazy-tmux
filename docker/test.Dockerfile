FROM golang:1.27

ENV PATH=/usr/local/go/bin:/go/bin:$PATH
ENV ENABLE_INTEGRATION_TESTS=true

COPY docker/install-tmux.sh /tmp/install-tmux.sh
RUN TMUX_VERSION=3.7b sh /tmp/install-tmux.sh && \
    rm /tmp/install-tmux.sh && \
    apt-get install -y --no-install-recommends fzf && \
    rm -rf /var/lib/apt/lists/*

RUN useradd -m -s /bin/bash appuser && \
    mkdir -p /workspace && \
    chown -R appuser:appuser /workspace

WORKDIR /workspace

COPY go.mod go.sum ./
RUN go mod download

RUN go install gotest.tools/gotestsum@v1.13.0

COPY --chown=appuser:appuser . .

USER appuser

CMD ["gotestsum", "--format", "testname", "--no-color=false", "--", "-p", "1", "./..."]

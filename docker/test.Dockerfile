FROM golang:1.27

ENV PATH=/usr/local/go/bin:/go/bin:$PATH
ENV ENABLE_INTEGRATION_TESTS=true
ENV ENABLE_REAL_PI_TESTS=true

RUN apt-get update && \
    apt-get install -y --no-install-recommends tmux fzf jq nodejs npm lsof && \
    rm -rf /var/lib/apt/lists/*

RUN npm install -g @mariozechner/pi-coding-agent@0.73.1

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

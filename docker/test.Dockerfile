FROM golang:1.27

ENV PATH=/usr/local/go/bin:/go/bin:$PATH
ENV ENABLE_INTEGRATION_TESTS=true

RUN apt-get update && \
    apt-get install -y --no-install-recommends tmux fzf jq && \
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

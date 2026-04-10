FROM golang:1.26

ENV PATH=/usr/local/go/bin:/go/bin:$PATH
ENV ENABLE_INTEGRATION_TESTS=true

RUN apt-get update && \
    apt-get install -y --no-install-recommends tmux fzf && \
    rm -rf /var/lib/apt/lists/*

RUN useradd -m -s /bin/bash appuser && \
    mkdir -p /workspace && \
    chown -R appuser:appuser /workspace

WORKDIR /workspace
USER appuser

COPY go.mod go.sum ./
RUN go mod download

RUN go install gotest.tools/gotestsum@v1.13.0

COPY . .

CMD ["gotestsum", "--format", "testname", "--no-color=false", "--", "-p", "1", "./..."]

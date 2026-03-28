FROM golang:1.26

ENV PATH=/usr/local/go/bin:/go/bin:$PATH

RUN apt-get update && \
    apt-get install -y --no-install-recommends tmux && \
    rm -rf /var/lib/apt/lists/*

RUN useradd -m -s /bin/bash appuser && \
    mkdir -p /workspace && \
    chown -R appuser:appuser /workspace

WORKDIR /workspace
USER appuser

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go install gotest.tools/gotestsum@v1.13.0

CMD ["gotestsum", "--", "-tags=integration", "./..."]

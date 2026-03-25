FROM golang:1.25.7

ENV PATH=/usr/local/go/bin:/go/bin:$PATH

RUN apt-get update && \
    apt-get install -y tmux && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /workspace

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go install gotest.tools/gotestsum@latest

CMD ["gotestsum", "--", "-tags=integration", "./..."]

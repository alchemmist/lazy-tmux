FROM golang:1.27

ENV PATH=/usr/local/go/bin:/go/bin:$PATH
ENV ENABLE_INTEGRATION_TESTS=true
ENV ENABLE_REAL_PI_TESTS=true
ENV REQUIRE_TEST_ANTEX=true

RUN apt-get update && \
    apt-get install -y --no-install-recommends tmux fzf jq python3 nodejs npm lsof && \
    rm -rf /var/lib/apt/lists/*

RUN if [ "$(dpkg --print-architecture)" = amd64 ]; then \
        curl -fsSL --retry 3 -o /tmp/antex.tgz https://github.com/alchemmist/antex/releases/download/v0.2.12/antex-package-x86_64-unknown-linux-gnu.tar.gz && \
        echo '7b2f3547fe7d8ed60c9d899b9d0d64dc53176c37a14e1245a06e8788aa0cc94c  /tmp/antex.tgz' | sha256sum -c - && \
        mkdir -p /opt/antex && tar -xzf /tmp/antex.tgz -C /opt/antex && \
        ln -s /opt/antex/bin/antex /usr/local/bin/antex && rm /tmp/antex.tgz && antex --version; \
    fi

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

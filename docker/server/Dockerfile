FROM golang:1.24.5-bullseye

WORKDIR /app

RUN apt-get update \
    && apt-get install -y --no-install-recommends upx-ucl gcc-mingw-w64 ca-certificates \
    && rm -rf /var/lib/apt/lists/*

RUN go install mvdan.cc/garble@v0.14.2

ENV PATH="${PATH}:$(go env GOPATH)/bin"
ENV GOTOOLCHAIN=go1.24.5+auto

COPY go.mod go.sum ./
RUN go mod download -x

COPY . .
COPY docker/server/entrypoint.sh /app/entrypoint.sh
RUN make server

RUN chmod +x /app/entrypoint.sh

ENTRYPOINT ["/app/entrypoint.sh"]

FROM golang:1.24.5-bullseye

WORKDIR /app

RUN set -eu; \
    native_arch="$(dpkg --print-architecture)"; \
    for arch in amd64 arm64 i386; do \
        if [ "$arch" != "$native_arch" ]; then dpkg --add-architecture "$arch"; fi; \
    done; \
    apt-get update; \
    apt-get install -y --no-install-recommends upx-ucl gcc-mingw-w64 ca-certificates busybox-static; \
    mkdir -p /usr/local/share/wrssh/busybox; \
    case "$native_arch" in \
        amd64) native_goarch=amd64 ;; \
        arm64) native_goarch=arm64 ;; \
        i386) native_goarch=386 ;; \
        *) native_goarch="" ;; \
    esac; \
    if [ -n "$native_goarch" ] && [ -x /bin/busybox ]; then \
        install -m 0755 /bin/busybox "/usr/local/share/wrssh/busybox/linux-$native_goarch"; \
    fi; \
    download_dir="$(mktemp -d)"; \
    cd "$download_dir"; \
    for pair in "amd64 amd64" "arm64 arm64" "386 i386"; do \
        set -- $pair; \
        goarch="$1"; \
        debarch="$2"; \
        if [ "$debarch" = "$native_arch" ]; then continue; fi; \
        apt-get download "busybox-static:$debarch"; \
        deb="$(find . -maxdepth 1 -name "busybox-static_*_${debarch}.deb" -print -quit)"; \
        dpkg-deb -x "$deb" "$download_dir/$goarch"; \
        install -m 0755 "$download_dir/$goarch/bin/busybox" "/usr/local/share/wrssh/busybox/linux-$goarch"; \
    done; \
    rm -rf "$download_dir" /var/lib/apt/lists/*

RUN go install mvdan.cc/garble@v0.14.2

ENV PATH="${PATH}:$(go env GOPATH)/bin"
ENV GOTOOLCHAIN=go1.24.5+auto

COPY go.mod go.sum ./
RUN go mod download -x

COPY . .
COPY docker/platform/entrypoint.sh /app/entrypoint.sh
RUN make platform

WORKDIR /app/bin

RUN chmod +x /app/entrypoint.sh

ENTRYPOINT ["/app/entrypoint.sh"]

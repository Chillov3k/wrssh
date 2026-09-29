FROM golang:1.24.5-bullseye

WORKDIR /app

RUN set -eu; \
    snap="20260901T000000Z"; \
    echo "deb [check-valid-until=no] http://snapshot.debian.org/archive/debian/$snap bullseye main" > /etc/apt/sources.list; \
    echo "deb [check-valid-until=no] http://snapshot.debian.org/archive/debian-security/$snap bullseye-security main" >> /etc/apt/sources.list; \
    rm -f /etc/apt/sources.list.d/debian.sources

RUN set -eu; \
    native_arch="$(dpkg --print-architecture)"; \
    for arch in amd64 arm64 i386; do \
        if [ "$arch" != "$native_arch" ]; then dpkg --add-architecture "$arch"; fi; \
    done; \
    apt-get update -o Acquire::Retries=5; \
    apt-get install -y --no-install-recommends -o Acquire::Retries=5 upx-ucl gcc-mingw-w64 ca-certificates busybox-static; \
    apt-get install -y --no-install-recommends -o Acquire::Retries=5 gcc-aarch64-linux-gnu libc6-dev-arm64-cross || echo "warn: aarch64 cross toolchain unavailable, fury linux/arm64 builds disabled on this host"; \
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

RUN set -eu; \
    curl -sSf https://sh.rustup.rs | sh -s -- -y --default-toolchain stable --profile minimal; \
    . "$HOME/.cargo/env"; \
    rustup target add x86_64-pc-windows-gnu i686-pc-windows-gnu; \
    if [ "$(dpkg --print-architecture)" != "arm64" ] && command -v aarch64-linux-gnu-gcc >/dev/null 2>&1; then \
        rustup target add aarch64-unknown-linux-gnu; \
    fi
ENV PATH="${PATH}:/root/.cargo/bin"

RUN go install mvdan.cc/garble@v0.14.2

ENV PATH="${PATH}:$(go env GOPATH)/bin"
ENV GOTOOLCHAIN=go1.24.5+auto

COPY fury/ ./fury/

RUN set -eu; \
    . "$HOME/.cargo/env"; \
    cd fury; \
    RUSTFLAGS= cargo fetch --locked --target x86_64-pc-windows-gnu; \
    RUSTFLAGS= cargo fetch --locked --target i686-pc-windows-gnu; \
    if rustup target list --installed | grep -q aarch64-unknown-linux-gnu; then RUSTFLAGS= cargo fetch --locked --target aarch64-unknown-linux-gnu; fi; \
    RUSTFLAGS= cargo fetch --locked

COPY go.mod go.sum ./
RUN go mod download -x

COPY . .
COPY docker/runtime/entrypoint.sh /app/entrypoint.sh
RUN make runtime-agent

WORKDIR /app/bin

RUN chmod +x /app/entrypoint.sh

ENTRYPOINT ["/app/entrypoint.sh"]

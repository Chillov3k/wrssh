FROM rust:1.91.1

WORKDIR /app

RUN set -eu; \
    apt-get update; \
    apt-get install -y --no-install-recommends gcc-mingw-w64 ca-certificates; \
    apt-get install -y --no-install-recommends gcc-aarch64-linux-gnu libc6-dev-arm64-cross || echo "warn: aarch64 cross toolchain unavailable, fury linux/arm64 builds disabled on this host"; \
    rustup target add x86_64-pc-windows-gnu i686-pc-windows-gnu; \
    if command -v aarch64-linux-gnu-gcc >/dev/null 2>&1; then rustup target add aarch64-unknown-linux-gnu; fi

COPY fury/ ./fury/

RUN set -eu; \
    cd fury; \
    RUSTFLAGS= cargo fetch --locked --target x86_64-pc-windows-gnu; \
    RUSTFLAGS= cargo fetch --locked --target i686-pc-windows-gnu; \
    if rustup target list --installed | grep -q aarch64-unknown-linux-gnu; then RUSTFLAGS= cargo fetch --locked --target aarch64-unknown-linux-gnu; fi; \
    RUSTFLAGS= cargo fetch --locked

# Fury — Rust implant for WRSSH

Fury is a drop-in alternative implant for the WRSSH C2, rewritten in Rust for
significantly smaller binaries (~3 MB vs ~18 MB for the Go client) and a much
smaller static footprint. It speaks the same reverse-SSH protocol as the
standard Go client, so the server, web UI and operator tooling work unchanged.

## Supported functionality

| Capability | Status |
|---|---|
| Control connection (plain TCP / TLS with optional SNI) | done |
| Server fingerprint pinning (SHA256 hex, same format as Go client) | done |
| Public key auth (ed25519, build-time embedded key) | done |
| Control-connection `session` channel (`exec`, legacy rssh path) | done |
| `jump` channel — in-process SSH server for `ssh -J` operators | done |
| Interactive shell (ConPTY on Windows, pipes fallback on Unix) | done |
| `direct-tcpip` port forwarding through the agent | done |
| `tcpip-forward` remote forwards (agent-side listeners) | done |
| SCP (`scp -f` / `scp -t`) | done |
| SFTP subsystem (full fs read/write/stat/rename/mkdir...) | done |
| URL download-and-exec (http/https, minimal client, no reqwest) | done |
| `list` subsystem | done |
| Reconnect loop with jitter | done |

Out of scope for v1 (present in the Go client): busybox fallback, TUN/VPN
mode, `pscan` / `execass` modules, service installation, setuid/setgid,
NTLM/Kerberos proxy auth, WebSocket and HTTP polling transports.

## Stealth properties

Everything below is enforced by `tools/postbuild.py` (and its Go twin inside
`buildmanager.go`): the build **fails** if any indicator survives into the
final binary.

- `--remap-path-prefix` for every path class that Rust embeds: build machine
  homes, cargo registry (all crates), the vendored crates, our own source
  tree, and the prebuilt std's `/rustc/<hash>` (byte-patched equal-length
  because remap cannot touch precompiled rlibs).
- All source files renamed to neutral short names (`hd/jp.rs`, `ev/pb.rs`,
  ...) and the package itself is `svc` — symbol names carry no hints.
- XOR-obfuscated protocol literals (`jump`, `sftp`, `list`, `scp `, UA
  string, temp dir) — nothing greppable in `.rodata`.
- SSH identification strings default to `SSH-2.0-OpenSSH_9.6` on both the
  control client and the in-process jump server.
- No PE version resource by default (unsigned Microsoft-branding is itself a
  heuristic); opt in per-engagement with `FURY_MASQ=1`.
- Panic paths, compiler hashes and registry names are neutralized with
  equal-length in-place patches, so offsets and layout stay valid.

- Rust + `panic=abort` + `strip` + `opt-level=z`: small, no Go runtime
  signatures, no Go build IDs.
- Minimal imports (kernel32/ntdll/ws2_32/bcrypt only).
- PE version-info resource masquerading as a Windows OS binary.
- PEB `ImagePathName`/`CommandLine` masking (own process only).
- No console window on Windows, `CREATE_NO_WINDOW` for child processes.
- Shell history suppression (`HISTFILE`, PSReadLine) like the Go client.
- Noisy techniques (unhooking, AMSI/ETW patching, sleep obfuscation) are
  deliberately absent: on the KES 14.1 test bench the binary passes static
  on-write scan, on-demand scan and runtime monitoring without them, and
  quiet operation is the stronger OPSEC position. They can be layered on
  per-engagement if a specific EDR requires it.

## Building

Requires a Rust toolchain with the `x86_64-pc-windows-gnu` target and
`x86_64-w64-mingw32-gcc` (mingw-w64).

```
# from the repository root
make fury FURY=1                                  # host platform
make fury FURY=1 FURY_OS=windows                  # windows/amd64
make fury FURY=1 FURY_OS=windows FURY_ARCH=386    # windows/386
```

Compile-time settings (Rust equivalent of the Go client's `-ldflags -X`):

| Env var | Go equivalent | Purpose |
|---|---|---|
| `FURY_KEY_B64` | `keys.EmbeddedPrivateKeyBase64` | agent private key (base64 PEM) |
| `FURY_DEST` | `main.destination` | connect-back address (`[scheme://]host:port`) |
| `FURY_FINGERPRINT` | `main.fingerprint` | server host key SHA256 hex |
| `FURY_MASQ` | — | set to `1` to embed the Microsoft-style version resource (off by default) |

Runtime flags: `--destination/-d`, `--fingerprint`, `--sni`,
`--version-string`, `--foreground` (mirrors the Go client; the binary also
accepts a bare address as the last argument).

## Server-side integration

- `make fury FURY=1` — Makefile target (opt-in, Go client stays default).
- `link --fury ...` — server console command builds Fury via cargo.
- `buildFury()` in `internal/server/webserver/buildmanager.go` — the build
  manager path used by the API.

The vendored `russh` under `vendor/russh` carries small patches required for
reverse-ssh protocol compatibility (keepalive payload on the control
connection, server-initiated session requests on the client side, exit-status
replies). See the patch markers in `vendor/russh/src/client/`.

## Test bench results (KES 14.1.0.423, September 2026)

- On-write static scan: clean (the Go client is deleted on write).
- On-demand `avp.com SCAN`: 0 detections.
- Runtime: process survives, control connection and exec/jump/sftp operate,
  File_Monitoring detection counters unchanged.

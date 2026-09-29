#!/usr/bin/env python3
"""Post-build hardening + IOC gate for release binaries.

1. Rewrites /rustc/<commit-hash> paths baked into the prebuilt std (remap
   flags cannot touch them) with an equal-length neutral value.
2. Fails the build if any known indicator still leaks into the image.

Usage: postbuild.py <binary> [--gate-only]
"""
import re
import sys

IOC_PATTERNS = [
    rb"Users/[A-Za-z0-9_.-]+",      # build user paths
    rb"/private/tmp/rust-",          # homebrew std build tree
    rb"\.cargo", rb"\.rustup",
    rb"wrssh", rb"russh", rb"keepalive-rssh", rb"reverse_ssh",
    rb"\bfury\b", rb"\bevasion\b", rb"\bsvchost\b", rb"\bconpty\b",
    rb"ssh_client",
    rb"/rustc/(?!0{40})[0-9a-f]{8}",  # real compiler hashes (patched ones are 0s)
    rb"index\.crates\.io",
    # our own source tree layout (post-rename neutral paths must not leak)
    rb"src/hd/", rb"src/ev/", rb"src/pt/",
    rb"src/(main|sc|cf|ky|ob|pr|sy|tl|dl)\.rs",
    rb"vendor/",
]

def main():
    if len(sys.argv) < 2:
        sys.exit("usage: postbuild.py <binary> [--gate-only]")
    path = sys.argv[1]
    gate_only = "--gate-only" in sys.argv
    data = open(path, "rb").read()

    if not gate_only:
        pat = re.compile(rb"/rustc/[0-9a-f]{40}")
        data = pat.sub(b"/rustc/" + b"0" * 40, data)
        # homebrew rust ships a std built in its own build tree
        pat2 = re.compile(rb"/private/tmp/rust-[0-9A-Za-z_-]+/rustc-[0-9.]+-src/vendor/[^\x00]*?\.rs")
        data = pat2.sub(lambda m: b"/s" + b"0" * (len(m.group(0)) - 2), data)
        # any crates.io registry path that survived remapping (different build
        # hosts use different cargo homes); equal-length neutral fill
        pat3 = re.compile(rb"(?:/[a-z0-9]{1,8}/)?index\.crates\.io-[0-9a-f]{8,32}[\-/][A-Za-z0-9_.\-]+-[0-9]+\.[0-9]+\.[0-9]+[^\x00]*?\.rs")
        data = pat3.sub(lambda m: b"/r" + b"0" * (len(m.group(0)) - 2), data)
        open(path, "wb").write(data)

    leaked = {}
    for p in IOC_PATTERNS:
        m = re.findall(p, data)
        if m:
            leaked[p.decode()] = sorted(set(x.decode("latin1") for x in m))[:3]
    if leaked:
        for k, v in leaked.items():
            print(f"LEAK [{k}]: {v}", file=sys.stderr)
        sys.exit(1)
    print(f"postbuild: clean ({len(data)} bytes)")

if __name__ == "__main__":
    main()

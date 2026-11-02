#!/bin/sh
H="${HOME%/}"
F="${1:-$PWD}"
F="${F%/}"
printf '%s' "--remap-path-prefix=$H=/h --remap-path-prefix=$H/.rustup/toolchains=/z --remap-path-prefix=$H/.cargo/registry/src=/y --remap-path-prefix=$F/src=/sv --remap-path-prefix=$F/vendor/r=/d1 --remap-path-prefix=$F/vendor/c=/d2 --remap-path-prefix=$F/vendor/s=/d3 --remap-path-prefix=$F/vendor/u=/d4 --remap-path-prefix=src=/sv --remap-path-prefix=vendor/r=/d1 --remap-path-prefix=vendor/c=/d2 --remap-path-prefix=vendor/s=/d3 --remap-path-prefix=vendor/u=/d4"

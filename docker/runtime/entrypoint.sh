#!/bin/bash
set -e

if [ ! -d "/data" ]; then
    mkdir -p /data
fi

mkdir -p /data/keys
touch /data/authorized_keys /data/authorized_controllee_keys

if [ ! -z "$SEED_AUTHORIZED_KEYS" ]; then
    if [ -s /data/authorized_keys ]; then
        echo "authorized_keys is not empty, ignoring SEED_AUTHORIZED_KEYS"
    else
        echo "Seeding authorized_keys"
        echo "$SEED_AUTHORIZED_KEYS" > /data/authorized_keys
    fi
fi

export RSSH_DATA_DIR=/data

exec ./runtime-agent

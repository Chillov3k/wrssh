#!/bin/bash
set -e

if [ ! -d "/data" ]; then
    echo "Please mount /data"
    exit 1
fi

EXTERNAL_ADDRESS_VALUE="${RSSH_EXTERNAL_ADDRESS:-$EXTERNAL_ADDRESS}"
LISTEN_ADDRESS_VALUE="${RSSH_LISTEN_ADDR:-$LISTEN_ADDRESS}"

if [ -z "$EXTERNAL_ADDRESS_VALUE" ]; then
    echo "Please specify RSSH_EXTERNAL_ADDRESS or EXTERNAL_ADDRESS"
    exit 1
fi

if [ -z "$LISTEN_ADDRESS_VALUE" ]; then
    echo "Please specify RSSH_LISTEN_ADDR or LISTEN_ADDRESS"
    exit 1
fi

touch /data/authorized_keys /data/authorized_controllee_keys

# Allow user to seed the authorized_keys file
if [ ! -z "$SEED_AUTHORIZED_KEYS" ]; then
    if [ -s /data/authorized_keys ]; then
        echo "authorized_keys is not empty, ignoring SEED_AUTHORIZED_KEYS\n"
    else
        echo "Seeding authorized_keys...\n"
        echo $SEED_AUTHORIZED_KEYS > /data/authorized_keys
    fi
fi

cd /app/bin
exec ./server --datadir /data --enable-client-downloads --tls --external_address "$EXTERNAL_ADDRESS_VALUE" "$LISTEN_ADDRESS_VALUE"

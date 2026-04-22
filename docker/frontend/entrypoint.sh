#!/bin/sh
set -eu

ENTRY_PATH_FILE="/data/.web_entry_path"

if [ -z "${WRSSH_ENTRY_PATH:-}" ]; then
  if [ -s "$ENTRY_PATH_FILE" ]; then
    WRSSH_ENTRY_PATH="$(tr -d '\r\n' < "$ENTRY_PATH_FILE")"
  else
    WRSSH_ENTRY_PATH="/$(tr -dc 'a-z0-9' < /dev/urandom | head -c 16)"
    printf '%s\n' "$WRSSH_ENTRY_PATH" > "$ENTRY_PATH_FILE"
    chmod 600 "$ENTRY_PATH_FILE"
  fi
fi

case "$WRSSH_ENTRY_PATH" in
  /*) ;;
  *) WRSSH_ENTRY_PATH="/$WRSSH_ENTRY_PATH" ;;
esac

export WRSSH_ENTRY_PATH
exec /docker-entrypoint.sh nginx -g 'daemon off;'

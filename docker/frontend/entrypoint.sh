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

HTTP_TEMPLATE="/etc/nginx/templates/default.http.conf.template"
HTTPS_TEMPLATE="/etc/nginx/templates/default.https.conf.template"
ACTIVE_TEMPLATE="/etc/nginx/templates/default.conf.template"

rm -f /etc/nginx/conf.d/default.http.conf /etc/nginx/conf.d/default.https.conf
rm -f "$ACTIVE_TEMPLATE"

if [ -n "${WEB_DOMAIN:-}" ]; then
  if [ "${WEB_UI_PORT:-}" = "80" ]; then
    echo "frontend: WEB_UI_PORT cannot be 80 when WEB_DOMAIN is set; port 80 is reserved for HTTP -> HTTPS redirect" >&2
    exit 1
  fi

  : "${WEB_TLS_CERT_PATH:=/etc/letsencrypt/live/${WEB_DOMAIN}/fullchain.pem}"
  : "${WEB_TLS_KEY_PATH:=/etc/letsencrypt/live/${WEB_DOMAIN}/privkey.pem}"

  if [ ! -r "$WEB_TLS_CERT_PATH" ]; then
    echo "frontend: TLS certificate not found: $WEB_TLS_CERT_PATH" >&2
    exit 1
  fi
  if [ ! -r "$WEB_TLS_KEY_PATH" ]; then
    echo "frontend: TLS private key not found: $WEB_TLS_KEY_PATH" >&2
    exit 1
  fi

  cp "$HTTPS_TEMPLATE" "$ACTIVE_TEMPLATE"
  rm -f "$HTTP_TEMPLATE" "$HTTPS_TEMPLATE"
else
  cp "$HTTP_TEMPLATE" "$ACTIVE_TEMPLATE"
  rm -f "$HTTP_TEMPLATE" "$HTTPS_TEMPLATE"
fi

WEB_HTTPS_PORT_SUFFIX=":${WEB_UI_PORT}"
if [ "${WEB_UI_PORT:-}" = "443" ]; then
  WEB_HTTPS_PORT_SUFFIX=""
fi

export WRSSH_ENTRY_PATH
export WEB_DOMAIN
export WEB_TLS_CERT_PATH
export WEB_TLS_KEY_PATH
export WEB_UI_PORT
export WEB_HTTPS_PORT_SUFFIX
exec /docker-entrypoint.sh nginx -g 'daemon off;'

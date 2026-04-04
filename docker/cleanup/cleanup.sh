#!/bin/sh
set -eu

stack="${WRSSH_STACK_NAME:-wrssh}"

remove_named_containers() {
  for name in \
    "${stack}-frontend-1" \
    "${stack}-platform-1" \
    "${stack}-runtime-image-1" \
    "${stack}-postgres-1"
  do
    docker rm -f "$name" >/dev/null 2>&1 || true
  done
}

remove_labeled_containers() {
  ids="$(docker ps -aq --filter label=wrssh.managed=true)"
  if [ -n "$ids" ]; then
    docker rm -f $ids
  fi
}

remove_labeled_networks() {
  ids="$(docker network ls -q --filter label=wrssh.managed=true)"
  if [ -n "$ids" ]; then
    docker network rm $ids
  fi
}

remove_labeled_volumes() {
  ids="$(docker volume ls -q --filter label=wrssh.managed=true)"
  if [ -n "$ids" ]; then
    docker volume rm -f $ids
  fi
}

remove_named_resources() {
  docker network rm "${stack}_default" >/dev/null 2>&1 || true
  docker volume rm -f "${stack}_postgres_data" "${stack}_rssh_data" >/dev/null 2>&1 || true
}

remove_named_containers
remove_labeled_containers
remove_labeled_networks
remove_labeled_volumes
remove_named_resources

echo "wrssh cleanup completed"

# WRSSH

```text
browser
  |
  v
frontend (nginx, :WEB_UI_PORT)
  |
  v
platform (web API + auth + project registry + Docker orchestration)
  |
  +--> control-plane postgres
  |
  +--> runtime-agent for project A -> rssh runtime A -> postgres A
  |
  +--> runtime-agent for project B -> rssh runtime B -> postgres B
```

## Architecture

- `frontend` serves the UI and proxies `/api` and `/ws` to `platform`.
- `platform` stores:
  - users;
  - projects;
  - project permissions;
  - runtime registry;
  - host metadata and project assignments.
- Each project gets its own isolated runtime:
  - its own `wrssh-runtime-*` container;
  - its own Postgres container;
  - its own SSH port;
  - its own internal `runtime-agent` port.
- `platform` creates and deletes project runtimes through the Docker API.
- Normal users only work with the projects assigned to them. Direct SSH access to hosts goes through the SSH port of the specific project runtime, not through the shared control plane.


## Get started

1. Copy the example env:

```bash
cp .env.example .env
```

2. Fill in `.env`.

Required fields:

- `WEB_UI_PORT`  
  Web UI port on the host.

- `CONTROL_PLANE_SSH_PORT`  
  Shared control-plane SSH port. Mainly kept for compatibility and admin scenarios.

- `RSSH_PORT`  
  Internal SSH/download port used by `rssh` inside the container.

- `RSSH_EXTERNAL_ADDRESS`  
  External control-plane address in `host:port` format. Used for links and connect-back templates.

- `RSSH_ADVERTISED_ADDRESSES`  
  Address list shown in the build UI, for example:  
  `eth0=192.168.1.1,lo0=127.0.0.1`

- `POSTGRES_DB`
- `POSTGRES_USER`
- `POSTGRES_PASSWORD`  
  Main Postgres settings for the control plane.

- `WEB_USER`  
  Initial admin username.

- `WEB_ADMIN_PASSWORD`  
  Initial admin password.

- `SEED_AUTHORIZED_KEYS`  
  Public SSH key that will be added to `/data/authorized_keys` on first start.

- `WEB_RUNTIME_SSH_PORT_START`
- `WEB_RUNTIME_SSH_PORT_END`  
  SSH port range for project runtimes.

- `WEB_RUNTIME_AGENT_PORT_START`
- `WEB_RUNTIME_AGENT_PORT_END`  
  Internal runtime-agent port range.

- `WEB_RUNTIME_AGENT_HOST`  
  Host used by `platform` to reach the runtime-agent. Usually `host.docker.internal`.

- `WEB_RUNTIME_PUBLISHED_HOST`  
  External IP/hostname baked into project runtime connect-back addresses.

- `WEB_RUNTIME_BIND_IP`  
  IP used to publish per-project runtime ports. Usually `0.0.0.0`.

Example:

```dotenv
WEB_UI_PORT=8080
CONTROL_PLANE_SSH_PORT=2222
RSSH_PORT=2222
RSSH_EXTERNAL_ADDRESS=192.168.1.1:2222
RSSH_ADVERTISED_ADDRESSES=eth0=192.168.1.1,lo0=127.0.0.1
POSTGRES_DB=rssh
POSTGRES_USER=rssh
POSTGRES_PASSWORD=change-me

WEB_USER=admin
WEB_ADMIN_PASSWORD=change-me-too
SEED_AUTHORIZED_KEYS='ssh-ed25519 AAAA... you@host'

WEB_RUNTIME_SSH_PORT_START=2300
WEB_RUNTIME_SSH_PORT_END=2399
WEB_RUNTIME_AGENT_PORT_START=2400
WEB_RUNTIME_AGENT_PORT_END=2499
WEB_RUNTIME_AGENT_HOST=host.docker.internal
WEB_RUNTIME_PUBLISHED_HOST=192.168.1.1
WEB_RUNTIME_BIND_IP=0.0.0.0
```

## How To Start

```bash
docker compose up -d --build
```

What happens:

- `platform`, `frontend`, and the `wrssh-runtime:local` template image are built;
- the main Postgres is started;
- the control plane is started;
- the web UI becomes available at `http://<host>:<WEB_UI_PORT>`.

Then log in with `WEB_USER` / `WEB_ADMIN_PASSWORD`.

## How To Stop

To stop only the main compose stack:

```bash
docker compose down -v
```

Important: this is not enough for a full cleanup because project runtimes are created dynamically through the Docker API and are not regular compose services.

For a full reset of everything:

```bash
docker compose --profile ops run --rm cleanup
```

This cleanup removes:

- the main compose stack;
- all `wrssh-runtime-*` containers;
- all `wrssh-runtime-db-*` containers;
- all runtime networks and volumes labeled `wrssh.managed=true`.



## Main UI Paths

- `/projects`  
  Project selection and entry point for creating a new project.

- `/overview?project=<name>`  
  Project summary.

- `/hosts?project=<name>`  
  Project hosts.

- `/builds?project=<name>`  
  Build new clients for that specific project.

- `/downloads?project=<name>`  
  Project artifacts.

- `/profile`  
  Change your own password and SSH keys.


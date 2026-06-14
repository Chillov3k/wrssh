# Commands

```sh
pwd && rg --files
git status --short
find . -maxdepth 3 -type d | sort
sed -n '1,240p' internal/client/handlers/subsystems/init.go
sed -n '1,240p' internal/client/handlers/subsystems/list.go
sed -n '1,260p' internal/client/handlers/subsystems/sftp.go
sed -n '1,260p' internal/client/handlers/session.go
sed -n '1,260p' internal/server/commands/exec.go
sed -n '1,220p' internal/server/commands/init.go
sed -n '1,280p' internal/platform/rssh/service.go
sed -n '1,280p' internal/platform/api/host_exec.go
sed -n '1,260p' internal/platform/orchestrator/runtime_api.go
sed -n '1,220p' internal/platform/runtimeagent/runtime_api.go
go test ./internal/client/handlers/subsystems
go test -tags pscan ./internal/client/handlers/subsystems/pscan
go test -tags execass ./internal/client/handlers/subsystems/execass ./cmd/client
go test ./internal/platform/rssh ./internal/server/commands ./internal/platform/orchestrator ./internal/platform/runtimeagent ./internal/platform/api
go test ./internal/server/commands -run TestModuleCommandListIntegration -count=1 -timeout=10s -v
go test ./internal/terminal
go test ./...
go test -tags pscan ./...
go test -tags execass ./...
go test -tags 'pscan execass' ./...
GOOS=windows GOARCH=amd64 go test -c -tags execass ./cmd/client -o /tmp/wrssh-client-execass-windows.test.exe
make e2e
(cd e2e && ./e2e)
ssh root@46.62.143.173 'hostname; uname -a; command -v docker || true'
rsync -a --delete --exclude='.git/' --exclude='.env' --exclude='bin/' --exclude='e2e/client' --exclude='e2e/server' --exclude='e2e/e2e' --exclude='e2e/cache/' --exclude='e2e/downloads/' --exclude='e2e/keys/' --exclude='e2e/data.db' --exclude='e2e/authorized_keys' --exclude='e2e/authorized_controllee_keys' --exclude='e2e/id_e2e' --exclude='e2e/id_e2e.pub' ./ root@46.62.143.173:/root/wrssh-e2e-work/
ssh root@46.62.143.173 'cd /root/wrssh-e2e-work && docker run --rm -v "$PWD:/app" -w /app --entrypoint /bin/bash wrssh-platform:local -lc "export PATH=/usr/local/go/bin:/root/go/bin:/go/bin:\$PATH; make e2e && cd e2e && ./e2e"'
ssh root@46.62.143.173 'tar --exclude=.git --exclude=bin --exclude=e2e/cache --exclude=e2e/downloads --exclude=e2e/keys -czf /root/wrssh-backup-20260613-114454.tar.gz -C /root wrssh'
rsync -a --delete --exclude='.git/' --exclude='.env' --exclude='bin/' --exclude='e2e/client' --exclude='e2e/server' --exclude='e2e/e2e' --exclude='e2e/cache/' --exclude='e2e/downloads/' --exclude='e2e/keys/' --exclude='e2e/data.db' --exclude='e2e/authorized_keys' --exclude='e2e/authorized_controllee_keys' --exclude='e2e/id_e2e' --exclude='e2e/id_e2e.pub' ./ root@46.62.143.173:/root/wrssh/
ssh root@46.62.143.173 'cd /root/wrssh && docker compose up -d --build'
ssh root@46.62.143.173 'cd /root/wrssh && docker compose ps'
ssh root@46.62.143.173 'curl -k -I --max-time 10 https://127.0.0.1/hrcfeqomn4ye39om/'
ssh -o BatchMode=yes -o ConnectTimeout=15 -o StrictHostKeyChecking=accept-new -J admin@46.62.143.173:2301 f5e679203fc816f4b5e0f2e2455200f364033b83 -s list --json
go test ./internal/server/webserver ./internal/server/commands ./internal/platform/api
go test -tags pscan ./internal/client/handlers/subsystems/pscan ./internal/client/handlers/subsystems ./cmd/client
go test ./...
go test -tags pscan ./...
go test -tags execass ./...
go test -tags 'pscan execass' ./...
node --check frontend/host.js
node --check frontend/builds.js
rsync -a --delete --exclude='.git/' --exclude='.env' --exclude='bin/' --exclude='e2e/client' --exclude='e2e/server' --exclude='e2e/e2e' --exclude='e2e/cache/' --exclude='e2e/downloads/' --exclude='e2e/keys/' --exclude='e2e/data.db' --exclude='e2e/authorized_keys' --exclude='e2e/authorized_controllee_keys' --exclude='e2e/id_e2e' --exclude='e2e/id_e2e.pub' ./ root@46.62.143.173:/root/wrssh-e2e-work/
ssh root@46.62.143.173 'cd /root/wrssh-e2e-work && docker run --rm -v "$PWD:/app" -w /app --entrypoint /bin/bash wrssh-platform:local -lc "export PATH=/usr/local/go/bin:/root/go/bin:/go/bin:\$PATH; go test ./... && go test -tags pscan ./... && go test -tags execass ./... && make e2e && cd e2e && ./e2e"'
ssh root@46.62.143.173 'set -e; ts=$(date +%Y%m%d-%H%M%S); tar --exclude=.git --exclude=bin --exclude=e2e/cache --exclude=e2e/downloads --exclude=e2e/keys -czf /root/wrssh-backup-$ts.tar.gz -C /root wrssh; echo /root/wrssh-backup-$ts.tar.gz'
rsync -a --delete --exclude='.git/' --exclude='.env' --exclude='bin/' --exclude='e2e/client' --exclude='e2e/server' --exclude='e2e/e2e' --exclude='e2e/cache/' --exclude='e2e/downloads/' --exclude='e2e/keys/' --exclude='e2e/data.db' --exclude='e2e/authorized_keys' --exclude='e2e/authorized_controllee_keys' --exclude='e2e/id_e2e' --exclude='e2e/id_e2e.pub' ./ root@46.62.143.173:/root/wrssh/
ssh root@46.62.143.173 'cd /root/wrssh && docker compose up -d --build && docker compose ps'
ssh root@46.62.143.173 'set -e; cd /root/wrssh; docker compose ps; docker ps --format "table {{.Names}}\t{{.Image}}\t{{.Status}}\t{{.Ports}}" | grep -E "wrssh|NAMES"; curl -k -I --max-time 10 https://127.0.0.1/hrcfeqomn4ye39om/; docker logs --tail=80 wrssh-platform-1 2>&1'
ssh -o BatchMode=yes -o ConnectTimeout=15 -o StrictHostKeyChecking=accept-new -p 2301 admin@46.62.143.173 'link --name codex-pscan-e2e --goos linux --goarch amd64 --pscan --s 46.62.143.173:2301'
ssh root@46.62.143.173 'set -e; rm -f /tmp/codex-pscan-e2e /tmp/codex-pscan-e2e.log; curl -fsSL http://127.0.0.1:2301/codex-pscan-e2e -o /tmp/codex-pscan-e2e; chmod 700 /tmp/codex-pscan-e2e; nohup /tmp/codex-pscan-e2e >/tmp/codex-pscan-e2e.log 2>&1 & echo $!'
ssh -o BatchMode=yes -o ConnectTimeout=15 -o StrictHostKeyChecking=accept-new -J admin@46.62.143.173:2301 caf646ff6765d67627674f1a64a5d94c52cafa73 -s list --json
ssh -o BatchMode=yes -o ConnectTimeout=15 -o StrictHostKeyChecking=accept-new -J admin@46.62.143.173:2301 caf646ff6765d67627674f1a64a5d94c52cafa73 -s pscan -ips localhost --json
ssh -o BatchMode=yes -o ConnectTimeout=15 -o StrictHostKeyChecking=accept-new -J admin@46.62.143.173:2301 caf646ff6765d67627674f1a64a5d94c52cafa73 -s pscan -ips scanme.nmap.org --timeout 2s --json
ssh root@46.62.143.173 'kill 2516912 2>/dev/null || true; rm -f /tmp/codex-pscan-e2e /tmp/codex-pscan-e2e.log'
ssh -o BatchMode=yes -o ConnectTimeout=15 -o StrictHostKeyChecking=accept-new -p 2301 admin@46.62.143.173 'link -r codex-pscan-e2e'
ssh root@46.62.143.173 'set -e; curl -ks --max-time 10 https://127.0.0.1/hrcfeqomn4ye39om/builds | grep -E "pscan module|execass stub"; curl -ks --max-time 10 https://127.0.0.1/hrcfeqomn4ye39om/host | grep -E "Subsystems|moduleRunForm|refreshModulesButton"'
go test -tags pscan ./internal/client/handlers/subsystems/pscan -count=1 -v
go test ./...
go test -tags pscan ./...
node --check frontend/host.js
rsync -a --delete --exclude='.git/' --exclude='.env' --exclude='bin/' --exclude='e2e/client' --exclude='e2e/server' --exclude='e2e/e2e' --exclude='e2e/cache/' --exclude='e2e/downloads/' --exclude='e2e/keys/' --exclude='e2e/data.db' --exclude='e2e/authorized_keys' --exclude='e2e/authorized_controllee_keys' --exclude='e2e/id_e2e' --exclude='e2e/id_e2e.pub' ./ root@46.62.143.173:/root/wrssh-e2e-work/
ssh root@46.62.143.173 'cd /root/wrssh-e2e-work && docker run --rm -v "$PWD:/app" -w /app --entrypoint /bin/bash wrssh-platform:local -lc "export PATH=/usr/local/go/bin:/root/go/bin:/go/bin:\$PATH; go test -tags pscan ./... && make e2e && cd e2e && ./e2e"'
ssh root@46.62.143.173 'set -e; ts=$(date +%Y%m%d-%H%M%S); tar --exclude=.git --exclude=bin --exclude=e2e/cache --exclude=e2e/downloads --exclude=e2e/keys -czf /root/wrssh-backup-$ts.tar.gz -C /root wrssh; echo /root/wrssh-backup-$ts.tar.gz'
rsync -a --delete --exclude='.git/' --exclude='.env' --exclude='bin/' --exclude='e2e/client' --exclude='e2e/server' --exclude='e2e/e2e' --exclude='e2e/cache/' --exclude='e2e/downloads/' --exclude='e2e/keys/' --exclude='e2e/data.db' --exclude='e2e/authorized_keys' --exclude='e2e/authorized_controllee_keys' --exclude='e2e/id_e2e' --exclude='e2e/id_e2e.pub' ./ root@46.62.143.173:/root/wrssh/
ssh root@46.62.143.173 'cd /root/wrssh && docker compose up -d --build && docker compose ps'
ssh root@46.62.143.173 'set -e; cd /root/wrssh; docker ps --format "table {{.Names}}\t{{.Image}}\t{{.Status}}\t{{.Ports}}" | grep -E "wrssh|NAMES"; curl -k -I --max-time 10 https://127.0.0.1/hrcfeqomn4ye39om/; docker logs --tail=60 wrssh-platform-1 2>&1'
ssh -o BatchMode=yes -o ConnectTimeout=15 -o StrictHostKeyChecking=accept-new -p 2301 admin@46.62.143.173 'link --name codex-pscan-all-e2e --goos linux --goarch amd64 --pscan --s 46.62.143.173:2301'
ssh root@46.62.143.173 'set -e; rm -f /tmp/codex-pscan-all-e2e /tmp/codex-pscan-all-e2e.log; curl -fsSL http://127.0.0.1:2301/codex-pscan-all-e2e -o /tmp/codex-pscan-all-e2e; chmod 700 /tmp/codex-pscan-all-e2e; nohup /tmp/codex-pscan-all-e2e >/tmp/codex-pscan-all-e2e.log 2>&1 & echo $!'
ssh -o BatchMode=yes -o ConnectTimeout=15 -o StrictHostKeyChecking=accept-new -J admin@46.62.143.173:2301 cb1d6aa519c8f11dad3977a7f3a9f02c18ee17d8 -s list --json
ssh -o BatchMode=yes -o ConnectTimeout=15 -o StrictHostKeyChecking=accept-new -J admin@46.62.143.173:2301 cb1d6aa519c8f11dad3977a7f3a9f02c18ee17d8 -s pscan -h localhost -p 80,443 --json
ssh -o BatchMode=yes -o ConnectTimeout=15 -o ServerAliveInterval=10 -o StrictHostKeyChecking=accept-new -J admin@46.62.143.173:2301 cb1d6aa519c8f11dad3977a7f3a9f02c18ee17d8 -s pscan -h scanme.nmap.org -p all --timeout 200ms
ssh root@46.62.143.173 'kill 2544780 2>/dev/null || true; rm -f /tmp/codex-pscan-all-e2e /tmp/codex-pscan-all-e2e.log'
ssh -o BatchMode=yes -o ConnectTimeout=15 -o StrictHostKeyChecking=accept-new -p 2301 admin@46.62.143.173 'link -r codex-pscan-all-e2e'
git clone --depth 1 https://github.com/nu11zy/rscc.git /private/tmp/rscc-src
go list -m -versions github.com/Ne0nd0g/go-clr
go get github.com/Ne0nd0g/go-clr@v1.0.3 github.com/google/shlex@v0.0.0-20191202100458-e7afc7fbc510
go test -tags execass ./internal/client/handlers/subsystems/execass ./cmd/client
GOOS=windows GOARCH=amd64 go test -c -tags execass ./cmd/client -o /tmp/wrssh-client-execass-windows.test.exe
go test ./...
go test -tags execass ./...
go test -tags 'pscan execass' ./...
GOOS=windows GOARCH=amd64 go test -c -tags execass ./internal/client/handlers/subsystems/execass -o /tmp/wrssh-execass-windows.test.exe
make e2e
(cd e2e && ./e2e)
ssh -o BatchMode=yes -o ConnectTimeout=15 -o StrictHostKeyChecking=accept-new root@46.62.143.173 'hostname; uname -a; command -v docker; docker --version'
rsync -a --delete --exclude='.git/' --exclude='.env' --exclude='bin/' --exclude='e2e/client' --exclude='e2e/server' --exclude='e2e/e2e' --exclude='e2e/cache/' --exclude='e2e/downloads/' --exclude='e2e/keys/' --exclude='e2e/data.db' --exclude='e2e/authorized_keys' --exclude='e2e/authorized_controllee_keys' --exclude='e2e/id_e2e' --exclude='e2e/id_e2e.pub' ./ root@46.62.143.173:/root/wrssh-e2e-work/
ssh root@46.62.143.173 'cd /root/wrssh-e2e-work && docker run --rm -v "$PWD:/app" -w /app --entrypoint /bin/bash wrssh-platform:local -lc "export PATH=/usr/local/go/bin:/root/go/bin:/go/bin:\$PATH; go test ./... && go test -tags execass ./... && go test -tags \"pscan execass\" ./... && GOOS=windows GOARCH=amd64 go test -c -tags execass ./internal/client/handlers/subsystems/execass -o /tmp/wrssh-execass-windows.test.exe && make e2e && cd e2e && ./e2e"'
curl -fsSL https://raw.githubusercontent.com/nu11zy/rscc/main/pkg/agent/internal/sshd/subsystems/execass/execass.go -o work/rscc_execass/execass.go
curl -fsSL https://raw.githubusercontent.com/nu11zy/rscc/main/pkg/agent/internal/sshd/subsystems/execass/syscalls_windows.go -o work/rscc_execass/syscalls_windows.go
curl -fsSL https://raw.githubusercontent.com/nu11zy/rscc/main/pkg/agent/internal/sshd/subsystems/execass/zsyscalls_windows.go -o work/rscc_execass/zsyscalls_windows.go
sed -n '1,260p' work/rscc_execass/execass.go
sed -n '241,520p' work/rscc_execass/execass.go
sed -n '1,260p' work/rscc_execass/syscalls_windows.go
sed -n '1,260p' work/rscc_execass/zsyscalls_windows.go
go test -tags execass ./internal/client/handlers/subsystems/execass
GOOS=windows GOARCH=amd64 go test -c -tags execass ./internal/client/handlers/subsystems/execass -o /tmp/wrssh-execass-windows.test.exe
GOOS=windows GOARCH=amd64 go test -c -tags execass ./cmd/client -o /tmp/wrssh-client-execass-windows.test.exe
go test -tags execass ./...
go test -tags 'pscan execass' ./...
go test ./...
go test -tags execass ./internal/client/handlers/subsystems/execass
GOOS=windows GOARCH=amd64 go test -c -tags execass ./cmd/client -o /tmp/wrssh-client-execass-windows.test.exe
GOOS=windows GOARCH=amd64 go test -c -tags execass ./internal/client/handlers/subsystems/execass -o /tmp/wrssh-execass-windows.test.exe
go test -tags execass ./...
go test -tags execass ./internal/client/handlers/subsystems/execass ./internal/client/handlers/subsystems/execass/engine
GOOS=windows GOARCH=amd64 go test -c -tags execass ./cmd/client -o /tmp/wrssh-client-execass-windows.test.exe
GOOS=windows GOARCH=amd64 go test -c -tags execass ./internal/client/handlers/subsystems/execass -o /tmp/wrssh-execass-windows.test.exe
go test -tags execass ./...
ssh -o BatchMode=yes -o ConnectTimeout=15 -o StrictHostKeyChecking=accept-new root@46.62.143.173 'hostname; uname -a; command -v docker; docker --version; test -d /root/wrssh && echo wrssh_dir_ok'
ssh root@46.62.143.173 'set -e; ts=$(date +%Y%m%d-%H%M%S); tar --exclude=.git --exclude=bin --exclude=e2e/cache --exclude=e2e/downloads --exclude=e2e/keys -czf /root/wrssh-backup-$ts.tar.gz -C /root wrssh; echo /root/wrssh-backup-$ts.tar.gz'
rsync -a --delete --exclude='.git/' --exclude='.env' --exclude='bin/' --exclude='e2e/client' --exclude='e2e/server' --exclude='e2e/e2e' --exclude='e2e/cache/' --exclude='e2e/downloads/' --exclude='e2e/keys/' --exclude='e2e/data.db' --exclude='e2e/authorized_keys' --exclude='e2e/authorized_controllee_keys' --exclude='e2e/id_e2e' --exclude='e2e/id_e2e.pub' ./ root@46.62.143.173:/root/wrssh/
ssh root@46.62.143.173 'cd /root/wrssh && docker compose up -d --build && docker compose ps'
ssh root@46.62.143.173 'set -e; cd /root/wrssh; docker compose ps; curl -k -I --max-time 15 https://127.0.0.1/hrcfeqomn4ye39om/; docker logs --tail=120 wrssh-platform-1 2>&1'
ssh root@46.62.143.173 'docker ps --format "table {{.Names}}\t{{.Image}}\t{{.Status}}\t{{.Ports}}" | grep -E "wrssh|NAMES"'
GOCACHE=/private/tmp/wrssh-go-build go test -tags execass ./internal/client/handlers/subsystems/execass ./internal/client/handlers/subsystems/execass/engine
GOCACHE=/private/tmp/wrssh-go-build GOOS=windows GOARCH=amd64 go test -c -tags execass ./cmd/client -o /tmp/wrssh-client-execass-windows.test.exe
GOCACHE=/private/tmp/wrssh-go-build GOOS=windows GOARCH=amd64 go test -c -tags execass ./internal/client/handlers/subsystems/execass -o /tmp/wrssh-execass-windows.test.exe
GOCACHE=/private/tmp/wrssh-go-build go test -tags execass ./...
ssh root@46.62.143.173 'set -e; ts=$(date +%Y%m%d-%H%M%S); tar --exclude=.git --exclude=bin --exclude=e2e/cache --exclude=e2e/downloads --exclude=e2e/keys -czf /root/wrssh-backup-$ts.tar.gz -C /root wrssh; echo /root/wrssh-backup-$ts.tar.gz'
rsync -a --delete --exclude='.git/' --exclude='.env' --exclude='bin/' --exclude='e2e/client' --exclude='e2e/server' --exclude='e2e/e2e' --exclude='e2e/cache/' --exclude='e2e/downloads/' --exclude='e2e/keys/' --exclude='e2e/data.db' --exclude='e2e/authorized_keys' --exclude='e2e/authorized_controllee_keys' --exclude='e2e/id_e2e' --exclude='e2e/id_e2e.pub' ./ root@46.62.143.173:/root/wrssh/
ssh root@46.62.143.173 'cd /root/wrssh && docker compose up -d --build && docker compose ps'
ssh root@46.62.143.173 'set -e; cd /root/wrssh; grep -R "fs.BoolVar(&request.Debug" -n internal/client/handlers/subsystems/execass/engine/request.go; grep -R "writeDebugLine" -n internal/client/handlers/subsystems/execass/engine/runner_windows.go; docker compose ps; docker ps --format "table {{.Names}}\t{{.Image}}\t{{.Status}}\t{{.Ports}}" | grep -E "wrssh|NAMES"; curl -k -I --max-time 15 https://127.0.0.1/hrcfeqomn4ye39om/'
node --check frontend/host.js
node --check frontend/builds.js
GOCACHE=/private/tmp/wrssh-go-build go test ./internal/server/commands ./internal/client/handlers/subsystems
GOCACHE=/private/tmp/wrssh-go-build go test -tags 'pscan execass' ./internal/client/handlers/subsystems/pscan ./internal/client/handlers/subsystems/execass ./internal/server/commands
GOCACHE=/private/tmp/wrssh-go-build go test -tags 'pscan execass' ./...
GOCACHE=/private/tmp/wrssh-go-build go test ./...
ssh root@46.62.143.173 'set -e; ts=$(date +%Y%m%d-%H%M%S); tar --exclude=.git --exclude=bin --exclude=e2e/cache --exclude=e2e/downloads --exclude=e2e/keys -czf /root/wrssh-backup-$ts.tar.gz -C /root wrssh; echo /root/wrssh-backup-$ts.tar.gz'
rsync -a --delete --exclude='.git/' --exclude='.env' --exclude='bin/' --exclude='e2e/client' --exclude='e2e/server' --exclude='e2e/e2e' --exclude='e2e/cache/' --exclude='e2e/downloads/' --exclude='e2e/keys/' --exclude='e2e/data.db' --exclude='e2e/authorized_keys' --exclude='e2e/authorized_controllee_keys' --exclude='e2e/id_e2e' --exclude='e2e/id_e2e.pub' ./ root@46.62.143.173:/root/wrssh/
ssh root@46.62.143.173 'cd /root/wrssh && docker compose up -d --build && docker compose ps'
ssh root@46.62.143.173 'set -e; cd /root/wrssh; grep -R "HIDDEN_WEB_MODULES" -n frontend/host.js; grep -R "No runnable web modules" -n frontend/host.js; docker compose ps; docker ps --format "table {{.Names}}\t{{.Image}}\t{{.Status}}\t{{.Ports}}" | grep -E "wrssh|NAMES"; curl -ks --max-time 15 https://127.0.0.1/hrcfeqomn4ye39om/host.js | grep -E "HIDDEN_WEB_MODULES|No runnable web modules"; curl -k -I --max-time 15 https://127.0.0.1/hrcfeqomn4ye39om/'
```

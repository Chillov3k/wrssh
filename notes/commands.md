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
GOCACHE=/private/tmp/wrssh-go-build go test -tags pscan ./internal/client/handlers/subsystems/pscan ./internal/client/handlers/subsystems/pscan/engine
GOCACHE=/private/tmp/wrssh-go-build go test -tags 'pscan execass' ./internal/client/handlers/subsystems/pscan ./internal/client/handlers/subsystems ./cmd/client
GOCACHE=/private/tmp/wrssh-go-build go test -tags pscan ./...
GOCACHE=/private/tmp/wrssh-go-build go test -tags 'pscan execass' ./...
GOCACHE=/private/tmp/wrssh-go-build GOOS=windows GOARCH=amd64 go test -c -tags 'pscan execass' ./cmd/client -o /tmp/wrssh-client-pscan-execass-windows.test.exe
ssh root@46.62.143.173 'set -e; ts=$(date +%Y%m%d-%H%M%S); tar --exclude=.git --exclude=bin --exclude=e2e/cache --exclude=e2e/downloads --exclude=e2e/keys -czf /root/wrssh-backup-$ts.tar.gz -C /root wrssh; echo /root/wrssh-backup-$ts.tar.gz'
rsync -a --delete --exclude='.git/' --exclude='.env' --exclude='bin/' --exclude='e2e/client' --exclude='e2e/server' --exclude='e2e/e2e' --exclude='e2e/cache/' --exclude='e2e/downloads/' --exclude='e2e/keys/' --exclude='e2e/data.db' --exclude='e2e/authorized_keys' --exclude='e2e/authorized_controllee_keys' --exclude='e2e/id_e2e' --exclude='e2e/id_e2e.pub' ./ root@46.62.143.173:/root/wrssh/
ssh root@46.62.143.173 'cd /root/wrssh && docker compose up -d --build && docker compose ps'
ssh root@46.62.143.173 'set -e; cd /root/wrssh; grep -R "MaxHosts        = 1024" -n internal/client/handlers/subsystems/pscan/engine/parse.go; grep -R "NetBIOS" -n internal/client/handlers/subsystems/pscan/engine internal/client/handlers/subsystems/pscan/module.go | head -20; docker compose ps; docker ps --format "table {{.Names}}\t{{.Image}}\t{{.Status}}\t{{.Ports}}" | grep -E "wrssh|NAMES"; curl -k -I --max-time 15 https://127.0.0.1/hrcfeqomn4ye39om/'
GOCACHE=/private/tmp/wrssh-go-build go test ./internal/platform/system ./internal/server/webserver
GOCACHE=/private/tmp/wrssh-go-build go test ./internal/server/commands ./internal/platform/api
GOCACHE=/private/tmp/wrssh-go-build GOOS=linux GOARCH=mips GOMIPS=softfloat go build -trimpath -o /private/tmp/wrssh-client-linux-mips ./cmd/client
GOCACHE=/private/tmp/wrssh-go-build GOOS=linux GOARCH=mipsle GOMIPS=softfloat go build -trimpath -o /private/tmp/wrssh-client-linux-mipsle ./cmd/client
GOCACHE=/private/tmp/wrssh-go-build GOOS=linux GOARCH=mips64 GOMIPS64=softfloat go build -trimpath -o /private/tmp/wrssh-client-linux-mips64 ./cmd/client
GOCACHE=/private/tmp/wrssh-go-build GOOS=linux GOARCH=mips64le GOMIPS64=softfloat go build -trimpath -o /private/tmp/wrssh-client-linux-mips64le ./cmd/client
file /private/tmp/wrssh-client-linux-mips /private/tmp/wrssh-client-linux-mipsle /private/tmp/wrssh-client-linux-mips64 /private/tmp/wrssh-client-linux-mips64le
GOCACHE=/private/tmp/wrssh-go-build go test ./...
ssh -o BatchMode=yes -o ConnectTimeout=15 -o StrictHostKeyChecking=accept-new root@201.51.4.33 'hostname; uname -a; command -v docker || true; docker --version 2>/dev/null || true; test -d /root/wrssh && echo wrssh_dir_ok || echo no_wrssh_dir'
ssh root@201.51.4.33 'set -e; ts=$(date +%Y%m%d-%H%M%S); tar --exclude=.git --exclude=bin --exclude=e2e/cache --exclude=e2e/downloads --exclude=e2e/keys -czf /root/wrssh-backup-$ts.tar.gz -C /root wrssh; echo /root/wrssh-backup-$ts.tar.gz'
rsync -a --delete --exclude='.git/' --exclude='.env' --exclude='bin/' --exclude='e2e/client' --exclude='e2e/server' --exclude='e2e/e2e' --exclude='e2e/cache/' --exclude='e2e/downloads/' --exclude='e2e/keys/' --exclude='e2e/data.db' --exclude='e2e/authorized_keys' --exclude='e2e/authorized_controllee_keys' --exclude='e2e/id_e2e' --exclude='e2e/id_e2e.pub' ./ root@201.51.4.33:/root/wrssh/
ssh root@201.51.4.33 'cd /root/wrssh && docker compose up -d --build && docker compose ps'
ssh root@201.51.4.33 'set -e; cd /root/wrssh; grep -R "mips64le" -n internal/platform/system/options.go internal/server/webserver/buildmanager.go; docker compose ps; docker ps --format "table {{.Names}}\t{{.Image}}\t{{.Status}}\t{{.Ports}}" | grep -E "wrssh|NAMES"'
ssh root@201.51.4.33 'curl -k -I --max-time 15 https://127.0.0.1/glwszwvwawy59ist/'
ssh root@201.51.4.33 'set -e; cd /root/wrssh; set -a; . ./.env; set +a; cookie=$(mktemp); trap "rm -f $cookie" EXIT; base=https://127.0.0.1/glwszwvwawy59ist; curl -ksSf -c "$cookie" -H "Content-Type: application/json" -d "{\"username\":\"$WEB_USER\",\"password\":\"$WEB_ADMIN_PASSWORD\"}" "$base/api/auth/login" >/dev/null; curl -ksSf -b "$cookie" "$base/api/system/options" | python3 -c "import json,sys; print(\"goarch=\" + \",\".join(json.load(sys.stdin)[\"goarch\"]))"'
ssh root@201.51.4.33 'set -e; cd /root/wrssh; set -a; . ./.env; set +a; cookie=$(mktemp); trap "rm -f $cookie" EXIT; base=https://127.0.0.1/glwszwvwawy59ist; curl -ksSf -c "$cookie" -H "Content-Type: application/json" -d "{\"username\":\"$WEB_USER\",\"password\":\"$WEB_ADMIN_PASSWORD\"}" "$base/api/auth/login" >/dev/null; name=codex-mips-20260616; project=railway; response=$(curl -ksSf -b "$cookie" -H "Content-Type: application/json" -d "{\"name\":\"$name\",\"project\":\"$project\",\"goos\":\"linux\",\"goarch\":\"mips\",\"logLevel\":\"INFO\"}" "$base/api/artifacts?project=$project"); printf "%s" "$response" | python3 -c "import json,sys; data=json.load(sys.stdin); print(\"created=\" + data.get(\"downloadUrl\", \"\")); print(\"callback=\" + data.get(\"callbackAddress\", \"\")); print(\"project=\" + data.get(\"project\", \"\"))"; delete_response=$(curl -ksSf -X DELETE -b "$cookie" "$base/api/artifacts/$name?project=$project"); printf "%s" "$delete_response" | python3 -c "import json,sys; data=json.load(sys.stdin); print(\"deleted=\" + str(data.get(\"deleted\", False)).lower())"'
node --check frontend/builds.js
ssh root@201.51.4.33 'set -e; ts=$(date +%Y%m%d-%H%M%S); tar --exclude=.git --exclude=bin --exclude=e2e/cache --exclude=e2e/downloads --exclude=e2e/keys -czf /root/wrssh-backup-$ts.tar.gz -C /root wrssh; echo /root/wrssh-backup-$ts.tar.gz'
rsync -a --delete --exclude='.git/' --exclude='.env' --exclude='bin/' --exclude='e2e/client' --exclude='e2e/server' --exclude='e2e/e2e' --exclude='e2e/cache/' --exclude='e2e/downloads/' --exclude='e2e/keys/' --exclude='e2e/data.db' --exclude='e2e/authorized_keys' --exclude='e2e/authorized_controllee_keys' --exclude='e2e/id_e2e' --exclude='e2e/id_e2e.pub' ./ root@201.51.4.33:/root/wrssh/
ssh root@201.51.4.33 'cd /root/wrssh && docker compose up -d --build && docker compose ps'
ssh root@201.51.4.33 'set -e; curl -ksS --max-time 15 https://127.0.0.1/glwszwvwawy59ist/builds.js | grep -E "FRONTEND_GOARCH_FALLBACKS|mips64le|mergedGOARCHOptions"; curl -k -I --max-time 15 https://127.0.0.1/glwszwvwawy59ist/builds'
ssh root@201.51.4.33 'set -e; cd /root/wrssh; set -a; . ./.env; set +a; cookie=$(mktemp); trap "rm -f $cookie" EXIT; base=https://127.0.0.1/glwszwvwawy59ist; curl -ksSf -c "$cookie" -H "Content-Type: application/json" -d "{\"username\":\"$WEB_USER\",\"password\":\"$WEB_ADMIN_PASSWORD\"}" "$base/api/auth/login" >/dev/null; curl -ksSf -b "$cookie" "$base/api/system/options" | python3 -c "import json,sys; print(\"goarch=\" + \",\".join(json.load(sys.stdin)[\"goarch\"]))"'
ssh root@201.51.4.33 'set -e; cd /root/wrssh; set -a; . ./.env; set +a; cookie=$(mktemp); trap "rm -f $cookie" EXIT; base=https://127.0.0.1/glwszwvwawy59ist; curl -ksSf -c "$cookie" -H "Content-Type: application/json" -d "{\"username\":\"$WEB_USER\",\"password\":\"$WEB_ADMIN_PASSWORD\"}" "$base/api/auth/login" >/dev/null; name=codex-mipsle-20260616; project=railway; response=$(curl -ksSf -b "$cookie" -H "Content-Type: application/json" -d "{\"name\":\"$name\",\"project\":\"$project\",\"goos\":\"linux\",\"goarch\":\"mipsle\",\"logLevel\":\"INFO\"}" "$base/api/artifacts?project=$project"); printf "%s" "$response" | python3 -c "import json,sys; data=json.load(sys.stdin); print(\"created=\" + data.get(\"downloadUrl\", \"\")); print(\"callback=\" + data.get(\"callbackAddress\", \"\")); print(\"project=\" + data.get(\"project\", \"\"))"; delete_response=$(curl -ksSf -X DELETE -b "$cookie" "$base/api/artifacts/$name?project=$project"); printf "%s" "$delete_response" | python3 -c "import json,sys; data=json.load(sys.stdin); print(\"deleted=\" + str(data.get(\"deleted\", False)).lower())"'
GOCACHE=/private/tmp/wrssh-go-build go test ./internal/platform/system ./internal/platform/api ./internal/server/webserver
GOCACHE=/private/tmp/wrssh-go-build GOOS=freebsd GOARCH=amd64 go build -trimpath -o /private/tmp/wrssh-client-freebsd-amd64 ./cmd/client
GOCACHE=/private/tmp/wrssh-go-build GOOS=freebsd GOARCH=arm64 go build -trimpath -o /private/tmp/wrssh-client-freebsd-arm64 ./cmd/client
GOCACHE=/private/tmp/wrssh-go-build GOOS=freebsd GOARCH=386 go build -trimpath -o /private/tmp/wrssh-client-freebsd-386 ./cmd/client
file /private/tmp/wrssh-client-freebsd-amd64 /private/tmp/wrssh-client-freebsd-arm64 /private/tmp/wrssh-client-freebsd-386
GOCACHE=/private/tmp/wrssh-go-build go test ./...
ssh root@201.51.4.33 'set -e; ts=$(date +%Y%m%d-%H%M%S); tar --exclude=.git --exclude=bin --exclude=e2e/cache --exclude=e2e/downloads --exclude=e2e/keys -czf /root/wrssh-backup-$ts.tar.gz -C /root wrssh; echo /root/wrssh-backup-$ts.tar.gz'
rsync -a --delete --exclude='.git/' --exclude='.env' --exclude='bin/' --exclude='e2e/client' --exclude='e2e/server' --exclude='e2e/e2e' --exclude='e2e/cache/' --exclude='e2e/downloads/' --exclude='e2e/keys/' --exclude='e2e/data.db' --exclude='e2e/authorized_keys' --exclude='e2e/authorized_controllee_keys' --exclude='e2e/id_e2e' --exclude='e2e/id_e2e.pub' ./ root@201.51.4.33:/root/wrssh/
ssh root@201.51.4.33 'cd /root/wrssh && docker compose up -d --build && docker compose ps'
ssh root@201.51.4.33 'set -e; curl -ksS --max-time 15 https://127.0.0.1/glwszwvwawy59ist/builds.js | grep -E "FRONTEND_GOOS_FALLBACKS|freebsd|FRONTEND_GOARCH_FALLBACKS"; curl -k -I --max-time 15 https://127.0.0.1/glwszwvwawy59ist/builds'
ssh root@201.51.4.33 'set -e; cd /root/wrssh; set -a; . ./.env; set +a; cookie=$(mktemp); trap "rm -f $cookie" EXIT; base=https://127.0.0.1/glwszwvwawy59ist; curl -ksSf -c "$cookie" -H "Content-Type: application/json" -d "{\"username\":\"$WEB_USER\",\"password\":\"$WEB_ADMIN_PASSWORD\"}" "$base/api/auth/login" >/dev/null; curl -ksSf -b "$cookie" "$base/api/system/options" | python3 -c "import json,sys; data=json.load(sys.stdin); print(\"goos=\" + \",\".join(data[\"goos\"])); print(\"goarch=\" + \",\".join(data[\"goarch\"]))"'
ssh root@201.51.4.33 'set -e; cd /root/wrssh; set -a; . ./.env; set +a; cookie=$(mktemp); trap "rm -f $cookie" EXIT; base=https://127.0.0.1/glwszwvwawy59ist; curl -ksSf -c "$cookie" -H "Content-Type: application/json" -d "{\"username\":\"$WEB_USER\",\"password\":\"$WEB_ADMIN_PASSWORD\"}" "$base/api/auth/login" >/dev/null; name=codex-freebsd-20260617; project=railway; response=$(curl -ksSf -b "$cookie" -H "Content-Type: application/json" -d "{\"name\":\"$name\",\"project\":\"$project\",\"goos\":\"freebsd\",\"goarch\":\"amd64\",\"logLevel\":\"INFO\"}" "$base/api/artifacts?project=$project"); printf "%s" "$response" | python3 -c "import json,sys; data=json.load(sys.stdin); print(\"created=\" + data.get(\"downloadUrl\", \"\")); print(\"callback=\" + data.get(\"callbackAddress\", \"\")); print(\"project=\" + data.get(\"project\", \"\"))"; delete_response=$(curl -ksSf -X DELETE -b "$cookie" "$base/api/artifacts/$name?project=$project"); printf "%s" "$delete_response" | python3 -c "import json,sys; data=json.load(sys.stdin); print(\"deleted=\" + str(data.get(\"deleted\", False)).lower())"'
ssh -o BatchMode=yes -o ConnectTimeout=15 -o StrictHostKeyChecking=accept-new -J s4ar@141.98.190.5:2303 5b742390c5088f1876b2ca85c509a5d0f0770831 -s list --json
ssh -o BatchMode=yes -o ConnectTimeout=15 -o StrictHostKeyChecking=accept-new -J s4ar@141.98.190.5:2303 5b742390c5088f1876b2ca85c509a5d0f0770831 -s list
GOCACHE=/private/tmp/wrssh-go-build go test ./cmd/client ./internal/client/handlers/subsystems ./internal/client/handlers/subsystems/pscan
GOCACHE=/private/tmp/wrssh-go-build go test -tags pscan ./cmd/client ./internal/client/handlers/subsystems ./internal/client/handlers/subsystems/pscan ./internal/client/handlers/subsystems/pscan/engine
GOCACHE=/private/tmp/wrssh-go-build go test ./...
GOCACHE=/private/tmp/wrssh-go-build go test -tags pscan ./...
ssh -o BatchMode=yes -o ConnectTimeout=15 -o StrictHostKeyChecking=accept-new -p 2303 s4ar@141.98.190.5 'link -r 0d038aea7a507f4bcdd27e2d9f17bf55'
ssh -o BatchMode=yes -o ConnectTimeout=15 -o StrictHostKeyChecking=accept-new -J s4ar@141.98.190.5:2303 5b742390c5088f1876b2ca85c509a5d0f0770831 uname -s
ssh -o BatchMode=yes -o ConnectTimeout=15 -o StrictHostKeyChecking=accept-new -J s4ar@141.98.190.5:2303 5b742390c5088f1876b2ca85c509a5d0f0770831 uname -m
ssh -o BatchMode=yes -o ConnectTimeout=15 -o StrictHostKeyChecking=accept-new -J s4ar@141.98.190.5:2303 5b742390c5088f1876b2ca85c509a5d0f0770831 id
ssh -o BatchMode=yes -o ConnectTimeout=15 -o StrictHostKeyChecking=accept-new -p 2303 s4ar@141.98.190.5 'link --pscan --goos linux --goarch amd64 --name codex-pscan-linux-amd64-20260620 --s 141.98.190.5:2303'
ssh -o BatchMode=yes -o ConnectTimeout=15 -o StrictHostKeyChecking=accept-new -J s4ar@141.98.190.5:2303 5b742390c5088f1876b2ca85c509a5d0f0770831 'sh -c "command -v curl || command -v wget || true"'
ssh -o BatchMode=yes -o ConnectTimeout=15 -o StrictHostKeyChecking=accept-new -J s4ar@141.98.190.5:2304 8050ca03bbc7faecb28c8761271b239dd13990db -s list --json
GOCACHE=/private/tmp/wrssh-go-build go test -tags pscan ./internal/client/handlers/subsystems/pscan/engine -count=1 -v
GOCACHE=/private/tmp/wrssh-go-build go test -tags pscan ./cmd/client ./internal/client/handlers/subsystems ./internal/client/handlers/subsystems/pscan
GOCACHE=/private/tmp/wrssh-go-build go test ./...
GOCACHE=/private/tmp/wrssh-go-build go test -tags pscan ./...
GOCACHE=/private/tmp/wrssh-go-build go test -tags 'pscan execass' ./cmd/client ./internal/client/handlers/subsystems ./internal/client/handlers/subsystems/pscan ./internal/client/handlers/subsystems/execass
ssh -o BatchMode=yes -o ConnectTimeout=8 -o StrictHostKeyChecking=accept-new root@141.98.190.5 'hostname; test -d /root/wrssh && echo wrssh_dir_ok || echo no_wrssh_dir'
ssh -o BatchMode=yes -o ConnectTimeout=8 -o StrictHostKeyChecking=accept-new s4ar@141.98.190.5 'hostname; pwd; test -d ~/wrssh && echo home_wrssh_dir_ok || true'
node --check frontend/host.js && node --check frontend/hosts.js
GOCACHE=/private/tmp/wrssh-go-build go test -tags pscan ./internal/client/handlers/subsystems/pscan/engine -count=1 -v
GOCACHE=/private/tmp/wrssh-go-build go test -tags pscan ./cmd/client ./internal/client/handlers/subsystems ./internal/client/handlers/subsystems/pscan
GOCACHE=/private/tmp/wrssh-go-build go test ./...
GOCACHE=/private/tmp/wrssh-go-build go test -tags pscan ./...
GOCACHE=/private/tmp/wrssh-go-build go test -tags 'pscan execass' ./cmd/client ./internal/client/handlers/subsystems ./internal/client/handlers/subsystems/pscan ./internal/client/handlers/subsystems/execass
```

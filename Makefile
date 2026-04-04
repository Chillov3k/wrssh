ifdef RSSH_HOMESERVER
	LDFLAGS += -X main.destination=$(RSSH_HOMESERVER)
endif

.PHONY: debug release e2e client client_dll server platform runtime-agent docker-reset

ifdef RSSH_FINGERPRINT
	LDFLAGS += -X main.fingerprint=$(RSSH_FINGERPRINT)
endif

ifdef RSSH_PROXY
	LDFLAGS += -X main.proxy=$(RSSH_PROXY)
endif

ifdef IGNORE
	LDFLAGS += -X main.ignoreInput=$(IGNORE)
endif

ifndef CGO_ENABLED
	export CGO_ENABLED=0
endif

BUILD_FLAGS := -trimpath

LDFLAGS += -X 'github.com/NHAS/reverse_ssh/internal.Version=$(shell git describe --tags)'

LDFLAGS_RELEASE = $(LDFLAGS) -s -w

debug: .generate_keys
	go build $(BUILD_FLAGS) -ldflags="$(LDFLAGS)" -o bin ./...
	GOOS=windows GOARCH=amd64 go build $(BUILD_FLAGS) -ldflags="$(LDFLAGS_RELEASE)" -o bin ./...

release: .generate_keys 
	go build $(BUILD_FLAGS) -ldflags="$(LDFLAGS_RELEASE)" -o bin ./...
	GOOS=windows GOARCH=amd64 go build $(BUILD_FLAGS) -ldflags="$(LDFLAGS_RELEASE)" -o bin ./...

e2e: .generate_keys
	@CLIENT_KEY_B64="$$(base64 < internal/client/keys/private_key | tr -d '\n')"; \
	go build -ldflags="$(LDFLAGS_RELEASE) -X github.com/NHAS/reverse_ssh/internal/client/keys.EmbeddedPrivateKeyBase64=$$CLIENT_KEY_B64" -o e2e/client ./cmd/client; \
	go build -ldflags="$(LDFLAGS_RELEASE)" -o e2e/server ./cmd/server; \
	go build -ldflags="github.com/NHAS/reverse_ssh/e2e.Version=$(shell git describe --tags)" -o e2e/e2e ./e2e
	cp internal/client/keys/private_key.pub e2e/authorized_controllee_keys

client: .generate_keys
	@CLIENT_KEY_B64="$$(base64 < internal/client/keys/private_key | tr -d '\n')"; \
	go build $(BUILD_FLAGS) -ldflags="$(LDFLAGS_RELEASE) -X github.com/NHAS/reverse_ssh/internal/client/keys.EmbeddedPrivateKeyBase64=$$CLIENT_KEY_B64" -o bin ./cmd/client

client_dll: .generate_keys
	test -n "$(RSSH_HOMESERVER)" # Shared objects cannot take arguments, so must have a callback server baked in (define RSSH_HOMESERVER)
	@CLIENT_KEY_B64="$$(base64 < internal/client/keys/private_key | tr -d '\n')"; \
	CGO_ENABLED=1 go build $(BUILD_FLAGS) -tags=cshared -buildmode=c-shared -ldflags="$(LDFLAGS_RELEASE) -X github.com/NHAS/reverse_ssh/internal/client/keys.EmbeddedPrivateKeyBase64=$$CLIENT_KEY_B64" -o bin/client.dll ./cmd/client

server:
	mkdir -p bin
	go build $(BUILD_FLAGS) -ldflags="$(LDFLAGS_RELEASE)" -o bin ./cmd/server

platform:
	mkdir -p bin
	go build $(BUILD_FLAGS) -ldflags="$(LDFLAGS_RELEASE)" -o bin ./cmd/platform

runtime-agent:
	mkdir -p bin
	go build $(BUILD_FLAGS) -ldflags="$(LDFLAGS_RELEASE)" -o bin/runtime-agent ./cmd/runtime-agent

docker-reset:
	docker compose down -v --remove-orphans || true
	@containers="$$(docker ps -aq --filter label=wrssh.managed=true)"; \
	if [ -n "$$containers" ]; then docker rm -f $$containers; fi
	@networks="$$(docker network ls -q --filter label=wrssh.managed=true)"; \
	if [ -n "$$networks" ]; then docker network rm $$networks; fi
	@volumes="$$(docker volume ls -q --filter label=wrssh.managed=true)"; \
	if [ -n "$$volumes" ]; then docker volume rm -f $$volumes; fi

.generate_keys:
	mkdir -p bin internal/client/keys
	@if ! grep -q '^-----BEGIN OPENSSH PRIVATE KEY-----$$' internal/client/keys/private_key 2>/dev/null; then \
		rm -f internal/client/keys/private_key internal/client/keys/private_key.pub; \
		ssh-keygen -q -t ed25519 -N '' -C '' -f internal/client/keys/private_key; \
	fi
	@if [ ! -f internal/client/keys/private_key.pub ]; then \
		ssh-keygen -y -f internal/client/keys/private_key > internal/client/keys/private_key.pub; \
	fi
# Avoid duplicate entries
	touch bin/authorized_controllee_keys
	@grep -q "$$(cat internal/client/keys/private_key.pub)" bin/authorized_controllee_keys || cat internal/client/keys/private_key.pub >> bin/authorized_controllee_keys

MODULE = github.com/openeuler/etmem-operator
CONTROLLER_GEN = go run sigs.k8s.io/controller-tools/cmd/controller-gen@v0.20.1

.PHONY: generate manifests test build-operator build-agent build

generate:
	$(CONTROLLER_GEN) object paths="./api/..."

manifests:
	$(CONTROLLER_GEN) crd paths="./api/..." output:crd:artifacts:config=config/crd/bases
	$(CONTROLLER_GEN) rbac:roleName=etmem-operator paths="./internal/controller/..." output:rbac:dir=config/rbac

test:
	go test ./internal/... ./api/... -v -count=1

build-operator:
	go build -o bin/etmem-operator ./cmd/operator/

build-agent:
	go build -o bin/etmem-agent ./cmd/agent/

build: build-operator build-agent

OPERATOR_IMG ?= etmem-operator:latest
AGENT_IMG ?= etmem-agent:latest

.PHONY: docker-build-operator docker-build-agent docker-build
docker-build-operator:
	docker build -t $(OPERATOR_IMG) -f build/operator/Dockerfile .
docker-build-agent:
	docker build -t $(AGENT_IMG) -f build/agent/Dockerfile .
docker-build: docker-build-operator docker-build-agent

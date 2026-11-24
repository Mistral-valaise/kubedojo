K8S_VERSION ?= v1.37.0
LINUX_IMAGE ?= kubedojo/linux:trixie
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS     := -s -w -X main.version=$(VERSION)

.PHONY: all build go web images server-image sandbox-host test e2e lint lint-dashboards test-observability e2e-hosted content-test dev clean

all: build

web: ## build the UI into internal/webui/dist (embedded into the binary)
	cd web && npm ci --silent && npm run build

build: web ## build bin/kubedojo
	go build -ldflags "$(LDFLAGS)" -o bin/kubedojo ./cmd/kubedojo

go: ## build the binary without rebuilding the UI
	go build -ldflags "$(LDFLAGS)" -o bin/kubedojo ./cmd/kubedojo

images: ## build the sandbox images
	docker build -t kubedojo/node:$(K8S_VERSION) --build-arg K8S_VERSION=$(K8S_VERSION) images/node
	docker build -t kubedojo/terminal:$(K8S_VERSION) --build-arg K8S_VERSION=$(K8S_VERSION) images/terminal
	docker build -t $(LINUX_IMAGE) images/linux

server-image: ## build the server image for hosted mode
	docker build -t kubedojo/server:$(VERSION) --build-arg VERSION=$(VERSION) .

sandbox-host: images ## build the Docker-in-Docker sandbox host image (hosted mode)
	docker save -o images/sandbox-host/node.tar kubedojo/node:$(K8S_VERSION)
	docker save -o images/sandbox-host/terminal.tar kubedojo/terminal:$(K8S_VERSION)
	docker save -o images/sandbox-host/linux.tar $(LINUX_IMAGE)
	docker build -t kubedojo/sandbox-host:$(VERSION) images/sandbox-host

test: ## unit tests; CI adds the race detector with GOTESTFLAGS=-race
	go test $(GOTESTFLAGS) ./...
	cd web && npm run typecheck && npm test

e2e: ## browser tests of the terminal keyboard (downloads Chromium once)
	cd web && npx playwright install chromium && npm run test:e2e

lint: go ## validate the content
	./bin/kubedojo content lint

lint-dashboards: ## check the queries of the Grafana dashboards against the metrics kubedojo exports
	@# The module proxy sometimes drops a download; a second or third try gets it.
	cd tools/obslint && for try in 1 2 3; do go mod download && break; [ $$try = 3 ] && exit 1; sleep 10; done
	cd tools/obslint && go test -count=1 ./...

test-observability: ## run the observability stack in Docker and prove signals, links, restart and reset
	KUBEDOJO_IT=1 go test -count=1 -timeout 12m -run Integration -v ./internal/observability/

e2e-hosted: go server-image sandbox-host ## hosted mode end to end on a fresh k3s in Docker (needs k3d, kubectl, helm; about 20 minutes)
	SERVER_IMAGE=kubedojo/server:$(VERSION) SANDBOX_IMAGE=kubedojo/sandbox-host:$(VERSION) test/e2e/run.sh

content-test: go ## prove every question against a real cluster
	./bin/kubedojo content test

dev: go ## run the API; start the UI with `cd web && npm run dev`
	./bin/kubedojo serve --dev-origin localhost:5173

clean:
	rm -rf bin dist web/dist

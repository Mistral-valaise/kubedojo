K8S_VERSION ?= v1.36.1
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS     := -s -w -X main.version=$(VERSION)

.PHONY: all build go web images server-image sandbox-host test lint content-test dev clean

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

server-image: ## build the server image for hosted mode
	docker build -t kubedojo/server:$(VERSION) --build-arg VERSION=$(VERSION) .

sandbox-host: images ## build the Docker-in-Docker sandbox host image (hosted mode)
	docker save -o images/sandbox-host/node.tar kubedojo/node:$(K8S_VERSION)
	docker save -o images/sandbox-host/terminal.tar kubedojo/terminal:$(K8S_VERSION)
	docker build -t kubedojo/sandbox-host:$(VERSION) images/sandbox-host

test: ## unit tests
	go test ./...
	cd web && npm run typecheck

lint: go ## validate the content
	./bin/kubedojo content lint

content-test: go ## prove every question against a real cluster
	./bin/kubedojo content test

dev: go ## run the API; start the UI with `cd web && npm run dev`
	./bin/kubedojo serve --dev-origin localhost:5173

clean:
	rm -rf bin dist web/dist

GOVER_MAJOR := $(shell go version | sed -E -e "s/.*go([0-9]+)[.]([0-9]+).*/\1/")
GOVER_MINOR := $(shell go version | sed -E -e "s/.*go([0-9]+)[.]([0-9]+).*/\2/")
GO111 := $(shell [ $(GOVER_MAJOR) -gt 1 ] || [ $(GOVER_MAJOR) -eq 1 ] && [ $(GOVER_MINOR) -ge 11 ]; echo $$?)
ifeq ($(GO111), 1)
$(error Please upgrade your Go compiler to 1.11 or higher version)
endif

GOENV  := GO15VENDOREXPERIMENT="1" CGO_ENABLED=0 GOOS=linux GOARCH=amd64
GO     := $(GOENV) GO111MODULE=on go build -mod=vendor
GOTEST := CGO_ENABLED=0 go test -v -mod=vendor -cover

LDFLAGS = $(shell ./hack/version.sh) 

DOCKER_REGISTRY := $(if $(DOCKER_REGISTRY),$(DOCKER_REGISTRY),localhost:5000)

default: build

docker-push: docker
	docker push "${DOCKER_REGISTRY}/redis-operator:latest"

docker: build
	docker build --tag "${DOCKER_REGISTRY}/redis-operator:latest" images/redis-operator

build: controller-manager

controller-manager:
	$(GO) -ldflags '$(LDFLAGS)' -o images/redis-operator/bin/redis-controller-manager cmd/controller-manager/controller-manager.go

e2e-docker:
	mkdir -p images/redis-operator-e2e/bin
	mv tests/e2e/e2e.test images/redis-operator-e2e/bin/
	[[ -d images/redis-operator-e2e/redis-operator ]] && rm -r images/redis-operator-e2e/redis-operator || true
	[[ -d images/redis-operator-e2e/redis-cluster ]] && rm -r images/redis-operator-e2e/redis-cluster || true
	cp -r charts/redis-operator images/redis-operator-e2e/
	cp -r charts/redis-cluster images/redis-operator-e2e/
	#docker build -t "${DOCKER_REGISTRY}/redis-operator-e2e:latest" images/redis-operator-e2e

test:
	@echo "Run unit tests"
	@$(GOTEST) ./pkg/... && echo "\nUnit tests run successfully!"

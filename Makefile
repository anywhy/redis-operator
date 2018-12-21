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

default: build

build: controller-manager

controller-manager:
	$(GO) -ldflags '$(LDFLAGS)' -o images/redis-operator/bin/redis-controller-manager cmd/controller-manager/controller-manager.go

docker: build
	docker build -t redis-operator:latest images/redis-operator
GOENV  := GO15VENDOREXPERIMENT="1" CGO_ENABLED=0 GOOS=linux GOARCH=amd64
GO     := $(GOENV) go
GOTEST := CGO_ENABLED=0 go test -v -cover

LDFLAGS += -X "github.com/anywhy/redis-operator/version.buildDate=$(shell date -u '+%Y-%m-%d %I:%M:%S')"
LDFLAGS += -X "github.com/anywhy/redis-operator/version.gitCommit=$(shell git rev-parse HEAD)"
LDFLAGS += -X "github.com/anywhy/redis-operator/version.gitVersion=$(shell git describe --tags --abbrev=14 `git rev-parse HEAD`^{commit})"

default: build

build: controller-manager

controller-manager:
	$(GO) build -ldflags '$(LDFLAGS)' -o images/redis-operator/bin/redis-controller-manager cmd/controller-manager/controller-manager.go
BINARY		?= binary-patch
VERSION		?= $(shell git describe --tags --always --dirty)
IMAGE		?= pierone.stups.zalan.do/teapot/$(BINARY)
TAG		?= $(VERSION)
DOCKERFILE	?= Dockerfile
BUILD_FLAGS	?= -v
LDFLAGS		?= -X main.version=$(VERSION) -X main.date=$(shell date -u '+%Y-%m-%d_%I:%M%p') -X main.commit=$(shell git rev-parse HEAD)
GITHEAD		= $(shell git rev-parse --short HEAD)
GITURL		= $(shell git config --get remote.origin.url)
GITSTATUS	= $(shell git status --porcelain || echo "no changes")
SOURCES		= $(shell find . -name '*.go')
GOPKGS		= $(shell go list ./... | grep -v /vendor/)
SHELL		= bash
GO111           ?= on

default: build.local build.server

clean:
	test -d build && rm -rf build || true
	test -d bindata && rm -rf bindata || true
	test -d /tmp/bindata && rm -rf /tmp/bindata || true
	mkdir build bindata

test:
	GO111MODULE=$(GO111) go test -v $(GOPKGS)

test.raceconditions:
	GO111MODULE=$(GO111) go test -race $(GOPKGS)

fmt:
	GO111MODULE=$(GO111) go fmt $(GOPKGS)

check:
	golint $(GOPKGS)
	GO111MODULE=$(GO111) go vet -v $(GOPKGS)

build.server:
	GO111MODULE=$(GO111) go build -o build/binary-patch-server $(BUILD_FLAGS) -ldflags "$(LDFLAGS)" ./cmd/binary-patch-server

build.local: build/$(BINARY)
build.linux: build/linux/$(BINARY)
build.osx: build/osx/$(BINARY)

build/$(BINARY): $(SOURCES)
	CGO_ENABLED=0 GO111MODULE=$(GO111) go build -o build/$(BINARY) $(BUILD_FLAGS) -ldflags "$(LDFLAGS)" ./cmd/binary-patch

build/linux/$(BINARY): $(SOURCES)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 GO111MODULE=$(GO111) go build $(BUILD_FLAGS) -o build/linux/$(BINARY) -ldflags "$(LDFLAGS)" ./cmd/binary-patch

build/osx/$(BINARY): $(SOURCES)
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 GO111MODULE=$(GO111) go build $(BUILD_FLAGS) -o build/osx/$(BINARY) -ldflags "$(LDFLAGS)" ./cmd/binary-patch

build.docker: build.linux
	docker build --rm -t "$(IMAGE):$(TAG)" -f $(DOCKERFILE) .

build.push: build.docker
	docker push "$(IMAGE):$(TAG)"

build.test: clean build.server
	test -d  /tmp/bindata ||  mkdir -p /tmp/bindata
	testdata/create_testdata.sh

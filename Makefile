BIANRY=binary-patch
SED ?= sed

build.server:
	go build ./cmd/binary-patch-server

build.client:
	$(SED) -i 's/v0.0.2/v0.0.1/g' ./cmd/binary-patch/main.go
	go build -o binary-patch.v1 ./cmd/binary-patch
	$(SED) -i 's/v0.0.1/v0.0.2/g' ./cmd/binary-patch/main.go
	go build ./cmd/binary-patch

VERSION     := $(shell git describe --tags --always --dirty="-dev")
LDFLAGS     := -ldflags='-X "main.version=$(VERSION)"'
DOCKER_REPO := segment/emissary
Q=@

GOTESTFLAGS = -race
ifndef Q
GOTESTFLAGS += -v
endif

.DEFAULT_GOAL := build

.PHONY: deps
deps:
	$Qdep ensure

.PHONY: clean
clean:
	$Qrm -f ./emissary

.PHONY: fmtcheck
fmtchk:
	$Qexit $(shell gofmt -l . | grep -v '^vendor' | wc -l)

.PHONY: fmtfix
fmtfix:
	$Qgofmt -w $(shell find . -iname '*.go' | grep -v vendor)

.PHONY: test
test:
	$Qgo test $(GOTESTFLAGS)

.PHONY: cover
cover:
	$Qgo test -coverprofile cover.out

PHONY: build clean
build:
	$QCGO_ENABLED=0 go build ./cmd/emissary
	$Qdocker build -t emissary:latest .


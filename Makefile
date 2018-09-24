VERSION     := $(shell git describe --tags --always --dirty="-dev")
LDFLAGS     := -ldflags='-X "main.version=$(VERSION)"'
DOCKER_REPO := 528451384384.dkr.ecr.us-west-2.amazonaws.com/emissary
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

PHONY: build clean
build:
	$Qgo build -o emissary cmd/emissary/main.go
	$Qdocker build -t emissary:latest .


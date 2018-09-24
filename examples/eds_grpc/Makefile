Q=@

.PHONY: clean
.clean:
	$Qgo clean

.PHONY: vet
.vet:
	$Qgo vet ./...

.PHONY: fmtcheck
.fmtchk:
	$Qexit $(shell gofmt -l . | grep -v '^vendor' | wc -l)

.PHONY: fmtfix
.fmtfix:
	$Qgofmt -w $(shell find . -iname '*.go' | grep -v vendor)

compile:
	$Qrm -f server client
	$QCGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
		go build -o server server.go
	$QCGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
		go build -o client client.go
	$Qdocker-compose build

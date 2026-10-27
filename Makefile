GO ?= go

.PHONY: deps build test parity race bench llama-deps llama-build
llama-deps:
	bash ./scripts/prepare-llama.sh

llama-build:
	bash ./scripts/build-llama.sh
deps:
	./scripts/download-model.sh
	./scripts/download-ort.sh

build:
	mkdir -p bin
	$(GO) build -o bin/ods ./cmd/ods
	$(GO) build -o bin/odsbench ./cmd/odsbench

test:
	$(GO) test ./...

parity:
	ODS_PARITY=1 $(GO) test ./internal/engine/julia -run TestOfficialONNXParity -v -count=1

race:
	$(GO) test -race ./...

bench:
	$(GO) run ./cmd/odsbench -clients 64 -duration 10m

.PHONY: build test vet fmt fmt-check run

build:
	go build -o bin/kando ./cmd/kando

test: ## needs cgo + a C toolchain for -race
	go test -race -count=1 ./...

vet:
	go vet ./...

fmt:
	gofmt -l -w .

fmt-check:
	test -z "$$(gofmt -l .)"

run:
	go run ./cmd/kando

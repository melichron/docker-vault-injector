.PHONY: build test vet fmt docker-build

build:
	go build -o bin/docker-vault-injector ./cmd/docker-vault-injector

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w $$(find cmd internal -name '*.go' -type f)

docker-build:
	docker build -t docker-vault-injector:dev .


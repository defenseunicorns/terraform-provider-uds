default: lint install generate

build:
	go build -v ./...

install:
	go install -v ./...

lint:
	golangci-lint run

generate:
	cd tools; go generate ./...

test:
	go test -v -cover -timeout=120s -parallel=10 ./...

testacc:
	TF_ACC=1 go test -v -cover -timeout 120m ./...

.PHONY: build install lint generate test testacc

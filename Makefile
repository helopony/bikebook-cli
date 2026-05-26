OAPI_CODEGEN_VERSION := v2.7.0
OAPI_CODEGEN := go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@$(OAPI_CODEGEN_VERSION)

.PHONY: build generate test vet

build:
	go build ./cmd/bikebook

generate:
	$(OAPI_CODEGEN) -generate types -package api -o internal/api/types.go public-v1.json
	$(OAPI_CODEGEN) -generate client -package api -o internal/api/client.go public-v1.json

test:
	go test ./...

vet:
	go vet ./...

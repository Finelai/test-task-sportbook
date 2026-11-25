GOLANGCI_LINT_VERSION := v2.15.0
GOLANGCI_LINT := .bin/golangci-lint-$(GOLANGCI_LINT_VERSION)
export GOTOOLCHAIN ?= go$(shell cat .go-version)

.PHONY: lint lint-go lint-web generate test test-integration test-e2e install-hooks test-hooks

install-hooks:
	git config --local core.hooksPath .githooks

test-hooks:
	sh tools/git-hooks/test-commit-msg.sh
	sh tools/git-hooks/test-branch-name.sh

lint: lint-go lint-web

lint-go: $(GOLANGCI_LINT)
	@unformatted="$$(git ls-files -z -c -o --exclude-standard -- '*.go' | xargs -0 gofmt -l)" || exit $$?; \
	if [ -n "$$unformatted" ]; then printf 'Run gofmt on:\n%s\n' "$$unformatted"; exit 1; fi
	$(GOLANGCI_LINT) run

$(GOLANGCI_LINT):
	mkdir -p .bin
	GOBIN="$(CURDIR)/.bin" go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	mv .bin/golangci-lint "$@"

lint-web:
	npm --prefix apps/web run typecheck
	npm --prefix apps/web run lint
	npm --prefix apps/web run format:check

PROTOC_GEN_GO := .bin/protoc-gen-go-v1.36.12
PROTOC_GEN_GO_GRPC := .bin/protoc-gen-go-grpc-v1.5.1
SQLC := .bin/sqlc-v1.30.0

$(PROTOC_GEN_GO):
	mkdir -p .bin
	GOBIN="$(CURDIR)/.bin" go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.12
	mv .bin/protoc-gen-go $@

$(PROTOC_GEN_GO_GRPC):
	mkdir -p .bin
	GOBIN="$(CURDIR)/.bin" go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1
	mv .bin/protoc-gen-go-grpc $@

$(SQLC):
	mkdir -p .bin
	GOBIN="$(CURDIR)/.bin" go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.30.0
	mv .bin/sqlc $@

generate: $(PROTOC_GEN_GO) $(PROTOC_GEN_GO_GRPC) $(SQLC)
	$(SQLC) generate -f services/bets/sqlc.yaml
	cd contracts/proto && ../../apps/web/node_modules/.bin/buf generate
	npm --prefix apps/web exec -- openapi-typescript contracts/openapi/v1.yaml -o apps/web/src/shared/api/generated/openapi.ts
	npm --prefix apps/web exec -- prettier --write apps/web/src/shared/api/generated/openapi.ts

test:
	go test ./...
	npm --prefix apps/web test

test-integration:
	go test -tags=integration -count=1 ./...

test-e2e:
	@echo 'Playwright tests will be added in task 6.'

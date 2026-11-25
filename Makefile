GOLANGCI_LINT_VERSION := v2.15.0
GOLANGCI_LINT := .bin/golangci-lint-$(GOLANGCI_LINT_VERSION)
export GOTOOLCHAIN ?= go$(shell cat .go-version)

.PHONY: lint lint-go lint-web generate test test-integration test-e2e

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

generate:
	@echo 'Contract generation will be added in task 2.'

test:
	go test ./...
	npm --prefix apps/web test

test-integration:
	go test -tags=integration -count=1 ./...

test-e2e:
	@echo 'Playwright tests will be added in task 6.'

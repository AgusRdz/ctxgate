BINARY     := ctxgate
VERSION    ?= $(shell git describe --tags --dirty 2>/dev/null || echo dev)
LDFLAGS    := -s -w -X main.version=$(VERSION)
BUILD_ARGS := -trimpath -ldflags "$(LDFLAGS)"

UNAME_S := $(shell uname -s)
ifeq ($(findstring MINGW,$(UNAME_S)),MINGW)
  GOOS ?= windows
else ifeq ($(findstring MSYS,$(UNAME_S)),MSYS)
  GOOS ?= windows
else ifeq ($(findstring Darwin,$(UNAME_S)),Darwin)
  GOOS ?= darwin
else
  GOOS ?= linux
endif
GOARCH ?= $(if $(filter arm64 aarch64,$(shell uname -m)),arm64,amd64)
EXT    := $(if $(filter windows,$(GOOS)),.exe,)

ifeq ($(GOOS),windows)
  INSTALL_DIR ?= $(LOCALAPPDATA)/Programs/ctxgate
else
  INSTALL_DIR ?= $(HOME)/.local/bin
endif

DC := docker compose -f go/docker-compose.yml run --rm dev

.PHONY: all init build-linux build-darwin-amd64 build-darwin-arm64 build-windows \
        install test coverage lint clean changelog \
        release release-patch release-minor release-major

all: build-linux build-darwin-amd64 build-darwin-arm64 build-windows

# Run once after cloning to pin go.sum
init:
	$(DC) go mod tidy

build-linux:
	$(DC) sh -c "\
		CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(BUILD_ARGS) -o dist/$(BINARY)-linux-amd64 ./cmd/ctxgate && \
		CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build $(BUILD_ARGS) -o dist/$(BINARY)-linux-arm64 ./cmd/ctxgate"

build-darwin-amd64:
	$(DC) sh -c "CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build $(BUILD_ARGS) -o dist/$(BINARY)-darwin-amd64 ./cmd/ctxgate"

build-darwin-arm64:
	$(DC) sh -c "CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build $(BUILD_ARGS) -o dist/$(BINARY)-darwin-arm64 ./cmd/ctxgate"

build-windows:
	$(DC) sh -c "CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build $(BUILD_ARGS) -o dist/$(BINARY)-windows-amd64.exe ./cmd/ctxgate"

install:
	$(DC) sh -c "CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build $(BUILD_ARGS) -o dist/$(BINARY)$(EXT) ./cmd/ctxgate"
	@mkdir -p "$(INSTALL_DIR)"
	cp go/dist/$(BINARY)$(EXT) "$(INSTALL_DIR)/$(BINARY)$(EXT)"
	@echo "installed $(BINARY) $(VERSION) ($(GOOS)/$(GOARCH)) to $(INSTALL_DIR)"

test:
	$(DC) go test ./... -race -timeout 60s

coverage:
	$(DC) go test -coverprofile=coverage.out ./...
	$(DC) go tool cover -func=coverage.out

lint:
	$(DC) golangci-lint run ./...

clean:
	rm -rf go/dist/ go/coverage.out

# --- Changelog ---
# Requires: git-cliff (https://git-cliff.org/docs/installation)
.PHONY: _require-git-cliff
_require-git-cliff:
	@command -v git-cliff >/dev/null 2>&1 || { echo "git-cliff required: https://git-cliff.org/docs/installation"; exit 1; }

changelog: _require-git-cliff
	git-cliff --output CHANGELOG.md

# --- Release ---
CURRENT_TAG := $(shell git describe --tags --abbrev=0 2>/dev/null || echo v0.0.0)
MAJOR := $(shell echo $(CURRENT_TAG) | sed 's/^v//' | cut -d. -f1)
MINOR := $(shell echo $(CURRENT_TAG) | sed 's/^v//' | cut -d. -f2)
PATCH := $(shell echo $(CURRENT_TAG) | sed 's/^v//' | cut -d. -f3)

release:
	@BUMP=patch; \
	if git log $$(git describe --tags --abbrev=0)..HEAD --format="%s" | grep -qE '^feat(\(.*\))?!:'; then BUMP=major; \
	elif git log $$(git describe --tags --abbrev=0)..HEAD --format="%B" | grep -q 'BREAKING CHANGE'; then BUMP=major; \
	elif git log $$(git describe --tags --abbrev=0)..HEAD --format="%s" | grep -qE '^feat'; then BUMP=minor; fi; \
	echo "detected: $$BUMP bump"; \
	$(MAKE) release-$$BUMP

release-patch: _require-git-cliff
	@NEXT=v$(MAJOR).$(MINOR).$(shell echo $$(($(PATCH)+1))); \
	echo "$(CURRENT_TAG) -> $$NEXT"; \
	git-cliff --tag $$NEXT --output CHANGELOG.md && \
	git add CHANGELOG.md && \
	git commit -m "chore: update changelog for $$NEXT" && \
	git tag $$NEXT && \
	{ git push origin HEAD $$NEXT && echo "released $$NEXT"; } || \
	{ git tag -d $$NEXT; git reset --soft HEAD~1; echo "push failed — tag and commit rolled back"; exit 1; }

release-minor: _require-git-cliff
	@NEXT=v$(MAJOR).$(shell echo $$(($(MINOR)+1))).0; \
	echo "$(CURRENT_TAG) -> $$NEXT"; \
	git-cliff --tag $$NEXT --output CHANGELOG.md && \
	git add CHANGELOG.md && \
	git commit -m "chore: update changelog for $$NEXT" && \
	git tag $$NEXT && \
	{ git push origin HEAD $$NEXT && echo "released $$NEXT"; } || \
	{ git tag -d $$NEXT; git reset --soft HEAD~1; echo "push failed — tag and commit rolled back"; exit 1; }

release-major: _require-git-cliff
	@NEXT=v$(shell echo $$(($(MAJOR)+1))).0.0; \
	echo "$(CURRENT_TAG) -> $$NEXT"; \
	git-cliff --tag $$NEXT --output CHANGELOG.md && \
	git add CHANGELOG.md && \
	git commit -m "chore: update changelog for $$NEXT" && \
	git tag $$NEXT && \
	{ git push origin HEAD $$NEXT && echo "released $$NEXT"; } || \
	{ git tag -d $$NEXT; git reset --soft HEAD~1; echo "push failed — tag and commit rolled back"; exit 1; }

CGO_ENABLED ?= 0
GO_LDFLAGS ?=
GO_BUILD_ARGS ?= -v

GOPATH := $(shell go env GOPATH)
ADDLICENSE := $(GOPATH)/bin/addlicense
ADDLICENSE_ARGS := -v -s=only -l=apache -c "Posit Software, PBC" -ignore 'coverage.html' -ignore '.github/**'

.PHONY: all
all: build

.PHONY: build
build:
	CGO_ENABLED=$(CGO_ENABLED) go build -ldflags="$(GO_LDFLAGS)" $(GO_BUILD_ARGS) ./...

.PHONY: test
test:
	go test ./... $(GO_BUILD_ARGS) -coverprofile coverage.out
	go tool cover -html=coverage.out -o coverage.html

.PHONY: check
check: fmt vet

.PHONY: fmt
fmt:
	go fmt ./...

.PHONY: vet
vet:
	go vet ./...

.PHONY: check-license
check-license:
	$(ADDLICENSE) $(ADDLICENSE_ARGS) -check .

.PHONY: license
license:
	$(ADDLICENSE) $(ADDLICENSE_ARGS) .

.PHONY: clean
clean:
	$(RM) coverage.out coverage.html

$(ADDLICENSE):
	GOBIN=$(GOPATH)/bin go install github.com/google/addlicense@latest

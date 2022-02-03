CGO_ENABLED = 0
GO_LDFLAGS =
GO_BUILD_ARGS = -v

all: build

.PHONY: build
build:
	GO111MODULE=on CGO_ENABLED=$(CGO_ENABLED) go build \
		-ldflags="$(GO_LDFLAGS)" $(GO_BUILD_ARGS) ./...

.PHONY: test
test:
	GO111MODULE=on go test ./... $(GO_BUILD_ARGS) -coverprofile coverage.out
	go tool cover -html=coverage.out -o coverage.html

check: fmt vet

.PHONY: fmt
fmt:
	GO111MODULE=on go fmt ./...

.PHONY: vet
vet:
	GO111MODULE=on go vet ./...

.PHONY: clean
clean:
	rm -f coverage.out coverage.html

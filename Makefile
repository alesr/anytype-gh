.PHONY: test-unit fmt vet build

test-unit:
	go test -race -count=1 -v ./...

fmt:
	gofmt -w $$(go list -f '{{.Dir}}' ./...)

vet:
	go vet ./...

build:
	mkdir -p bin
	go build -o bin/anytype-gh ./cmd/anytype-gh

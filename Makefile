PKG := ./cmd/gitws

.PHONY: tidy fmt build test install clean

tidy:
	go mod tidy

fmt:
	gofmt -w ./cmd ./internal

build:
	go build $(PKG)

test:
	go test ./...

install:
	go install $(PKG)

clean:
	rm -f gitws

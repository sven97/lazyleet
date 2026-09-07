.PHONY: build run test vet fmt lint tidy clean

build:
	go build -o dist/lazyleet ./cmd/lazyleet

run:
	go run ./cmd/lazyleet

test:
	go test -race -count=1 ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

lint:
	go vet ./...
	staticcheck ./... || (echo "install: go install honnef.co/go/tools/cmd/staticcheck@latest" && exit 1)

tidy:
	go mod tidy

clean:
	rm -rf dist

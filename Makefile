BINARY := recipe-lint

.PHONY: build install test clean

build:
	go build -o $(BINARY) ./cmd/recipe-lint

install: build
	@echo "Install with: wk plugins install ."

test:
	go test ./...

clean:
	rm -f $(BINARY)

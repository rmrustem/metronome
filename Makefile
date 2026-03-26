.PHONY: build test run clean

BINARY_NAME=metronome

build:
	go build -v -o $(BINARY_NAME) .

test:
	go test -v ./...

run: build
	./$(BINARY_NAME)

clean:
	go clean
	rm -f $(BINARY_NAME)

MAELSTROM := $(HOME)/repos/maelstrom/pkg/maelstrom
BIN_DIR   := ./bin

.PHONY: build test test-echo clean

build:
	go build -o $(BIN_DIR)/echo ./cmd/echo

test:
	go test ./node/... -v

test-echo: build
	$(MAELSTROM) test -w echo --bin $(BIN_DIR)/echo --node-count 1 --time-limit 10 --log-stderr

clean:
	rm -rf $(BIN_DIR)

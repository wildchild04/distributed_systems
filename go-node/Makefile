MAELSTROM := $(HOME)/repos/maelstrom/pkg/maelstrom
BIN_DIR   := ./bin

.PHONY: build test test-echo test-broadcast clean

build:
	go build -o $(BIN_DIR)/echo ./cmd/echo
	go build -o $(BIN_DIR)/broadcast ./cmd/broadcast

test:
	go test ./... -v

test-echo: build
	$(MAELSTROM) test -w echo --bin $(BIN_DIR)/echo --node-count 1 --time-limit 10 --log-stderr

test-broadcast: build
	$(MAELSTROM) test -w broadcast --bin $(BIN_DIR)/broadcast --node-count 5 --time-limit 20 --rate 10 --log-stderr

clean:
	rm -rf $(BIN_DIR)

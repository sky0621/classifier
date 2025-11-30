BIN_NAME := classifier
CMD_DIR := ./cmd/classifier
BUILD_DIR := bin

.PHONY: build clean

build:
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BIN_NAME) $(CMD_DIR)

clean:
	@rm -rf $(BUILD_DIR)

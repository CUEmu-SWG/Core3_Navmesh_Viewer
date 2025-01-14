# Makefile for building Go binaries for Windows and Linux

.PHONY: all
all: windows linux

APP_NAME := navmesh_viewer
OUTPUT_DIR := build
SRC_DIR := .

.PHONY: windows
windows:
	@echo "Building for Windows..."
	CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc GOOS=windows GOARCH=amd64 \
		go build -o $(OUTPUT_DIR)/$(APP_NAME).exe $(SRC_DIR)

.PHONY: linux
linux:
	@echo "Building for Linux..."
	CGO_ENABLED=1 GOOS=linux GOARCH=amd64 \
		go build -o $(OUTPUT_DIR)/$(APP_NAME) $(SRC_DIR)

.PHONY: clean
clean:
	@echo "Cleaning up..."
	rm -rf $(OUTPUT_DIR)

$(OUTPUT_DIR):
	@mkdir -p $(OUTPUT_DIR)

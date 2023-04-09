# Makefile for Go application
.PHONY: build run logs execute

# Get version from constants/constants.go
VERSION := $(shell grep -oP 'Version = "\K(.*)(?=")' pkg/constants/constants.go)

# Set IMAGE_NAME to the name of the directory containing the Makefile
IMAGE_NAME := $(shell basename `pwd`)
CONTAINER_NAME := $(IMAGE_NAME)-container

build:
	@echo "Building Docker image..."
	docker build -t $(IMAGE_NAME):$(VERSION) .

run:
	@echo "Running Docker container..."
	docker run --name $(CONTAINER_NAME) -d $(IMAGE_NAME):$(VERSION)

logs:
	@echo "Fetching Docker logs..."
	docker logs -f $(CONTAINER_NAME)

execute: build run logs

clean:
	@echo "Cleaning up Docker resources..."
	docker stop $(CONTAINER_NAME) || true
	docker rm $(CONTAINER_NAME) || true

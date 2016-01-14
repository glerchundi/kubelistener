# Makefile for the Docker image quay.io/glerchundi/kubelistener
# MAINTAINER: Gorka Lerchundi Osa <glertxundi@gmail.com>
# If you update this image please bump the tag value before pushing.

.PHONY: all build test static container push clean

VERSION = 0.2.0
PREFIX = quay.io/saltosystems

all: build

build:
	@echo "Building kubelistener..."
	ROOTPATH=$(shell pwd -P); \
	GO15VENDOREXPERIMENT=1 go build -o $$ROOTPATH/bin/kubelistener

test:
	@echo "Running tests..."
	GO15VENDOREXPERIMENT=1 go test

static:
	ROOTPATH=$(shell pwd -P); \
	mkdir -p $$ROOTPATH/bin; \
	GO15VENDOREXPERIMENT=1 \
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
	go build \
		-a -tags netgo -installsuffix cgo -ldflags '-extld ld -extldflags -static' -a -x \
		-o $$ROOTPATH/bin/kubelistener-linux-amd64 \
		.

container: static
	docker build -t $(PREFIX)/kubelistener:$(VERSION) .

push: container
	docker push $(PREFIX)/kubelistener:$(VERSION)

clean:
	rm -f bin/kubelistener*

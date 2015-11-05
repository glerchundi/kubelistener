# Makefile for the Docker image quay.io/glerchundi/kubelistener
# MAINTAINER: Gorka Lerchundi Osa <glertxundi@gmail.com>
# If you update this image please bump the tag value before pushing.

.PHONY: all kubelistener container push clean

TAG = 0.1
PREFIX = quay.io/saltosystems

all: container

kubelistener:
	ROOTPATH=$(shell pwd -P); \
	mkdir -p $$ROOTPATH/bin; \
	cd $$ROOTPATH/src/github.com/glerchundi/kubelistener; \
	GOPATH=$$ROOTPATH/vendor:$$ROOTPATH \
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
	  go build \
	    -a -tags netgo -installsuffix cgo -ldflags '-extld ld -extldflags -static' -a -x \
	    -o $$ROOTPATH/bin/kubelistener-linux-amd64 \
	    .

container: kubelistener
	docker build -t $(PREFIX)/kubelistener:$(TAG) .

push: container
	docker push $(PREFIX)/kubelistener:$(TAG)

clean:
	rm -f bin/kubelistener*

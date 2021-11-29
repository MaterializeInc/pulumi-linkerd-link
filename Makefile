VERSION ?= $(patsubst v%,%,$(shell git describe))

bin/pulumi-sdkgen-linkerd-link: cmd/pulumi-sdkgen-linkerd-link/*.go
	go build -o bin/pulumi-sdkgen-linkerd-link ./cmd/pulumi-sdkgen-linkerd-link

python-sdk: bin/pulumi-sdkgen-linkerd-link
	rm -rf sdk
	bin/pulumi-sdkgen-linkerd-link $(VERSION)
	cp README.md sdk/python/
	cd sdk/python/ && \
		sed -i.bak -e "s/\$${VERSION}/$(VERSION)/g" -e "s/\$${PLUGIN_VERSION}/$(VERSION)/g" setup.py && \
		rm setup.py.bak

bin/pulumi-resource-linkerd-link: ./cmd/pulumi-resource-linkerd-link/*.go linkerd2
	go build -tags=prod -o bin/pulumi-resource-linkerd-link ./cmd/pulumi-resource-linkerd-link

linkerd2: gen
	./gen -f

.PHONY: install dev linkerd2
dev: python-sdk bin/pulumi-resource-linkerd-link

dev-from-tmpdir: python-sdk # build the provider such that any non-embedded files are obvious
	TMPD=`mktemp -d` ; \
	cp -Rp `pwd`/. $$TMPD && \
	make -C $$TMPD bin/pulumi-resource-linkerd-link && \
	cp $$TMPD/bin/pulumi-resource-linkerd-link ./bin && \
	rm -rf $$TMPD

install:
	go install ./cmd/pulumi-resource-linkerd-link

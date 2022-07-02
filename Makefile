include .bingo/Variables.mk

.DEFAULT_GOAL=build

BIN_BUILD_FLAGS?=CGO_ENABLED=0
BIN_VERSION?="git"
GOVVV_FLAGS=$(shell $(GOVVV) -flags -version $(BIN_VERSION) -pkg $(shell go list ./buildinfo))

build: $(GOVVV)
	$(BIN_BUILD_FLAGS) go build -ldflags="${GOVVV_FLAGS}" ./...
.PHONY: build

build-mail: $(GOVVV)
	$(BIN_BUILD_FLAGS) go build -ldflags="${GOVVV_FLAGS}" ./cmd/mail
.PHONY: build-mail

build-maild: $(GOVVV)
	$(BIN_BUILD_FLAGS) go build -ldflags="${GOVVV_FLAGS}" ./cmd/maild
.PHONY: build-maild

install: $(GOVVV)
	$(BIN_BUILD_FLAGS) go install -ldflags="${GOVVV_FLAGS}" ./...
.PHONY: install

install-mail: $(GOVVV)
	$(BIN_BUILD_FLAGS) go install -ldflags="${GOVVV_FLAGS}" ./cmd/mail
.PHONY: install-mail

install-maild: $(GOVVV)
	$(BIN_BUILD_FLAGS) go install -ldflags="${GOVVV_FLAGS}" ./cmd/maild
.PHONY: install-maild

define gen_release_files
	$(GOX) -osarch=$(3) -output="build/$(2)/$(2)_${BIN_VERSION}_{{.OS}}-{{.Arch}}/$(2)" -ldflags="${GOVVV_FLAGS}" $(1)
	mkdir -p build/dist; \
	cd build/$(2); \
	for release in *; do \
		cp ../../LICENSE ../../README.md $${release}/; \
		if [ $${release} != *"windows"* ]; then \
  		BIN_FILE=$(2) $(GOMPLATE) -f ../../dist/install.tmpl -o "$${release}/install"; \
			tar -czvf ../dist/$${release}.tar.gz $${release}; \
		else \
			zip -r ../dist/$${release}.zip $${release}; \
		fi; \
	done
endef

build-mail-release: $(GOX) $(GOVVV) $(GOMPLATE)
	$(call gen_release_files,./cmd/mail,mail,"linux/amd64 linux/386 linux/arm darwin/amd64 windows/amd64")
.PHONY: build-mail-release

build-maild-release: $(GOX) $(GOVVV) $(GOMPLATE)
	$(call gen_release_files,./cmd/maild,maild,"linux/amd64 linux/386 linux/arm darwin/amd64 windows/amd64")
.PHONY: build-maild-release

build-releases: build-mail-release build-maild-release
.PHONY: build-releases

mail-up:
	docker-compose -f cmd/maild/docker-compose-dev.yml up --build

mail-stop:
	docker-compose -f cmd/maild/docker-compose-dev.yml stop

mail-clean:
	docker-compose -f cmd/maild/docker-compose-dev.yml down -v --remove-orphans

test:
	go test -race -timeout 30m ./...
.PHONY: test

clean-protos:
	find . -type f -name '*.pb.go' -delete
	find . -type f -name '*pb_test.go' -delete
.PHONY: clean-protos

clean-js-protos:
	find . -type f -name '*pb.js' ! -path "*/node_modules/*" -delete
	find . -type f -name '*pb.d.ts' ! -path "*/node_modules/*" -delete
	find . -type f -name '*pb_service.js' ! -path "*/node_modules/*" -delete
	find . -type f -name '*pb_service.d.ts' ! -path "*/node_modules/*" -delete
.PHONY: clean-js-protos

install-protoc:
	cd buildtools && ./install_protoc.bash

PROTOCGENGO=$(shell pwd)/buildtools/protoc-gen-go
protos: install-protoc clean-protos
	PATH=$(PROTOCGENGO):$(PATH) ./scripts/protoc_gen_plugin.bash \
	--proto_path=. \
	--plugin_name=go \
	--plugin_out=. \
	--plugin_opt=plugins=grpc,paths=source_relative
.PHONY: protos

js-protos: install-protoc clean-js-protos
	./scripts/gen_js_protos.bash

# local is what we run when testing locally.
# This does breaking change detection against our local git repository.
.PHONY: buf-local
buf-local: $(BUF)
	$(BUF) check lint
	# $(BUF) check breaking --against-input '.git#branch=master'

# https is what we run when testing in most CI providers.
# This does breaking change detection against our remote HTTPS git repository.
.PHONY: buf-https
buf-https: $(BUF)
	$(BUF) check lint
	# $(BUF) check breaking --against-input "$(HTTPS_GIT)#branch=master"

# ssh is what we run when testing in CI providers that provide ssh public key authentication.
# This does breaking change detection against our remote HTTPS ssh repository.
# This is especially useful for private repositories.
.PHONY: buf-ssh
buf-ssh: $(BUF)
	$(BUF) check lint
	# $(BUF) check breaking --against-input "$(SSH_GIT)#branch=master"

NAME				:= terraform-repo-executor
REPO				:= quay.io/app-sre/$(NAME)
REVIVE_VERSION		:= v1.3.7
STATICCHECK_VERSION	:= 2023.1.7
TAG					:= $(shell git rev-parse --short HEAD)

PKGS				:= $(shell go list ./... | grep -v -E '/vendor/|/test')
FIRST_GOPATH		:= $(firstword $(subst :, ,$(shell go env GOPATH)))
CONTAINER_ENGINE    ?= $(shell which podman >/dev/null 2>&1 && echo podman || echo docker)

ifneq (,$(wildcard $(CURDIR)/.docker))
	DOCKER_CONF := $(CURDIR)/.docker
else
	DOCKER_CONF := $(HOME)/.docker
endif

.PHONY: all
all: test image

.PHONY: clean
clean:
	# Remove all files and directories ignored by git.
	git clean -Xfd .

############
# Building #
############

.PHONY: build
build: vet
	go build -o $(NAME) .

.PHONY: image
image:
ifeq ($(CONTAINER_ENGINE), podman)
	@DOCKER_BUILDKIT=1 $(CONTAINER_ENGINE) build --no-cache -t $(REPO):latest . --progress=plain
else
	@DOCKER_BUILDKIT=1 $(CONTAINER_ENGINE) --config=$(DOCKER_CONF) build --no-cache -t $(REPO):latest . --progress=plain
endif
	@$(CONTAINER_ENGINE) tag $(REPO):latest $(REPO):$(TAG)

.PHONY: image-push
image-push:
	$(CONTAINER_ENGINE) --config=$(DOCKER_CONF) push $(REPO):$(TAG)
	$(CONTAINER_ENGINE) --config=$(DOCKER_CONF) push $(REPO):latest

##############
# Formatting #
##############

.PHONY: format
format: go-fmt

.PHONY: go-fmt
go-fmt:
	go fmt $(PKGS)

###########
# Testing #
###########

.PHONY: lint
lint: revive staticcheck

.PHONY: revive
revive:
	go install github.com/mgechev/revive@$(REVIVE_VERSION)
	go run github.com/mgechev/revive@$(REVIVE_VERSION) -config revive.toml -set_exit_status ./...

.PHONY: staticcheck
staticcheck:
	go install honnef.co/go/tools/cmd/staticcheck@$(STATICCHECK_VERSION)
	go run honnef.co/go/tools/cmd/staticcheck@$(STATICCHECK_VERSION) ./...

.PHONY: vet
vet: test
	go vet ./...

.PHONY: test
test:
	CGO_ENABLED=0 GOOS=$(GOOS) go test -v ./...

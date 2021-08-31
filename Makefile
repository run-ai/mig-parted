# Copyright (c) 2021, NVIDIA CORPORATION.  All rights reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

MODULE := github.com/run-ai/mig-parted

DOCKER ?= docker

GOLANG_VERSION := 1.15

ifeq ($(IMAGE),)
REGISTRY ?= gcr.io/run-ai-
STAGE ?= lab
IMAGE=$(REGISTRY)$(STAGE)/mig-parted
endif
GIT_TAG ?= $(shell git tag --points-at=HEAD | head -1)
IMAGE_TAG ?= $(shell git rev-parse --short HEAD)-devel
VERSION ?= IMAGE_TAG
BUILDIMAGE ?= $(IMAGE):$(IMAGE_TAG)

TARGETS := binary build all check fmt assert-fmt lint vet test
DOCKER_TARGETS := $(patsubst %, docker-%, $(TARGETS))
.PHONY: $(TARGETS) $(DOCKER_TARGETS)

GOOS := linux

binary:
	GOOS=$(GOOS) go build ./cmd/nvidia-mig-parted

build:
	GOOS=$(GOOS) go build ./...

all: check build binary
check: assert-fmt lint vet

# Apply go fmt to the codebase
fmt:
	go list -f '{{.Dir}}' $(MODULE)/... \
		| xargs gofmt -s -l -w

assert-fmt:
	go list -f '{{.Dir}}' $(MODULE)/... \
		| xargs gofmt -s -l > fmt.out
	@if [ -s fmt.out ]; then \
		echo "\nERROR: The following files are not formatted:\n"; \
		cat fmt.out; \
		rm fmt.out; \
		exit 1; \
	else \
		rm fmt.out; \
	fi

lint:
	# We use `go list -f '{{.Dir}}' $(MODULE)/...` to skip the `vendor` folder.
	go list -f '{{.Dir}}' $(MODULE)/... | xargs golint -set_exit_status

vet:
	go vet $(MODULE)/...

test:
	go test $(MODULE)/...

.PHONY: .build-image .pull-build-image .push-build-image
# Generate an image for containerized builds
# Note: This image is local only
.build-image: docker/Dockerfile.devel
	if [ x"$(SKIP_IMAGE_BUILD)" = x"" ]; then \
		$(DOCKER) build \
			--progress=plain \
			--build-arg GOLANG_VERSION="$(GOLANG_VERSION)" \
			--tag $(BUILDIMAGE) \
			-f $(^) \
			docker; \
	fi

.pull-build-image:
	$(DOCKER) pull $(BUILDIMAGE)

.push-build-image:
	$(DOCKER) push $(BUILDIMAGE)


$(DOCKER_TARGETS): docker-%: .build-image
	@echo "Running 'make $(*)' in docker container $(BUILDIMAGE)"
	$(DOCKER) run \
		--rm \
		-e GOCACHE=/tmp/.cache \
		-v $(PWD):$(PWD) \
		-w $(PWD) \
		--user $$(id -u):$$(id -g) \
		$(BUILDIMAGE) \
			make $(*)

# Deployment targets are forwarded to the Makefile in the following directory
DEPLOYMENT_DIR = deployments/gpu-operator

DEPLOYMENT_TARGETS = ubuntu20.04 ubi8
BUILD_DEPLOYMENT_TARGETS := $(patsubst %, build-%, $(DEPLOYMENT_TARGETS))
PUSH_DEPLOYMENT_TARGETS := $(patsubst %, push-%, $(DEPLOYMENT_TARGETS))
.PHONY: $(DEPLOYMENT_TARGETS) $(BUILD_DEPLOYMENT_TARGETS) $(PUSH_DEPLOYMENT_TARGETS)

$(BUILD_DEPLOYMENT_TARGETS): build-%:
	@echo "Running 'make $(*)' in $(DEPLOYMENT_DIR)"
	make -C $(DEPLOYMENT_DIR) $(*)

$(PUSH_DEPLOYMENT_TARGETS): %:
	@echo "Running 'make $(*)' in $(DEPLOYMENT_DIR)"
	make -C $(DEPLOYMENT_DIR) $(*)

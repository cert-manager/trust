# Copyright 2021 The cert-manager Authors.
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

BINDIR ?= $(CURDIR)/bin

ARCH   ?= $(shell go env GOARCH)
OS     ?= $(shell go env GOOS)

HELM_VERSION ?= 3.6.3
KUBEBUILDER_TOOLS_VERISON ?= 1.21.2
IMAGE_PLATFORMS ?= linux/amd64,linux/arm64,linux/arm/v7,linux/ppc64le

RELEASE_VERSION ?= 0.1.0

.PHONY: help
help:  ## display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n\nTargets:\n"} /^[a-zA-Z0-9_-]+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

.PHONY: all
all: depend generate test build image ## runs test, build and image

.PHONY: test
test: depend lint ## test trust
	KUBEBUILDER_ASSETS=$(BINDIR)/kubebuilder/bin go test -v ./pkg/... ./cmd/...

.PHONY: lint
lint: vet
	./hack/verify-boilerplate.sh

.PHONY: vet
vet:
	go vet ./...

.PHONY: verify-helm-docs
verify-helm-docs: $(BINDIR)/helm-docs # verify helm-docs
	./hack/verify-helm-docs.sh

.PHONY: build
build: | $(BINDIR) ## build trust
	CGO_ENABLED=0 go build -o $(BINDIR)/cert-manager-trust ./cmd/.

.PHONY: generate
generate: depend ## generate code
	./hack/update-codegen.sh

.PHONY: verify
verify: depend test build ## tests and builds trust

# image will only build and store the image locally, targeted in OCI format.
# To actually push an image to the public repo, replace the `--output` flag and
# arguments to `--push`.
.PHONY: image
image: | $(BINDIR) ## build docker image targeting all supported platforms
	docker buildx build --platform=$(IMAGE_PLATFORMS) -t quay.io/jetstack/cert-manager-trust:v$(RELEASE_VERSION) --output type=local,dest=$(BINDIR)/cert-manager-trust .

.PHONY: chart
chart: | $(BINDIR)/helm $(BINDIR)/chart
	$(BINDIR)/helm package --app-version=$(RELEASE_VERSION) --version=$(RELEASE_VERSION) --destination "$(BINDIR)/chart" ./deploy/charts/trust

.PHONY: clean
clean: ## clean up created files
	rm -rf \
		$(BINDIR) \
		_artifacts

.PHONY: demo
demo: depend ## create cluster and deploy trust
	REPO_ROOT=$(shell pwd) ./hack/ci/create-cluster.sh

.PHONY: smoke
smoke: demo ## create cluster, deploy trust and run smoke tests
	REPO_ROOT=$(shell pwd) ./hack/ci/run-smoke-test.sh

.PHONY: depend
depend: $(BINDIR)/deepcopy-gen $(BINDIR)/controller-gen $(BINDIR)/ginkgo $(BINDIR)/kubectl $(BINDIR)/kind $(BINDIR)/helm $(BINDIR)/kubebuilder/bin/kube-apiserver $(BINDIR)/helm-docs

$(BINDIR)/deepcopy-gen: | $(BINDIR)
	mkdir -p $(BINDIR)
	cd hack/bin && go build -o $@ k8s.io/code-generator/cmd/deepcopy-gen

$(BINDIR)/controller-gen: | $(BINDIR)
	mkdir -p $(BINDIR)
	cd hack/bin && go build -o $@ sigs.k8s.io/controller-tools/cmd/controller-gen

$(BINDIR)/ginkgo: | $(BINDIR)
	cd hack/bin && go build -o $(BINDIR)/ginkgo github.com/onsi/ginkgo/ginkgo

$(BINDIR)/kind: | $(BINDIR)
	cd hack/bin && go build -o $(BINDIR)/kind sigs.k8s.io/kind

$(BINDIR)/helm: | $(BINDIR)
	curl -o $(BINDIR)/helm.tar.gz -LO "https://get.helm.sh/helm-v$(HELM_VERSION)-$(OS)-$(ARCH).tar.gz"
	tar -C $(BINDIR) -xzf $(BINDIR)/helm.tar.gz
	cp $(BINDIR)/$(OS)-$(ARCH)/helm $@
	rm -r $(BINDIR)/$(OS)-$(ARCH) $(BINDIR)/helm.tar.gz

$(BINDIR)/helm-docs: | $(BINDIR)
	cd hack/bin && go build -o $(BINDIR)/helm-docs github.com/norwoodj/helm-docs/cmd/helm-docs

$(BINDIR)/kubectl: | $(BINDIR)
	curl -o $@ -LO "https://storage.googleapis.com/kubernetes-release/release/$(shell curl -s https://storage.googleapis.com/kubernetes-release/release/stable.txt)/bin/$(OS)/$(ARCH)/kubectl"
	chmod +x $@

$(BINDIR)/kubebuilder/bin/kube-apiserver: | $(BINDIR)/kubebuilder
	curl -sSLo $(BINDIR)/envtest-bins.tar.gz "https://storage.googleapis.com/kubebuilder-tools/kubebuilder-tools-$(KUBEBUILDER_TOOLS_VERISON)-$(OS)-$(ARCH).tar.gz"
	tar -C $(BINDIR)/kubebuilder --strip-components=1 -zvxf $(BINDIR)/envtest-bins.tar.gz

$(BINDIR) $(BINDIR)/kubebuilder $(BINDIR)/chart:
	@mkdir -p $@

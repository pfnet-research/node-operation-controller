DOCKER_BUILD ?= DOCKER_BUILDKIT=1 docker build --progress plain

# Image URL to use all building/pushing image targets
TAG := $(shell git describe --tags --always --dirty)
IMG ?= ghcr.io/pfnet-research/node-operation-controller:$(TAG)

# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.25

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# Setting SHELL to bash allows bash commands to be executed by recipes.
# This is a requirement for 'setup-envtest.sh' in the test target.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk commands is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Tools
PROJECT_DIR := $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))

CONTROLLER_GEN := $(CURDIR)/bin/controller-gen
CONTROLLER_GEN_VERSION ?= v0.11.3
$(CONTROLLER_GEN): ## Download controller-gen locally if necessary.
	GOBIN=$(PROJECT_DIR)/bin go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_GEN_VERSION)

KUSTOMIZE := $(CURDIR)/bin/kustomize
KUSTOMIZE_VERSION ?= v4.5.7
$(KUSTOMIZE): ## Download kustomize locally if necessary.
	GOBIN=$(PROJECT_DIR)/bin go install sigs.k8s.io/kustomize/kustomize/v4@$(KUSTOMIZE_VERSION)

ENVTEST := $(CURDIR)/bin/setup-envtest
$(ENVTEST): ## Download envtest-setup locally if necessary.
	GOBIN=$(PROJECT_DIR)/bin go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest

KUBECTL := $(CURDIR)/bin/kubectl
KUBECTL_VERSION ?= v1.26.1
$(KUBECTL): ## Download kubectl locally if necessary.
	curl -Lo $(PROJECT_DIR)/bin/kubectl "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/amd64/kubectl"
	chmod +x $(PROJECT_DIR)/bin/kubectl

KIND := $(CURDIR)/bin/kind
KIND_VERSION ?= v0.17.0
$(KIND): ## Download kind locally if necessary.
	curl -Lo $(PROJECT_DIR)/bin/kind "https://kind.sigs.k8s.io/dl/${KIND_VERSION}/kind-linux-amd64"
	chmod +x $(PROJECT_DIR)/bin/kind

##@ Development

.PHONY: manifests
manifests: $(CONTROLLER_GEN) ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd:generateEmbeddedObjectMeta=true webhook paths="./..." output:crd:artifacts:config=config/crd/bases

.PHONY: generate
generate: $(CONTROLLER_GEN) ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: lint
lint: ## Run golangci-lint against code.
	$(DOCKER_BUILD) --target lint .

KUBECONFIG := $(CURDIR)/tmp/node-operation-controller-test.kubeconfig.yaml

.PHONY: test
test: manifests generate fmt vet lint $(ENVTEST) kind-for-test ## Run tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" KUBECONFIG=$(KUBECONFIG) go test ./... -coverprofile cover.out

test-focus: generate fmt vet manifests kind-for-test
	ginkgo -focus "${FOCUS}" ./...

kind-for-test: $(KIND) $(KUBECTL)
	$(KIND) delete cluster --name=node-operation-controller-test || true
	$(KIND) create cluster --name=node-operation-controller-test --config=config/kind/test.yaml --kubeconfig=$(KUBECONFIG)
	KUBECONFIG=$(KUBECONFIG) $(KUBECTL) delete deploy -n kube-system coredns
	KUBECONFIG=$(KUBECONFIG) $(KUBECTL) delete deploy -n local-path-storage local-path-provisioner

##@ Build

.PHONY: build
build: generate fmt vet ## Build manager binary.
	go build -o bin/manager main.go

.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host.
	go run ./main.go

.PHONY: docker-build
docker-build: ## Build docker image with the manager.
	$(DOCKER_BUILD) -t ${IMG} .

.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	docker push ${IMG}

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: install
install: manifests $(KUSTOMIZE) $(KUBECTL) ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) apply -f -

.PHONY: uninstall
uninstall: manifests $(KUSTOMIZE) $(KUBECTL) ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: deploy
deploy: manifests $(KUSTOMIZE) $(KUBECTL) ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default | $(KUBECTL) apply -f -

.PHONY: undeploy
undeploy: $(KUSTOMIZE) $(KUBECTL) ## Undeploy controller from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/default | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: clean
clean:
	rm -rf $(PROJECT_DIR)/bin

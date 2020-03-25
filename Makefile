export GO111MODULE=on

.PHONY: kind-for-test prepare-ci

# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS ?= "crd:trivialVersions=true"

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

all: manager

# Run tests
test: generate fmt vet manifests kind-for-test
	KUBECONFIG=$(shell kind get kubeconfig-path --name=node-operation-controller-test) go test ./... -coverprofile cover.out -v

test-focus: generate fmt vet manifests kind-for-test
	KUBECONFIG=$(shell kind get kubeconfig-path --name=node-operation-controller-test) ginkgo -focus "${FOCUS}" ./...

kind-for-test:
	(kind get clusters | grep -q node-operation-controller-test && kind delete cluster --name=node-operation-controller-test) || true
	kind create cluster --name=node-operation-controller-test --config=config/kind/test.yaml
	KUBECONFIG=$(shell kind get kubeconfig-path --name=node-operation-controller-test) kubectl delete deploy -n kube-system coredns

prepare-ci:
	curl -o /tmp/go.tar.gz https://dl.google.com/go/go1.13.linux-amd64.tar.gz
	tar -C /usr/local -xzf /tmp/go.tar.gz
	curl -Lo /tmp/kind https://github.com/kubernetes-sigs/kind/releases/download/v0.5.1/kind-$(shell uname)-amd64
	chmod +x /tmp/kind
	mv /tmp/kind /usr/local/bin/kind
	curl -Lo /tmp/kubectl https://storage.googleapis.com/kubernetes-release/release/v1.17.4/bin/linux/amd64/kubectl
	chmod +x /tmp/kubectl
	mv /tmp/kubectl /usr/local/bin/kubectl

# Build manager binary
manager: generate fmt vet
	go build -o bin/manager main.go

# Run against the configured Kubernetes cluster in ~/.kube/config
run: generate fmt vet manifests
	go run ./main.go

# Install CRDs into a cluster
install: manifests
	kustomize build config/crd | kubectl apply -f -

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy: manifests
	cd config/manager
	kustomize build config/default | kubectl apply -f -

kustomize-build: manifests
	cd config/manager
	kustomize build config/default | tee ${OUTPUT}

# Generate manifests e.g. CRD, RBAC etc.
manifests: controller-gen
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role webhook paths="./..." output:crd:artifacts:config=config/crd/bases

# Run go fmt against code
fmt:
	go fmt ./...

# Run go vet against code
vet:
	go vet ./...

# Generate code
generate: controller-gen
	$(CONTROLLER_GEN) object:headerFile=./hack/boilerplate.go.txt paths="./..."

# Build the docker image
docker-build-dev:
	docker build . -t ${IMG}

# Build the docker image
docker-build:
	git diff --shortstat --exit-code --quiet
	docker build . -t ${IMG}

# Push the docker image
docker-push:
	docker push ${IMG}

# find or download controller-gen
# download controller-gen if necessary
controller-gen:
ifeq (, $(shell which controller-gen))
	go get sigs.k8s.io/controller-tools/cmd/controller-gen@v0.2.0
CONTROLLER_GEN=$(GOBIN)/controller-gen
else
CONTROLLER_GEN=$(shell which controller-gen)
endif

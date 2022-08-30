#!/bin/bash
set -eux
set -o pipefail

GO_VERSION="1.19"

rm -rf /usr/local/go
curl -Lo /tmp/go.tar.gz "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz"
tar -C /usr/local -xzf /tmp/go.tar.gz
curl -Lo /tmp/kind https://kind.sigs.k8s.io/dl/v0.11.1/kind-linux-amd64
chmod +x /tmp/kind
mv /tmp/kind /usr/local/bin/kind
curl -Lo /tmp/kubectl https://storage.googleapis.com/kubernetes-release/release/v1.21.4/bin/linux/amd64/kubectl
chmod +x /tmp/kubectl
mv /tmp/kubectl /usr/local/bin/kubectl

export PATH="$PATH:/usr/local/go/bin"
go version

set +e
make test
EXITCODE="$?"
set -e

kubectl --kubeconfig ./tmp/node-operation-controller-test.kubeconfig.yaml get all --all-namespaces

exit "$EXITCODE"

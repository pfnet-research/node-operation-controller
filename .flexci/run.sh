#!/bin/bash
set -eux
set -o pipefail

curl -o /tmp/go.tar.gz https://dl.google.com/go/go1.14.1.linux-amd64.tar.gz
tar -C /usr/local -xzf /tmp/go.tar.gz
curl -Lo /tmp/kind https://github.com/kubernetes-sigs/kind/releases/download/v0.7.0/kind-$(uname)-amd64
chmod +x /tmp/kind
mv /tmp/kind /usr/local/bin/kind
curl -Lo /tmp/kubectl https://storage.googleapis.com/kubernetes-release/release/v1.17.4/bin/linux/amd64/kubectl
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

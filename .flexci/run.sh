#!/bin/bash
set -eux
set -o pipefail

make prepare-ci

set +e
PATH="$PATH:/usr/local/go/bin" make test
EXITCODE="$?"
set -e

curl -o /tmp/kubectl https://storage.googleapis.com/kubernetes-release/release/v1.17.0/bin/linux/amd64/kubectl
chmod +x /tmp/kubectl
/tmp/kubectl --kubeconfig $(kind get kubeconfig-path --name=node-operation-controller-test) get all --all-namespaces

exit "$EXITCODE"

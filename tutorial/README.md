# Tutorial

## Create a kind cluster

```
$ kind create cluster --config=tutorial/kind.yaml --name=nodeops-tutorial
$ export KUBECONFIG="$(kind get kubeconfig-path --name="nodeops-tutorial")"
```

```
$ kubectl get nodes
NAME                             STATUS   ROLES    AGE     VERSION
nodeops-tutorial-control-plane   Ready    master   2m30s   v1.15.3
nodeops-tutorial-worker          Ready    <none>   116s    v1.15.3
nodeops-tutorial-worker2         Ready    <none>   116s    v1.15.3
```

## Deploy a controller

```
$ make install
$ make run
```

## Create a first NodeOperation

Open another terminal (keep `make run` running):

```
$ export KUBECONFIG="$(kind get kubeconfig-path --name="nodeops-tutorial")"
$ cat tutorial/nodeoperation-tutorial1.yaml
$ kubectl apply -f tutorial/nodeoperation-tutorial1.yaml
```

## Create a NodeDisruptionBudget (NDB)

```
$ cat tutorial/nodedisruptionbudget-tutorial1.yaml
$ kubectl apply -f tutorial/nodedisruptionbudget-tutorial1.yaml
```

## Create NodeOperations and see NDB works

```
$ cat tutorial/nodeoperation-tutorial2.yaml
$ cat tutorial/nodeoperation-tutorial3.yaml

$ kubectl apply -f tutorial/nodeoperation-tutorial2.yaml
$ kubectl apply -f tutorial/nodeoperation-tutorial3.yaml

$ kubectl get nodeoperation
NAME        NODENAME                   PHASE       AGE
tutorial1   nodeops-tutorial-worker    Completed   10m
tutorial2   nodeops-tutorial-worker    Running     14s
tutorial3   nodeops-tutorial-worker2   Pending     11s

$ kubectl get nodeoperation
NAME        NODENAME                   PHASE       AGE
tutorial1   nodeops-tutorial-worker    Completed   12m
tutorial2   nodeops-tutorial-worker    Completed   2m37s
tutorial3   nodeops-tutorial-worker2   Running     2m34s

$ kubectl get nodeoperation
NAME        NODENAME                   PHASE       AGE
tutorial1   nodeops-tutorial-worker    Completed   15m
tutorial2   nodeops-tutorial-worker    Completed   5m7s
tutorial3   nodeops-tutorial-worker2   Completed   5m4s
```

## Create NodeRemediationTemplate

```
$ kubectl apply -f tutorial/nodeoperationtemplate-tutorial1.yaml
$ kubectl apply -f tutorial/noderemediationtemplate-tutorial1.yaml
```

```
$ kubectl label node nodeops-tutorial-worker 'auto-remediation='
```

```
$ kubectl proxy --port=8090 &
$ curl -H 'content-type: application/json-patch+json' -d '[{"op": "add", "path": "/status/conditions", "value": [{"status": "True", "type": "Tutorial"}] }]' -XPATCH 'localhost:8090/api/v1/nodes/nodeops-tutorial-worker/status'
```

## Clean up

```
$ kind delete cluster --name=nodeops-tutorial
```

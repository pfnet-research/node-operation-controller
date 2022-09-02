# Tutorial

## Create a kind cluster

```
$ kind create cluster --config tutorial/kind.yaml --name nodeops-tutorial
```

```
$ kubectl get node
NAME                             STATUS   ROLES           AGE   VERSION
nodeops-tutorial-control-plane   Ready    control-plane   59s   v1.24.0
nodeops-tutorial-worker          Ready    <none>          23s   v1.24.0
nodeops-tutorial-worker2         Ready    <none>          23s   v1.24.0
```

## Deploy a controller

```
$ make install
$ make run
```

## Create a first NodeOperation

Open another terminal (keep `make run` running):

```
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

```
$ kubectl get nodeoperation
NAME                                            NODENAME                   PHASE       AGE
tutorial1                                       nodeops-tutorial-worker    Completed   20m
tutorial1-nodeops-tutorial-worker-fpmg7-zs42p   nodeops-tutorial-worker    Completed   51s
tutorial2                                       nodeops-tutorial-worker    Completed   10m
tutorial3                                       nodeops-tutorial-worker2   Completed   10m
```

## Clean up

```
$ kind delete cluster --name=nodeops-tutorial
```

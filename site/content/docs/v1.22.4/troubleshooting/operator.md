# Operator Troubleshooting

[Contour Operator][1] runs in a Kubernetes cluster and is managed by a
[Deployment][2] resource. The following steps can be used to verify and
troubleshoot the operator:

Verify the operator deployment is available:
```bash
$ kubectl get deploy/contour-operator -n contour-operator
NAME               READY   UP-TO-DATE   AVAILABLE   AGE
contour-operator   1/1     1            1           39m
```

Check the logs of the operator:
```bash
$ kubectl logs deploy/contour-operator -n contour-operator -c contour-operator -f
2020-12-01T00:10:14.245Z	INFO	controller-runtime.metrics	metrics server is starting to listen	{"addr": "127.0.0.1:8080"}
2020-12-01T00:10:14.341Z	INFO	setup	starting contour-operator
I1201 00:10:14.343439       1 leaderelection.go:243] attempting to acquire leader lease  contour-operator/0d879e31.projectcontour.io...
2020-12-01T00:10:14.345Z	INFO	controller-runtime.manager	starting metrics server	{"path": "/metrics"}
I1201 00:10:14.442755       1 leaderelection.go:253] successfully acquired lease contour-operator/0d879e31.projectcontour.io
2020-12-01T00:10:14.447Z	INFO	controller	Starting EventSource	{"reconcilerGroup": "operator.projectcontour.io", "reconcilerKind": "Contour", "controller": "contour", "source": "kind source: /, Kind="}
2020-12-01T00:10:14.540Z	DEBUG	controller-runtime.manager.events	Normal	{"object": {"kind":"ConfigMap","namespace":"contour-operator","name":"0d879e31.projectcontour.io","uid":"40ebcf47-a105-4efc-b7e9-993df2070a07","apiVersion":"v1","resourceVersion":"33899"}, "reason": "LeaderElection", "message": "contour-operator-55d6c7b4b5-hkd78_e960056c-f959-45c9-8686-9b85e09e21d8 became leader"}
2020-12-01T00:10:14.842Z	INFO	controller	Starting EventSource	{"reconcilerGroup": "operator.projectcontour.io", "reconcilerKind": "Contour", "controller": "contour", "source": "kind source: /, Kind="}
2020-12-01T00:10:15.046Z	INFO	controller	Starting EventSource	{"reconcilerGroup": "operator.projectcontour.io", "reconcilerKind": "Contour", "controller": "contour", "source": "kind source: /, Kind="}
2020-12-01T00:10:15.340Z	INFO	controller	Starting Controller	{"reconcilerGroup": "operator.projectcontour.io", "reconcilerKind": "Contour", "controller": "contour"}
2020-12-01T00:10:15.341Z	INFO	controller	Starting workers	{"reconcilerGroup": "operator.projectcontour.io", "reconcilerKind": "Contour", "controller": "contour", "worker count": 1}
...
```

When a `Contour` is created, the operator should successfully reconcile the object:
```bash
...
2020-12-01T00:10:15.341Z	INFO	controllers.Contour	reconciling	{"request": "default/contour-sample"}
...
2020-12-01T00:10:15.442Z	DEBUG	controller	Successfully Reconciled	{"reconcilerGroup": "operator.projectcontour.io", "reconcilerKind": "Contour", "controller": "contour", "name": "contour-sample", "namespace": "default"}
```

Check the status of your `Contour` resource:
```bash
$ kubectl get contour/contour-sample -o yaml
apiVersion: operator.projectcontour.io/v1alpha1
kind: Contour
metadata:
  name: contour-sample
  namespace: default
...
status:
  availableContours: 2
  availableEnvoys: 1
  conditions:
  - lastTransitionTime: "2020-12-01T00:55:38Z"
    message: Contour has minimum availability.
    reason: ContourAvailable
    status: "True"
    type: Available
```

If the `Contour` does not become available, check the status of operands.
```bash
$ kubectl get po -n projectcontour
NAME                       READY   STATUS      RESTARTS   AGE
contour-7649c6f6cc-9hxn9   1/1     Running     0          6m18s
contour-7649c6f6cc-sb4nn   1/1     Running     0          6m18s
contour-certgen-vcsqd      0/1     Completed   0          6m18s
envoy-qshxf                2/2     Running     0          6m18s
```

Check the logs of the operands. The following example checks the logs of the
contour deployment operand:
```bash
$ kubectl logs deploy/contour -n projectcontour -c contour -f
2020-12-01T00:10:14.245Z	INFO	controller-runtime.metrics	metrics server is starting to listen	{"addr": "127.0.0.1:8080"}
2020-12-01T00:10:14.341Z	INFO	setup	starting contour-operator
I1201 00:10:14.343439       1 leaderelection.go:243] attempting to acquire leader lease  contour-operator/0d879e31.projectcontour.io...
2020-12-01T00:10:14.345Z	INFO	controller-runtime.manager	starting metrics server	{"path": "/metrics"}
I1201 00:10:14.442755       1 leaderelection.go:253] successfully acquired lease contour-operator/0d879e31.projectcontour.io
2020-12-01T00:10:14.447Z	INFO	controller	Starting EventSource	{"reconcilerGroup": "operator.projectcontour.io", "reconcilerKind": "Contour", "controller": "contour", "source": "kind source: /, Kind="}
2020-12-01T00:10:14.540Z	DEBUG	controller-runtime.manager.events	Normal	{"object": {"kind":"ConfigMap","namespace":"contour-operator","name":"0d879e31.projectcontour.io","uid":"40ebcf47-a105-4efc-b7e9-993df2070a07","apiVersion":"v1","resourceVersion":"33899"}, "reason": "LeaderElection", "message": "contour-operator-55d6c7b4b5-hkd78_e960056c-f959-45c9-8686-9b85e09e21d8 became leader"}
2020-12-01T00:10:14.842Z	INFO	controller	Starting EventSource	{"reconcilerGroup": "operator.projectcontour.io", "reconcilerKind": "Contour", "controller": "contour", "source": "kind source: /, Kind="}
2020-12-01T00:10:15.046Z	INFO	controller	Starting EventSource	{"reconcilerGroup": "operator.projectcontour.io", "reconcilerKind": "Contour", "controller": "contour", "source": "kind source: /, Kind="}
2020-12-01T00:10:15.340Z	INFO	controller	Starting Controller	{"reconcilerGroup": "operator.projectcontour.io", "reconcilerKind": "Contour", "controller": "contour"}
2020-12-01T00:10:15.341Z	INFO	controller	Starting workers	{"reconcilerGroup": "operator.projectcontour.io", "reconcilerKind": "Contour", "controller": "contour", "worker count": 1}
...
```

__Note:__ `projectcontour` is the default namespace used by operands of a `Contour`
when `spec.namespace.name` is unspecified.
 
[1]: https://github.com/projectcontour/contour-operator
[2]: https://kubernetes.io/docs/concepts/workloads/controllers/deployment/

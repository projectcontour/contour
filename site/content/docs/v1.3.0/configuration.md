# Contour Configuration

A configuration file can be passed to the `contour serve` command which specified additional properties that Contour should use when starting up.
This file is passed to Contour via a ConfigMap which is mounted as a volume to the Contour pod.

Following is an example ConfigMap with configuration file included: 

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: contour
  namespace: projectcontour
data:
  contour.yaml: |
    # should contour expect to be running inside a k8s cluster
    # incluster: true
    #
    # path to kubeconfig (if not running inside a k8s cluster)
    # kubeconfig: /path/to/.kube/config
    #
    # disable ingressroute permitInsecure field
    # disablePermitInsecure: false
    tls:
      # minimum TLS version that Contour will negotiate
      # minimumProtocolVersion: "1.1"
    # The following config shows the defaults for the leader election.
    # leaderelection:
      # configmap-name: leader-elect
      # configmap-namespace: projectcontour
```

_Note:_ The default example `contour` includes this [file][1] for easy deployment of Contour.

[1]: {{< param github_url >}}/tree/{{page.version}}/examples/contour/01-contour-config.yaml

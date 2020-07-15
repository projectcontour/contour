# Contour Configuration Reference

- [Configuration File](#configuration-file)
- [Environment Variables](#environment-variables)

## Configuration File

A configuration file can be passed to the `--config-path` argument of the `contour serve` command to specify additional configuration to Contour.
In most deployments, this file is passed to Contour via a ConfigMap which is mounted as a volume to the Contour pod.

The Contour configuration file is optional.
In its absence, Contour will operate with reasonable defaults.
Where Contour settings can also be specified with command-line flags, the command-line value takes precedence over the configuration file.

| Field Name | Type | Default | Description |
|------------|------|---------|-------------|
| accesslog-format | string | `envoy` | This key sets the global [access log format][2] for Envoy. Valid options are `envoy` or `json`. |
| debug | boolean | `false` | Enables debug logging. |
| default-http-versions | string array | <code style="white-space:nowrap">HTTP/1.1</code> <br> <code style="white-space:nowrap">HTTP/2</code> | This array specifies the HTTP versions that Contour should program Envoy to serve. HTTP versions are specified as strings of the form "HTTP/x". |
, where "x" represents the version number. |
| disablePermitInsecure | boolean | `false` | If this field is true, Contour will ignore `PermitInsecure` field in HTTPProxy documents. |
| envoy-service-name | string | `envoy` | This sets the service name that will be inspected for address details to be applied to Ingress objects. |
| envoy-service-namespace | string | `projectcontour` | This sets the namespace of the service that will be inspected for address details to be applied to Ingress objects. If the `CONTOUR_NAMESPACE` environment variable is present, Contour will populate this field with its value. |
| ingress-status-address | string | None | If present, this specifies the address that will be copied into the Ingress status for each Ingress that Contour manages. It is exclusive with `envoy-service-name` and `envoy-service-namespace`.|
| incluster | boolean | `false` | This field specifies that Contour is running in a Kubernetes cluster and should use the in-cluster client access configuration.  |
| json-fields | string array | [fields][5]| This is the list the field names to include in the JSON [access log format][2]. |
| kubeconfig | string | `$HOME/.kube/config` | Path to a Kubernetes [kubeconfig file][3] for when Contour is executed outside a cluster. |
| leaderelection | leaderelection | | The [leader election configuration](#leader-election-configuration). |
| request-timeout | [duration][4] | `0s` | This field specifies the default request timeout as a Go duration string. Zero means there is no timeout. |
| tls | TLS | | The default [TLS configuration](#tls-configuration). |
| timeouts | TimeoutConfig | | The [timeout configuration](#timeout-configuration). |
{: class="table thead-dark table-bordered"}
<br>

### TLS Configuration

The TLS configuration block can be used to configure default values for how
Contour should provision TLS hosts.

| Field Name | Type| Default  | Description |
|------------|-----|----------|-------------|
| minimum-protocol-version| string | `""` | This field specifies the minimum TLS protocol version that is allowed. Valid options are `1.2` and `1.3`. Any other value defaults to TLS 1.1. |
| fallback-certificate | | | [Fallback certificate configuration](#fallback-certificate). |
{: class="table thead-dark table-bordered"}
<br>

### Fallback Certificate

| Field Name | Type| Default  | Description |
|------------|-----|----------|-------------|
| name       | string | `""` | This field specifies the name of the Kubernetes secret to use as the fallback certificate.      |
| namespace  | string | `""` | This field specifies the namespace of the Kubernetes secret to use as the fallback certificate. |
{: class="table thead-dark table-bordered"}
<br>

### Leader Election Configuration

The leader election configuration block configures how a deployment with more than one Contour pod elects a leader.
The Contour leader is responsible for updating the status field on Ingress and HTTPProxy documents.
In the vast majority of deployments, only the `configmap-name` and `configmap-namespace` fields should require any configuration.

| Field Name | Type | Default | Description |
|------------|------|---------|-------------|
| configmap-name | string | `leader-elect` | The name of the ConfigMap that Contour leader election will lease. |
| configmap-namespace | string | `projectcontour` | The namespace of the ConfigMap that Contour leader election will lease. If the `CONTOUR_NAMESPACE` environment variable is present, Contour will populate this field with its value. |
| lease-duration | [duration][4] | `15s` | The duration of the leadership lease. |
| renew-deadline | [duration][4] | `10s` | The length of time that the leader will retry refreshing leadership before giving up. |
| retry-period | [duration][4] | `2s` | The interval at which Contour will attempt to the acquire leadership lease. |
{: class="table thead-dark table-bordered"}
<br>

### Timeout Configuration

The timeout configuration block can be used to configure various timeouts for the proxies.

| Field Name | Type| Default  | Description |
|------------|-----|----------|-------------|
| connection-idle-timeout| [duration][4] | `60s` | This field defines how long the proxy should wait while there are no active requests before terminating an HTTP connection. Set to 0 to disable the timeout. |
| stream-idle-timeout| [duration][4] | `5m` | This field defines how long the proxy should wait while there is no stream activity before terminating a stream. Set to 0 to disable the timeout. |
| max-connection-duration | [duration][4] | none | This field defines the maximum period of time after an HTTP connection has been established from the client to the proxy before it is closed by the proxy, regardless of whether there has been activity or not. Omit or set to 0 for no max duration. |
| drain-timeout | [duration][4] | `5s` | This field defines how long the proxy will wait between sending an initial GOAWAY frame and a second, final GOAWAY frame when terminating an HTTP/2 connection. During this grace period, the proxy will continue to respond to new streams. After the final GOAWAY frame has been sent, the proxy will refuse new streams. Set to 0 for no grace period. |
{: class="table thead-dark table-bordered"}
<br>

### Configuration Example

The following is an example ConfigMap with configuration file included:

```
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
    # disable httpproxy permitInsecure field
    # disablePermitInsecure: false
    tls:
      # minimum TLS version that Contour will negotiate
      # minimumProtocolVersion: "1.1"
      fallback-certificate:
      # name: fallback-secret-name
      # namespace: projectcontour
    # The following config shows the defaults for the leader election.
    # leaderelection:
      # configmap-name: leader-elect
      # configmap-namespace: projectcontour
    # Default HTTP versions.
    # default-http-versions:
    # - "HTTP/1.1"
    # - "HTTP/2"
    # The following shows the default proxy timeout settings.
    # timeouts:
    #  connection-idle-timeout: 60s
    #  stream-idle-timeout: 5m
    #  max-connection-duration: 0s
    #  drain-timeout: 5s
```

_Note:_ The default example `contour` includes this [file][1] for easy deployment of Contour.

## Environment Variables

### CONTOUR_NAMESPACE

If present, the value of the `CONTOUR_NAMESPACE` environment variable is used as:

1. The value for the `contour bootstrap --namespace` flag unless otherwise specified.
1. The value for the `contour certgen --namespace` flag unless otherwise specified.
1. The value for the `contour serve --envoy-service-namespace` flag unless otherwise specified.
1. The value for the `leaderelection.configmap-namespace` config file setting for `contour serve` unless otherwise specified.

The `CONTOUR_NAMESPACE` environment variable is set via the [Downward API][6] in the Contour [example manifests][7].


[1]: {{site.github.repository_url}}/tree/{{page.version}}/examples/contour/01-contour-config.yaml
[2]: /guides/structured-logs
[3]: https://kubernetes.io/docs/concepts/configuration/organize-cluster-access-kubeconfig/
[4]: https://golang.org/pkg/time/#ParseDuration
[5]: https://godoc.org/github.com/projectcontour/contour/internal/envoy#DefaultFields
[6]: https://kubernetes.io/docs/tasks/inject-data-application/environment-variable-expose-pod-information/
[7]: {{site.github.repository_url}}/tree/{{page.version}}/examples/contour

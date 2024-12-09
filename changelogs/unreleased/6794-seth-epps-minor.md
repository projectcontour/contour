## Overload Manager - Max Global Connections

Introduces an envoy bootstrap flag to enable the [global downstream connection limit overload manager resource monitors](https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/resource_monitors/downstream_connections/v3/downstream_connections.proto#envoy-v3-api-msg-extensions-resource-monitors-downstream-connections-v3-downstreamconnectionsconfig).

The new flag can be passed as an integer flag to the contour bootstrap subcommand, `overload-dowstream-max-conn`.

```sh
contour bootstrap --help
INFO[0000] maxprocs: Leaving GOMAXPROCS=10: CPU quota undefined
usage: contour bootstrap [<flags>] <path>

Generate bootstrap configuration.


Flags:
  -h, --[no-]help                Show context-sensitive help (also try --help-long and --help-man).
      --log-format=text          Log output format for Contour. Either text or json.
      --admin-address="/admin/admin.sock"
                                 Path to Envoy admin unix domain socket.
      --admin-port=ADMIN-PORT    DEPRECATED: Envoy admin interface port.
      --dns-lookup-family=DNS-LOOKUP-FAMILY
                                 Defines what DNS Resolution Policy to use for Envoy -> Contour cluster name lookup. Either v4, v6, auto, or all.
      --envoy-cafile=ENVOY-CAFILE
                                 CA Filename for Envoy secure xDS gRPC communication. ($ENVOY_CAFILE)
      --envoy-cert-file=ENVOY-CERT-FILE
                                 Client certificate filename for Envoy secure xDS gRPC communication. ($ENVOY_CERT_FILE)
      --envoy-key-file=ENVOY-KEY-FILE
                                 Client key filename for Envoy secure xDS gRPC communication. ($ENVOY_KEY_FILE)
      --namespace="projectcontour"
                                 The namespace the Envoy container will run in. ($CONTOUR_NAMESPACE)
      --overload-dowstream-max-conn=OVERLOAD-DOWSTREAM-MAX-CONN
                                 Defines the Envoy global downstream connection limit
      --overload-max-heap=OVERLOAD-MAX-HEAP
                                 Defines the maximum heap size in bytes until overload manager stops accepting new connections.
      --resources-dir=RESOURCES-DIR
                                 Directory where configuration files will be written to.
      --xds-address=XDS-ADDRESS  xDS gRPC API address.
      --xds-port=XDS-PORT        xDS gRPC API port.
      --xds-resource-version="v3"
                                 The versions of the xDS resources to request from Contour.

Args:
  <path>  Configuration file ('-' for standard output).
```

As part of this change, we also set the `ignore_global_conn_limit` flag to `true` on the existing admin listeners such
that envoy remains live, ready, and serving stats even though it is rejecting downstream connections.

To add some flexibility for health checks, in addition to adding a new bootstrap flag, there is a new configuration
option for the envoy health config to enforce the envoy overload manager actions, namely rejecting requests. This
"advanced" configuration gives the operator the ability to configure readiness and liveness to handle taking pods out
of the pool of pods that can serve traffic.

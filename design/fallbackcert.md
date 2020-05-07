# Fallback Certificate

Status: Accepted

Contour supports virtual host based routing over TLS and utilizes SNI which allows multiple fqdn's to be used on the same network endpoint.
Unfortunately, some requests are sent and do not have the SNI server name set. 
When this happens, the request fails since the request does not match any routing rules applied to Envoy.

This design doc looks to enable a fallback certificate, such that when a request is received at Envoy, it will still route to the proper set of endpoints even though standard SNI logic isn't applied.

## Goals

- Allow fallback certificate to be defined 
- Enable the fallback cert only for specific vhosts

## Non Goals

- Allowing multiple default certs to be defined

## Background

Contour provides virtual host based routing, so that the TLS request is routed to the appropriate service, based on the hostname specified in the HTTPS’s `HOST` header.
As HOST Header is encrypted during TLS handshake, it can’t be used for virtual host based routing unless client sends HTTPS request specifying hostname using the SNI or the request is first decrypted using a default TLS certificate.
Some users need to support clients which may not be using SNI.
When an HTTPS request is received, Envoy needs to first decrypt the request using a default TLS certificate and then based on the HOST header, route it to appropriate service.
As of now, Contour only provides certificate at virtual host level in an HTTPProxy and there is no way to define a default TLS certificate in Contour.

## High-Level Design

Contour will add a new argument to `contour serve` named `--fallback-certificate` which references a secret which is namespaced with a name (e.g. `namespace/name`).
This same configuration will be available in the Contour configuration file in the `tls.fallback-certificate` location.
Secondly, a new field will be added to the `HTTPProxy.Spec.VirtualHost.TLS` named `EnableFallbackCertificate` to allow virtual hosts to opt into this functionality.
This last point is important as by default, all vhosts will **not** be enabled for this feature.

In addition, the fallback certificate will need to be delegated to the namespace where the root `HTTPProxy` is defined using `CertificateDelegation`.
The certificate can be delegate to a single, many, or all (e.g. `*`) namespaces.
This ensures that any proxy that sets the `Spec.TLS.enableFallbackCertificate=true` has sufficient authority to configure this option.

Client auth is also not compatible with the fallback certificate logic.
If a root `HTTPProxy` defines both, the proxy will be set to an error.

## Detailed Design

### Envoy API

Contour defines `FilterChainMatches` (https://www.envoyproxy.io/docs/envoy/v1.14.1/api-v2/api/v2/listener/listener_components.proto.html?highlight=filterchainmatch#listener-filterchainmatch) on SNI names which allows for a single Envoy listener to proxy multiple vhosts over TLS.
This feature will add a new filter chain match on `TransportProtocol:  tls` which will match any request which is TLS but does not match a pre-configured SNI defined in the previous step.
Envoy processes `FilterChainMataches` with `SNI` matches before transport protocol.

Next this catch-all filter chain takes a `route_config_name` reference in the `envoy.http_connection_manager`.
For all non-http requests, an Envoy RDS config named `ingress_http` is configured with  all the routes.
For each virtual host that has enabled the `EnableFallbackCertificate` flag a new RDS route table will be created which will contain all the routes for vhosts which have opted into the fallback certificate.
If supplied, the `minimum-protocol-version` defined in the `TLS` section of the Contour configuration file will be used for the fallback filter chain, otherwise the default will be used. 

#### Example fallback route: 

```json
{
     "version_info": "2",
     "route_config": {
      "@type": "type.googleapis.com/envoy.api.v2.RouteConfiguration",
      "name": "ingress_fallback",
      "virtual_hosts": [
       {
        "name": "containersteve.com",
        "domains": [
         "containersteve.com",
         "containersteve.com:*"
        ],
        "routes": [
         {
          "match": {
           "prefix": "/"
          },
          "redirect": {
           "https_redirect": true
          }
         }
        ]
       },
       {
        "name": "demo.projectcontour.io",
        "domains": [
         "demo.projectcontour.io",
         "demo.projectcontour.io:*"
        ],
        "routes": [
         {
          "match": {
           "prefix": "/secure"
          },
          "redirect": {
           "https_redirect": true
          }
         }
        ]
       }
      ],
     "last_updated": "2020-04-22T17:25:41.290Z"
    }
   ]
  }
```

#### New catch-all filter chain
 
```go
 &envoy_api_v2_listener.FilterChain{
    Filters: filters,
     FilterChainMatch: &envoy_api_v2_listener.FilterChainMatch{
         TransportProtocol: "tls",
     },
 }
```

### Contour API

The new argument to `contour serve` must be in the format of `namespace/name`.
The Contour configuration file will also have a matching configuration item to define the fallback certificate.

The `HTTPProxy` spec will add a new field named `EnableFallbackCertificate` and will default to `false`:

```go
type TLS struct {
	// required, the name of a secret in the current namespace
	SecretName string `json:"secretName,omitempty"`
	// Minimum TLS version this vhost should negotiate
	// +optional
	MinimumProtocolVersion string `json:"minimumProtocolVersion,omitempty"`
	// If Passthrough is set to true, the SecretName will be ignored
	// and the encrypted handshake will be passed through to the
	// backing cluster.
	// +optional
	Passthrough bool `json:"passthrough,omitempty"`
    
    // +optional
    EnableFallbackCertificate bool `json:"enableFallbackCertificate,omitempty""`
}
```

### Certificate Delegation Example

Following is a sample delegation of how a fallbackCertificate can be delegated to a few specific namespaces:

```yaml
apiVersion: projectcontour.io/v1
kind: TLSCertificateDelegation
metadata:
  name: example-com-fallback
  namespace: www-admin
spec:
  delegations:
    - secretName: example-com-fallback
      targetNamespaces:
      - example-com
      - teama
      - teamb
```
# External Authentication

_Status_: Draft

This document outlines a specification to allow Contour to make use of the
[external authentication] http_filter of Envoy to implement authentication.

## Goals

- Allow usage of Basic Auth on an IngressRoute using a password database in
  `htpasswd` format stored in a secret

- Allow usage of External Authentication API similar to the
  [nginx-ingress external auth] configuration

- Re-usability of the authentication configurations across multiple
  IngressRoutes in a Namespace

## Non-goals

- Delegations of authentication configurations like TLSCertiticateDelegation
  provides

- Handling stateful authentication flows

- Handling authentication of non HTTP traffic

- Handling authentication on Kubernetes Ingress resources


## Background

Envoy 1.7.0 introduced an [external authentication] http_filter which can be
configured via a GRPC API. Users request to hand-off authentication from their
applications into the ingress controller. Nginx-Ingress provides similar
features using annotations.

## High-Level Design

* IngressRoutes can reference an new Authentication CRD on a per route basis

* Authentication CRDs define the method of authentication. Initially two
  methods: Basic and RequestService are supported

* Envoy's ingress listeners should configure Contour as authentication service
  and Contour should implement the GRPC API call [CheckRequest]

## Detailed Design

### Changes to the CRDs

To select an authentication method we can configure a reference on
IngressRoutes like that:

```
apiVersion: contour.heptio.com/v1beta1
kind: IngressRoute
[...]
spec:
  routes:
    - match: /
      authenticationRef:
        name: kuard-basic-auth
[...]
```

#### Authentication CRD

A new CRD allows to configure the details of how requests are authenticated.
Currently the two variants Basic and RequestService are supported. Only a
single one can be set on an Authentication resource


An Authentication object for BasicAuth, which references a BasicAuth file
stored in key `htpasswd` of a Kubernetes secret named `kuard-basic-auth` should
look like that:

```
apiVersion: contour.heptio.com/v1beta1
kind: Authentication
metadata:
  name: kuard-basic-auth
  namespace: kuard
spec:
  basic:
    realm: Kuard - Top Secret
    secretRef:
      name: kuard-basic-auth
      key: htpasswd
```

Alternatively a RequestService can be defined providing a HTTP URL in `url`.
The incoming end user request headers are forwarded to this service and only if
the service returns status code 200 the request is forwarded to the service
provided in the ingress route. Unauthenticated users are forwarded to the
`signInURL` specified.

While the `url` can be a cluster internal service, the `signInURL` needs to be
a publicly accessible authentication service. [oauth2proxy] is commonly used
as such a service. An example configuration could look like:

```
apiVersion: contour.heptio.com/v1beta1
kind: Authentication
metadata:
  name: kuard-oauth2-proxy
  namespace: kuard
spec:
  requestService:
    url: http://oauth2-proxy.oauth2-proxy.svc.cluster.local/oauth2/auth
    signInURL: https://auth.example.net/oauth2/start
```

### Changes to the Envoy config

Contour should implement the [CheckRequest] call as part if its GRPC server. 

#### Listeners

Envoy's listeners need to be configured to forward auth requests to Contour.
This should be the first http_filter setup:

```
{
    "name": "envoy.ext_authz",
        "config": {
            "grpc_service": {
                "timeout": "1s",
                "envoy_grpc": {
                    "cluster_name": "contour"
                }
            }
        }
}
```

#### Routes

Every route then needs to be configured like that for the case that no `authenticationRef` was set/found:

```
"per_filter_config": {
    "envoy.ext_authz": {
        "disabled": true
    }
}
```

And for the case where an authentication has been set:

```
"per_filter_config": {
    "envoy.ext_authz": {
        "check_settings": {
            "context_extensions": {
                "authentication_namespace": "kuard",
                "authentication_name": "kuard-basic-auth"
            }
        }
    }
}
```

### [CheckRequest]

[CheckRequest] needs to be implement in the GRPC server of Contour.

The request looks for the name and namespace of the Authentication object in
the context_extensions and looks that up in the resource cache.

If 

#### BasicAuth

* No auth header: return 401 + realm header

* auth header with correct user and password: return 200

* auth header without correct user and password or any other error reading htpasswd 403

#### RequestService

* Forward end user request headers (make sure to not expose envoy internal
  headers) to the server specified in the `url`.

* Return a 200 response to envoy with 200

* Everything else should be a redirect using 302 to the `signInURL`

## Alternatives Considered

The authentication server and CRD could be managed external to contour and only
a configuration option in contour and the addition of the `authenticationRef`
would be necessary. While this would provide a lower effort in contour, this
external service is heavily coupled with Contour and potentially not too useful
without Contour.

## Security considerations

**TODO**

* This adds quite a bit of complexity into contour

* Plan to use [tg123/go-htpasswd] for htpasswd validation

* Does basic auth need some rate limiting ?

* Routes more specific to one protected by `authenticationRef` also need to set
  `authenticationRef` otherwise it will be accessible by everyone


[nginx-ingress external auth]:(https://github.com/kubernetes/ingress-nginx/tree/master/docs/examples/auth/oauth-external-auth)
[external authentication]:(https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/ext_authz_filter)
[CheckRequest]:(https://www.envoyproxy.io/docs/envoy/v1.7.0/api-v2/service/auth/v2alpha/external_auth.proto#envoy-api-msg-service-auth-v2alpha-checkrequest)
[tg123/go-htpasswd]:(https://github.com/tg123/go-htpasswd)
[oauth2proxy]:(https://github.com/pusher/oauth2_proxy)

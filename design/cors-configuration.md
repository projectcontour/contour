# CORS configuration

This document proposes a way to enable CORS (Cross-origin resource sharing) policies and therefore, allow access to resources in a cluster from a webpage deployed at a different domain.

## Goals

- To allow configuration of CORS related policies, letting Contour process cross domain requests and add all the needed response headers.

## Background

Nowadays, most modern browsers don't allow requests to a domain from a webpage that has been fetched from a different domain. This is a protection mechanism against CSRF (Cross-Site Request Forgery) attacks. To explain what a CSRF attack is, it is necessary to explain how authentication and user sessions have been handled traditionally by most web applications:
- When a user tries to access a protected application (an online bank for instance), they are asked to enter their credentials on a login screen.
- When the credentials are sent to the server, these are validated against a database, an external service or whatever.
- The server generates a token which is stored in a cookie. This way, the browser will send the cookie with the token automatically in successive requests and the server will be able to validate that the user has already been authenticated without querying the database.

Knowing how authentication works, a malicious webpage could try to force a user to execute unwanted actions on a web application in which they're currently authenticated using some social engineering. This could be an attack example:
- A user who is authenticated on an online bank (`www.mybank.com`) receives an email saying that they won a brand new car. To get the prize, they only have to visit `www.malicious-site.com/winner` and fill out a form with some personal data.
- When the user goes to the malicious web page, a Javascript code is executed in the background sending an AJAX request to `www.mybank.com/transfer` which is the URL used for doing money transfers on the online bank.
- As the user is already authenticated on `mybank.com`, the authentication cookie is sent automatically.
- The web application checks the cookie and as it's valid, the money transfer is performed on the victim's behalf.

To avoid this kind of scenario, web browsers apply the same-origin policy.

### Same-origin policy

Under the same-origin policy, a web browser allows scripts contained on a web page to access data on another server, but only if both share the same origin. An origin is defined as a combination of URI scheme, host name, and port number. Thanks to same-origin policy, attacks like the one explained previously are prevented by the browser itself because `www.mybank.com` and `www.malicious-site.com` don't share the same origin.

However, the way web applications are developed has evolved, and nowadays it’s very frequent to separate the frontend from the backend, deploying them independently. For instance, the frontend could be a Javascript single page application deployed on a CDN (`myfrontend.com`) and the backend, a microservices cluster deployed somewhere else (`mybackend.com`).

As the Javascript application needs to send requests to the API exposed by the backend and they are hosted on different domains, the web browser will prevent any communication between them due to the same-origin policy. This is where CORS comes into play.

### CORS

CORS is a mechanism that allows bypassing the same-origin policy for trusted sources. This is the way it works in its simplest form:
- Every time a cross-origin AJAX request is about to send, the browser sets the `Origin` request header with the web page's origin as the value.
- The server checks the `Origin` header and if the origin is allowed, it sets the `Access-Control-Allow-Origin` header to the `Origin` value.
- When the response reaches the browser, it verifies that the value of the `Access-Control-Allow-Origin` header matches the origin of the tab the request  originated from. If it doesn't match, it throws an error.

The following are the criteria that define a simple request:
- Requests only use the GET or POST HTTP methods. If the POST method is used, then Content-Type can only be one of the following: application/x-www-form-urlencoded, multipart/form-data, or text/plain.
- Requests do not set custom headers, such as X-Other-Header.

If the content of the request doesn't meet the criteria above, the browser first checks whether the actual request should be sent. This is done by sending a special request (called preflight request) to the server in advance. A preflight request first sends an HTTP request to the resource using the OPTIONS method with the following headers:
- `Origin`: Specifies the domain that would like access to the resource. This is inserted by the browser in a cross-origin request.
- `Access-Control-Request-Method`: The HTTP method to be used in the actual request from the browser.
- `Access-Control-Request-Headers`: The custom headers to be sent in the actual cross-origin request.

In response to a preflight request, the server sends a response with the following headers:
- `Access-Control-Allow-Origin`: Specifies the domains allowed to access the resource.
- `Access-Control-Allow-Credentials`: Indicates whether browser credentials can be used to make the actual request (cookies for instance).
- `Access-Control-Allow-Private-Network`: Part of the PNA specification Pre-check requests will carry this header.
- `Access-Control-Expose-Headers`: Allows headers to be exposed to the browser.
- `Access-Control-Max-Age`: Specifies how long preflight request results can be cached.
- `Access-Control-Allow-Methods`: Indicates which methods are allowed when making an actual request.
- `Access-Control-Allow-Headers`: Indicates which headers can be used in the actual request.

When the browser gets the preflight response, it checks if the origin is allowed and if the HTTP method and headers of the main request are in the list returned by the server. If so, it sends the main request, which will be a regular cross-origin request, it will include the `Origin` header and the response will contain `Access-Control-Allow-Origin` once again.

This proposal introduces a way to set all the CORS related configuration in Contour, letting Envoy do all the heavy work.

## High-Level Design

Envoy supports [CORS via a filter](https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/cors_filter.html) and it can be configured [using the API](https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/route/v3/route_components.proto#envoy-v3-api-msg-config-route-v3-corspolicy). The changes proposed in this document will allow the configuration of CORS policies in Contour.

At a high level the proposed changes will imply:
- Adding new fields at virtual host level to configure the CORS policy in the YAML.
- Changing some structs in the code.
- Enabling the Envoy CORS filter.

### Proposed YAML fields

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
 name: google apis
 namespace: prod
spec:
 virtualhost:
   fqdn: www.googleapis.com
   # The CORS policy described here will apply to all the routes of the virtual host.
   corsPolicy:
     # Which domains can access the matched resources in a cross-site manner.
     allowOrigin:
       - "*"
     # Which HTTP methods are allowed for cross-origin requests (case-sensitive).
     allowMethods:
       - GET
       - POST
     # The headers the server is going to accept (case-insensitive).
     allowHeaders:
       - cache-control
       - content-type
       - custom-header
     # The non simple headers the client will be able to access (case-insensitive).
     exposeHeaders:
       - Content-Length
       - Content-Range
     # Whether the server allows sending credentials (cookies for instance) in cross-origin requests.
     allowCredentials: true
     allowPrivateNetwork: true
     # the amount of time the preflight response will be cached. It's expressed in the Go duration format. If not supplied, browser default values will apply.
     maxAge: 10m
   routes:
     - conditions:
       - prefix: /analytics
       services:
         - name: analytics-api
           port: 9999
```

The names and types of the proposed fields are inspired by the [HTTP headers](https://www.w3.org/TR/2010/WD-cors-20100727/#syntax). The `maxAge` is going to be parsed into a `time.Duration` as this is going to be the type used in the Dag. If no value is provided, [browser defaults](https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Access-Control-Max-Age#Directives) will apply.

## Detailed Design

### Reading from YAML
The virtual host struct will be updated to contain the CORS related information:

```go
// contour/apis/contour/v1beta1/projectcontour/v1/httpproxy.go
// VirtualHost appears at most once. If it is present, the object is considered
// to be a "root".
type VirtualHost struct {
  [... other members ...]

  // Specifies the CORS policy to apply for the matched path.
  // +optional
  CorsPolicy *CorsPolicy `json:"corsPolicy,omitempty"`
}
// CorsPolicy allows setting de CORS policy
type CorsPolicy struct {
	// Specifies whether the resource allows credentials.
	AllowCredentials bool `json:"allowCredentials"`
	// AllowOrigin specifies the origins that will be allowed to do CORS requests.
	AllowOrigin []string `json:"allowOrigin"`
	// AllowMethods specifies the content for the *access-control-allow-methods* header.
	AllowMethods []string `json:"allowMethods"`
	// AllowHeaders specifies the content for the *access-control-allow-headers* header.
	AllowHeaders []string `json:"allowHeaders"`
	// ExposeHeaders Specifies the content for the *access-control-expose-headers* header.
	ExposeHeaders []string `json:"exposeHeaders"`
	// MaxAgeSeconds specifies the content for the *access-control-max-age* header.
	MaxAge string `json:"maxAge"`
	// AllowPrivateNetwork specifies the content for the *access-control-allow-private-network* header.
    AllowPrivateNetwork bool `json:"allowPrivateNetwork,omitempty"`
}
```

The common DAG struct will be updated accordingly:

```go
// contour/internal/dag/dag.go
// A VirtualHost represents a named L4/L7 service.
type VirtualHost struct {
	[... other members ...]

	CorsPolicy *CorsPolicy
}
```

### Enabling the CORS filter in Envoy
In order to enable the CORS filter in Envoy we will update `contour/internal/envoy/route.go` and map the values from DAG's route to [protobuf](https://www.envoyproxy.io/docs/envoy/latest/api-v2/api/v2/route/route.proto.html?highlight=shadow#route-corspolicy).

## Alternatives Considered

### Delegate all the CORS related logic to the applications
All the CORS related tasks could be handled in each application using an application level middleware or similar. However, it isn't something people usually want to do at this level for the following reasons: 

- This kind of configuration doesn't usually rely on developers because they are mainly focused on developing the services (the what), and not on which domains those services are going to be deployed on (the where), nor which security rules should be applied. This is usually a task for the Ops/DevOps/DevSecOps teams and that's the reason why all the reverse proxies out there provide this functionality alongside SSL termination, gzip compression and so on.
- In microservices driven architectures probably all the microservices will share the same CORS policy. It's much easier to manage this for multiple services if the policy isn't embedded in the application.


### Enable CORS with a single flag and apply sane defaults
It would be great if we could just enable a flag and apply some sane defaults. Unfortunately, it's not easy to find some defaults that would work for most of the proposed fields:

- **allowOrigin:** We could avoid this field and use '*' as the default value. However, as this opens the route for any domain, people would probably want to be able to set more restrictive values.
- **allowMethods:** We could allow all the methods (GET, PUT, POST, DELETE, PATCH, OPTIONS) and get rid of this field.
- **allowHeaders:** This is a very application specific setting. For instance, if a service is going to be consumed by JQuery clients, you'll probably want to allow the `X-Requested-With` header while for grpc-web services, you'll want to allow headers like `x-grpc-web` or `grpc-timeout`. 
- **exposeHeaders:** This is a very application specific setting as well. You might want to expose different headers depending on the application and the technologies used.
- **allowCredentials:** Setting a default value for this field is tricky because it has [some implications](https://www.w3.org/TR/2014/REC-cors-20140116/#supports-credentials).
- **MaxAge:** We could get rid of this field and set a default value. As a reference, Firefox specifies a 24 hours default value while Chromium sets it to 10 minutes.

### Develop a generic response header setting facility and handle CORS headers with it
This approach wouldn't be valid for managing all the CORS related logic. Some of the headers like `Access-Control-Allow-Headers` are only set in response to preflight requests (method=OPTIONS). In addition to this, we have to take into account that CORS is not just about headers, some server side logic is needed as well. For example, the server should return the `Access-Control-Allow-Origin` header with the `Origin` value sent by the browser only if that `Origin` is allowed.

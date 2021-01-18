# Automatic redirection with arbitrary listeners

Status: Accepted

## Abstract
As part of implementing Service APIs, it's necessary to reconsider Contour's insecure to secure redirect functionality.
This document suggests keeping the functionality, while allowing additional listening ports to be defined.

## Background
One of Contour's oldest features is automatic HTTP->HTTPS redirects. That is, if you configure a HTTPS connection, Contour will, by default, create a HTTP 301 redirection for the FQDN that you've created.

In addition, when configuring objects via HTTPProxy, by default Contour will not allow you to configure HTTP routes on a HTTPS virtualhost unless the 
`permitInsecure` option is enabled for the route. There's also `disablePermitInsecure` in the config file to disable this behavior.

In general, this is a useful feature for new users, and works well to increase security by default (since you have to work at disabling it.)

This feature is achieved by having Contour only serve two ports - a secure and an insecure one, and choosing which port to publish a route on by some rules around TLS.

However, when designing Contour's support for the Service APIs, this feature appears to conflict part of the Service APIs contract; namely that you can create an arbitrary number of listeners, on any port, even for the subset of the Service APIs that Contour is targeting.

This document outlines a design for adding support for arbitrary port listeners that attempts to allow marry to ease-of-use of the automatic redirect to the configurability of being able to add arbitrary extra listening ports to Envoy's configuration.

It should be noted here that using this feature, combined with a TCPProxy with no TLS configured, will allow a very bsaic facsimile of a Layer 4 load balancer to be implemented by Contour.
Contour's main purupose is still a Layer 7 Ingress Controller, not a Layer 4 load balancer.

## Goals
- Allow adding multiple (an arbitrary number) of extra listeners to Contour's Envoy fleet
- Maintain Contour's current insecure to secure redirection by default

## Non Goals
- TCP Layer 4 load balancing support on any address
- UDP Layer 4 load balancing support

## High-Level Design

### Configuring the secure and insecure ports

Currently, Contour allows the configuration of the secure and insecure listening ports for Envoy using the `--envoy-service-https-port` and the `--envoy-service-http-port` parameters respectively.
(It's fair to say that these parameters are confusingly named).

These should be exposed in the configuration file as and `secureListenerPort` and `insecureListenerPort` respectively.
They should also have aliases added for the command line flags to `--envoy-secure-listener-port` and `--envoy-insecure-listener-port` respectively.

The Contour Operator may then add support to its `Contour` CRD for tweaking this value.

For the redirect to be able to work currently, the example YAML and the Operator currently have the Envoy listen on port `8443` and `8080` respectively *inside its Pod*, but this is translated out to `80` and `443` by the `Service` or `Type: Loadbalancer` included there.
This means that the redirect generated can redirect from `http://somedomanin.com` to `https://somedomain.com` and have everything work.

For this feature to work more broadly, we must add a config parameter `IncludePortOnRedirect`, which, if set to `true`, will have the HTTP -> HTTPS redirect go from `http://somedomain.com/` to `https://omedomain.com:<secureport>/`.
This will resolve [#1300](https://github.com/projectcontour/contour/issues/1300) as well, and improve support for non-standard default HTTPS ports.
Note that this is only required if you change the **default** port, that gets the redirect to secure.
If you add additional HTTPS listener ports, they will **not** get a redirection.

### HTTPProxy processing

For HTTPProxy resources, we will add an optional `port` field to the `virtualhost` YAML stanza.

The rules for how this `port` field interacts with the rest of Contour would be this:
- no port set
    - TLS details set - default secure port, redirect will be created for you
    - no TLS details set - default insecure port, same `permitInsecure` behavior.
- port set
    - TLS details set - any port other than the insecure or secure ports, allow. No redirection created for you.
    - no TLS details set - any port other than the insecure or secure ports, allow.
    - use the insecure or the secure port - fatal error, HTTPProxy will not be processed.

The documents for the `port` field must clearly state that it is used for specifying a non-default port *only*. Don't use it for port 80 and 443.

### Service APIs processing

This design will be included in the Service APIs design document.

## Detailed Design
Pending agreement on the high-level design
## Alternatives Considered
### Don't change anything
We can keep the same "only two ports" requirements going forward, but that will not allow us to solve the issues:
[TCP Layer 4 Routing](https://github.com/projectcontour/contour/issues/3086)
[Exposing TCP with FQDN for mongodb](https://github.com/projectcontour/contour/issues/2922)

## Compatibility
The primary compatibility issue here is keeping the current HTTPProxy and Contour contracts, that is, that we keep the HTTP->HTTPS redirect by default.

## Implementation

TBD


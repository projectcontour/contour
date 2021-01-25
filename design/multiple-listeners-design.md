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

This document outlines a design for adding support for **additional** arbitrary port listeners that attempts to allow marry to ease-of-use of the automatic redirect to the configurability of being able to add arbitrary extra listening ports to Envoy's configuration.

It should be noted here that using this feature, combined with a TCPProxy with no TLS configured, will allow a very bsaic facsimile of a Layer 4 load balancer to be implemented by Contour.
Contour's main purpose is still a Layer 7 Ingress Controller, not a Layer 4 load balancer.
Contour will still require a HTTP port and a HTTPS port to be configured, this proposal allows for **additional** ports to be added.
These ports must also have a place to go to on your backend's Service.

## Goals
- Allow adding multiple (an arbitrary number) of extra listeners to Contour's Envoy fleet
- Maintain Contour's current insecure to secure redirection by default

## Non Goals
- TCP Layer 4 load balancing support on any address
- UDP Layer 4 load balancing support

## High-Level Design

### Extra problems to consider

Currently, Contour has command line flags to specify the ports that the Envoy containers should listen on.
There are only two ports you can specify, secure and insecure, which default to `8443` and `8080` respectively.
It's assumed that as part of the installation, your ports are translated from port `80` to `8080` and `443` to `8443`.
In the example YAMLs, this is done with a `hostPort` on the Envoy daemonset, and the `Service` refers to `80` and `443`, not the translated ports.
The important part for most Contour installations is the ports that are defined on the Envoy Service.

If you have a Service in the packet forwarding path, then any extra ports you ask Envoy to serve on *must* be exposed via that Service for traffic to reach them.

If you do something else (like running a separate set of machines for Envoy by themselves), then currently you *must* have the secure port set to `443` and the insecure to `80`.
Otherwise generated URLs will have no port, which is not correct if you are running on nonstandard ports.

### Configuring the secure and insecure ports

Currently, Contour allows the configuration of the secure and insecure listening ports for Envoy using the `--envoy-service-https-port` and the `--envoy-service-http-port` parameters respectively.
(It's fair to say that these parameters are confusingly named).

These will also be exposed in the configuration file as `secureListenerPort` and `insecureListenerPort` respectively.
They should also have aliases added for the command line flags to `--envoy-secure-listener-port` and `--envoy-insecure-listener-port` respectively.

The Contour Operator may then add support to its `Contour` CRD for tweaking this value.

At startup, and throughout its lifetime, Contour will watch the Envoy service and check that the ports specified are exposed, and, if they are translated, what they are translated *to*.
Contour will use the externally visible port as the basis for creating redirections and matching ports for listeners.
These ports are referred to as "externally visible ports" throughout this document,
with "externally visible secure port" meaning "whatever port an external user will go to to get to the secure port",
and similarly for the "externally visible insecure port".

Importantly, pecifying a port for Envoy's secure or insecure listening port that does not match the a port on the Service will be a fatal error for Contour.
This makes a simple misconfiguration that would result in no traffic flowing much more obvious.

If users require, at a later date, Contour may add a feature that disables the service lookup, and assumes that the externally visible ports are the specified ones (that is, that there is no port translation to the Envoy processes).

### Creating a redirection
For the redirect to be able to work currently, the example YAML and the Operator currently have the Envoy listen on port `8443` and `8080` respectively *inside its Pod*, but this is translated out to `80` and `443` by the `Service` or `Type: Loadbalancer` included there.
This means that the redirect generated can redirect from `http://somedomanin.com` to `https://somedomain.com` and have everything work.

As part of this change, Contour will include the port number on the HTTPS redirect, if the externally visible secure port is anything other than `443`.

### HTTPProxy processing

For HTTPProxy resources, Contour will add an optional `port` field to the `virtualhost` YAML stanza, which matches the externally visible port for the Envoys.
Specifying a port that does not match at least one `port` on the Envoy `Service` will result a fatal error for HTTPProxy processing, and an Error condition on the HTTPProxy.

The rules for how this `port` field interacts with the rest of Contour would then be this:
- no port set
    - TLS details set - default secure port, redirect will be created for you
    - no TLS details set - default insecure port, same `permitInsecure` behavior.
- port set
    - TLS details set - any port other than the external version of the secure or insecure ports, allow. A new listener will be created if it doesn't exist. No redirection created for you.
    - TLS details set - port is the external version of the secure or insecure ports - treat as though no port was set. Redirection will be created.
    - no TLS details set - any port other than the insecure or secure ports, allow. A new listener will be created if one doesn't exist.

A `port` setting, if set, will be included in the uniqueness check that currently only checks the FQDN.
Two HTTPProxies that both contain the same FQDN and port will both be rendered invalid (which mirrors the current behavior when only checking FQDN).

Contour will watch the `envoy` Service, and check that the supplied port matches one port on that Service, and apply slightly different logic if it's also one of the ports specified as secure or insecure in the config.

### Service APIs processing

This design will be included in the Service APIs design document.

## Detailed Design
Pending agreement on the high-level design
## Alternatives Considered
### Don't change anything
We can keep the same "only two ports" requirements going forward, but that will not allow us to solve this issue:
[Exposing TCP with FQDN for mongodb](https://github.com/projectcontour/contour/issues/2922)

## Compatibility
The primary compatibility issue here is keeping the current HTTPProxy and Contour contracts, that is, that we keep the HTTP->HTTPS redirect by default.

## Implementation

TBD


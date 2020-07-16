# Status Conditions for HTTPProxy

Status: Draft

## Abstract

This document describes a design and schema for adding Conditions to HTTPProxy’s `status` section. The purpose of these Conditions is to allow a HTTPProxy user to have an accurate view of what the HTTPProxy is configuring Contour to do.

## Background

HTTPProxy historically has had a very basic `status` section, which indicated whether it was “valid” or “invalid”, in the Validity field, and a reason, in the Description field.
Because HTTPProxy models a complex domain, there are many ways in which a configuration can be invalid, and it’s helpful to the user to be able to expose more than a single one at once.

In addition, it’s possible for a HTTPProxy or set of HTTPProxies to produce a partially-valid configuration, where Contour will configure Envoy with *part* of the desired config, but not all. The status should be able to represent this.

The Kubernetes API standard way to represent states in an extensible way is via a `conditions` stanza, defined in the [API conventions](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md), but with additional conversation ongoing at time of writing in PR [#4521](https://github.com/kubernetes/community/pull/4521). As a result of this, we've tried to steer a middle path here, and add only one top-level condition, with a similar behavior to existing conditions (that is, Ready). In addition, [KEP-1623](https://github.com/kubernetes/enhancements/tree/master/keps/sig-api-machinery/1623-standardize-conditions) has been moved to implementable, so what Conditions we do add will conform to that schema.


## Goals

- To define a standard Condition schema for HTTPProxy and its sibling CRDs (currently TLSDelegation, but should be applicable to any new CRDs also).
- To add a place in the CRDs for the Conditions to live.

## Non-goals
Add support for some method of exposing the inclusion graph in the HTTPProxy’s status. That is a separate issue (#xxx)

## High-level design

We're going to do the following things:
- add a `conditions` section to HTTPProxy `status`. We're doing this to allow for additional extensibility in the standard way.
- Add a `Valid` condition as the standard HTTPProxy condition. We've chosen a `Valid` condition here rather than a more traditional `Ready` because `Ready` implies that we're sure that the Envoy config has been accepted - which we don't have the mechanisms to ensure yet. We may add a `Ready` condition at a later date.
- The `Valid` condition's `Reason` field and the `validity` field will have the same value, as will the `Valid` condition's message field and the `description` field. That is, we will keep the functionality of the `validity` and `description` fields, but also duplicate it in the `Valid` Condition.
- Add `warnings` and `errors` as special sets of sub-conditions under the `Valid` condition. We need a way to provide additional detail to `Valid: false` HTTPProxies, and we also need a way to surface warnings. Tim Hockin suggested sub-conditions as a possible way of handling this, and it seems like this may be a good fit for our particular use case.
- The TLSDelegation `status` will also have a `conditions` section, with a `Valid` condition as above.

The `conditions` stanza also allows for external controllers to add information to the HTTPProxy (in particular, this allows `external-dns` to possibly add a `DNSProvisioned` condition or similar in the future).

Yes, the overloading of the term `conditions` between `status` and `spec` is unfortunate, but in order to be able to use standard tooling, we are stuck with the YAML key name for `status`.

## Detailed design

We will create the following generic type as a stand-in until the `metav1.Condition` type is available upstream.

```go
type Condition struct {
	// Type of condition in CamelCase or in foo.example.com/CamelCase.
	// Many .condition.type values are consistent across resources like Available, but because arbitrary conditions can be
	// useful (see .node.status.conditions), the ability to deconflict is important.
	// +required
	Type string `json:"type" protobuf:"bytes,1,opt,name=type"`
	// Status of the condition, one of True, False, Unknown.
	// +required
	Status ConditionStatus `json:"status" protobuf:"bytes,2,opt,name=status"`
	// If set, this represents the .metadata.generation that the condition was set based upon.
	// For instance, if .metadata.generation is currently 12, but the .status.condition[x].observedGeneration is 9, the condition is out of date
	// with respect to the current state of the instance.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty" protobuf:"varint,3,opt,name=observedGeneration"`
	// Last time the condition transitioned from one status to another.
	// This should be when the underlying condition changed.  If that is not known, then using the time when the API field changed is acceptable.
	// +required
	LastTransitionTime metav1.Time `json:"lastTransitionTime" protobuf:"bytes,4,opt,name=lastTransitionTime"`
	// The reason for the condition's last transition in CamelCase.
	// The specific API may choose whether or not this field is considered a guaranteed API.
	// This field may not be empty.
	// +required
	Reason string `json:"reason" protobuf:"bytes,5,opt,name=reason"`
	// A human readable message indicating details about the transition.
	// This field may be empty.
	// +required
	Message string `json:"message" protobuf:"bytes,6,opt,name=message"`
}
```
This will also have some convenience methods like `AddorUpdateCondition`, `GetCondition`, `IsPresent` and so on.

We'll also add an interface to cover this new struct, and then we'll add a slice of those interfaces of to `status` as `conditions`.

This will allow us to extend the `Condition` into a `DetailedCondition` as follows, while also adding a `SubCondition` to hold only the semantically-relevant fields:

```go
// SubCondition holds a subset of the fields of a Condition, since not all of them are semantically relevant
// for sub-conditions.
type SubCondition struct {
	// Type of condition in CamelCase or in foo.example.com/CamelCase.
	// Many .condition.type values are consistent across resources like Available, but because arbitrary conditions can be
	// useful (see .node.status.conditions), the ability to deconflict is important.
	// +required
	Type string `json:"type" protobuf:"bytes,1,opt,name=type"`
	// Status of the condition, one of True, False, Unknown.
	// +required
	Status ConditionStatus `json:"status" protobuf:"bytes,2,opt,name=status"`
	// The reason for the condition's last transition in CamelCase.
	// The specific API may choose whether or not this field is considered a guaranteed API.
	// This field may not be empty.
	// +required
	Reason string `json:"reason" protobuf:"bytes,5,opt,name=reason"`
	// A human readable message indicating details about the transition.
	// This field may be empty.
	// +required
	Message string `json:"message" protobuf:"bytes,6,opt,name=message"`
}


// DetailedCondition is an extension of the normal Kubernetes conditions, with two extra
// fields to hold sub-conditions, which provide more detailed reasons for the state (True or False)
// of the condition.
// `errors` holds information about sub-conditions which are fatal to that condition and render its state False.
// `warnings` holds information about sub-conditions which are not fatal to that condition and do not force the state to be False.
// Remember that Conditions have a type, a status, and a reason.
// The type is the type of the condition, the most important one in this CRD set is `Valid`.
// In the case of `Valid`, `status: true` means that the object is has been ingested into Contour with no errors.
// `warnings` may still be present, and will be indicated in the Reason field.
// `Valid`, `status: false` means that the object has had one or more fatal errors during processing into Contour.
//  The details of the errors will be present under the `errors` field.
type DetailedCondition struct {
  Condition
  // Errors contains a slice of relevant error subconditions for this object.
  // Subconditions are expected to appear when relevant (when there is an error), and disappear when not relevant.
  // An empty slice here indicates no errors.
  Errors []SubCondition `json:errors`
  // Warnings contains a slice of relevant error subconditions for this object.
  // Subconditions are expected to appear when relevant (when there is a warning), and disappear when not relevant.
  // An empty slice here indicates no warnings.
  Warnings []SubCondition `json:warnings`
}

```

The HTTPProxy `Status` struct will contain a `[]DetailedCondition` under the `conditions:` stanza.

The `Valid` condition is a positive-polarity summary condition like `Ready` is on other objects, while `warnings` and `errors` are slices of abnormal-true polarity conditions that further describe problems with the configuration.

The `Valid` condition must always be present when a HTTPProxy status is updated, although it may be set to either `status` `true` or `false`, meaning valid or invalid respectively.

So, for a `Valid` condition with `status: true`, `errors:` must be empty, and `warnings` may have entries.

If `errors` and `warnings` are empty, and `status` is true, then everything is good.

`status` should not be `Unknown` for the `Valid` condition, as the updates should be effectively atomic at the end of a DAG build.

If `status` is `false`, then there must be at least one entry under `errors`. If there is only one error, then that error's `reason` and `message` may be propagated up to `Valid`.

This will allow us to add extra conditions covering the broad categories of HTTPProxies currently present, like so:

```yaml
---
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: tls-example
  namespace: default
spec:
  routes:
  - conditions:
    - prefix: /
    services:
    - name: service-does-not-exist
      port: 80
  virtualhost:
    fqdn: foo2.bar.com
    tls:
      secretName: testsecret-does-not-exist
status:
  conditions:
  - type: Valid
    status: false
    observedGeneration: 1
    lastTransitionTime: <recently>
    reason: MultipleReasons
    message: "Multiple reasons, see the errors stanza for more"
    errors:
    - type: ServiceError
      status: true
      reason: ServiceNotFound
      message: "Service service-does-not-exist not found"
    - type: TLSError
      status: true
      reason: TLSSecretNotFound
      message: "TLS Secret testsecret-does-not-exist not found"
```

Or, to show an example we might do for warnings (not final, don't know if we would do this, but serves to illustrate how it would work):

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: warnings-example
  namespace: default
spec:
  routes:
  - conditions:
    - prefix: /
    services:
    - name: service-has-endpoints
      port: 80
    - name: service-has-no-endpoints
  virtualhost:
    fqdn: foo2.bar.com
    tls:
      secretName: testsecret
status:
  conditions:
  - type: Valid
    status: true
    observedGeneration: 1
    lastTransitionTime: <recently>
    reason: ValidHTTPProxy
    message: "This is a valid HTTPProxy with warnings"
    warnings:
    - type: ServiceError
      status: true
      reason: NoEndpoints
      message: "Service service-has-no-endpoints has no endpoints"
```

In terms of what sub-conditions we will add, after a quick survey of the current `internal/dag` module, we suggest something like this, which covers all the current error messages in that module (names obviously subject to change):

```
RootNamespaceError
VirtualHostError
TLSError
TLSFallbackError
TLSClientValidationError
IncludeError
PathConditionsError
HeaderConditionsError
HeadersPolicyError
TCPProxyError
```

There are a few implementation details to consider here:
- Do we have a generic `type` names that can be used between `errors` and `warnings`? Or do we just call everything an `Error` and have done?
- What are the exact details of the interface for these conditions?

## Alternatives considered

### All HTTPProxy errors as top-level Conditions

This created a problem - there are certain things that the community is starting to define as accepted Condition behavior, like the polarity, what a condition not being present means, and so on.

In particular, having a `Valid` or similar condition, plus error conditions that come and go as errors do means that the behavior of the `conditions` is not even internally consistent, let alone consistent with wider community standards.

In an effort to dodge some of this, we've chosen to only add one top-level Condition that mimics the existing `Ready` behavior, and do an experiment with sub-conditions instead.

## Appendix

### All errors possible in a DAG build at time of writing

RootNamespaceError
"root HTTPProxy cannot be defined in this namespace"

VirtualHostError
"Spec.VirtualHost.Fqdn must be specified"
"Spec.VirtualHost.Fqdn %q cannot use wildcards", host

TLSError
"Spec.VirtualHost.TLS Secret %q is invalid: %s", tls.SecretName, err
"Spec.VirtualHost.TLS Secret %q certificate delegation not permitted", tls.SecretName

TLSFallbackError
"Spec.Virtualhost.TLS fallback & client validation are incompatible together"
"Spec.Virtualhost.TLS enabled fallback but the fallback Certificate Secret is not configured in Contour configuration file"
"Spec.Virtualhost.TLS Secret %q fallback certificate is invalid: %s", b.FallbackCertificate, err
"Spec.VirtualHost.TLS fallback Secret %q is not configured for certificate delegation", b.FallbackCertificate


"Spec.VirtualHost.TLS client validation is invalid: %s", err
"Spec.VirtualHost.TLS passthrough cannot be combined with tls.clientValidation"
"tcpproxy: missing tls.passthrough or tls.secretName"

IncludeError
"include creates a delegation cycle: %s", strings.Join(path, " -> ")
"duplicate conditions defined on an include"
"include %s/%s not found", namespace, include.Name
"root httpproxy cannot delegate to another root httpproxy"
"include: %s", err (pathConditionsValid, path conditions check)
"route: %s", err (route conditions check using pathConditionsValid)
  - "prefix conditions must start with /, %s was supplied", cond.Prefix
  - "more than one prefix is not allowed in a condition block"
PathConditionsError

headerConditionsValid
  - "cannot specify duplicate header 'exact match' conditions in the same route"
  - "cannot specify contradictory 'exact' and 'notexact' conditions for the same route and header"
  - "cannot specify contradictory 'exact' and 'notexact' conditions for the same route and header"
  - "cannot specify contradictory 'contains' and 'notcontains' conditions for the same route and header"
  - "cannot specify contradictory 'contains' and 'notcontains' conditions for the same route and header"
  - 

headersPolicy, requestheaderspolicy
  - "duplicate header addition: %q", key
  - "rewriting %q header is not supported", key (Host not supported)
  - "invalid set header %q: %v", key, msgs (IsHTTPHeaderName)
  - "duplicate header removal: %q", key
  - "invalid remove header %q: %v", key, msgs

headersPolicy, responseheaderspolicy
"route.services must have at least one entry"

"cannot specify prefix replacements without a prefix condition"
prefixReplacementsAreValid (per-route)
- "duplicate replacement prefix '%s'", r.Prefix
- "ambiguous prefix replacement" Can't replace the empty prefix multiple times.

"service %q: port must be in the range 1-65535", service.Name
"Service [%s:%d] is invalid or missing", service.Name, service.Port
Service protocol is unsupported
"Service [%s:%d] TLS upstream validation policy error: %s",
						service.Name, service.Port, err
service requestheaderspolicy
service responseheaderspolicy
"only one service per route may be nominated as mirror
"tcpproxy: cannot specify services and include in the same httpproxy"
"tcpproxy: service %s/%s/%d: not found", httpproxy.Namespace, service.Name, service.Port
"tcpproxy: either services or inclusion must be specified"
"tcpproxy: include %s/%s not found", m.Namespace, m.Name
"root httpproxy cannot delegate to another root httpproxy"
"tcpproxy include creates a cycle: %s", strings.Join(path, " -> ")

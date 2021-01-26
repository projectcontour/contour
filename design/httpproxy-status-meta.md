# HTTPProxy Status Metadata

Status: Draft

## Abstract
Since the addition of `Includes` for HTTPProxy, it can be difficult for owners of child HTTPProxy resources to understand what has been included to them without having access to a parent HTTPProxy.
 
Users should have a way to understand this extra metadata about the ingress resources. 
In addition, with this information, additional tooling can be developed against HTTPProxy resources to drive dashboards (i.e. Octant) and other `kubectl plugins`.

## Goals
- Expose extra metadata information to HTTPProxy resources

## Use-Cases
- Allow owners of child HTTPProxy resource to understand what has been included to them without having access to a parent HTTPProxy

## Future Use-Cases
- If authentication is enabled
- VHost has TLS

## Detailed Design
Extend the HTTPProxy.Status struct to include additional fields to support exposing metadata information:

```
type HTTPProxyStatus struct {
  ...
  <existing fields>
  ...
  
  Metadata HTTPProxyMetadata `json:"metadata,omitempty"`
}

type HTTPProxyMetadata struct {
  // ParentsRef shows one level parent of any HTTPProxy resources 
  // which have included this resource.
  Includes []HTTPProxyParent `json:"includes,omitempty"`
}

// HTTPProxyParent describes an HTTPProxy who included
// any MatchConditions or Fqdn to a child HTTPProxy.
type HTTPProxyParent struct {
  ObjectReference `json:
  // Name of HTTPProxy object
  Name string  `json:"name,omitempty"`

  // Namespace of HTTPProxy Object
  Namespace string `json:"namespace,omitempty"`

  // Fqdn of HTTPProxy Object
  Fqdn string `json:"fqdn,omitempty"`

  // IncludedConditions show the set of MatchConditions available 
  // to this HTTPProxy summing together any include that a parent
  // may have passed to this child resource. 
  IncludedConditions []MatchCondition  `json:"includedConditions,omitempty"`
}
``` 

## Open Issues
- https://github.com/projectcontour/contour/issues/1598
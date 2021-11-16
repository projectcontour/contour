---
title: Contour Deprecation Policy
layout: page
---

Contour publishes a few APIs, the most notable being the `projectcontour.io` api group of Kubernetes objects and their associated helper code, the command line for `contour`, and the Contour configuration file.
Each of these have deprecation policies, with all of them being based on the [Kubernetes API deprecation policy](https://kubernetes.io/docs/reference/using-api/deprecation-policy/).



## `projectcontour.io` API groups (aka Contour CRDs)

Our Kubernetes CRDs use the Kubernetes API deprecation conventions, including a similar deprecation timeline.

- There are three levels of stability, Alpha, Beta and GA (in increasing order).
- We may skip the Beta level when we are reasonably confident the schema is good.

We use similar rules as Kubernetes:
- For Beta and GA versions, API elements may only be removed by incrementing the version of the API group.
- For Alpha versions, rules on behavior changes and API field removal are more lenient, a version bump will not always be required.
- An API version in a given track may not be deprecated until a new API version at least as stable is released.


| Sample Version Tag | State | Deprecation timeframe | Notes                                                                                                      |
| ------------------ | ----- | --------------------- | ---------------------------------------------------------------------------------------------------------- |
| v1alpha1           | Alpha | Any time              | Behavior can change any time, Fields can be removed at any time                                            |
| v1beta1            | Beta  | 1 release             | Behavior can change any time, fields won't be removed without a version bump (ie `v1beta1` to `v1beta2`) |
| v1                 | GA    | 1 year                | No fields will be removed, no behavior will substantially change. Fields can be added.                     |


## `projectcontour.io` CRD helper code

The `projectcontour.io` CRDs contain some helper code, for accessing various parts of the Go structs inside.

The API guarantees apply here as well, in the following way:

| Sample Version Tag | State | Change/Deprecation timeframe | Notes                                                                                                      |
| ------------------ | ----- | --------------------- | ---------------------------------------------------------------------------------------------------------- |
| v1alpha1           | Alpha | Any time              | Function and method signatures can change any time. Implementation may change any time.                         |
| v1beta1            | Beta  | 1 release            | Function and method signatures won't change without a version bump. Implementation may change any time. |
| v1                 | GA    | 1 year                | Function and method signatures won't change without a version bump. Implementation may change any time, but behavior changes must be restricted to minor ones (that is, you can change how a return value is made, but not what it means)|



## Contour command line arguments

Because removing command line arguments is a breaking operation (that is, the program won't start without them), Contour is committed to a gradual transition for changes here.

We try to use the following cycle for arguments:
- Argument is announced deprecated, with a timeline for removal. This timeframe must never be shorter than 3 releases, but may be longer if required.
- At the same time, Argument has a warning added, saying that it's deprecated.
- We wait the timeline period
- The argument is removed.



## Configuration file settings

We use the following cycle for config file settings:
- Setting is announced deprecated, with a plan for how the functionality will be moved or removed, and a timeline.
- Warning log added for deprecated setting.
- We wait the timeline period
- The setting is moved or removed.


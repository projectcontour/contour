# Contour Operator Gateway API Support Migration

Status: Draft

## Abstract
Currently the Contour Operator and Contour Controller have both started implementing Gateway API support.
Both components contain controllers that reconcile GatewayClass and Gateway resources and that conflict when reconciling status.
This document describes options for how we should proceed to untangle these.

## Background
Currently the state of the world is as below:

### Contour Operator
- Configured with a GatewayClass object reference (name) to reconcile
- Includes GatewayClass controller
  - Checks if named GatewayClass has the correct (hardcoded to `projectcontour.io/contour-operator`) controller string
  - Checks for Contour CRD in the parameters ref of the GatewayClass, uses parameters for provisioning Contour instances
  - Sets admitted status on object if all valid
- Includes Gateway controller
  - Checks if Gateway refers to correct GatewayClass and that GatewayClass has been admitted
  - Sets up finalizer for Gateway
  - Creates Contour deployment resources with the Gateway being reconciled named in Contour's ConfigMap
  - Sets up finalizer for GatewayClass
  - Sets up finalizer for Contour CRD referenced in GatewayClass
  - Sets admitted status on object if all valid

### Contour Controller
- Needs to be configured with a GatewayClass controller name to ensure it reconciles the correct GatewayClass
- Also needs a Gateway namespace/name which is being deprecated
- Includes GatewayClass controller
  - Expects parameter ref field to be unset, otherwise GatewayClass is not admitted
  - Admits GatewayClass that has the configured controller name
- Includes Gateway controller
  - Checks if Gateway references Contour owned GatewayClass
  - Passes off reconciliation to dag package Gateway API processor
  - Gateway is validated, status set on it and relevant *Routes

### Current Problems
- Contour requires Gateway namespace/name *and* controller name, the latter of which the operator does not provide
- Contour expects GatewayClass parameters ref to be unset while Operator expects it to reference a Contour CRD
- If the above is rectified, Contour and the Operator still will conflict on setting status
- CI failures that occur because of conflicts between Contour and the Operator are not detected unless we manually look at the scheduled Operator CI job

## Goals
- Improve implementation in Contour and Operator to mitigate current issues
- Improve development processes and practices to ensure this conflicting state does not occur again

## Non Goals
- Drop support for features or projects

## High-Level Design
One to two paragraphs that describe the high level changes that will be made to implement this proposal.

## Detailed Design
A detailed design describing how the changes to the product should be made.

The names of types, fields, interfaces, and methods should be agreed on here, not debated in code review.
The same applies to changes in CRDs, YAML examples, and so on.

Ideally the changes should be made in sequence so that the work required to implement this design can be done incrementally, possibly in parallel.

## Alternatives Considered
If there are alternative high level or detailed designs that were not pursued they should be called out here with a brief explanation of why they were not pursued.

## Security Considerations
If this proposal has an impact to the security of the product, its users, or data stored or transmitted via the product, they must be addressed here.

## Compatibility
A discussion of any compatibility issues that need to be considered

## Implementation
A description of the implementation, timelines, and any resources that have agreed to contribute.

## Open Issues
A discussion of issues relating to this proposal for which the author does not know the solution. This section may be omitted if there are none.

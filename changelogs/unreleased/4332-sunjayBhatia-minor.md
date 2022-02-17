## Leader election now only uses Lease object

Contour now only uses the Lease object to coordinate leader election.
RBAC in example manifests has been updated accordingly.

**Note:** Upgrading to this version of Contour will explicitly require you to upgrade to Contour v1.20.0 *first* to ensure proper migration of leader election coordination resources.

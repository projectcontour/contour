### Gateway API: set Gateway Listener status fields

Contour now sets the `.status.listeners.supportedKinds` and `.status.listeners.attachedRoutes` fields on Gateways for Gateway API.
The first describes the list of route groups/kinds that the listener supports, and the second captures the total number of routes that are successfully attached to the listener.

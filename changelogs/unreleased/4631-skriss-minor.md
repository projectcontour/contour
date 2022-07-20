## Gateway API: enforce correct TLS modes for HTTPS and TLS listener protocols

Contour now enforces that the correct TLS modes are used for the HTTPS and TLS listener protocols.
For an HTTPS listener, the TLS mode "Terminate" must be used (this is compatible with HTTPRoutes).
For a TLS listener, the TLS mode "Passthrough" must be used (this is compatible with TLSRoutes).

## Host header including port is passed through unmodified to backend

Previously Contour would strip any port from the Host header in a downstream request for convenience in routing.
This resulted in backends not receiving the Host header with a port.
We no longer do this, for conformance with Gateway API (this change also applies to HTTPProxy and Ingress configuration).

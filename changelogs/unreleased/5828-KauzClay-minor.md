## Allow Configuration of Upstream TLS Options

In a similar way to how Contour users can configure Min/Max TLS version and
Cipher Suites for Envoy's listeners, this change allows users to specify the
same information for upstream connections. This change also defaults the Max TLS
version for upstream connections to 1.3, instead of the Envoy default of 1.2.
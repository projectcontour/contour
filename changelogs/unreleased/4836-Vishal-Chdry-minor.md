Added support for `ALL` DNS lookup family
If ALL is specified, the DNS resolver will perform a lookup for
both IPv4 and IPv6 families, and return all resolved addresses.
When this is used, Happy Eyeballs will be enabled for upstream connections.
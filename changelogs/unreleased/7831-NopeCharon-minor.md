## Copy JWT claims to request headers

HTTPProxy JWT providers have a new `claimToHeaders` field that copies claims of a successfully verified JWT to request headers, so backend services can consume the claims without decoding the JWT themselves.
Nested claims can be referenced with `.`, for example `org.tenant`.
Client supplied values for the configured headers are removed from requests to the virtual host, including on routes where JWT verification is disabled.
See the [JWT verification documentation](https://projectcontour.io/docs/main/config/jwt-verification/) for details.

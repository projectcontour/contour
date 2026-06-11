## HTTPProxy JWT providers can load JWKS from a Kubernetes Secret

HTTPProxy JWT providers now support loading JWKS from a Kubernetes Secret via `localJWKS`, in addition to fetching keys from a remote endpoint with `remoteJWKS`.
Each provider must specify exactly one of these sources.
See the [JWT verification documentation](https://projectcontour.io/docs/main/config/jwt-verification) for configuration details.

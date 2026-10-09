## Derive X-Forwarded-Proto from the PROXY protocol destination port

Contour now exposes Envoy's `forward_proto_config` as `listener.forward-proto-config` in the configuration file and as `spec.envoy.listener.forwardProtoConfig` in `ContourConfiguration`.
When a layer 4 load balancer in front of Envoy terminates TLS and forwards with PROXY protocol, Envoy can set `X-Forwarded-Proto` from the destination port in the PROXY protocol header (for example `https` for 443 and `http` for 80) instead of always `http`, so HTTPProxy virtual hosts with TLS no longer redirect their own HTTPS traffic in a loop.

## Disable Envoy removing TE header

As of version v1.29.0, Envoy removes the hop-by-hop TE header.
However, this causes issues with HTTP/2, particularly gRPC, with implementations expecting the header to be present (and set to `trailers`).
Contour disables this via Envoy runtime setting and reverts to the v1.28.x and prior behavior of allowing the header to be proxied.

Once [this Envoy PR that enables the TE header including `trailers` to be forwarded](https://github.com/envoyproxy/envoy/pull/32255) is backported to a release or a new minor is cut, Contour will no longer set the aforementioned runtime key.

Make it possible to enable TLS fingerprinting in Envoy's TLS Inspector Listener filter, useful for security monitoring, analytics, and bot detection. Provides independent control over JA3 and JA4 fingerprinting methods.

Fingerprints can be consumed by:
- Logging in access logs using `%TLS_JA3_FINGERPRINT%` / `%TLS_JA4_FINGERPRINT%` format operators or the `tls_ja3_fingerprint` / `tls_ja4_fingerprint` JSON log fields.
- Setting dynamic request headers to forward fingerprints to backend services (e.g. `%TLS_JA3_FINGERPRINT%` / `%TLS_JA4_FINGERPRINT%` in header policy values).

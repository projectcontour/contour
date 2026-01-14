# Implementation Plan for JA3/JA4 Fingerprinting Support in Contour

## Overview
Add support for enabling JA3 and JA4 TLS fingerprinting in Contour's EnvoyTLS configuration. This will configure Envoy's TLS Inspector listener filter to populate JA3/JA4 hashes in dynamic metadata for HTTPS connections.

## Prerequisites
- Envoy 1.37+ supports `enable_ja3_fingerprinting` and `enable_ja4_fingerprinting` in the TLS Inspector filter
- Fingerprints are MD5 (JA3) and hex (JA4) strings available in dynamic metadata under `envoy.filters.listener.tls_inspector`

## Detailed Steps

### 1. Update API Types
**File:** `apis/projectcontour/v1alpha1/contourconfig.go`

Add the following fields to the `EnvoyTLS` struct after `CipherSuites`:

```go
	// EnableJA3Fingerprinting enables JA3 fingerprinting in the TLS Inspector.
	// When true, populates JA3 hash in dynamic metadata.
	// +optional
	EnableJA3Fingerprinting *bool `json:"enableJA3Fingerprinting,omitempty"`

	// EnableJA4Fingerprinting enables JA4 fingerprinting in the TLS Inspector.
	// When true, populates JA4 hash in dynamic metadata.
	// +optional
	EnableJA4Fingerprinting *bool `json:"enableJA4Fingerprinting,omitempty"`
```

**Rationale:** Adds optional boolean pointers to match Kubernetes API patterns and allow nil (unset) vs false distinction.

### 2. Regenerate Generated Code
**Command:** `make generate`

This will update:
- `apis/projectcontour/v1alpha1/zz_generated.deepcopy.go`
- Other generated files for the API changes

**Validation:** Ensure the deepcopy functions include the new fields.

### 3. Update ListenerConfig
**File:** `internal/xdscache/v3/listener.go`

Add to the `ListenerConfig` struct:

```go
	// EnableJA3Fingerprinting enables JA3 fingerprinting for HTTPS listeners.
	EnableJA3Fingerprinting *bool

	// EnableJA4Fingerprinting enables JA4 fingerprinting for HTTPS listeners.
	EnableJA4Fingerprinting *bool
```

### 4. Update ServeContext
**File:** `cmd/contour/servecontext.go`

In the `NewServeContext` function, after setting `cfg.CipherSuites`, add:

```go
cfg.EnableJA3Fingerprinting = ctx.Config.TLS.EnableJA3Fingerprinting
cfg.EnableJA4Fingerprinting = ctx.Config.TLS.EnableJA4Fingerprinting
```

### 5. Modify TLS Inspector Builder
**File:** `internal/envoy/v3/listener.go`

Update the `TLSInspector` function signature and implementation:

```go
// TLSInspector returns a new TLS inspector listener filter with optional fingerprinting.
func TLSInspector(enableJA3, enableJA4 *bool) *envoy_config_listener_v3.ListenerFilter {
	inspector := &envoy_filter_listener_tls_inspector_v3.TlsInspector{}
	if enableJA3 != nil && *enableJA3 {
		inspector.EnableJa3Fingerprinting = wrapperspb.Bool(true)
	}
	if enableJA4 != nil && *enableJA4 {
		inspector.EnableJa4Fingerprinting = wrapperspb.Bool(true)
	}
	return &envoy_config_listener_v3.ListenerFilter{
		Name: wellknown.TlsInspector,
		ConfigType: &envoy_config_listener_v3.ListenerFilter_TypedConfig{
			TypedConfig: protobuf.MustMarshalAny(inspector),
		},
	}
}
```

**Imports needed:** Add `"google.golang.org/protobuf/types/known/wrapperspb"` if not present.

### 6. Update Secure Proxy Protocol
**File:** `internal/xdscache/v3/listener.go`

Update `secureProxyProtocol` function:

```go
func secureProxyProtocol(useProxy bool, enableJA3, enableJA4 *bool) []*envoy_config_listener_v3.ListenerFilter {
	filters := proxyProtocol(useProxy)
	filters = append(filters, envoy_v3.TLSInspector(enableJA3, enableJA4))
	return filters
}
```

Update the call site in `OnChange()` method around line 425:

```go
secureProxyProtocol(cfg.UseProxyProto, cfg.EnableJA3Fingerprinting, cfg.EnableJA4Fingerprinting)
```

### 7. Update Tests
Update all files calling `TLSInspector()` to pass `nil, nil` for the new parameters.

**Files to update:**
- `internal/xdscache/v3/listener_test.go`
- `internal/featuretests/v3/*.go` (multiple files)
- `internal/envoy/v3/listener_test.go`

Example change:
```go
envoy_v3.TLSInspector()  // old
envoy_v3.TLSInspector(nil, nil)  // new
```

### 8. Validation
**Commands:**
- `make generate` - Regenerate code
- `make check` - Run lint, typecheck, unit tests
- `make test` - Run full test suite

**Manual Testing:**
- Configure Contour with `enableJA3Fingerprinting: true`
- Verify TLS Inspector config in Envoy
- Check dynamic metadata in access logs or routing

## Usage Example
```yaml
apiVersion: projectcontour.io/v1alpha1
kind: ContourConfiguration
spec:
  envoy:
    listener:
      tls:
        enableJA3Fingerprinting: true
        enableJA4Fingerprinting: true
```

## Backward Compatibility
- Fields are optional with nil default (disabled)
- Existing configurations continue to work unchanged
- Only HTTPS listeners get the TLS Inspector (unchanged)

## Risks
- Slight performance impact from fingerprinting computation
- Metadata bloat if fingerprints are logged extensively
- Requires Envoy 1.37+ in production
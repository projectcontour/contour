// Copyright Project Contour Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package annotation

import (
	"fmt"
	"strconv"
	"strings"

	networking_v1 "k8s.io/api/networking/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/projectcontour/contour/internal/timeout"
)

// IsKnown checks if an annotation is one Contour knows about.
func IsKnown(key string) bool {
	// We should know about everything with a Contour prefix.
	if strings.HasPrefix(key, "projectcontour.io/") {
		return true
	}

	// We could reasonably be expected to know about all Ingress
	// annotations.
	if strings.HasPrefix(key, "ingress.kubernetes.io/") {
		return true
	}

	switch key {
	case "kubernetes.io/ingress.class",
		"kubernetes.io/ingress.allow-http",
		"kubernetes.io/ingress.global-static-ip-name":
		return true
	default:
		return false
	}
}

var annotationsByKind = map[string]map[string]struct{}{
	"Ingress": {
		"ingress.kubernetes.io/force-ssl-redirect":       {},
		"kubernetes.io/ingress.allow-http":               {},
		"kubernetes.io/ingress.class":                    {},
		"projectcontour.io/ingress.class":                {},
		"projectcontour.io/num-retries":                  {},
		"projectcontour.io/response-timeout":             {},
		"projectcontour.io/retry-on":                     {},
		"projectcontour.io/tls-minimum-protocol-version": {},
		"projectcontour.io/tls-maximum-protocol-version": {},
		"projectcontour.io/tls-cert-namespace":           {},
		"projectcontour.io/websocket-routes":             {},
	},
	"Service": {
		"projectcontour.io/max-connections":          {},
		"projectcontour.io/max-pending-requests":     {},
		"projectcontour.io/max-requests":             {},
		"projectcontour.io/max-retries":              {},
		"projectcontour.io/per-host-max-connections": {},
		"projectcontour.io/upstream-protocol.h2":     {},
		"projectcontour.io/upstream-protocol.h2c":    {},
		"projectcontour.io/upstream-protocol.tls":    {},
	},
	"HTTPProxy": {
		"kubernetes.io/ingress.class":     {},
		"projectcontour.io/ingress.class": {},
	},
	"Secret": {
		"projectcontour.io/generated-by-version": {},
	},
}

// ValidForKind checks if a particular annotation is valid for a given Kind.
func ValidForKind(kind, key string) bool {
	if a, ok := annotationsByKind[kind]; ok {
		_, ok := a[key]
		return ok
	}

	// We should know about every kind with a Contour annotation prefix.
	if strings.HasPrefix(key, "projectcontour.io/") {
		return false
	}

	// This isn't a kind we know about so assume it is valid.
	return true
}

// ContourAnnotation checks the Object for the given annotation with the
// "projectcontour.io/" prefix.
func ContourAnnotation(o meta_v1.Object, key string) string {
	a := o.GetAnnotations()

	return a["projectcontour.io/"+key]
}

// ParseUInt32 parses the supplied string as if it were a uint32.
// If the value is not present, or malformed, or outside uint32's range, zero is returned.
func parseUInt32(s string) uint32 {
	v, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		return 0
	}
	return uint32(v)
}

// ParseInt32 parses the supplied string as if it were a int32.
// If the value is not present, or malformed, zero is returned.
func parseInt32(s string) int32 {
	v, err := strconv.ParseInt(s, 10, 32)
	if err != nil {
		return 0
	}
	return int32(v)
}

// ParseUpstreamProtocols parses the annotations map for
// projectcontour.io/upstream-protocol.{protocol} annotations.
// 'protocol' identifies which protocol must be used in the upstream.
func ParseUpstreamProtocols(m map[string]string) map[string]string {
	protocols := []string{"h2", "h2c", "tls"}
	up := make(map[string]string)
	for _, protocol := range protocols {
		ports := m[fmt.Sprintf("projectcontour.io/upstream-protocol.%s", protocol)]
		for _, v := range strings.Split(ports, ",") {
			port := strings.TrimSpace(v)
			if port != "" {
				up[port] = protocol
			}
		}
	}
	return up
}

// HTTPAllowed returns true unless the kubernetes.io/ingress.allow-http annotation is
// present and set to false.
func HTTPAllowed(i *networking_v1.Ingress) bool {
	return !(i.Annotations["kubernetes.io/ingress.allow-http"] == "false")
}

// TLSRequired returns true if the ingress.kubernetes.io/force-ssl-redirect annotation is
// present and set to true.
func TLSRequired(i *networking_v1.Ingress) bool {
	return i.Annotations["ingress.kubernetes.io/force-ssl-redirect"] == "true"
}

// TLSCertNamespace returns the namespace name of the delegated certificate if
// projectcontour.io/tls-cert-namespace annotation is present and non-empty
func TLSCertNamespace(i *networking_v1.Ingress) string {
	return ContourAnnotation(i, "tls-cert-namespace")
}

// WebsocketRoutes retrieves the details of routes that should have websockets enabled from the
// associated websocket-routes annotation.
func WebsocketRoutes(i *networking_v1.Ingress) map[string]bool {
	routes := make(map[string]bool)
	for _, v := range strings.Split(i.Annotations["projectcontour.io/websocket-routes"], ",") {
		route := strings.TrimSpace(v)
		if route != "" {
			routes[route] = true
		}
	}
	return routes
}

// NumRetries returns the number of retries specified by the
// "projectcontour.io/num-retries" annotation.
func NumRetries(i *networking_v1.Ingress) uint32 {
	val := parseInt32(ContourAnnotation(i, "num-retries"))

	// If set to -1, then retries set to 0. If set to 0 or
	// not supplied, the value is set to the Envoy default of 1.
	// Otherwise the value supplied is returned.
	switch val {
	case -1:
		return 0
	case 1, 0:
		return 1
	}

	// If set to other negative value than 1, then fall back to Envoy default.
	if val < 0 {
		return 1
	}

	return uint32(val)
}

// PerTryTimeout returns the duration envoy will wait per retry cycle.
func PerTryTimeout(i *networking_v1.Ingress) (timeout.Setting, error) {
	return timeout.Parse(ContourAnnotation(i, "per-try-timeout"))
}

// IngressClass returns the first matching ingress class for the following
// annotations:
// 1. projectcontour.io/ingress.class
// 2. kubernetes.io/ingress.class
func IngressClass(o meta_v1.Object) string {
	a := o.GetAnnotations()
	if class, ok := a["projectcontour.io/ingress.class"]; ok {
		return class
	}
	if class, ok := a["kubernetes.io/ingress.class"]; ok {
		return class
	}
	return ""
}

// TLSVersion returns the TLS protocol version specified by an ingress annotation
// or default if non present.
func TLSVersion(version, defaultVal string) string {
	switch version {
	case "1.2", "1.3":
		return version
	default:
		return defaultVal
	}
}

// MaxConnections returns the value of the first matching max-connections
// annotation for the following annotations:
// 1. projectcontour.io/max-connections
//
// '0' is returned if the annotation is absent or unparsable.
func MaxConnections(o meta_v1.Object) uint32 {
	return parseUInt32(ContourAnnotation(o, "max-connections"))
}

// MaxPendingRequests returns the value of the first matching max-pending-requests
// annotation for the following annotations:
// 1. projectcontour.io/max-pending-requests
//
// '0' is returned if the annotation is absent or unparsable.
func MaxPendingRequests(o meta_v1.Object) uint32 {
	return parseUInt32(ContourAnnotation(o, "max-pending-requests"))
}

// MaxRequests returns the value of the first matching max-requests
// annotation for the following annotations:
// 1. projectcontour.io/max-requests
//
// '0' is returned if the annotation is absent or unparsable.
func MaxRequests(o meta_v1.Object) uint32 {
	return parseUInt32(ContourAnnotation(o, "max-requests"))
}

// MaxRetries returns the value of the first matching max-retries
// annotation for the following annotations:
// 1. projectcontour.io/max-retries
//
// '0' is returned if the annotation is absent or unparsable.
func MaxRetries(o meta_v1.Object) uint32 {
	return parseUInt32(ContourAnnotation(o, "max-retries"))
}

// PerHostMaxConnections returns the value of the first matching
// per-host-max-connectionss annotation for the following annotations:
// 1. projectcontour.io/per-host-max-connections
//
// '0' is returned if the annotation is absent or unparsable.
func PerHostMaxConnections(o meta_v1.Object) uint32 {
	return parseUInt32(ContourAnnotation(o, "per-host-max-connections"))
}

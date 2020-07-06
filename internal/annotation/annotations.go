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
	"time"

	envoy_api_v2_auth "github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	"k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DEFAULT_INGRESS_CLASS is the Contour default.
const DEFAULT_INGRESS_CLASS = "contour"

// IsKnown checks if an annotation is one Contour knows about.
func IsKnown(key string) bool {
	// We should know about everything with a Contour prefix.
	if strings.HasPrefix(key, "projectcontour.io/") ||
		strings.HasPrefix(key, "contour.heptio.com/") {
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
		"projectcontour.io/websocket-routes":             {},
	},
	"Service": {
		"projectcontour.io/max-connections":       {},
		"projectcontour.io/max-pending-requests":  {},
		"projectcontour.io/max-requests":          {},
		"projectcontour.io/max-retries":           {},
		"projectcontour.io/upstream-protocol.h2":  {},
		"projectcontour.io/upstream-protocol.h2c": {},
		"projectcontour.io/upstream-protocol.tls": {},
	},
	"HTTPProxy": {
		"kubernetes.io/ingress.class":     {},
		"projectcontour.io/ingress.class": {},
	},
}

// ValidForKind checks if a particular annotation is valid for a given Kind.
func ValidForKind(kind string, key string) bool {
	if a, ok := annotationsByKind[kind]; ok {
		// Canonicalize the name while we still have legacy support.
		key = strings.Replace(key, "contour.heptio.com/", "projectcontour.io/", -1)
		_, ok := a[key]
		return ok
	}

	// We should know about every kind with a Contour annotation prefix.
	if strings.HasPrefix(key, "projectcontour.io/") ||
		strings.HasPrefix(key, "contour.heptio.com/") {
		return false
	}

	// This isn't a kind we know about so assume it is valid.
	return true
}

// CompatAnnotation checks the Object for the given annotation, first with the
// "projectcontour.io/" prefix, and then with the "contour.heptio.com/" prefix
// if that is not found.
func CompatAnnotation(o metav1.ObjectMetaAccessor, key string) string {
	a := o.GetObjectMeta().GetAnnotations()

	if val, ok := a["projectcontour.io/"+key]; ok {
		return val
	}

	return a["contour.heptio.com/"+key]
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

// ParseUpstreamProtocols parses the annotations map for contour.heptio.com/upstream-protocol.{protocol}
// and projectcontour.io/upstream-protocol.{protocol} annotations.
// 'protocol' identifies which protocol must be used in the upstream.
func ParseUpstreamProtocols(m map[string]string) map[string]string {
	annotations := []string{
		"contour.heptio.com/upstream-protocol",
		"projectcontour.io/upstream-protocol",
	}
	protocols := []string{"h2", "h2c", "tls"}
	up := make(map[string]string)
	for _, annotation := range annotations {
		for _, protocol := range protocols {
			ports := m[fmt.Sprintf("%s.%s", annotation, protocol)]
			for _, v := range strings.Split(ports, ",") {
				port := strings.TrimSpace(v)
				if port != "" {
					up[port] = protocol
				}
			}
		}
	}
	return up
}

// HTTPAllowed returns true unless the kubernetes.io/ingress.allow-http annotation is
// present and set to false.
func HTTPAllowed(i *v1beta1.Ingress) bool {
	return !(i.Annotations["kubernetes.io/ingress.allow-http"] == "false")
}

// TLSRequired returns true if the ingress.kubernetes.io/force-ssl-redirect annotation is
// present and set to true.
func TLSRequired(i *v1beta1.Ingress) bool {
	return i.Annotations["ingress.kubernetes.io/force-ssl-redirect"] == "true"
}

// WebsocketRoutes retrieves the details of routes that should have websockets enabled from the
// associated websocket-routes annotation.
func WebsocketRoutes(i *v1beta1.Ingress) map[string]bool {
	routes := make(map[string]bool)
	for _, v := range strings.Split(i.Annotations["projectcontour.io/websocket-routes"], ",") {
		route := strings.TrimSpace(v)
		if route != "" {
			routes[route] = true
		}
	}
	for _, v := range strings.Split(i.Annotations["contour.heptio.com/websocket-routes"], ",") {
		route := strings.TrimSpace(v)
		if route != "" {
			routes[route] = true
		}
	}
	return routes
}

// NumRetries returns the number of retries specified by the "contour.heptio.com/num-retries"
// or "projectcontour.io/num-retries" annotation.
func NumRetries(i *v1beta1.Ingress) uint32 {
	return parseUInt32(CompatAnnotation(i, "num-retries"))
}

// PerTryTimeout returns the duration envoy will wait per retry cycle.
func PerTryTimeout(i *v1beta1.Ingress) time.Duration {
	return ParseTimeout(CompatAnnotation(i, "per-try-timeout"))
}

// IngressClass returns the first matching ingress class for the following
// annotations:
// 1. projectcontour.io/ingress.class
// 2. contour.heptio.com/ingress.class
// 3. kubernetes.io/ingress.class
func IngressClass(o metav1.ObjectMetaAccessor) string {
	a := o.GetObjectMeta().GetAnnotations()
	if class, ok := a["projectcontour.io/ingress.class"]; ok {
		return class
	}
	if class, ok := a["contour.heptio.com/ingress.class"]; ok {
		return class
	}
	if class, ok := a["kubernetes.io/ingress.class"]; ok {
		return class
	}
	return ""
}

// MatchesIngressClass checks that the passed object has an ingress class that matches
// either the passed ingress-class string, or DEFAULT_INGRESS_CLASS if it's empty.
func MatchesIngressClass(o metav1.ObjectMetaAccessor, ic string) bool {

	switch IngressClass(o) {
	case ic:
		// Handles ic == "" and ic == "custom".
		return true
	case DEFAULT_INGRESS_CLASS:
		// ic == "" implicitly matches the default too.
		if ic == "" {
			return true
		}
	}

	return false

}

// MinTLSVersion returns the TLS protocol version specified by an ingress annotation
// or default if non present.
func MinTLSVersion(version string) envoy_api_v2_auth.TlsParameters_TlsProtocol {
	switch version {
	case "1.3":
		return envoy_api_v2_auth.TlsParameters_TLSv1_3
	case "1.2":
		return envoy_api_v2_auth.TlsParameters_TLSv1_2
	default:
		// any other value is interpreted as TLS/1.1
		return envoy_api_v2_auth.TlsParameters_TLSv1_1
	}
}

// MaxConnections returns the value of the first matching max-connections
// annotation for the following annotations:
// 1. projectcontour.io/max-connections
// 2. contour.heptio.com/max-connections
//
// '0' is returned if the annotation is absent or unparseable.
func MaxConnections(o metav1.ObjectMetaAccessor) uint32 {
	return parseUInt32(CompatAnnotation(o, "max-connections"))
}

// MaxPendingRequests returns the value of the first matching max-pending-requests
// annotation for the following annotations:
// 1. projectcontour.io/max-pending-requests
// 2. contour.heptio.com/max-pending-requests
//
// '0' is returned if the annotation is absent or unparseable.
func MaxPendingRequests(o metav1.ObjectMetaAccessor) uint32 {
	return parseUInt32(CompatAnnotation(o, "max-pending-requests"))
}

// MaxRequests returns the value of the first matching max-requests
// annotation for the following annotations:
// 1. projectcontour.io/max-requests
// 2. contour.heptio.com/max-requests
//
// '0' is returned if the annotation is absent or unparseable.
func MaxRequests(o metav1.ObjectMetaAccessor) uint32 {
	return parseUInt32(CompatAnnotation(o, "max-requests"))
}

// MaxRetries returns the value of the first matching max-retries
// annotation for the following annotations:
// 1. projectcontour.io/max-retries
// 2. contour.heptio.com/max-retries
//
// '0' is returned if the annotation is absent or unparseable.
func MaxRetries(o metav1.ObjectMetaAccessor) uint32 {
	return parseUInt32(CompatAnnotation(o, "max-retries"))
}

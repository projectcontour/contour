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
	"testing"

	contour_api_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	networking_v1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestParseUint32(t *testing.T) {
	tests := map[string]struct {
		s    string
		want uint32
	}{
		"blank": {
			s:    "",
			want: 0,
		},
		"negative": {
			s:    "-6",
			want: 0,
		},
		"explicit": {
			s:    "0",
			want: 0,
		},
		"positive": {
			s:    "2",
			want: 2,
		},
		"too large": {
			s:    "144115188075855872", // larger than uint32
			want: 0,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := parseUInt32(tc.s)
			if got != tc.want {
				t.Fatalf("expected: %v, got %v", tc.want, got)
			}
		})
	}
}

func TestParseint32(t *testing.T) {
	tests := map[string]struct {
		s    string
		want int32
	}{
		"blank": {
			s:    "",
			want: 0,
		},
		"negative": {
			s:    "-1",
			want: -1,
		},
		"explicit": {
			s:    "0",
			want: 0,
		},
		"positive": {
			s:    "2",
			want: 2,
		},
		"too large": {
			s:    "144115188075855872", // larger than int32
			want: 0,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := parseInt32(tc.s)
			if got != tc.want {
				t.Fatalf("expected: %v, got %v", tc.want, got)
			}
		})
	}
}

func TestNumRetries(t *testing.T) {
	tests := map[string]struct {
		ingress *networking_v1.Ingress
		want    uint32
	}{
		"blank": {
			ingress: &networking_v1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name: "ing",
					Annotations: map[string]string{
						"projectcontour.io/num-retries": "",
					},
				},
			},
			want: 1,
		},
		"Set to 1": {
			ingress: &networking_v1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name: "ing",
					Annotations: map[string]string{
						"projectcontour.io/num-retries": "1",
					},
				},
			},
			want: 1,
		},
		"Set to 0": {
			ingress: &networking_v1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name: "ing",
					Annotations: map[string]string{
						"projectcontour.io/num-retries": "0",
					},
				},
			},
			want: 1,
		},
		"Set to -1": {
			ingress: &networking_v1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name: "ing",
					Annotations: map[string]string{
						"projectcontour.io/num-retries": "-1",
					},
				},
			},
			want: 0,
		},
		"Set to 9": {
			ingress: &networking_v1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name: "ing",
					Annotations: map[string]string{
						"projectcontour.io/num-retries": "9",
					},
				},
			},
			want: 9,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := NumRetries(tc.ingress)
			if got != tc.want {
				t.Fatalf("expected: %v, got %v", tc.want, got)
			}
		})
	}
}

func TestParseUpstreamProtocols(t *testing.T) {
	tests := map[string]struct {
		a    map[string]string
		want map[string]string
	}{
		"nada": {
			a:    nil,
			want: map[string]string{},
		},
		"empty": {
			a:    map[string]string{"projectcontour.io/upstream-protocol.h2": ""},
			want: map[string]string{},
		},
		"empty with spaces": {
			a:    map[string]string{"projectcontour.io/upstream-protocol.h2": ", ,"},
			want: map[string]string{},
		},
		"single value": {
			a: map[string]string{"projectcontour.io/upstream-protocol.h2": "80"},
			want: map[string]string{
				"80": "h2",
			},
		},
		"tls": {
			a: map[string]string{"projectcontour.io/upstream-protocol.tls": "https,80"},
			want: map[string]string{
				"80":    "tls",
				"https": "tls",
			},
		},
		"multiple value": {
			a: map[string]string{"projectcontour.io/upstream-protocol.h2": "80,http,443,https"},
			want: map[string]string{
				"80":    "h2",
				"http":  "h2",
				"443":   "h2",
				"https": "h2",
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := ParseUpstreamProtocols(tc.a)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestWebsocketRoutes(t *testing.T) {
	tests := map[string]struct {
		a    *networking_v1.Ingress
		want map[string]bool
	}{
		"empty": {
			a: &networking_v1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"projectcontour.io/websocket-routes": "",
					},
				},
			},
			want: map[string]bool{},
		},
		"empty with spaces": {
			a: &networking_v1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"projectcontour.io/websocket-routes": ", ,",
					},
				},
			},
			want: map[string]bool{},
		},
		"single value": {
			a: &networking_v1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"projectcontour.io/websocket-routes": "/ws1",
					},
				},
			},
			want: map[string]bool{
				"/ws1": true,
			},
		},
		"multiple values": {
			a: &networking_v1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"projectcontour.io/websocket-routes": "/ws1,/ws2",
					},
				},
			},
			want: map[string]bool{
				"/ws1": true,
				"/ws2": true,
			},
		},
		"multiple values with spaces and invalid entries": {
			a: &networking_v1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"projectcontour.io/websocket-routes": " /ws1, , /ws2 ",
					},
				},
			},
			want: map[string]bool{
				"/ws1": true,
				"/ws2": true,
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := WebsocketRoutes(tc.a)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestTLSDelegationNamespace(t *testing.T) {
	tests := map[string]struct {
		a    *networking_v1.Ingress
		want string
	}{
		"absent": {
			a: &networking_v1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
			},
			want: "",
		},
		"empty": {
			a: &networking_v1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"projectcontour.io/tls-delegation-namespace": "",
					},
				},
			},
			want: "",
		},
		"valid value": {
			a: &networking_v1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"projectcontour.io/tls-delegation-namespace": "namespace-with-cert",
					},
				},
			},
			want: "namespace-with-cert",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := TLSDelegationNamespace(tc.a)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestHttpAllowed(t *testing.T) {
	tests := map[string]struct {
		i     *networking_v1.Ingress
		valid bool
	}{
		"basic ingress": {
			i: &networking_v1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "simple",
					Namespace: "default",
				},
				Spec: networking_v1.IngressSpec{
					TLS: []networking_v1.IngressTLS{{
						Hosts:      []string{"whatever.example.com"},
						SecretName: "secret",
					}},
					DefaultBackend: backend("backend", intstr.FromInt(80)),
				},
			},
			valid: true,
		},
		"kubernetes.io/ingress.allow-http: \"false\"": {
			i: &networking_v1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "simple",
					Namespace: "default",
					Annotations: map[string]string{
						"kubernetes.io/ingress.allow-http": "false",
					},
				},
				Spec: networking_v1.IngressSpec{
					TLS: []networking_v1.IngressTLS{{
						Hosts:      []string{"whatever.example.com"},
						SecretName: "secret",
					}},
					DefaultBackend: backend("backend", intstr.FromInt(80)),
				},
			},
			valid: false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := HTTPAllowed(tc.i)
			want := tc.valid
			if got != want {
				t.Fatalf("got: %v, want: %v", got, want)
			}
		})
	}
}

func TestAnnotationCompat(t *testing.T) {
	tests := map[string]struct {
		svc   *v1.Service
		value string
	}{
		"no annotations": {
			value: "",
			svc: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
			},
		},
		"projectcontour.io/annotation": {
			value: "200",
			svc: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"projectcontour.io/annotation": "200",
					},
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := ContourAnnotation(tc.svc, "annotation")
			want := tc.value
			if got != want {
				t.Fatalf("got: %v, want: %v", got, want)
			}
		})
	}
}

func TestAnnotationKindValidation(t *testing.T) {
	type status struct {
		known bool
		valid bool
	}
	tests := map[string]struct {
		obj         metav1.Object
		annotations map[string]status
	}{
		"service": {
			obj: &v1.Service{},
			annotations: map[string]status{
				"foo.invalid.com/annotation": {
					known: false, valid: false,
				},
				"projectcontour.io/annotation": {
					known: true, valid: false,
				},
			},
		},
		"httpproxy": {
			obj: &contour_api_v1.HTTPProxy{},
			annotations: map[string]status{
				// Valid only on Service.
				"projectcontour.io/max-requests": {
					known: true, valid: false,
				},
				// Valid only on Ingress.
				"projectcontour.io/websocket-routes": {
					known: true, valid: false,
				},
			},
		},
		"secrets": {
			obj: &v1.Secret{},
			annotations: map[string]status{
				// In our namespace but not valid on this kind.
				"projectcontour.io/ingress.class": {
					known: true, valid: false,
				},
				// Unknown, so potentially valid.
				"foo.io/secret-sauce": {
					known: false, valid: true,
				},
			},
		},
	}

	// Trivially check that everything specified in the global
	// table is valid.
	for _, kind := range []string{
		kindOf(&v1.Service{}),
		kindOf(&networking_v1.Ingress{}),
		kindOf(&contour_api_v1.HTTPProxy{}),
	} {
		for key := range annotationsByKind[kind] {
			t.Run(fmt.Sprintf("%s is known and valid for %s", key, kind),
				func(t *testing.T) {
					assert.Equal(t, true, IsKnown(key))
					assert.Equal(t, true, ValidForKind(kind, key))
				})
		}
	}

	// Check corner case combinations for different types.
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			for k, s := range tc.annotations {
				assert.Equal(t, s.known, IsKnown(k))
				assert.Equal(t, s.valid, ValidForKind(kindOf(tc.obj), k))
			}
		})
	}
}

func backend(name string, port intstr.IntOrString) *networking_v1.IngressBackend {
	var portObj networking_v1.ServiceBackendPort
	if port.Type == intstr.Int {
		portObj = networking_v1.ServiceBackendPort{
			Number: port.IntVal,
		}
	} else {
		portObj = networking_v1.ServiceBackendPort{
			Name: port.StrVal,
		}
	}

	return &networking_v1.IngressBackend{
		Service: &networking_v1.IngressServiceBackend{
			Name: name,
			Port: portObj,
		},
	}
}

// kindOf returns the kind string for the given Kubernetes object.
//
// The API machinery doesn't populate the metav1.TypeMeta field for
// objects, so we have to use a type assertion to detect kinds that
// we care about.
// TODO(youngnick): This is a straight copy from internal/k8s/kind.go
// Needs to be moved to a separate module somewhere.
func kindOf(obj interface{}) string {
	switch obj.(type) {
	case *v1.Secret:
		return "Secret"
	case *v1.Service:
		return "Service"
	case *networking_v1.Ingress:
		return "Ingress"
	case *contour_api_v1.HTTPProxy:
		return "HTTPProxy"
	case *contour_api_v1.TLSCertificateDelegation:
		return "TLSCertificateDelegation"
	default:
		return ""
	}
}

// Copyright © 2018 Heptio
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

package dag

import (
	"fmt"
	"math"
	"reflect"
	"testing"

	"k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestParseAnnotationUInt32(t *testing.T) {
	tests := map[string]struct {
		a     map[string]string
		want  uint32
		isNil bool
	}{
		"nada": {
			a:     nil,
			isNil: true,
		},
		"empty": {
			a:     map[string]string{annotationRequestTimeout: ""}, // not even sure this is possible via the API
			isNil: true,
		},
		"smallest": {
			a:    map[string]string{annotationRequestTimeout: "0"},
			want: 0,
		},
		"middle value": {
			a:    map[string]string{annotationRequestTimeout: "20"},
			want: 20,
		},
		"biggest": {
			a:    map[string]string{annotationRequestTimeout: "4294967295"},
			want: math.MaxUint32,
		},
		"invalid": {
			a:     map[string]string{annotationRequestTimeout: "10seconds"}, // not a duration
			isNil: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := parseAnnotationUInt32(tc.a, annotationRequestTimeout)
			if ((got == nil) != tc.isNil) || (got != nil && got.Value != tc.want) {
				t.Fatalf("parseAnnotationUInt32(%q): want: %v, isNil: %v, got: %v", tc.a, tc.want, tc.isNil, got)
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
			a:    map[string]string{fmt.Sprintf("%s.%s", annotationUpstreamProtocol, "h2"): ""},
			want: map[string]string{},
		},
		"empty with spaces": {
			a:    map[string]string{fmt.Sprintf("%s.%s", annotationUpstreamProtocol, "h2"): ", ,"},
			want: map[string]string{},
		},
		"single value": {
			a: map[string]string{fmt.Sprintf("%s.%s", annotationUpstreamProtocol, "h2"): "80"},
			want: map[string]string{
				"80": "h2",
			},
		},
		"tls": {
			a: map[string]string{fmt.Sprintf("%s.%s", annotationUpstreamProtocol, "tls"): "https,80"},
			want: map[string]string{
				"80":    "tls",
				"https": "tls",
			},
		},
		"multiple value": {
			a: map[string]string{fmt.Sprintf("%s.%s", annotationUpstreamProtocol, "h2"): "80,http,443,https"},
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
			got := parseUpstreamProtocols(tc.a, annotationUpstreamProtocol, "h2", "tls")
			if !reflect.DeepEqual(tc.want, got) {
				t.Fatalf("parseUpstreamProtocols(%q): want: %v, got: %v", tc.a, tc.want, got)
			}
		})
	}
}

func TestWebsocketRoutes(t *testing.T) {
	tests := map[string]struct {
		a    *v1beta1.Ingress
		want map[string]bool
	}{
		"empty": {
			a: &v1beta1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{annotationWebsocketRoutes: ""},
				},
			},
			want: map[string]bool{},
		},
		"empty with spaces": {
			a: &v1beta1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{annotationWebsocketRoutes: ", ,"},
				},
			},
			want: map[string]bool{},
		},
		"single value": {
			a: &v1beta1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{annotationWebsocketRoutes: "/ws1"},
				},
			},
			want: map[string]bool{
				"/ws1": true,
			},
		},
		"multiple values": {
			a: &v1beta1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{annotationWebsocketRoutes: "/ws1,/ws2"},
				},
			},
			want: map[string]bool{
				"/ws1": true,
				"/ws2": true,
			},
		},
		"multiple values with spaces and invalid entries": {
			a: &v1beta1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{annotationWebsocketRoutes: " /ws1, , /ws2 "},
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
			got := websocketRoutes(tc.a)
			if !reflect.DeepEqual(tc.want, got) {
				t.Fatalf("websocketRoutes(%q): want: %v, got: %v", tc.a, tc.want, got)
			}
		})
	}
}

func TestHttpAllowed(t *testing.T) {
	tests := map[string]struct {
		i     *v1beta1.Ingress
		valid bool
	}{
		"basic ingress": {
			i: &v1beta1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "simple",
					Namespace: "default",
				},
				Spec: v1beta1.IngressSpec{
					TLS: []v1beta1.IngressTLS{{
						Hosts:      []string{"whatever.example.com"},
						SecretName: "secret",
					}},
					Backend: backend("backend", intstr.FromInt(80)),
				},
			},
			valid: true,
		},
		"kubernetes.io/ingress.allow-http: \"false\"": {
			i: &v1beta1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "simple",
					Namespace: "default",
					Annotations: map[string]string{
						"kubernetes.io/ingress.allow-http": "false",
					},
				},
				Spec: v1beta1.IngressSpec{
					TLS: []v1beta1.IngressTLS{{
						Hosts:      []string{"whatever.example.com"},
						SecretName: "secret",
					}},
					Backend: backend("backend", intstr.FromInt(80)),
				},
			},
			valid: false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := httpAllowed(tc.i)
			want := tc.valid
			if got != want {
				t.Fatalf("got: %v, want: %v", got, want)
			}
		})
	}
}

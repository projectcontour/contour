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

package v3

import (
	"testing"

	envoy_service_runtime_v3 "github.com/envoyproxy/go-control-plane/envoy/service/runtime/v3"
	contour_api_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/protobuf"
	"github.com/projectcontour/contour/internal/ref"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestRuntimeCacheContents(t *testing.T) {
	testCases := map[string]struct {
		runtimeSettings  ConfigurableRuntimeSettings
		additionalFields map[string]*structpb.Value
	}{
		"no values set": {
			runtimeSettings: ConfigurableRuntimeSettings{},
		},
		"http max requests per io cycle set": {
			runtimeSettings: ConfigurableRuntimeSettings{
				MaxRequestsPerIOCycle: ref.To(uint32(1)),
			},
			additionalFields: map[string]*structpb.Value{
				"http.max_requests_per_io_cycle": structpb.NewNumberValue(1),
			},
		},
		"http max requests per io cycle set invalid": {
			runtimeSettings: ConfigurableRuntimeSettings{
				MaxRequestsPerIOCycle: ref.To(uint32(0)),
			},
		},
		"http max requests per io cycle set nil": {
			runtimeSettings: ConfigurableRuntimeSettings{
				MaxRequestsPerIOCycle: nil,
			},
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			rc := NewRuntimeCache(tc.runtimeSettings)
			fields := map[string]*structpb.Value{
				"re2.max_program_size.error_level": structpb.NewNumberValue(1 << 20),
				"re2.max_program_size.warn_level":  structpb.NewNumberValue(1000),
			}
			for k, v := range tc.additionalFields {
				fields[k] = v
			}
			protobuf.ExpectEqual(t, []proto.Message{
				&envoy_service_runtime_v3.Runtime{
					Name: "dynamic",
					Layer: &structpb.Struct{
						Fields: fields,
					},
				},
			}, rc.Contents())
		})
	}
}

func TestRuntimeCacheQuery(t *testing.T) {
	baseRuntimeLayers := []proto.Message{
		&envoy_service_runtime_v3.Runtime{
			Name: "dynamic",
			Layer: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"re2.max_program_size.error_level": structpb.NewNumberValue(1 << 20),
					"re2.max_program_size.warn_level":  structpb.NewNumberValue(1000),
				},
			},
		},
	}
	testCases := map[string]struct {
		names    []string
		expected []proto.Message
	}{
		"empty names": {
			names:    []string{},
			expected: []proto.Message{},
		},
		"names include dynamic": {
			names:    []string{"foo", "dynamic", "bar"},
			expected: baseRuntimeLayers,
		},
		"names excludes dynamic": {
			names:    []string{"foo", "bar", "baz"},
			expected: []proto.Message{},
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			rc := NewRuntimeCache(ConfigurableRuntimeSettings{})
			protobuf.ExpectEqual(t, tc.expected, rc.Query(tc.names))
		})
	}
}

func TestRuntimeVisit(t *testing.T) {
	service := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:     "http",
				Protocol: "TCP",
				Port:     8080,
			}},
		},
	}
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret",
			Namespace: "default",
		},
		Type: "kubernetes.io/tls",
		Data: secretdata(CERTIFICATE, RSA_PRIVATE_KEY),
	}
	tests := map[string]struct {
		ConfigurableRuntimeSettings
		fallbackCertificate *types.NamespacedName
		objs                []any
		expected            []proto.Message
	}{
		"nothing": {
			objs: nil,
			expected: []proto.Message{
				&envoy_service_runtime_v3.Runtime{
					Name: "dynamic",
					Layer: &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"re2.max_program_size.error_level": structpb.NewNumberValue(1 << 20),
							"re2.max_program_size.warn_level":  structpb.NewNumberValue(1000),
						},
					},
				},
			},
		},
		"configure max connection per listener for one listener": {
			ConfigurableRuntimeSettings: ConfigurableRuntimeSettings{
				MaxConnectionsPerListener: ref.To(uint32(100)),
			},
			objs: []any{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www.example.com",
						},
						Routes: []contour_api_v1.Route{{
							Conditions: []contour_api_v1.MatchCondition{{
								Prefix: "/",
							}},
							Services: []contour_api_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				service,
			},
			expected: []proto.Message{
				&envoy_service_runtime_v3.Runtime{
					Name: "dynamic",
					Layer: &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"envoy.resource_limits.listener.ingress_http.connection_limit": structpb.NewNumberValue(100),
							"re2.max_program_size.error_level":                             structpb.NewNumberValue(1 << 20),
							"re2.max_program_size.warn_level":                              structpb.NewNumberValue(1000),
						},
					},
				},
			},
		},
		"configure max connection per listener for two listeners": {
			ConfigurableRuntimeSettings: ConfigurableRuntimeSettings{
				MaxConnectionsPerListener: ref.To(uint32(100)),
			},
			objs: []any{
				&contour_api_v1.HTTPProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple",
						Namespace: "default",
					},
					Spec: contour_api_v1.HTTPProxySpec{
						VirtualHost: &contour_api_v1.VirtualHost{
							Fqdn: "www.example.com",
							TLS: &contour_api_v1.TLS{
								SecretName: "secret",
							},
						},
						Routes: []contour_api_v1.Route{{
							Conditions: []contour_api_v1.MatchCondition{{
								Prefix: "/",
							}},
							Services: []contour_api_v1.Service{{
								Name: "backend",
								Port: 80,
							}},
						}},
					},
				},
				service,
				secret,
			},
			expected: []proto.Message{
				&envoy_service_runtime_v3.Runtime{
					Name: "dynamic",
					Layer: &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"envoy.resource_limits.listener.ingress_http.connection_limit":  structpb.NewNumberValue(100),
							"envoy.resource_limits.listener.ingress_https.connection_limit": structpb.NewNumberValue(100),
							"re2.max_program_size.error_level":                              structpb.NewNumberValue(1 << 20),
							"re2.max_program_size.warn_level":                               structpb.NewNumberValue(1000),
						},
					},
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			rc := NewRuntimeCache(tc.ConfigurableRuntimeSettings)
			rc.OnChange(buildDAGFallback(t, tc.fallbackCertificate, tc.objs...))
			protobuf.ExpectEqual(t, tc.expected, rc.Query([]string{"dynamic"}))
		})
	}
}

func TestRuntimeCacheOnChangeDelete(t *testing.T) {
	configurableRuntimeSettings := ConfigurableRuntimeSettings{
		MaxConnectionsPerListener: ref.To(uint32(100)),
	}
	objs := []any{
		&contour_api_v1.HTTPProxy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "simple",
				Namespace: "default",
			},
			Spec: contour_api_v1.HTTPProxySpec{
				VirtualHost: &contour_api_v1.VirtualHost{
					Fqdn: "www.example.com",
				},
				Routes: []contour_api_v1.Route{{
					Conditions: []contour_api_v1.MatchCondition{{
						Prefix: "/",
					}},
					Services: []contour_api_v1.Service{{
						Name: "backend",
						Port: 80,
					}},
				}},
			},
		},
		&v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "kuard",
				Namespace: "default",
			},
			Spec: v1.ServiceSpec{
				Ports: []v1.ServicePort{{
					Name:     "http",
					Protocol: "TCP",
					Port:     8080,
				}},
			},
		},
	}

	rc := NewRuntimeCache(configurableRuntimeSettings)
	rc.OnChange(buildDAGFallback(t, nil, objs...))
	protobuf.ExpectEqual(t, []proto.Message{
		&envoy_service_runtime_v3.Runtime{
			Name: "dynamic",
			Layer: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"envoy.resource_limits.listener.ingress_http.connection_limit": structpb.NewNumberValue(100),
					"re2.max_program_size.error_level":                             structpb.NewNumberValue(1 << 20),
					"re2.max_program_size.warn_level":                              structpb.NewNumberValue(1000),
				},
			},
		},
	}, rc.Query([]string{"dynamic"}))

	rc.OnChange(buildDAGFallback(t, nil, nil))
	protobuf.ExpectEqual(t, []proto.Message{
		&envoy_service_runtime_v3.Runtime{
			Name: "dynamic",
			Layer: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"re2.max_program_size.error_level": structpb.NewNumberValue(1 << 20),
					"re2.max_program_size.warn_level":  structpb.NewNumberValue(1000),
				},
			},
		},
	}, rc.Query([]string{"dynamic"}))
}

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

package dag

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	"github.com/projectcontour/contour/internal/fixture"
)

func makeServicePort(name string, protocol core_v1.Protocol, port int32, extras ...any) core_v1.ServicePort {
	p := core_v1.ServicePort{
		Name:     name,
		Protocol: protocol,
		Port:     port,
	}

	if len(extras) > 0 {
		p.TargetPort = intstr.FromInt(extras[0].(int))
	}

	if len(extras) > 1 {
		p.AppProtocol = ptr.To(extras[1].(string))
	}

	return p
}

func TestBuilderLookupService(t *testing.T) {
	s1 := &core_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: core_v1.ServiceSpec{
			Ports: []core_v1.ServicePort{makeServicePort("http", "TCP", 8080, 8080)},
		},
	}

	s2 := &core_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "includehealth",
			Namespace: "default",
		},
		Spec: core_v1.ServiceSpec{
			Ports: []core_v1.ServicePort{makeServicePort("http", "TCP", 8080, 8080), makeServicePort("health", "TCP", 8998, 8998)},
		},
	}

	externalNameValid := &core_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "externalnamevalid",
			Namespace: "default",
		},
		Spec: core_v1.ServiceSpec{
			Type:         core_v1.ServiceTypeExternalName,
			ExternalName: "external.projectcontour.io",
			Ports:        []core_v1.ServicePort{makeServicePort("http", "TCP", 80, 80)},
		},
	}

	externalNameLocalhost := &core_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "externalnamelocalhost",
			Namespace: "default",
		},
		Spec: core_v1.ServiceSpec{
			Type:         core_v1.ServiceTypeExternalName,
			ExternalName: "localhost",
			Ports:        []core_v1.ServicePort{makeServicePort("http", "TCP", 80, 80)},
		},
	}

	annotatedService := &core_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:        "annotated-service",
			Namespace:   "default",
			Annotations: map[string]string{"projectcontour.io/upstream-protocol.tls": "8443"},
		},
		Spec: core_v1.ServiceSpec{
			Ports: []core_v1.ServicePort{{
				Name:       "foo",
				Protocol:   "TCP",
				Port:       8443,
				TargetPort: intstr.FromInt(26441),
			}},
		},
	}

	appProtoService := &core_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:        "app-protocol-service",
			Namespace:   "default",
			Annotations: map[string]string{"projectcontour.io/upstream-protocol.tls": "8443,8444"},
		},
		Spec: core_v1.ServiceSpec{
			Ports: []core_v1.ServicePort{
				{
					Name:        "k8s-h2c",
					Protocol:    "TCP",
					AppProtocol: ptr.To("kubernetes.io/h2c"),
					Port:        8443,
				},
				{
					Name:        "k8s-wss",
					Protocol:    "TCP",
					AppProtocol: ptr.To("kubernetes.io/wss"),
					Port:        8444,
				},
				{
					Name:        "iana-https",
					Protocol:    "TCP",
					AppProtocol: ptr.To("https"),
					Port:        8445,
				},
				{
					Name:        "iana-http",
					Protocol:    "TCP",
					AppProtocol: ptr.To("http"),
					Port:        8446,
				},
			},
		},
	}

	services := map[types.NamespacedName]*core_v1.Service{
		{Name: "service1", Namespace: "default"}:                             s1,
		{Name: "servicehealthcheck", Namespace: "default"}:                   s2,
		{Name: "externalnamevalid", Namespace: "default"}:                    externalNameValid,
		{Name: "externalnamelocalhost", Namespace: "default"}:                externalNameLocalhost,
		{Name: annotatedService.Name, Namespace: annotatedService.Namespace}: annotatedService,
		{Name: appProtoService.Name, Namespace: appProtoService.Namespace}:   appProtoService,
	}

	tests := map[string]struct {
		types.NamespacedName
		port                  int
		healthPort            int
		enableExternalNameSvc bool
		want                  *Service
		wantErr               error
	}{
		"lookup service by port number": {
			NamespacedName: types.NamespacedName{Name: "service1", Namespace: "default"},
			port:           8080,
			want:           service(s1),
		},

		"when service does not exist an error is returned": {
			NamespacedName: types.NamespacedName{Name: "nonexistent-service", Namespace: "default"},
			port:           8080,
			wantErr:        errors.New(`service "default/nonexistent-service" not found`),
		},
		"when service port does not exist an error is returned": {
			NamespacedName: types.NamespacedName{Name: "service1", Namespace: "default"},
			port:           9999,
			wantErr:        errors.New(`port "9999" on service "default/service1" not matched`),
		},
		"when health port and service port are different": {
			NamespacedName: types.NamespacedName{Name: "servicehealthcheck", Namespace: "default"},
			port:           8080,
			healthPort:     8998,
			want:           healthService(s2),
		},
		"when health port does not exist an error is returned": {
			NamespacedName: types.NamespacedName{Name: "servicehealthcheck", Namespace: "default"},
			port:           8080,
			healthPort:     8999,
			wantErr:        errors.New(`port "8999" on service "default/servicehealthcheck" not matched`),
		},
		"When ExternalName Services are not disabled no error is returned": {
			NamespacedName: types.NamespacedName{Name: "externalnamevalid", Namespace: "default"},
			port:           80,
			want: &Service{
				Weighted: WeightedService{
					Weight:           1,
					ServiceName:      "externalnamevalid",
					ServiceNamespace: "default",
					ServicePort:      makeServicePort("http", "TCP", 80, 80),
					HealthPort:       makeServicePort("http", "TCP", 80, 80),
				},
				ExternalName: "external.projectcontour.io",
			},
			enableExternalNameSvc: true,
		},
		"When ExternalName Services are disabled an error is returned": {
			NamespacedName: types.NamespacedName{Name: "externalnamevalid", Namespace: "default"},
			port:           80,
			wantErr:        errors.New(`default/externalnamevalid is an ExternalName service, these are not currently enabled. See the config.enableExternalNameService config file setting`),
		},
		"When ExternalName Services are enabled but a localhost ExternalName is used an error is returned": {
			NamespacedName:        types.NamespacedName{Name: "externalnamelocalhost", Namespace: "default"},
			port:                  80,
			wantErr:               errors.New(`default/externalnamelocalhost is an ExternalName service that points to localhost, this is not allowed`),
			enableExternalNameSvc: true,
		},
		"lookup service by port number with annotated number": {
			NamespacedName: types.NamespacedName{Name: annotatedService.Name, Namespace: annotatedService.Namespace},
			port:           8443,
			want:           appProtcolService(annotatedService, "tls"),
		},
		"lookup service by port number with k8s app protocol: h2c": {
			NamespacedName: types.NamespacedName{Name: appProtoService.Name, Namespace: appProtoService.Namespace},
			port:           8443,
			want:           appProtcolService(appProtoService, "h2c"),
		},

		"lookup service by port number with unsupported k8s app protocol: wss": {
			NamespacedName: types.NamespacedName{Name: appProtoService.Name, Namespace: appProtoService.Namespace},
			port:           8444,
			want:           appProtcolService(appProtoService, "", 1),
		},
		"lookup service by port number with supported IANA app protocol: https": {
			NamespacedName: types.NamespacedName{Name: appProtoService.Name, Namespace: appProtoService.Namespace},
			port:           8445,
			want:           appProtcolService(appProtoService, "tls", 2),
		},
		"lookup service by port number with supported IANA app protocol: http": {
			NamespacedName: types.NamespacedName{Name: appProtoService.Name, Namespace: appProtoService.Namespace},
			port:           8446,
			want:           appProtcolService(appProtoService, "", 3),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			b := Builder{
				Source: KubernetesCache{
					services:    services,
					FieldLogger: fixture.NewTestLogger(t),
				},
			}

			var dag DAG

			got, gotErr := dag.EnsureService(tc.NamespacedName, tc.port, tc.healthPort, &b.Source, tc.enableExternalNameSvc)
			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.wantErr, gotErr)
		})
	}
}

func TestGetSingleListener(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		d := &DAG{
			Listeners: map[string]*Listener{
				"http": {
					Protocol: "http",
					Port:     80,
				},
				"https": {
					Protocol: "https",
					Port:     443,
				},
			},
		}

		got, gotErr := d.GetSingleListener("http")
		assert.Equal(t, d.Listeners["http"], got)
		require.NoError(t, gotErr)

		got, gotErr = d.GetSingleListener("https")
		assert.Equal(t, d.Listeners["https"], got)
		require.NoError(t, gotErr)
	})

	t.Run("one HTTP listener, no HTTPS listener", func(t *testing.T) {
		d := &DAG{
			Listeners: map[string]*Listener{
				"http": {
					Protocol: "http",
					Port:     80,
				},
			},
		}

		got, gotErr := d.GetSingleListener("http")
		assert.Equal(t, d.Listeners["http"], got)
		require.NoError(t, gotErr)

		got, gotErr = d.GetSingleListener("https")
		assert.Nil(t, got)
		require.EqualError(t, gotErr, "no HTTPS listener configured")
	})

	t.Run("many HTTP listeners, one HTTPS listener", func(t *testing.T) {
		d := &DAG{
			Listeners: map[string]*Listener{
				"http-1": {
					Protocol: "http",
					Port:     80,
				},
				"http-2": {
					Protocol: "http",
					Port:     81,
				},
				"http-3": {
					Protocol: "http",
					Port:     82,
				},
				"https-1": {
					Protocol: "https",
					Port:     443,
				},
			},
		}

		got, gotErr := d.GetSingleListener("http")
		assert.Nil(t, got)
		require.EqualError(t, gotErr, "more than one HTTP listener configured")

		got, gotErr = d.GetSingleListener("https")
		assert.Equal(t, d.Listeners["https-1"], got)
		require.NoError(t, gotErr)
	})
}

func TestGetServiceClusters(t *testing.T) {
	d := &DAG{
		Listeners: map[string]*Listener{
			"http-1": {
				VirtualHosts: []*VirtualHost{
					{
						Routes: map[string]*Route{
							"foo": {
								Clusters: []*Cluster{
									{Upstream: &Service{ExternalName: "bar.com"}},
									{Upstream: &Service{}},
								},
							},
						},
					},
				},
			},
		},
	}
	// We should only get one cluster since the other is for an ExternalName
	// service.
	assert.Len(t, d.GetServiceClusters(), 1)

	dagWithMirror := &DAG{
		Listeners: map[string]*Listener{
			"http-1": {
				VirtualHosts: []*VirtualHost{
					{
						Routes: map[string]*Route{
							"foo": {
								Clusters: []*Cluster{
									{Upstream: &Service{}},
								},
								MirrorPolicies: []*MirrorPolicy{
									{
										Cluster: &Cluster{
											Upstream: &Service{},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	// We should get two clusters since we have mirrorPolicies.
	assert.Len(t, dagWithMirror.GetServiceClusters(), 2)
}

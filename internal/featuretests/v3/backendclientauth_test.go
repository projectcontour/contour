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

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"github.com/sirupsen/logrus"
	core_v1 "k8s.io/api/core/v1"
	networking_v1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	contour_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/internal/dag"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/featuretests"
	"github.com/projectcontour/contour/internal/fixture"
)

func proxyClientCertificateOpt(t *testing.T) func(*dag.Builder) {
	return func(b *dag.Builder) {
		secret := types.NamespacedName{
			Name:      "envoyclientsecret",
			Namespace: "default",
		}

		log := fixture.NewTestLogger(t)
		log.SetLevel(logrus.DebugLevel)

		b.Processors = []dag.Processor{
			&dag.ListenerProcessor{},
			&dag.IngressProcessor{
				ClientCertificate: &secret,
				FieldLogger:       log.WithField("context", "IngressProcessor"),
			},
			&dag.HTTPProxyProcessor{
				ClientCertificate: &secret,
			},
			&dag.ExtensionServiceProcessor{
				ClientCertificate: &secret,
				FieldLogger:       log.WithField("context", "ExtensionServiceProcessor"),
			},
			&dag.GatewayAPIProcessor{
				FieldLogger: log.WithField("context", "GatewayAPIProcessor"),
			},
		}

		b.Source.ConfiguredSecretRefs = []*types.NamespacedName{
			{Namespace: secret.Namespace, Name: secret.Name},
		}
	}
}

func TestBackendClientAuthenticationWithHTTPProxy(t *testing.T) {
	rh, c, done := setup(t, proxyClientCertificateOpt(t))
	defer done()

	clientSecret := featuretests.TLSSecret(t, "envoyclientsecret", &featuretests.ClientCertificate)
	serverSecret := featuretests.TLSSecret(t, "envoyserversecret", &featuretests.ServerCertificate)
	caSecret := featuretests.CASecret(t, "backendcacert", &featuretests.CACertificate)
	rh.OnAdd(clientSecret)
	rh.OnAdd(serverSecret)
	rh.OnAdd(caSecret)

	svc := fixture.NewService("backend").
		WithPorts(core_v1.ServicePort{Name: "http", Port: 443})
	rh.OnAdd(svc)

	proxy := fixture.NewProxy("authenticated").WithSpec(
		contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "www.example.com",
			},
			Routes: []contour_v1.Route{{
				Services: []contour_v1.Service{{
					Name:     svc.Name,
					Port:     443,
					Protocol: ptr.To("tls"),
					UpstreamValidation: &contour_v1.UpstreamValidation{
						CACertificate: caSecret.Name,
						SubjectName:   "subjname",
					},
				}},
			}},
		})
	rh.OnAdd(proxy)

	expectedResponse := &envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			tlsCluster(cluster("default/backend/443/950c17581f", "default/backend/http", "default_backend_443"), caSecret, "subjname", "", clientSecret, nil),
		),
		TypeUrl: clusterType,
	}

	c.Request(clusterType).Equals(expectedResponse)

	rh.OnDelete(proxy)

	tcpproxy := fixture.NewProxy("tcpproxy").WithSpec(
		contour_v1.HTTPProxySpec{
			VirtualHost: &contour_v1.VirtualHost{
				Fqdn: "www.example.com",
				TLS: &contour_v1.TLS{
					SecretName: serverSecret.Name,
				},
			},
			TCPProxy: &contour_v1.TCPProxy{
				Services: []contour_v1.Service{{
					Name:     svc.Name,
					Port:     443,
					Protocol: ptr.To("tls"),
					UpstreamValidation: &contour_v1.UpstreamValidation{
						CACertificate: caSecret.Name,
						SubjectName:   "subjname",
					},
				}},
			},
		})
	rh.OnAdd(tcpproxy)

	c.Request(clusterType).Equals(expectedResponse)

	// Test the error branch when Envoy client certificate secret does not exist.
	rh.OnDelete(clientSecret)
	c.Request(clusterType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: nil,
		TypeUrl:   clusterType,
	})
}

func TestBackendClientAuthenticationWithIngress(t *testing.T) {
	rh, c, done := setup(t, proxyClientCertificateOpt(t))
	defer done()

	clientSecret := featuretests.TLSSecret(t, "envoyclientsecret", &featuretests.ClientCertificate)
	caSecret := featuretests.CASecret(t, "backendcacert", &featuretests.CACertificate)
	rh.OnAdd(clientSecret)
	rh.OnAdd(caSecret)

	svc := fixture.NewService("backend").
		Annotate("projectcontour.io/upstream-protocol.tls", "443").
		WithPorts(core_v1.ServicePort{Name: "http", Port: 443})
	rh.OnAdd(svc)

	ingress := &networking_v1.Ingress{
		ObjectMeta: fixture.ObjectMeta("authenticated"),
		Spec: networking_v1.IngressSpec{
			DefaultBackend: featuretests.IngressBackend(svc),
		},
	}
	rh.OnAdd(ingress)

	c.Request(clusterType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: resources(t,
			tlsClusterWithoutValidation(cluster("default/backend/443/4929fca9d4", "default/backend/http", "default_backend_443"), "", clientSecret, nil),
		),
		TypeUrl: clusterType,
	})

	// Test the error branch when Envoy client certificate secret does not exist.
	rh.OnDelete(clientSecret)
	c.Request(clusterType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: nil,
		TypeUrl:   clusterType,
	})
}

func TestBackendClientAuthenticationWithExtensionService(t *testing.T) {
	envoyGen := envoy_v3.NewEnvoyGen(envoy_v3.EnvoyGenOpt{
		XDSClusterName: envoy_v3.DefaultXDSClusterName,
	})
	rh, c, done := setup(t, proxyClientCertificateOpt(t))
	defer done()

	clientSecret := featuretests.TLSSecret(t, "envoyclientsecret", &featuretests.ClientCertificate)
	caSecret := featuretests.CASecret(t, "backendcacert", &featuretests.CACertificate)
	rh.OnAdd(clientSecret)
	rh.OnAdd(caSecret)

	svc := fixture.NewService("backend").
		WithPorts(core_v1.ServicePort{Name: "grpc", Port: 6001})
	rh.OnAdd(svc)

	ext := &contour_v1alpha1.ExtensionService{
		ObjectMeta: fixture.ObjectMeta("ext"),
		Spec: contour_v1alpha1.ExtensionServiceSpec{
			Services: []contour_v1alpha1.ExtensionServiceTarget{
				{Name: svc.Name, Port: 6001},
			},
			UpstreamValidation: &contour_v1.UpstreamValidation{
				CACertificate: caSecret.Name,
				SubjectName:   "subjname",
			},
		},
	}

	rh.OnAdd(ext)

	tlsSocket := envoy_v3.UpstreamTLSTransportSocket(
		envoyGen.UpstreamTLSContext(
			&dag.PeerValidationContext{
				CACertificates: []*dag.Secret{{Object: featuretests.CASecret(t, "secret", &featuretests.CACertificate)}},
				SubjectNames:   []string{"subjname"},
			},
			"subjname",
			&dag.Secret{Object: clientSecret},
			nil,
			"h2",
		),
	)
	c.Request(clusterType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		TypeUrl: clusterType,
		Resources: resources(t,
			DefaultCluster(
				h2cCluster(cluster("extension/default/ext", "extension/default/ext", "extension_default_ext")),
				&envoy_config_cluster_v3.Cluster{TransportSocket: tlsSocket},
			),
		),
	})

	// Test the error branch when Envoy client certificate secret does not exist.
	rh.OnDelete(clientSecret)
	c.Request(clusterType).Equals(&envoy_service_discovery_v3.DiscoveryResponse{
		Resources: nil,
		TypeUrl:   clusterType,
	})
}

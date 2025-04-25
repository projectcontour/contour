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

package featuretests

// kubernetes helpers

import (
	"testing"

	"github.com/tsaarni/certyaml"
	core_v1 "k8s.io/api/core/v1"
	discovery_v1 "k8s.io/api/discovery/v1"
	networking_v1 "k8s.io/api/networking/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/fixture"
)

func IngressBackend(svc *core_v1.Service) *networking_v1.IngressBackend {
	return &networking_v1.IngressBackend{
		Service: &networking_v1.IngressServiceBackend{
			Name: svc.Name,
			Port: networking_v1.ServiceBackendPort{
				Number: svc.Spec.Ports[0].Port,
			},
		},
	}
}

var CACertificate = certyaml.Certificate{
	Subject: "CN=ca",
}

var ServerCertificate = certyaml.Certificate{
	Issuer:          &CACertificate,
	Subject:         "CN=www.example.com",
	SubjectAltNames: []string{"DNS:www.example.com"},
}

var ClientCertificate = certyaml.Certificate{
	Issuer:  &CACertificate,
	Subject: "CN=client",
}

var CRL = certyaml.CRL{
	Issuer: &CACertificate,
}

func TLSSecret(t *testing.T, name string, credential *certyaml.Certificate) *core_v1.Secret {
	cert, key, err := credential.PEM()
	if err != nil {
		t.Fatal(err)
	}
	return &core_v1.Secret{
		ObjectMeta: fixture.ObjectMeta(name),
		Type:       core_v1.SecretTypeTLS,
		Data: map[string][]byte{
			core_v1.TLSCertKey:       cert,
			core_v1.TLSPrivateKeyKey: key,
		},
	}
}

func CASecret(t *testing.T, name string, credential *certyaml.Certificate) *core_v1.Secret {
	cert, _, err := credential.PEM()
	if err != nil {
		t.Fatal(err)
	}
	return &core_v1.Secret{
		ObjectMeta: fixture.ObjectMeta(name),
		Type:       core_v1.SecretTypeOpaque,
		Data: map[string][]byte{
			dag.CACertificateKey: cert,
		},
	}
}

func CRLSecret(t *testing.T, name string, credential *certyaml.CRL) *core_v1.Secret {
	crl, err := credential.PEM()
	if err != nil {
		t.Fatal(err)
	}
	return &core_v1.Secret{
		ObjectMeta: fixture.ObjectMeta(name),
		Type:       core_v1.SecretTypeOpaque,
		Data: map[string][]byte{
			dag.CRLKey: crl,
		},
	}
}

func PEMBytes(t *testing.T, cert *certyaml.Certificate) []byte {
	c, _, err := cert.PEM()
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func EndpointSlice(ns, name, serviceName string, endpoints []discovery_v1.Endpoint, ports []discovery_v1.EndpointPort) *discovery_v1.EndpointSlice {
	return &discovery_v1.EndpointSlice{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			Labels: map[string]string{
				discovery_v1.LabelServiceName: serviceName,
			},
		},
		Endpoints: endpoints,
		Ports:     ports,
	}
}

func Endpoints(endpoints ...discovery_v1.Endpoint) []discovery_v1.Endpoint {
	return endpoints
}

func Endpoint(address string, ready bool) discovery_v1.Endpoint {
	return discovery_v1.Endpoint{
		Addresses: []string{address},
		Conditions: discovery_v1.EndpointConditions{
			Ready: ptr.To(ready),
		},
	}
}

func Ports(eps ...discovery_v1.EndpointPort) []discovery_v1.EndpointPort {
	return eps
}

func Port(name string, port int32) discovery_v1.EndpointPort {
	return discovery_v1.EndpointPort{
		Name:     ptr.To(name),
		Port:     ptr.To(port),
		Protocol: ptr.To(core_v1.ProtocolTCP),
	}
}

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

	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/fixture"
	"github.com/tsaarni/certyaml"
	v1 "k8s.io/api/core/v1"
	networking_v1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func IngressBackend(svc *v1.Service) *networking_v1.IngressBackend {
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

func TLSSecret(t *testing.T, name string, credential *certyaml.Certificate) *v1.Secret {
	cert, key, err := credential.PEM()
	if err != nil {
		t.Fatal(err)
	}
	return &v1.Secret{
		ObjectMeta: fixture.ObjectMeta(name),
		Type:       v1.SecretTypeTLS,
		Data: map[string][]byte{
			v1.TLSCertKey:       cert,
			v1.TLSPrivateKeyKey: key,
		},
	}
}

func CASecret(t *testing.T, name string, credential *certyaml.Certificate) *v1.Secret {
	cert, _, err := credential.PEM()
	if err != nil {
		t.Fatal(err)
	}
	return &v1.Secret{
		ObjectMeta: fixture.ObjectMeta(name),
		Type:       v1.SecretTypeOpaque,
		Data: map[string][]byte{
			dag.CACertificateKey: cert,
		},
	}
}

func CRLSecret(t *testing.T, name string, credential *certyaml.CRL) *v1.Secret {
	crl, err := credential.PEM()
	if err != nil {
		t.Fatal(err)
	}
	return &v1.Secret{
		ObjectMeta: fixture.ObjectMeta(name),
		Type:       v1.SecretTypeOpaque,
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

func Endpoints(ns, name string, subsets ...v1.EndpointSubset) *v1.Endpoints {
	return &v1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Subsets: subsets,
	}
}

func Ports(eps ...v1.EndpointPort) []v1.EndpointPort {
	return eps
}

func Port(name string, port int32) v1.EndpointPort {
	return v1.EndpointPort{
		Name:     name,
		Port:     port,
		Protocol: "TCP",
	}
}

func Addresses(ips ...string) []v1.EndpointAddress {
	var addrs []v1.EndpointAddress
	for _, ip := range ips {
		addrs = append(addrs, v1.EndpointAddress{IP: ip})
	}
	return addrs
}

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

package main

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"sort"
	"testing"

	"github.com/projectcontour/contour/internal/assert"
	"github.com/projectcontour/contour/internal/certgen"
	"github.com/projectcontour/contour/internal/dag"
	corev1 "k8s.io/api/core/v1"
)

func TestGeneratedSecretsValid(t *testing.T) {
	conf := certgenConfig{
		KubeConfig: "",
		InCluster:  false,
		Namespace:  t.Name(),
		OutputDir:  "",
		OutputKube: false,
		OutputYAML: false,
		OutputPEM:  false,
		Lifetime:   0,
		Overwrite:  false,
	}

	certmap, err := GenerateCerts(&conf)
	if err != nil {
		t.Fatalf("failed to generate certificates: %s", err)
	}

	secrets := certgen.AsSecrets(conf.Namespace, certmap)
	if len(secrets) != 2 {
		t.Errorf("expected 2 secrets, got %d", len(secrets))
	}

	wantedNames := map[string][]string{
		"envoycert": {
			"envoy",
			fmt.Sprintf("envoy.%s", conf.Namespace),
			fmt.Sprintf("envoy.%s.svc", conf.Namespace),
			fmt.Sprintf("envoy.%s.svc.cluster.local", conf.Namespace),
		},
		"contourcert": {
			"contour",
			fmt.Sprintf("contour.%s", conf.Namespace),
			fmt.Sprintf("contour.%s.svc", conf.Namespace),
			fmt.Sprintf("contour.%s.svc.cluster.local", conf.Namespace),
		},
	}

	for _, s := range secrets {
		if _, ok := wantedNames[s.Name]; !ok {
			t.Errorf("unexpected Secret name %q", s.Name)
			continue
		}

		// Check the keys we want are present.
		for _, key := range []string{
			dag.CACertificateKey,
			corev1.TLSCertKey,
			corev1.TLSPrivateKeyKey,
		} {
			if _, ok := s.Data[key]; !ok {
				t.Errorf("missing data key %q", key)
			}
		}

		pemBlock, _ := pem.Decode(s.Data[corev1.TLSCertKey])
		assert.Equal(t, pemBlock.Type, "CERTIFICATE")

		cert, err := x509.ParseCertificate(pemBlock.Bytes)
		if err != nil {
			t.Errorf("failed to parse X509 certificate: %s", err)
		}

		// Check that each certificate contains SAN entries for the right DNS names.
		sort.Strings(cert.DNSNames)
		sort.Strings(wantedNames[s.Name])
		assert.Equal(t, cert.DNSNames, wantedNames[s.Name])

	}
}

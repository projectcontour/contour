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
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	core_v1 "k8s.io/api/core/v1"

	"github.com/projectcontour/contour/internal/certgen"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/pkg/certs"
)

func TestGeneratedSecretsValid(t *testing.T) {
	conf := certgenConfig{
		KubeConfig: "",
		InCluster:  false,
		Namespace:  "foo",
		OutputDir:  "",
		OutputKube: false,
		OutputYAML: false,
		OutputPEM:  false,
		Lifetime:   0,
		Overwrite:  false,
	}

	certificates, err := certs.GenerateCerts(
		&certs.Configuration{
			Lifetime:  conf.Lifetime,
			Namespace: conf.Namespace,
		})
	require.NoError(t, err, "failed to generate certificates")

	secrets, errs := certgen.AsSecrets(conf.Namespace, "", certificates)
	if len(errs) > 0 {
		t.Errorf("expected no errors, got %d", len(errs))
	}
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
			core_v1.TLSCertKey,
			core_v1.TLSPrivateKeyKey,
		} {
			if _, ok := s.Data[key]; !ok {
				t.Errorf("missing data key %q", key)
			}
		}

		pemBlock, _ := pem.Decode(s.Data[core_v1.TLSCertKey])
		assert.Equal(t, "CERTIFICATE", pemBlock.Type)

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

func TestSecretNamePrefix(t *testing.T) {
	conf := certgenConfig{
		KubeConfig: "",
		InCluster:  false,
		Namespace:  "foo",
		OutputDir:  "",
		OutputKube: false,
		OutputYAML: false,
		OutputPEM:  false,
		Lifetime:   0,
		Overwrite:  false,
		NameSuffix: "-testsuffix",
	}

	certificates, err := certs.GenerateCerts(
		&certs.Configuration{
			Lifetime:  conf.Lifetime,
			Namespace: conf.Namespace,
		})
	require.NoError(t, err, "failed to generate certificates")

	secrets, errs := certgen.AsSecrets(conf.Namespace, conf.NameSuffix, certificates)
	if len(errs) > 0 {
		t.Errorf("expected no errors, got %d", len(errs))
	}
	if len(secrets) != 2 {
		t.Errorf("expected 2 secrets, got %d", len(secrets))
	}

	wantedNames := map[string][]string{
		"envoycert-testsuffix": {
			"envoy",
			fmt.Sprintf("envoy.%s", conf.Namespace),
			fmt.Sprintf("envoy.%s.svc", conf.Namespace),
			fmt.Sprintf("envoy.%s.svc.cluster.local", conf.Namespace),
		},
		"contourcert-testsuffix": {
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
			core_v1.TLSCertKey,
			core_v1.TLSPrivateKeyKey,
		} {
			if _, ok := s.Data[key]; !ok {
				t.Errorf("missing data key %q", key)
			}
		}

		pemBlock, _ := pem.Decode(s.Data[core_v1.TLSCertKey])
		assert.Equal(t, "CERTIFICATE", pemBlock.Type)

		cert, err := x509.ParseCertificate(pemBlock.Bytes)
		require.NoError(t, err, "failed to parse X509 certificate")

		// Check that each certificate contains SAN entries for the right DNS names.
		sort.Strings(cert.DNSNames)
		sort.Strings(wantedNames[s.Name])
		assert.Equal(t, cert.DNSNames, wantedNames[s.Name])
	}
}

func TestInvalidNamespaceAndName(t *testing.T) {
	conf := certgenConfig{
		KubeConfig: "",
		InCluster:  false,
		Namespace:  "foo!!", // contains invalid characters
		OutputDir:  "",
		OutputKube: false,
		OutputYAML: false,
		OutputPEM:  false,
		Lifetime:   0,
		Overwrite:  false,
		NameSuffix: "-testsuffix$", // contains invalid characters
	}

	certificates, err := certs.GenerateCerts(
		&certs.Configuration{
			Lifetime:  conf.Lifetime,
			Namespace: conf.Namespace,
		})
	require.NoError(t, err, "failed to generate certificates")

	secrets, errs := certgen.AsSecrets(conf.Namespace, conf.NameSuffix, certificates)
	if len(errs) != 2 {
		t.Errorf("expected 2 errors, got %d", len(errs))
	}
	if len(secrets) != 0 {
		t.Errorf("expected no secrets, got %d", len(secrets))
	}
}

func TestOutputFileMode(t *testing.T) {
	testCases := []struct {
		name         string
		insecureFile string
		cc           *certgenConfig
	}{
		{
			name: "pem format no overwrite",
			cc: &certgenConfig{
				OutputPEM: true,
				Overwrite: false,
			},
		},
		{
			name:         "pem format with overwrite",
			insecureFile: "contourcert.pem",
			cc: &certgenConfig{
				OutputPEM: true,
				Overwrite: true,
			},
		},
		{
			name: "yaml format no overwrite",
			cc: &certgenConfig{
				OutputYAML: true,
				Overwrite:  false,
				Format:     "legacy",
				Namespace:  "foo",
			},
		},
		{
			name:         "yaml format with overwrite",
			insecureFile: "contourcert.yaml",
			cc: &certgenConfig{
				OutputYAML: true,
				Overwrite:  true,
				Format:     "legacy",
				Namespace:  "foo",
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			outputDir, err := os.MkdirTemp("", "")
			require.NoError(t, err)
			defer os.RemoveAll(outputDir)
			tc.cc.OutputDir = outputDir

			// Write a file with insecure mode to ensure overwrite works as expected.
			if tc.cc.Overwrite {
				_, err = os.Create(filepath.Join(outputDir, tc.insecureFile))
				require.NoError(t, err)
			}

			generatedCerts, err := certs.GenerateCerts(
				&certs.Configuration{
					Lifetime:  tc.cc.Lifetime,
					Namespace: tc.cc.Namespace,
				})
			require.NoError(t, err)

			require.NoError(t, OutputCerts(tc.cc, nil, generatedCerts))

			err = filepath.Walk(outputDir, func(path string, info os.FileInfo, _ error) error {
				if !info.IsDir() {
					assert.Equal(t, os.FileMode(0o600), info.Mode(), "incorrect mode for file "+path)
				}
				return nil
			})
			require.NoError(t, err)
		})
	}
}

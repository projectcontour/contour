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

package httpsvc_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tsaarni/certyaml"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/projectcontour/contour/internal/fixture"
	"github.com/projectcontour/contour/internal/httpsvc"
)

func TestHTTPService(t *testing.T) {
	svc := httpsvc.Service{
		Addr:        "localhost",
		Port:        8001,
		FieldLogger: fixture.NewTestLogger(t),
	}
	svc.ServeMux.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		// Returns once the context is cancelled.
		// nolint:errcheck
		svc.Start(ctx)

		wg.Done()
	}()

	assert.Eventually(t, func() bool {
		resp, err := http.Get("http://localhost:8001/test")
		if err != nil {
			return false
		}
		resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, 1*time.Second, 100*time.Millisecond)

	// Gracefully shut down.
	cancel()
	wg.Wait()
}

func TestHTTPSService(t *testing.T) {
	// Create trusted CA, server and client certs.
	trustedCACert := certyaml.Certificate{
		Subject: "cn=ca",
	}
	contourCertBeforeRotation := certyaml.Certificate{
		Subject:         "cn=contour-before-rotation",
		SubjectAltNames: []string{"DNS:localhost"},
		Issuer:          &trustedCACert,
	}
	contourCertAfterRotation := certyaml.Certificate{
		Subject:         "cn=contour-after-rotation",
		SubjectAltNames: []string{"DNS:localhost"},
		Issuer:          &trustedCACert,
	}
	trustedClientCert := certyaml.Certificate{
		Subject: "cn=trusted-client",
		Issuer:  &trustedCACert,
	}

	// Create another CA and a client cert to test that untrusted clients are denied.
	untrustedCACert := certyaml.Certificate{
		Subject: "cn=untrusted-ca",
	}
	untrustedClientCert := certyaml.Certificate{
		Subject: "cn=untrusted-client",
		Issuer:  &untrustedCACert,
	}

	// Create temporary directory to store certificates and key for the server.
	configDir, err := os.MkdirTemp("", "contour-testdata-")
	checkFatalErr(t, err)
	defer os.RemoveAll(configDir)

	svc := httpsvc.Service{
		Addr:        "localhost",
		Port:        8001,
		CABundle:    filepath.Join(configDir, "ca.pem"),
		Cert:        filepath.Join(configDir, "server.pem"),
		Key:         filepath.Join(configDir, "server-key.pem"),
		FieldLogger: fixture.NewTestLogger(t),
	}

	// Write server credentials to temp directory.
	err = trustedCACert.WritePEM(svc.CABundle, filepath.Join(configDir, "ca-key.pem"))
	checkFatalErr(t, err)
	err = contourCertBeforeRotation.WritePEM(svc.Cert, svc.Key)
	checkFatalErr(t, err)

	svc.ServeMux.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		// nolint:errcheck
		svc.Start(ctx)
		wg.Done()
	}()

	// Create HTTPS client with trusted client certificate.
	trustedTLSClientCert, _ := trustedClientCert.TLSCertificate()
	caCertPool := x509.NewCertPool()
	ca, err := trustedCACert.X509Certificate()
	checkFatalErr(t, err)
	caCertPool.AddCert(&ca)

	// Wrap the first HTTP request in Eventually() since the server takes bit time to start.
	assert.Eventually(t, func() bool {
		resp, err := tryGet("https://localhost:8001/test", trustedTLSClientCert, caCertPool)
		if err != nil {
			return false
		}
		resp.Body.Close()
		expectedCert, _ := contourCertBeforeRotation.X509Certificate()
		assert.Equal(t, &expectedCert, resp.TLS.PeerCertificates[0])
		assert.GreaterOrEqual(t, uint16(tls.VersionTLS13), resp.TLS.Version)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		return true
	}, 1*time.Second, 100*time.Millisecond)

	// Rotate server certificates.
	err = contourCertAfterRotation.WritePEM(svc.Cert, svc.Key)
	checkFatalErr(t, err)

	resp, err := tryGet("https://localhost:8001/test", trustedTLSClientCert, caCertPool)
	require.NoError(t, err)
	resp.Body.Close()
	expectedCert, _ := contourCertAfterRotation.X509Certificate()
	assert.Equal(t, &expectedCert, resp.TLS.PeerCertificates[0])

	// Connection should fail when trying to connect with untrusted client cert.
	untrustedTLSClientCert, _ := untrustedClientCert.TLSCertificate()
	_, err = tryGet("https://localhost:8001/test", untrustedTLSClientCert, caCertPool) // nolint // false positive: response body must be closed
	require.Error(t, err)

	// Gracefully shut down.
	cancel()
	wg.Wait()
}

func checkFatalErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func tryGet(url string, clientCert tls.Certificate, caCertPool *x509.CertPool) (*http.Response, error) {
	client := &http.Client{
		Transport: &http.Transport{
			// Ignore "TLS MinVersion too low" to test that TLSv1.3 will be negotiated.
			// #nosec G402
			TLSClientConfig: &tls.Config{
				RootCAs:      caCertPool,
				Certificates: []tls.Certificate{clientCert},
			},
		},
	}
	return client.Get(url)
}

func TestServiceNotRequireLeaderElection(t *testing.T) {
	var s manager.LeaderElectionRunnable = &httpsvc.Service{}
	require.False(t, s.NeedLeaderElection())
}

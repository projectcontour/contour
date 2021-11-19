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

package httpsvc

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"net/http"
	"testing"
	"time"

	"github.com/projectcontour/contour/internal/fixture"
	"github.com/projectcontour/contour/internal/workgroup"
	"github.com/stretchr/testify/assert"
)

func TestHTTPService(t *testing.T) {
	svc := Service{
		Addr:        "localhost",
		Port:        8001,
		FieldLogger: fixture.NewTestLogger(t),
	}
	svc.ServeMux.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	ctx, cancel := context.WithCancel(context.Background())
	var wg workgroup.Group
	wg.Add(svc.Start)
	done := make(chan error)
	go func() {
		done <- wg.Run(ctx)
	}()

	assert.Eventually(t, func() bool {
		resp, err := http.Get("http://localhost:8001/test")
		if err != nil {
			return false
		}
		resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, 1*time.Second, 100*time.Millisecond)

	cancel()
	<-done
}

func TestHTTPSService(t *testing.T) {
	svc := Service{
		Addr:        "localhost",
		Port:        8001,
		CABundle:    "testdata/ca.pem",
		Cert:        "testdata/server.pem",
		Key:         "testdata/server-key.pem",
		FieldLogger: fixture.NewTestLogger(t),
	}
	svc.ServeMux.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	ctx, cancel := context.WithCancel(context.Background())
	var wg workgroup.Group
	wg.Add(svc.Start)
	done := make(chan error)
	go func() {
		done <- wg.Run(ctx)
	}()

	buf, err := ioutil.ReadFile("testdata/ca.pem")
	if err != nil {
		t.Fatal(err)
	}
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(buf)

	clientCert, err := tls.LoadX509KeyPair("testdata/client.pem", "testdata/client-key.pem")
	if err != nil {
		t.Fatal(err)
	}

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

	assert.Eventually(t, func() bool {
		resp, err := client.Get("https://localhost:8001/test")
		if err != nil {
			return false
		}
		resp.Body.Close()
		return resp.TLS.Version == tls.VersionTLS13 && resp.StatusCode == http.StatusOK
	}, 1*time.Second, 100*time.Millisecond)

	cancel()
	<-done
}

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
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/projectcontour/contour/internal/envoy"
	"github.com/projectcontour/contour/internal/k8s"

	"github.com/google/go-cmp/cmp"
	"github.com/projectcontour/contour/internal/assert"
	"google.golang.org/grpc"
	"gopkg.in/yaml.v2"
)

func TestServeContextProxyRootNamespaces(t *testing.T) {
	tests := map[string]struct {
		ctx  serveContext
		want []string
	}{
		"empty": {
			ctx: serveContext{
				rootNamespaces: "",
			},
			want: nil,
		},
		"blank-ish": {
			ctx: serveContext{
				rootNamespaces: " \t ",
			},
			want: nil,
		},
		"one value": {
			ctx: serveContext{
				rootNamespaces: "projectcontour",
			},
			want: []string{"projectcontour"},
		},
		"multiple, easy": {
			ctx: serveContext{
				rootNamespaces: "prod1,prod2,prod3",
			},
			want: []string{"prod1", "prod2", "prod3"},
		},
		"multiple, hard": {
			ctx: serveContext{
				rootNamespaces: "prod1, prod2, prod3 ",
			},
			want: []string{"prod1", "prod2", "prod3"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := tc.ctx.proxyRootNamespaces()
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("expected: %q, got: %q", tc.want, got)
			}
		})
	}
}

func TestServeContextTLSParams(t *testing.T) {
	tests := map[string]struct {
		ctx         serveContext
		expecterror bool
	}{
		"tls supplied correctly": {
			ctx: serveContext{
				caFile:      "cacert.pem",
				contourCert: "contourcert.pem",
				contourKey:  "contourkey.pem",
			},
			expecterror: false,
		},
		"tls partially supplied": {
			ctx: serveContext{
				contourCert: "contourcert.pem",
				contourKey:  "contourkey.pem",
			},
			expecterror: true,
		},
		"tls not supplied": {
			ctx:         serveContext{},
			expecterror: true,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := tc.ctx.verifyTLSFlags()
			goterror := err != nil
			if goterror != tc.expecterror {
				t.Errorf("TLS Config: %s", err)
			}
		})
	}
}

func TestConfigFileDefaultOverrideImport(t *testing.T) {
	tests := map[string]struct {
		yamlIn string
		want   func() *serveContext
	}{
		"empty configuration": {
			yamlIn: ``,
			want:   newServeContext,
		},
		"defaults in yaml": {
			yamlIn: `
incluster: false
disablePermitInsecure: false
leaderelection:
  configmap-name: leader-elect
  configmap-namespace: projectcontour
  lease-duration: 15s
  renew-deadline: 10s
  retry-period: 2s
`,
			want: newServeContext,
		},
		"blank tls configuration": {
			yamlIn: `
tls:
`,
			want: newServeContext,
		},
		"tls configuration only": {
			yamlIn: `
tls:
  minimum-protocol-version: 1.2
`,
			want: func() *serveContext {
				ctx := newServeContext()
				ctx.TLSConfig.MinimumProtocolVersion = "1.2"
				return ctx
			},
		},
		"leader election namespace and configmap only": {
			yamlIn: `
leaderelection:
  configmap-name: foo
  configmap-namespace: bar
`,
			want: func() *serveContext {
				ctx := newServeContext()
				ctx.LeaderElectionConfig.Name = "foo"
				ctx.LeaderElectionConfig.Namespace = "bar"
				return ctx
			},
		},
		"leader election all fields set": {
			yamlIn: `
leaderelection:
  configmap-name: foo
  configmap-namespace: bar
  lease-duration: 600s
  renew-deadline: 500s
  retry-period: 60s
`,
			want: func() *serveContext {
				ctx := newServeContext()
				ctx.LeaderElectionConfig.Name = "foo"
				ctx.LeaderElectionConfig.Namespace = "bar"
				ctx.LeaderElectionConfig.LeaseDuration = 600 * time.Second
				ctx.LeaderElectionConfig.RenewDeadline = 500 * time.Second
				ctx.LeaderElectionConfig.RetryPeriod = 60 * time.Second
				return ctx
			},
		},
		"default http versions": {
			yamlIn: `
default-http-versions:
- http/1.1
- http/2
- http/99
`,
			want: func() *serveContext {
				ctx := newServeContext()
				// Note that version validity isn't checked at this point.
				ctx.DefaultHTTPVersions = []string{"http/1.1", "http/2", "http/99"}
				return ctx
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := newServeContext()
			err := yaml.UnmarshalStrict([]byte(tc.yamlIn), got)
			checkFatalErr(t, err)
			want := tc.want()

			if diff := cmp.Diff(*want, *got, cmp.AllowUnexported(serveContext{})); diff != "" {
				t.Error(diff)
			}
		})
	}
}

func TestFallbackCertificateParams(t *testing.T) {
	tests := map[string]struct {
		ctx         serveContext
		want        *k8s.FullName
		expecterror bool
	}{
		"fallback cert params passed correctly": {
			ctx: serveContext{
				TLSConfig: TLSConfig{
					FallbackCertificate: FallbackCertificate{
						Name:      "fallbacksecret",
						Namespace: "root-namespace",
					},
				},
			},
			want: &k8s.FullName{
				Name:      "fallbacksecret",
				Namespace: "root-namespace",
			},
			expecterror: false,
		},
		"missing namespace": {
			ctx: serveContext{
				TLSConfig: TLSConfig{
					FallbackCertificate: FallbackCertificate{
						Name: "fallbacksecret",
					},
				},
			},
			want:        nil,
			expecterror: true,
		},
		"missing name": {
			ctx: serveContext{
				TLSConfig: TLSConfig{
					FallbackCertificate: FallbackCertificate{
						Namespace: "root-namespace",
					},
				},
			},
			want:        nil,
			expecterror: true,
		},
		"fallback cert not defined": {
			ctx:         serveContext{},
			want:        nil,
			expecterror: false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := tc.ctx.fallbackCertificate()

			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatal(diff)
			}

			goterror := err != nil
			if goterror != tc.expecterror {
				t.Errorf("Expected Fallback Certificate error: %s", err)
			}
		})
	}
}

// Testdata for this test case can be re-generated by running:
// make gencerts
// cp certs/*.pem cmd/contour/testdata/X/
func TestServeContextCertificateHandling(t *testing.T) {
	tests := map[string]struct {
		serverCredentialsDir string
		clientCredentialsDir string
		expectedServerCert   string
		expectError          bool
	}{
		"successful TLS connection established": {
			serverCredentialsDir: "testdata/1",
			clientCredentialsDir: "testdata/1",
			expectedServerCert:   "testdata/1/contourcert.pem",
			expectError:          false,
		},
		"rotating server credentials returns new server cert": {
			serverCredentialsDir: "testdata/2",
			clientCredentialsDir: "testdata/2",
			expectedServerCert:   "testdata/2/contourcert.pem",
			expectError:          false,
		},
		"rotating server credentials again to ensure rotation can be repeated": {
			serverCredentialsDir: "testdata/1",
			clientCredentialsDir: "testdata/1",
			expectedServerCert:   "testdata/1/contourcert.pem",
			expectError:          false,
		},
		"fail to connect with client certificate which is not signed by correct CA": {
			serverCredentialsDir: "testdata/2",
			clientCredentialsDir: "testdata/1",
			expectedServerCert:   "testdata/2/contourcert.pem",
			expectError:          true,
		},
	}

	// Create temporary directory to store certificates and key for the server.
	configDir, err := ioutil.TempDir("", "contour-testdata-")
	checkFatalErr(t, err)
	defer os.RemoveAll(configDir)

	ctx := serveContext{
		caFile:      filepath.Join(configDir, "CAcert.pem"),
		contourCert: filepath.Join(configDir, "contourcert.pem"),
		contourKey:  filepath.Join(configDir, "contourkey.pem"),
	}

	// Initial set of credentials must be linked into temp directory before
	// starting the tests to avoid error at server startup.
	err = linkFiles("testdata/1", configDir)
	checkFatalErr(t, err)

	// Start a dummy server.
	opts := ctx.grpcOptions()
	g := grpc.NewServer(opts...)
	if g == nil {
		t.Error("failed to create server")
	}

	address := "localhost:8001"
	l, err := net.Listen("tcp", address)
	checkFatalErr(t, err)

	go func() {
		err = g.Serve(l)
		checkFatalErr(t, err)
	}()
	defer g.Stop()

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// Link certificates and key to temp dir used by serveContext.
			err = linkFiles(tc.serverCredentialsDir, configDir)
			checkFatalErr(t, err)
			receivedCert, err := tryConnect(address, tc.clientCredentialsDir)
			gotError := err != nil
			if gotError != tc.expectError {
				t.Errorf("Unexpected result when connecting to the server: %s", err)
			}
			if err == nil {
				expectedCert, err := loadCertificate(tc.expectedServerCert)
				checkFatalErr(t, err)
				assert.Equal(t, receivedCert, expectedCert)
			}
		})
	}
}

func checkFatalErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// linkFiles creates symbolic link of files in src directory to the dst directory.
func linkFiles(src string, dst string) error {
	absSrc, err := filepath.Abs(src)
	if err != nil {
		return err
	}

	matches, err := filepath.Glob(filepath.Join(absSrc, "*"))
	if err != nil {
		return err
	}

	for _, filename := range matches {
		basename := filepath.Base(filename)
		os.Remove(filepath.Join(dst, basename))
		err := os.Symlink(filename, filepath.Join(dst, basename))
		if err != nil {
			return err
		}
	}

	return nil
}

// tryConnect tries to establish TLS connection to the server.
// If successful, return the server certificate.
func tryConnect(address string, clientCredentialsDir string) (*x509.Certificate, error) {
	clientCert := filepath.Join(clientCredentialsDir, "envoycert.pem")
	clientKey := filepath.Join(clientCredentialsDir, "envoykey.pem")
	cert, err := tls.LoadX509KeyPair(clientCert, clientKey)
	if err != nil {
		return nil, err
	}

	clientConfig := &tls.Config{
		ServerName:         "localhost",
		Certificates:       []tls.Certificate{cert},
		InsecureSkipVerify: true, // nolint:gosec
	}
	conn, err := tls.Dial("tcp", address, clientConfig)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	err = peekError(conn)
	if err != nil {
		return nil, err
	}

	return conn.ConnectionState().PeerCertificates[0], nil
}

func loadCertificate(path string) (*x509.Certificate, error) {
	buf, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(buf)
	return x509.ParseCertificate(block.Bytes)
}

// peekError is a workaround for TLS 1.3: due to shortened handshake, TLS alert
// from server is received at first read from the socket.
// To receive alert for bad certificate, this function tries to read one byte.
// Adapted from https://golang.org/src/crypto/tls/handshake_client_test.go
func peekError(conn net.Conn) error {
	_ = conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	_, err := conn.Read(make([]byte, 1))
	if err != nil {
		if netErr, ok := err.(net.Error); !ok || !netErr.Timeout() {
			return err
		}
	}
	return nil
}

func TestParseHTTPVersions(t *testing.T) {
	cases := map[string]struct {
		versions      []string
		parseError    error
		parseVersions []envoy.HTTPVersionType
	}{
		"empty": {
			versions:      []string{},
			parseError:    nil,
			parseVersions: nil,
		},
		"invalid proto": {
			versions:      []string{"foo"},
			parseError:    errors.New("invalid"),
			parseVersions: nil,
		},
		"http/1.1": {
			versions:      []string{"http/1.1", "HTTP/1.1"},
			parseError:    nil,
			parseVersions: []envoy.HTTPVersionType{envoy.HTTPVersion1},
		},
		"http/1.1+http/2": {
			versions:      []string{"http/1.1", "http/2"},
			parseError:    nil,
			parseVersions: []envoy.HTTPVersionType{envoy.HTTPVersion1, envoy.HTTPVersion2},
		},
		"http/1.1+http/2 duplicated": {
			versions:      []string{"http/1.1", "http/2", "http/1.1", "http/2"},
			parseError:    nil,
			parseVersions: []envoy.HTTPVersionType{envoy.HTTPVersion1, envoy.HTTPVersion2},
		},
	}

	for name, testcase := range cases {
		testcase := testcase
		t.Run(name, func(t *testing.T) {
			vers, err := parseDefaultHTTPVersions(testcase.versions)

			// parseDefaultHTTPVersions doesn't guarantee a stable result, but the order doesn't matter.
			sort.Slice(vers,
				func(i, j int) bool { return vers[i] < vers[j] })
			sort.Slice(testcase.parseVersions,
				func(i, j int) bool { return testcase.parseVersions[i] < testcase.parseVersions[j] })

			assert.Equal(t, err, testcase.parseError)
			assert.Equal(t, vers, testcase.parseVersions)
		})
	}
}

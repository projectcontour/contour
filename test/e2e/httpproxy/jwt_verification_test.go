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

//go:build e2e

package httpproxy

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/test/e2e"
)

type jwtClaims struct {
	Issuer   string   `json:"iss,omitempty"`
	Audience []string `json:"aud,omitempty"`
	Expiry   int64    `json:"exp,omitempty"`
}

// remoteJWKSProvider sets up a JWT provider that refers to a JWKS served by an HTTP server within the cluster.
func remoteJWKSProvider(namespace string, jwksJSON []byte) contour_v1.JWTProvider {
	setHandler := e2e.StartLocalHTTPService(f.T(), f.Client, namespace, "jwks-server")
	setHandler(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(jwksJSON)
	})

	return contour_v1.JWTProvider{
		Name:    "test-provider",
		Default: true,
		RemoteJWKS: contour_v1.RemoteJWKS{
			URI:             fmt.Sprintf("http://jwks-server.%s.svc.cluster.local/jwks.json", namespace),
			Timeout:         "5s",
			DNSLookupFamily: "auto",
		},
	}
}

// localJWKSProvider sets up a JWT provider that refers to a JWKS stored in a Kubernetes Secret.
func localJWKSProvider(namespace string, jwksJSON []byte) contour_v1.JWTProvider {
	secret := &core_v1.Secret{
		ObjectMeta: meta_v1.ObjectMeta{Name: "jwks", Namespace: namespace},
		Type:       core_v1.SecretTypeOpaque,
		Data:       map[string][]byte{"jwks.json": jwksJSON},
	}
	require.NoError(f.T(), f.Client.Create(context.TODO(), secret))

	return contour_v1.JWTProvider{
		Name:    "test-provider",
		Default: true,
		LocalJWKS: contour_v1.LocalJWKS{
			SecretName: "jwks",
			Key:        "jwks.json",
		},
	}
}

// signJWT creates a JWT signed with the given RSA private key and claims.
func signJWT(key *rsa.PrivateKey, claims jwtClaims) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))

	payload, err := json.Marshal(claims)
	if err != nil {
		panic(err)
	}
	payloadEnc := base64.RawURLEncoding.EncodeToString(payload)

	signingInput := header + "." + payloadEnc

	hash := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, hash[:])
	if err != nil {
		panic(err)
	}

	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig)
}

// marshalJWKS creates a JWKS JSON to be served by the JWKS provider.
func marshalJWKS(key *rsa.PrivateKey) []byte {
	jwks := map[string]any{
		"keys": []map[string]any{
			{
				"kty": "RSA",
				"alg": "RS256",
				"use": "sig",
				"n":   base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
				"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
			},
		},
	}
	data, err := json.Marshal(jwks)
	if err != nil {
		panic(err)
	}
	return data
}

// testJWTVerification tests JWT verification using the given JWKS source setup function.
func testJWTVerification(setupProvider func(namespace string, jwksJSON []byte) contour_v1.JWTProvider) e2e.NamespacedTestBody {
	return func(namespace string) {
		Specify("JWT verification", func() {
			t := f.T()

			privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
			require.NoError(t, err)
			jwksJSON := marshalJWKS(privateKey)

			wrongKey, err := rsa.GenerateKey(rand.Reader, 2048)
			require.NoError(t, err)

			f.Fixtures.Echo.Deploy(namespace, "echo")
			f.Certs.CreateSelfSignedCert(namespace, "echo-tls", "echo-tls", "jwt.projectcontour.io")

			bearerToken := func(token string) func(*http.Request) {
				return func(r *http.Request) {
					r.Header.Set("Authorization", "Bearer "+token)
				}
			}

			provider := setupProvider(namespace, jwksJSON)
			provider.Issuer = "correct-issuer"
			provider.Audiences = []string{"correct-audience"}
			provider.ForwardJWT = true

			p := &contour_v1.HTTPProxy{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: namespace,
					Name:      "jwt-verification",
				},
				Spec: contour_v1.HTTPProxySpec{
					VirtualHost: &contour_v1.VirtualHost{
						Fqdn: "jwt.projectcontour.io",
						TLS: &contour_v1.TLS{
							SecretName: "echo-tls",
						},
						JWTProviders: []contour_v1.JWTProvider{provider},
					},
					Routes: []contour_v1.Route{
						{
							Conditions: []contour_v1.MatchCondition{{Prefix: "/secured"}},
							Services:   []contour_v1.Service{{Name: "echo", Port: 80}},
						},
						{
							Conditions:            []contour_v1.MatchCondition{{Prefix: "/open"}},
							JWTVerificationPolicy: &contour_v1.JWTVerificationPolicy{Disabled: true},
							Services:              []contour_v1.Service{{Name: "echo", Port: 80}},
						},
					},
				},
			}
			require.True(t, f.CreateHTTPProxyAndWaitFor(p, e2e.HTTPProxyValid))

			validToken := signJWT(privateKey, jwtClaims{
				Issuer:   "correct-issuer",
				Audience: []string{"correct-audience"},
				Expiry:   time.Now().Add(time.Hour).Unix(),
			})

			By("valid JWT is accepted")
			res, ok := f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
				Host:        p.Spec.VirtualHost.Fqdn,
				Path:        "/secured",
				RequestOpts: []func(*http.Request){bearerToken(validToken)},
				Condition:   e2e.HasStatusCode(200),
			})
			require.NotNil(t, res, "request never succeeded")
			assert.True(t, ok, "expected 200, got %d", res.StatusCode)

			By("JWT is forwarded to backend")
			body := f.GetEchoResponseBody(res.Body)
			assert.Contains(t, body.RequestHeaders.Get("Authorization"), "Bearer ", "JWT should be forwarded to backend")

			By("missing JWT is rejected")
			res, ok = f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
				Host:      p.Spec.VirtualHost.Fqdn,
				Path:      "/secured",
				Condition: e2e.HasStatusCode(401),
			})
			require.NotNil(t, res, "request never succeeded")
			assert.True(t, ok, "expected 401, got %d", res.StatusCode)

			By("JWT signed with wrong key is rejected")
			res, ok = f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
				Host: p.Spec.VirtualHost.Fqdn,
				Path: "/secured",
				RequestOpts: []func(*http.Request){
					bearerToken(signJWT(wrongKey, jwtClaims{
						Issuer:   "correct-issuer",
						Audience: []string{"correct-audience"},
						Expiry:   time.Now().Add(time.Hour).Unix(),
					})),
				},
				Condition: e2e.HasStatusCode(401),
			})
			require.NotNil(t, res, "request never succeeded")
			assert.True(t, ok, "expected 401, got %d", res.StatusCode)

			By("expired JWT is rejected")
			res, ok = f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
				Host: p.Spec.VirtualHost.Fqdn,
				Path: "/secured",
				RequestOpts: []func(*http.Request){
					bearerToken(signJWT(privateKey, jwtClaims{
						Issuer:   "correct-issuer",
						Audience: []string{"correct-audience"},
						Expiry:   time.Now().Add(-time.Hour).Unix(),
					})),
				},
				Condition: e2e.HasStatusCode(401),
			})
			require.NotNil(t, res, "request never succeeded")
			assert.True(t, ok, "expected 401, got %d", res.StatusCode)

			By("wrong issuer is rejected")
			res, ok = f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
				Host: p.Spec.VirtualHost.Fqdn,
				Path: "/secured",
				RequestOpts: []func(*http.Request){
					bearerToken(signJWT(privateKey, jwtClaims{
						Issuer:   "wrong-issuer",
						Audience: []string{"correct-audience"},
						Expiry:   time.Now().Add(time.Hour).Unix(),
					})),
				},
				Condition: e2e.HasStatusCode(401),
			})
			require.NotNil(t, res, "request never succeeded")
			assert.True(t, ok, "expected 401, got %d", res.StatusCode)

			By("wrong audience is rejected")
			res, ok = f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
				Host: p.Spec.VirtualHost.Fqdn,
				Path: "/secured",
				RequestOpts: []func(*http.Request){
					bearerToken(signJWT(privateKey, jwtClaims{
						Issuer:   "correct-issuer",
						Audience: []string{"wrong-audience"},
						Expiry:   time.Now().Add(time.Hour).Unix(),
					})),
				},
				Condition: e2e.HasStatusCode(403),
			})
			require.NotNil(t, res, "request never succeeded")
			assert.True(t, ok, "expected 403, got %d", res.StatusCode)

			By("route bypasses JWT verification when the policy is disabled")
			res, ok = f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
				Host:      p.Spec.VirtualHost.Fqdn,
				Path:      "/open",
				Condition: e2e.HasStatusCode(200),
			})
			require.NotNil(t, res, "request never succeeded")
			assert.True(t, ok, "expected 200 (auth bypassed), got %d", res.StatusCode)
		})
	}
}

// testJWTVerificationRemoteJWKSRotation tests that rotated JWKS on the remote server is picked up by Envoy after cache expiry.
func testJWTVerificationRemoteJWKSRotation(namespace string) {
	Specify("remote JWKS rotation", func() {
		t := f.T()

		// Generate initial key and JWKS.
		initialKey, err := rsa.GenerateKey(rand.Reader, 2048)
		require.NoError(t, err)

		// Start JWKS server with initial key.
		setHandler := e2e.StartLocalHTTPService(t, f.Client, namespace, "jwks-server")
		setHandler(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(marshalJWKS(initialKey))
		})

		f.Fixtures.Echo.Deploy(namespace, "echo")
		f.Certs.CreateSelfSignedCert(namespace, "echo-tls", "echo-tls", "jwt-rotation.projectcontour.io")

		p := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "jwt-rotation",
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "jwt-rotation.projectcontour.io",
					TLS: &contour_v1.TLS{
						SecretName: "echo-tls",
					},
					JWTProviders: []contour_v1.JWTProvider{{
						Name:    "test-provider",
						Default: true,
						Issuer:  "test-issuer",
						RemoteJWKS: contour_v1.RemoteJWKS{
							URI:             fmt.Sprintf("http://jwks-server.%s.svc.cluster.local/jwks.json", namespace),
							Timeout:         "5s",
							CacheDuration:   "1s",
							DNSLookupFamily: "auto",
						},
					}},
				},
				Routes: []contour_v1.Route{{
					Services: []contour_v1.Service{{Name: "echo", Port: 80}},
				}},
			},
		}
		require.True(t, f.CreateHTTPProxyAndWaitFor(p, e2e.HTTPProxyValid))

		bearerToken := func(token string) func(*http.Request) {
			return func(r *http.Request) {
				r.Header.Set("Authorization", "Bearer "+token)
			}
		}

		initialToken := signJWT(initialKey, jwtClaims{
			Issuer: "test-issuer",
			Expiry: time.Now().Add(time.Hour).Unix(),
		})

		By("token signed with initial key is accepted")
		res, ok := f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
			Host:        p.Spec.VirtualHost.Fqdn,
			Path:        "/",
			RequestOpts: []func(*http.Request){bearerToken(initialToken)},
			Condition:   e2e.HasStatusCode(200),
		})
		require.NotNil(t, res, "request never succeeded")
		require.True(t, ok, "expected 200, got %d", res.StatusCode)

		By("rotating to new key on the JWKS server")
		newKey, err := rsa.GenerateKey(rand.Reader, 2048)
		require.NoError(t, err)
		setHandler(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(marshalJWKS(newKey))
		})

		newToken := signJWT(newKey, jwtClaims{
			Issuer: "test-issuer",
			Expiry: time.Now().Add(time.Hour).Unix(),
		})

		By("token signed with new key is accepted after cache expires")
		// Wait for cache to expire (CacheDuration=1s).
		time.Sleep(2 * time.Second)
		res, ok = f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
			Host:        p.Spec.VirtualHost.Fqdn,
			Path:        "/",
			RequestOpts: []func(*http.Request){bearerToken(newToken)},
			Condition:   e2e.HasStatusCode(200),
		})
		require.NotNil(t, res, "request never succeeded")
		require.True(t, ok, "expected 200, got %d", res.StatusCode)

		By("token signed with initial key is rejected after rotation")
		res, ok = f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
			Host:        p.Spec.VirtualHost.Fqdn,
			Path:        "/",
			RequestOpts: []func(*http.Request){bearerToken(initialToken)},
			Condition:   e2e.HasStatusCode(401),
		})
		require.NotNil(t, res, "request never succeeded")
		require.True(t, ok, "expected 401, got %d", res.StatusCode)
	})
}

// testJWTVerificationLocalJWKSRotation tests that rotated JWKS in the Secret is pushed to Envoy at update.
func testJWTVerificationLocalJWKSRotation(namespace string) {
	Specify("local JWKS rotation", func() {
		t := f.T()

		initialKey, err := rsa.GenerateKey(rand.Reader, 2048)
		require.NoError(t, err)

		secret := &core_v1.Secret{
			ObjectMeta: meta_v1.ObjectMeta{Name: "jwks", Namespace: namespace},
			Type:       core_v1.SecretTypeOpaque,
			Data:       map[string][]byte{"jwks.json": marshalJWKS(initialKey)},
		}
		require.NoError(t, f.Client.Create(context.TODO(), secret))

		f.Fixtures.Echo.Deploy(namespace, "echo")
		f.Certs.CreateSelfSignedCert(namespace, "echo-tls", "echo-tls", "jwt-rotation-local.projectcontour.io")

		p := &contour_v1.HTTPProxy{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: namespace,
				Name:      "jwt-rotation-local",
			},
			Spec: contour_v1.HTTPProxySpec{
				VirtualHost: &contour_v1.VirtualHost{
					Fqdn: "jwt-rotation-local.projectcontour.io",
					TLS: &contour_v1.TLS{
						SecretName: "echo-tls",
					},
					JWTProviders: []contour_v1.JWTProvider{{
						Name:    "test-provider",
						Default: true,
						Issuer:  "test-issuer",
						LocalJWKS: contour_v1.LocalJWKS{
							SecretName: "jwks",
							Key:        "jwks.json",
						},
					}},
				},
				Routes: []contour_v1.Route{{
					Services: []contour_v1.Service{{Name: "echo", Port: 80}},
				}},
			},
		}
		require.True(t, f.CreateHTTPProxyAndWaitFor(p, e2e.HTTPProxyValid))

		bearerToken := func(token string) func(*http.Request) {
			return func(r *http.Request) {
				r.Header.Set("Authorization", "Bearer "+token)
			}
		}

		initialToken := signJWT(initialKey, jwtClaims{
			Issuer: "test-issuer",
			Expiry: time.Now().Add(time.Hour).Unix(),
		})

		By("token signed with initial key is accepted")
		res, ok := f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
			Host:        p.Spec.VirtualHost.Fqdn,
			Path:        "/",
			RequestOpts: []func(*http.Request){bearerToken(initialToken)},
			Condition:   e2e.HasStatusCode(200),
		})
		require.NotNil(t, res, "request never succeeded")
		require.True(t, ok, "expected 200, got %d", res.StatusCode)

		By("rotating to new key in the Secret")
		newKey, err := rsa.GenerateKey(rand.Reader, 2048)
		require.NoError(t, err)
		secret.Data = map[string][]byte{"jwks.json": marshalJWKS(newKey)}
		require.NoError(t, f.Client.Update(context.TODO(), secret))

		newToken := signJWT(newKey, jwtClaims{
			Issuer: "test-issuer",
			Expiry: time.Now().Add(time.Hour).Unix(),
		})

		By("token signed with new key is accepted after Secret update propagates")
		res, ok = f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
			Host:        p.Spec.VirtualHost.Fqdn,
			Path:        "/",
			RequestOpts: []func(*http.Request){bearerToken(newToken)},
			Condition:   e2e.HasStatusCode(200),
		})
		require.NotNil(t, res, "request never succeeded")
		require.True(t, ok, "expected 200, got %d", res.StatusCode)

		By("token signed with initial key is rejected after rotation")
		res, ok = f.HTTP.SecureRequestUntil(&e2e.HTTPSRequestOpts{
			Host:        p.Spec.VirtualHost.Fqdn,
			Path:        "/",
			RequestOpts: []func(*http.Request){bearerToken(initialToken)},
			Condition:   e2e.HasStatusCode(401),
		})
		require.NotNil(t, res, "request never succeeded")
		require.True(t, ok, "expected 401, got %d", res.StatusCode)
	})
}

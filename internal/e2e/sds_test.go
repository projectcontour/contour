// Copyright © 2019 Heptio
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

package e2e

import (
	"context"
	"testing"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	"github.com/gogo/protobuf/types"
	"github.com/heptio/contour/internal/dag"
	"github.com/heptio/contour/internal/envoy"
	"google.golang.org/grpc"
	v1 "k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v2"
)

func TestSDSVisibility(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	// s1 is a tls secret
	s1 := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			v1.TLSCertKey:       []byte("certificate"),
			v1.TLSPrivateKeyKey: []byte("key"),
		},
	}
	// add secret
	rh.OnAdd(s1)

	// assert that the secret is _not_ visible as it is
	// not referenced by any ingress/ingressroute
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "1",
		Resources:   []types.Any{},
		TypeUrl:     secretType,
		Nonce:       "1",
	}, streamSDS(t, cc))

	// i1 is a tls ingress
	i1 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: v1beta1.IngressSpec{
			Backend: backend("backend", intstr.FromInt(80)),
			TLS: []v1beta1.IngressTLS{{
				Hosts:      []string{"kuard.example.com"},
				SecretName: "secret",
			}},
		},
	}
	rh.OnAdd(i1)

	// TODO(dfc) #1165: secret should not be present if the ingress does not
	// have any valid routes.
	// i1 has a default route to backend:80, but there is no matching service.
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "2",
		Resources: []types.Any{
			any(t, secret(s1)),
		},
		TypeUrl: secretType,
		Nonce:   "2",
	}, streamSDS(t, cc))
}

func streamSDS(t *testing.T, cc *grpc.ClientConn, rn ...string) *v2.DiscoveryResponse {
	t.Helper()
	sds := discovery.NewSecretDiscoveryServiceClient(cc)
	st, err := sds.StreamSecrets(context.TODO())
	check(t, err)
	return stream(t, st, &v2.DiscoveryRequest{
		TypeUrl:       secretType,
		ResourceNames: rn,
	})
}

func secret(sec *v1.Secret) *auth.Secret {
	return envoy.Secret(&dag.Secret{
		Object: sec,
	})
}

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

package k8s

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	core_v1 "k8s.io/api/core/v1"
	networking_v1 "k8s.io/api/networking/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayapi_v1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayapi_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayapi_v1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	contour_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
)

func TestIsObjectEqual(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		equals   bool
	}{
		{
			name:     "Secret with content change",
			filename: "testdata/secret-content-change.yaml",
			equals:   false,
		},
		{
			name:     "Secret with metadata change",
			filename: "testdata/secret-metadata-change.yaml",
			equals:   true,
		},
		{
			name:     "ConfigMap with content change",
			filename: "testdata/configmap-content-change.yaml",
			equals:   false,
		},
		{
			name:     "ConfigMap with metadata change",
			filename: "testdata/configmap-metadata-change.yaml",
			equals:   true,
		},
		{
			name:     "Service with status change",
			filename: "testdata/service-status-change.yaml",
			equals:   false,
		},
		{
			name:     "Service with annotation change",
			filename: "testdata/service-annotation-change.yaml",
			equals:   false,
		},
		{
			name:     "Endpoint with content change",
			filename: "testdata/endpoint-content-change.yaml",
			equals:   false,
		},
		{
			name:     "HTTPProxy with annotation change",
			filename: "testdata/httpproxy-annotation-change.yaml",
			equals:   false,
		},
		{
			name:     "Ingress with annotation change",
			filename: "testdata/ingress-annotation-change.yaml",
			equals:   false,
		},
		{
			name:     "Namespace with label change",
			filename: "testdata/namespace-label-change.yaml",
			equals:   false,
		},
	}

	scheme := runtime.NewScheme()
	_ = core_v1.AddToScheme(scheme)
	_ = networking_v1.AddToScheme(scheme)
	_ = contour_v1.AddKnownTypes(scheme)

	deserializer := serializer.NewCodecFactory(scheme).UniversalDeserializer()

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			buf, err := os.ReadFile(tc.filename)
			require.NoError(t, err)

			// Each file contains two YAML records, which should be compared with each other.
			objects := strings.Split(string(buf), "---")
			assert.Len(t, objects, 2, "expected 2 objects in file")

			// Decode the objects.
			oldObj, _, err := deserializer.Decode([]byte(objects[0]), nil, nil)
			require.NoError(t, err)
			newObj, _, err := deserializer.Decode([]byte(objects[1]), nil, nil)
			require.NoError(t, err)

			got, err := IsObjectEqual(oldObj.(client.Object), newObj.(client.Object))
			require.NoError(t, err)
			assert.Equal(t, tc.equals, got)
		})
	}
}

func TestIsEqualForResourceVersion(t *testing.T) {
	oldS := &core_v1.Secret{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:            "test",
			Namespace:       "default",
			ResourceVersion: "123",
		},
		Data: map[string][]byte{
			"foo": []byte("bar"),
		},
	}

	newS := oldS.DeepCopy()

	// Objects with equal ResourceVersion should evaluate to true.
	got, err := IsObjectEqual(oldS, newS)
	require.NoError(t, err)
	assert.True(t, got)

	// Differences in data should be ignored.
	newS.Data["foo"] = []byte("baz")
	got, err = IsObjectEqual(oldS, newS)
	require.NoError(t, err)
	assert.True(t, got)
}

// TestIsEqualFallback compares with ServiceAccount objects, which are not supported.
func TestIsEqualFallback(t *testing.T) {
	oldObj := &core_v1.ServiceAccount{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:            "test",
			Namespace:       "default",
			ResourceVersion: "123",
		},
		Secrets: []core_v1.ObjectReference{
			{
				Kind:      "Secret",
				Name:      "test",
				Namespace: "default",
			},
		},
	}

	newObj := oldObj.DeepCopy()

	// Any object (even unsupported types) with equal ResourceVersion should evaluate to true.
	got, err := IsObjectEqual(oldObj, newObj)
	require.NoError(t, err)
	assert.True(t, got)

	// Unsupported types with unequal ResourceVersion should return an error.
	newObj.ResourceVersion = "456"
	got, err = IsObjectEqual(oldObj, newObj)
	require.Error(t, err)
	assert.False(t, got)
}

func TestIsEqualForGeneration(t *testing.T) {
	run := func(t *testing.T, oldObj client.Object) {
		t.Helper()
		newObj := oldObj.DeepCopyObject().(client.Object)

		// Set different ResourceVersion to ensure that Generation is the only difference.
		oldObj.SetResourceVersion("123")
		newObj.SetResourceVersion("456")

		// Objects with equal Generation should evaluate to true.
		got, err := IsObjectEqual(oldObj, newObj)
		require.NoError(t, err)
		assert.True(t, got)

		// Objects with unequal Generation should evaluate to false.
		newObj.SetGeneration(oldObj.GetGeneration() + 1)
		got, err = IsObjectEqual(oldObj, newObj)
		require.NoError(t, err)
		assert.False(t, got)
	}

	run(t, &networking_v1.Ingress{})
	run(t, &contour_v1.HTTPProxy{})
	run(t, &contour_v1alpha1.ExtensionService{})
	run(t, &contour_v1.TLSCertificateDelegation{})
	run(t, &gatewayapi_v1.GatewayClass{})
	run(t, &gatewayapi_v1.Gateway{})
	run(t, &gatewayapi_v1.HTTPRoute{})
	run(t, &gatewayapi_v1alpha2.TLSRoute{})
	run(t, &gatewayapi_v1beta1.ReferenceGrant{})
	run(t, &gatewayapi_v1alpha2.GRPCRoute{})
	run(t, &gatewayapi_v1alpha2.TCPRoute{})
}

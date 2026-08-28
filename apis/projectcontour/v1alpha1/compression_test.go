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

package v1alpha1_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	contour_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
)

func TestValidateEnvoyCompressionAlgorithmType(t *testing.T) {
	require.Error(t, contour_v1alpha1.CompressionAlgorithm("foo").Validate())

	require.NoError(t, contour_v1alpha1.CompressionAlgorithm("").Validate())
	require.NoError(t, contour_v1alpha1.BrotliCompression.Validate())
	require.NoError(t, contour_v1alpha1.DisabledCompression.Validate())
	require.NoError(t, contour_v1alpha1.GzipCompression.Validate())
	require.NoError(t, contour_v1alpha1.ZstdCompression.Validate())
}

func TestValidateEnvoyCompression(t *testing.T) {
	require.NoError(t, (*contour_v1alpha1.EnvoyCompression)(nil).Validate())
	require.NoError(t, (&contour_v1alpha1.EnvoyCompression{}).Validate())
	require.NoError(t, (&contour_v1alpha1.EnvoyCompression{Algorithm: contour_v1alpha1.GzipCompression}).Validate())
	require.NoError(t, (&contour_v1alpha1.EnvoyCompression{
		Algorithms: []contour_v1alpha1.CompressionAlgorithm{
			contour_v1alpha1.BrotliCompression,
			contour_v1alpha1.GzipCompression,
		},
	}).Validate())

	require.ErrorContains(t, (&contour_v1alpha1.EnvoyCompression{Algorithm: "bogus"}).Validate(), "invalid compression type")
	require.ErrorContains(t, (&contour_v1alpha1.EnvoyCompression{
		Algorithm:  contour_v1alpha1.GzipCompression,
		Algorithms: []contour_v1alpha1.CompressionAlgorithm{contour_v1alpha1.BrotliCompression},
	}).Validate(), "mutually exclusive")
	require.ErrorContains(t, (&contour_v1alpha1.EnvoyCompression{
		Algorithms: []contour_v1alpha1.CompressionAlgorithm{contour_v1alpha1.DisabledCompression},
	}).Validate(), "cannot include")
	require.ErrorContains(t, (&contour_v1alpha1.EnvoyCompression{
		Algorithms: []contour_v1alpha1.CompressionAlgorithm{contour_v1alpha1.GzipCompression, contour_v1alpha1.GzipCompression},
	}).Validate(), "duplicate")
	require.ErrorContains(t, (&contour_v1alpha1.EnvoyCompression{
		Algorithms: []contour_v1alpha1.CompressionAlgorithm{""},
	}).Validate(), "empty value")
	require.ErrorContains(t, (&contour_v1alpha1.EnvoyCompression{
		Algorithms: []contour_v1alpha1.CompressionAlgorithm{"bogus"},
	}).Validate(), "invalid compression type")
}

func TestEffectiveAlgorithms(t *testing.T) {
	require.Equal(t, []contour_v1alpha1.CompressionAlgorithm{contour_v1alpha1.GzipCompression},
		(*contour_v1alpha1.EnvoyCompression)(nil).EffectiveAlgorithms())
	require.Equal(t, []contour_v1alpha1.CompressionAlgorithm{contour_v1alpha1.GzipCompression},
		(&contour_v1alpha1.EnvoyCompression{}).EffectiveAlgorithms())
	require.Nil(t, (&contour_v1alpha1.EnvoyCompression{Algorithm: contour_v1alpha1.DisabledCompression}).EffectiveAlgorithms())
	require.Equal(t, []contour_v1alpha1.CompressionAlgorithm{contour_v1alpha1.BrotliCompression},
		(&contour_v1alpha1.EnvoyCompression{Algorithm: contour_v1alpha1.BrotliCompression}).EffectiveAlgorithms())
	require.Equal(t, []contour_v1alpha1.CompressionAlgorithm{
		contour_v1alpha1.BrotliCompression,
		contour_v1alpha1.GzipCompression,
	}, (&contour_v1alpha1.EnvoyCompression{
		Algorithms: []contour_v1alpha1.CompressionAlgorithm{
			contour_v1alpha1.BrotliCompression,
			contour_v1alpha1.GzipCompression,
		},
	}).EffectiveAlgorithms())
	require.Equal(t, []contour_v1alpha1.CompressionAlgorithm{contour_v1alpha1.GzipCompression},
		(&contour_v1alpha1.EnvoyCompression{Algorithm: "bogus"}).EffectiveAlgorithms())
}

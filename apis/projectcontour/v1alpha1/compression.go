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

package v1alpha1

import "fmt"

// CompressionAlgorithm defines the type of compression algorithm applied in default HTTP listener filter chain.
// Allowable values are defined as names of well known compression algorithms (plus "disabled").
type CompressionAlgorithm string

// EnvoyCompression defines configuration related to compression in the default HTTP Listener filter chain.
type EnvoyCompression struct {
	// Algorithm selects the response compression type applied in the compression HTTP filter of the default Listener filters.
	// Values: `gzip` (default), `brotli`, `zstd`, `disabled`.
	// Setting this to `disabled` will make Envoy skip "Accept-Encoding: gzip,deflate" request header and always return uncompressed response.
	// +kubebuilder:validation:Enum="gzip";"brotli";"zstd";"disabled"
	// +optional
	Algorithm CompressionAlgorithm `json:"algorithm,omitempty"`
}

func (a CompressionAlgorithm) Validate() error {
	switch a {
	case BrotliCompression, DisabledCompression, GzipCompression, ZstdCompression, "":
		return nil
	default:
		return fmt.Errorf("invalid compression type: %q", a)
	}
}

const (
	// BrotliCompression specifies brotli as the default HTTP filter chain compression mechanism
	BrotliCompression CompressionAlgorithm = "brotli"

	// DisabledCompression specifies that there will be no compression in the default HTTP filter chain
	DisabledCompression CompressionAlgorithm = "disabled"

	// GzipCompression specifies gzip as the default HTTP filter chain compression mechanism
	GzipCompression CompressionAlgorithm = "gzip"

	// ZstdCompression specifies zstd as the default HTTP filter chain compression mechanism
	ZstdCompression CompressionAlgorithm = "zstd"
)

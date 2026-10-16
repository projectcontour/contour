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
// Allowable values are defined as names of well known compression algorithms.
type CompressionAlgorithm string

// EnvoyCompression defines configuration related to compression in the default HTTP Listener filter chain.
// +kubebuilder:validation:XValidation:rule="!(has(self.algorithm) && has(self.algorithms))",message="compression algorithm and algorithms are mutually exclusive"
type EnvoyCompression struct {
	// Algorithm selects a single response compression type applied in the compression HTTP filter of the default Listener filters.
	// Values: `gzip` (default), `brotli`, `zstd`, `disabled`.
	// Setting this to `disabled` will make Envoy skip "Accept-Encoding: gzip,deflate" request header and always return uncompressed response.
	// Mutually exclusive with Algorithms.
	//
	// Deprecated: use Algorithms instead. An empty Algorithms list disables compression.
	// +kubebuilder:validation:Enum="gzip";"brotli";"zstd";"disabled"
	// +optional
	Algorithm CompressionAlgorithm `json:"algorithm,omitempty"`

	// Algorithms selects the response compression types applied in the default HTTP Listener filter chain.
	// This field is a tri-state:
	// - omitted: use the Envoy default (gzip)
	// - empty list: disable compression
	// - populated list: install one compressor filter per entry and negotiate from Accept-Encoding
	//
	// List order is the preference when q-values are equal (the first entry is preferred).
	// Values: `gzip`, `brotli`, `zstd`. Cannot include `disabled`; use an empty list to disable.
	// Mutually exclusive with Algorithm.
	// +kubebuilder:validation:MaxItems=3
	// +kubebuilder:validation:items:Enum=gzip;brotli;zstd
	// +optional
	Algorithms *[]CompressionAlgorithm `json:"algorithms,omitempty"`
}

func (a CompressionAlgorithm) Validate() error {
	switch a {
	case BrotliCompression, DisabledCompression, GzipCompression, ZstdCompression, "":
		return nil
	default:
		return fmt.Errorf("invalid compression type: %q", a)
	}
}

// Validate reports whether the compression configuration is internally consistent.
func (c *EnvoyCompression) Validate() error {
	if c == nil {
		return nil
	}
	if err := c.Algorithm.Validate(); err != nil {
		return err
	}
	if c.Algorithm != "" && c.Algorithms != nil {
		return fmt.Errorf("compression algorithm and algorithms are mutually exclusive")
	}
	if c.Algorithms == nil {
		return nil
	}

	seen := make(map[CompressionAlgorithm]struct{}, len(*c.Algorithms))
	for _, algorithm := range *c.Algorithms {
		if algorithm == DisabledCompression {
			return fmt.Errorf("compression algorithms cannot include %q; set algorithms: [] to disable", DisabledCompression)
		}
		if err := algorithm.Validate(); err != nil {
			return err
		}
		if algorithm == "" {
			return fmt.Errorf("compression algorithms cannot include an empty value")
		}
		if _, ok := seen[algorithm]; ok {
			return fmt.Errorf("duplicate compression algorithm %q", algorithm)
		}
		seen[algorithm] = struct{}{}
	}
	return nil
}

// EffectiveAlgorithms returns the ordered list of compressor libraries to program
// on the default HTTP listener. A nil result means compression is disabled.
// When neither field is set, gzip is used. An empty Algorithms list disables compression.
func (c *EnvoyCompression) EffectiveAlgorithms() []CompressionAlgorithm {
	if c == nil {
		return []CompressionAlgorithm{GzipCompression}
	}
	if c.Algorithms != nil {
		if len(*c.Algorithms) == 0 {
			return nil
		}
		return append([]CompressionAlgorithm(nil), *c.Algorithms...)
	}
	if c.Algorithm == DisabledCompression {
		return nil
	}
	switch c.Algorithm {
	case BrotliCompression, ZstdCompression, GzipCompression:
		return []CompressionAlgorithm{c.Algorithm}
	default:
		// Unset or unknown values fall back to gzip, matching historical behavior.
		return []CompressionAlgorithm{GzipCompression}
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

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

type EnvoyCompressionType string

func (c EnvoyCompressionType) Validate() error {
	switch c {
	case BrotliCompression, DisabledCompression, GzipCompression, ZstdCompression:
		return nil
	default:
		return fmt.Errorf("invalid compression type: %q", c)
	}
}

const (
	// BrotliCompression specifies brotli as the default HTTP filter chain compression mechanism
	BrotliCompression EnvoyCompressionType = "brotli"

	// DisabledCompression specifies that there will be no compression in the default HTTP filter chain
	DisabledCompression EnvoyCompressionType = "disabled"

	// GzipCompression specifies gzip as the default HTTP filter chain compression mechanism
	GzipCompression EnvoyCompressionType = "gzip"

	// ZstdCompression specifies zstd as the default HTTP filter chain compression mechanism
	ZstdCompression EnvoyCompressionType = "zstd"
)

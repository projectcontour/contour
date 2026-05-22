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

package config

import (
	contour_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
)

// ECDHCurvesList holds a list of ECDH curves.
type ECDHCurvesList []string

// DefaultECDHCurves contains the default list of ECDH curves.
// When nil, Envoy uses its compiled-in defaults (X25519, P-256).
var DefaultECDHCurves = ECDHCurvesList(contour_v1alpha1.DefaultECDHCurves)

// ValidECDHCurves contains the set of ECDH curves that Envoy supports.
var ValidECDHCurves = contour_v1alpha1.ValidECDHCurves

// Validate ECDH curves. Returns error on unsupported curve.
func (curves ECDHCurvesList) Validate() error {
	e := &contour_v1alpha1.EnvoyTLS{
		ECDHCurves: curves,
	}
	return e.Validate()
}

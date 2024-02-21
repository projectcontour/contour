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

package v3

import (
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
)

// nolint:revive
const CONSTANT_HASH_VALUE = "contour"

// ConstantHash is a specialized node ID hasher used to allow
// any instance of Envoy to connect to Contour regardless of the
// service-node flag configured on Envoy.
type ConstantHash struct{}

func (c ConstantHash) ID(*envoy_config_core_v3.Node) string {
	return CONSTANT_HASH_VALUE
}

func (c ConstantHash) String() string {
	return CONSTANT_HASH_VALUE
}

var Hash = ConstantHash{}

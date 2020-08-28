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

package xds

import (
	envoy_api_v2_core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	cache "github.com/envoyproxy/go-control-plane/pkg/cache/v2"
)

// ConstantHash is a specialized node ID hasher used to allow
// any instance of Envoy to connect to Contour regardless of the
// service-node flag configured on Envoy.
type ConstantHash string

func (c ConstantHash) ID(*envoy_api_v2_core.Node) string {
	return string(c)
}

func (c ConstantHash) String() string {
	return string(c)
}

var _ cache.NodeHash = ConstantHash("")

var DefaultHash = ConstantHash("contour")

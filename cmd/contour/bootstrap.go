// Copyright © 2018 Heptio
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

package main

import (
	"os"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/heptio/contour/internal/envoy"
)

// writeBootstrapConfig writes a bootstrap configuration to the supplied path.
// in v2 JSON format.
func writeBootstrapConfig(config *envoy.BootstrapConfig, path string) {
	f, err := os.Create(path)
	check(err)
	bs := envoy.Bootstrap(config)
	m := &jsonpb.Marshaler{OrigName: true}
	err = m.Marshal(f, bs)
	check(err)
	check(f.Close())
}

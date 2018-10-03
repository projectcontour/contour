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
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type configWriter interface {
	WriteYAML(io.Writer) error
}

// writeBootstrapConfig writes a bootstrap configuration to the supplied path.
// If the path ends in .yaml, the configuration file will be in v2 YAML format.
func writeBootstrapConfig(config configWriter, path string) {
	switch filepath.Ext(path) {
	case ".yaml":
		f, err := os.Create(path)
		check(err)
		err = config.WriteYAML(f)
		check(err)
		check(f.Close())
	default:
		check(fmt.Errorf("path %s must end in .yaml", path))
	}
}

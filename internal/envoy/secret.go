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

package envoy

import (
	"crypto/sha1" // nolint:gosec
	"fmt"

	"github.com/projectcontour/contour/internal/dag"
)

// Secretname returns the name of the SDS secret for this secret.
func Secretname(s *dag.Secret) string {
	// This isn't a crypto hash, we just want a unique name.
	hash := sha1.Sum(s.Cert()) // nolint:gosec
	ns := s.Namespace()
	name := s.Name()
	return Hashname(120, ns, name, fmt.Sprintf("%x", hash[:5]))
}

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

import (
	"fmt"
)

// ClusterDNSFamilyType is the Ip family to use for resolving DNS
// names in an Envoy cluster config.
type ClusterDNSFamilyType string

func (c ClusterDNSFamilyType) Validate() error {
	switch c {
	case AutoClusterDNSFamily, IPv4ClusterDNSFamily, IPv6ClusterDNSFamily:
		return nil
	default:
		return fmt.Errorf("invalid cluster DNS lookup family %q", c)
	}
}

const AutoClusterDNSFamily ClusterDNSFamilyType = "auto"
const IPv4ClusterDNSFamily ClusterDNSFamilyType = "v4"
const IPv6ClusterDNSFamily ClusterDNSFamilyType = "v6"

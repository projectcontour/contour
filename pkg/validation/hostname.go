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

package validation

import (
	"fmt"
	"net"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation"
)

// Hostname validates s is not an IP address and that s conforms to the definition of
// a subdomain in DNS (RFC 1123) or wildcard subdomain in DNS (RFC 1034 section 4.3.3).
func Hostname(s string) error {
	if isIP := net.ParseIP(s) != nil; isIP {
		return fmt.Errorf("hostname %q must be a DNS name, not an IP address", s)
	}
	if strings.Contains(s, "*") {
		if err := validation.IsWildcardDNS1123Subdomain(s); err != nil {
			return fmt.Errorf("hostname %q must be a DNS name: %v", s, err)
		}
	} else {
		if err := validation.IsDNS1123Subdomain(s); err != nil {
			return fmt.Errorf("hostname %q must be a DNS name: %v", s, err)
		}
	}
	return nil
}

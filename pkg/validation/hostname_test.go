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
	"testing"
)

func TestHostname(t *testing.T) {
	testCases := []struct {
		description string
		hostname    string
		expected    bool
	}{
		{
			description: "fqdn hostname",
			hostname:    "www.example.com",
			expected:    true,
		},
		{
			description: "subdomain hostname",
			hostname:    "example.com",
			expected:    true,
		},
		{
			description: "wildcard subdomain hostname",
			hostname:    "*.example.com",
			expected:    true,
		},
		{
			description: "subdomain with a port number hostname",
			hostname:    "example.com:8080",
			expected:    false,
		},
		{
			description: "IPv4 address hostname",
			hostname:    "1.2.3.4",
			expected:    false,
		},
		{
			description: "IPv4 invalid address hostname",
			hostname:    "1.2.3..4",
			expected:    false,
		},
		{
			description: "IPv4 address and port hostname",
			hostname:    "1.2.3.4:8080",
			expected:    false,
		},
		{
			description: "IPv6 address hostname",
			hostname:    "2001:db8::68",
			expected:    false,
		},
		{
			description: "IPv6 link-local address hostname",
			hostname:    "fe80::/10",
			expected:    false,
		},
		{
			description: "wildcard hostname",
			hostname:    "*",
			expected:    false,
		},
		{
			description: "empty string hostname",
			hostname:    "",
			expected:    false,
		},
		{
			description: "subdomain with multiple wildcard labels hostname",
			hostname:    "*.*.com",
			expected:    false,
		},
		{
			description: "subdomain with wildcard as root label hostname",
			hostname:    "www.foo.*",
			expected:    false,
		},
		{
			description: "subdomain with invalid wildcard label hostname",
			hostname:    "foo.*.com",
			expected:    false,
		},
	}

	for _, tc := range testCases {
		err := Hostname(tc.hostname)
		if err != nil && tc.expected {
			t.Fatalf("%q: failed with error: %#v", tc.description, err)
		}
		if err == nil && !tc.expected {
			t.Fatalf("%q: expected to fail but received no error", tc.description)
		}
	}
}

// Copyright © 2017 Heptio
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

package contour

import (
	"github.com/heptio/contour/internal/envoy"
	"github.com/pkg/errors"
	"k8s.io/api/core/v1"
)

// EndpointsToSDSHosts translates a *v1.Endpoints document to []*envoy.SDSHost.
func EndpointsToSDSHosts(e *v1.Endpoints, port int) ([]*envoy.SDSHost, error) {
	if err := validateEndpoints(e); err != nil {
		return nil, err
	}
	var hosts []*envoy.SDSHost
	for _, s := range e.Subsets {
		// skip any subsets that don't ahve ready addresses or ports
		if len(s.Addresses) == 0 || len(s.Ports) == 0 {
			continue
		}

		for _, p := range s.Ports {
			// TODO(dfc) handle s.Name when s.Port is not specified
			if int(p.Port) != port {
				continue
			}
			for _, a := range s.Addresses {
				hosts = append(hosts, &envoy.SDSHost{
					IPAddress: a.IP,
					Port:      port,
				})
			}
		}
	}

	return hosts, nil
}

// validateEndpoints asserts that the required fields in e are present.
// Fields which are required for conversion must be present or an error is returned.
// For the fields that are converted, if Envoy places a limit on their contents or length,
// and error is returned if those fields are invalid.
// Many fields in *v1.Endpoints which are not needed for conversion and are ignored.
func validateEndpoints(e *v1.Endpoints) error {
	if e.ObjectMeta.Name == "" {
		return errors.New("Endpoints.Meta.Name is blank")
	}
	if e.ObjectMeta.Namespace == "" {
		return errors.New("Endpoints.Meta.Namespace is blank")
	}
	return nil
}

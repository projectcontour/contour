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
	"fmt"

	"github.com/heptio/contour/internal/envoy"
	"github.com/pkg/errors"
	"k8s.io/client-go/pkg/api/v1"
)

// ServiceToClusters translates a *v1.Service document to a []envoy.Cluster.
func ServiceToClusters(s *v1.Service) ([]envoy.Cluster, error) {
	if err := validateService(s); err != nil {
		return nil, err
	}

	// The translation logic is as follows:
	// For each Service.Spec.Port create a unique Cluster.
	// The Service Namespace and Name is used as the Cluster service_name
	// which is passed to SDS to discover the Endpoints for this Service.
	// The remaining non optional parameters like connect_timeout_ms and
	// lb_type are hard coded at the moment.
	// TLS is not supported.

	const (
		defaultConnectTimeoutMs = 250
		defaultLBType           = "round_robin"
	)

	clusters := make([]envoy.Cluster, 0, len(s.Spec.Ports))
	for _, p := range s.Spec.Ports {
		switch p.Protocol {
		case "TCP":
			if p.Name != "" {
				// service port is named, so we must generate both a cluster for the port name
				// and a cluster for the port number.
				clusters = append(clusters, envoy.Cluster{
					Name:             fmt.Sprintf("%s/%s/%s", s.ObjectMeta.Namespace, s.ObjectMeta.Name, p.Name),
					Type:             "sds", // hard coded to specify SDS Endpoint discovery
					ConnectTimeoutMs: defaultConnectTimeoutMs,
					LBType:           defaultLBType,
					// TODO(dfc) need check if targetport is missing.
					ServiceName: fmt.Sprintf("%s/%s/%s", s.ObjectMeta.Namespace, s.ObjectMeta.Name, p.TargetPort.String()),
				})
			}
			clusters = append(clusters, envoy.Cluster{
				Name:             fmt.Sprintf("%s/%s/%d", s.ObjectMeta.Namespace, s.ObjectMeta.Name, p.Port),
				Type:             "sds", // hard coded to specify SDS Endpoint discovery
				ConnectTimeoutMs: defaultConnectTimeoutMs,
				LBType:           defaultLBType,
				// TODO(dfc) need check if targetport is missing.
				ServiceName: fmt.Sprintf("%s/%s/%s", s.ObjectMeta.Namespace, s.ObjectMeta.Name, p.TargetPort.String()),
			})
		default:
			// ignore UDP and other port types.
		}
	}
	if len(clusters) < 1 {
		return nil, errors.Errorf("service %s/%s: no usable ServicePorts", s.ObjectMeta.Namespace, s.ObjectMeta.Name)
	}
	return clusters, nil
}

// validateService asserts that the required fields in s are present.
// Fields which are required for conversion must be present or an error is returned.
// For the fields that are converted, if Envoy place a limit on their contents or length,
// and error is returned if those fields are invalid.
// Many fields in *v1.Service which are not needed for conversion and are ignored.
func validateService(s *v1.Service) error {
	if s.ObjectMeta.Name == "" {
		return errors.New("Service.Meta.Name is blank")
	}
	if s.ObjectMeta.Namespace == "" {
		return errors.New("Service.Meta.Namespace is blank")
	}
	return nil
}

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

	"github.com/projectcontour/contour/pkg/config"
)

// Validate configuration that is not already covered by CRD validation.
func (c *ContourConfigurationSpec) Validate() error {
	// Validation of root configuration fields.
	if err := endpointsInConfict(c.Health, c.Metrics); err != nil {
		return fmt.Errorf("invalid contour configuration: %v", err)
	}

	// Validation of nested configuration structs.
	var validateFuncs []func() error

	if c.XDSServer != nil {
		validateFuncs = append(validateFuncs, c.XDSServer.Type.Validate)
	}
	if c.Envoy != nil {
		validateFuncs = append(validateFuncs, c.Envoy.Validate)
	}
	if c.Gateway != nil {
		validateFuncs = append(validateFuncs, c.Gateway.Validate)
	}

	for _, validate := range validateFuncs {
		if err := validate(); err != nil {
			return err
		}
	}

	return nil
}

func (x XDSServerType) Validate() error {
	switch x {
	case ContourServerType, EnvoyServerType:
		return nil
	default:
		return fmt.Errorf("invalid xDS server type %q", x)
	}
}

func (l LogLevel) Validate() error {
	switch l {
	case InfoLog, DebugLog:
		return nil
	default:
		return fmt.Errorf("invalid log level %q", l)
	}
}

func (d ClusterDNSFamilyType) Validate() error {
	switch d {
	case AutoClusterDNSFamily, IPv4ClusterDNSFamily, IPv6ClusterDNSFamily:
		return nil
	default:
		return fmt.Errorf("invalid cluster dns family type %q", d)
	}
}

// Validate configuration that cannot be handled with CRD validation.
func (e *EnvoyConfig) Validate() error {
	if err := endpointsInConfict(e.Health, e.Metrics); err != nil {
		return fmt.Errorf("invalid envoy configuration: %v", err)
	}

	if err := e.Logging.Validate(); err != nil {
		return err
	}

	// DefaultHTTPVersions
	var invalidHTTPVersions []string
	for _, v := range e.DefaultHTTPVersions {
		switch v {
		case HTTPVersion1, HTTPVersion2:
			continue
		default:
			invalidHTTPVersions = append(invalidHTTPVersions, string(v))
		}
	}
	if len(invalidHTTPVersions) > 0 {
		return fmt.Errorf("invalid HTTP versions %q", invalidHTTPVersions)
	}

	// Cluster.DNSLookupFamily
	if e.Cluster != nil {
		if err := e.Cluster.DNSLookupFamily.Validate(); err != nil {
			return err
		}
	}

	// CipherSuites
	if e.Listener != nil && e.Listener.TLS != nil {
		var invalidCipherSuites []string
		for _, c := range e.Listener.TLS.CipherSuites {
			if _, ok := config.ValidTLSCiphers[string(c)]; !ok {
				invalidCipherSuites = append(invalidCipherSuites, string(c))
			}
		}
		if len(invalidCipherSuites) > 0 {
			return fmt.Errorf("invalid cipher suites %q", invalidCipherSuites)
		}
	}

	return nil
}

// Validate ensures that exactly one of ControllerName or GatewayRef are specified.
func (g *GatewayConfig) Validate() error {
	if g == nil {
		return nil
	}

	if len(g.ControllerName) > 0 && g.GatewayRef != nil {
		return fmt.Errorf("invalid gateway configuration: exactly one of controller name or gateway ref must be specified")
	}

	if len(g.ControllerName) == 0 && g.GatewayRef == nil {
		return fmt.Errorf("invalid gateway configuration: exactly one of controller name or gateway ref must be specified")
	}

	return nil
}

func (e *EnvoyLogging) Validate() error {
	if e == nil {
		return nil
	}

	if err := e.AccessLogFormat.Validate(); err != nil {
		return err
	}
	if err := e.AccessLogJSONFields.Validate(); err != nil {
		return err
	}
	return validateAccessLogFormatString(e.AccessLogFormatString)
}

// endpointsInConfict returns error if different protocol are configured to use single port.
func endpointsInConfict(health *HealthConfig, metrics *MetricsConfig) error {
	if health != nil && metrics != nil && metrics.TLS != nil && health.Address == metrics.Address && health.Port == metrics.Port {
		return fmt.Errorf("cannot use single port for health over HTTP and metrics over HTTPS")
	}
	return nil
}

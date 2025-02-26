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
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/util/sets"
)

const featureFlagUseEndpointSlices string = "useEndpointSlices"

var featureFlagsMap = map[string]struct{}{
	featureFlagUseEndpointSlices: {},
}

// Validate configuration that is not already covered by CRD validation.
func (c *ContourConfigurationSpec) Validate() error {
	// Validation of root configuration fields.
	if err := endpointsInConfict(c.Health, c.Metrics); err != nil {
		return fmt.Errorf("invalid contour configuration: %v", err)
	}

	// Validation of nested configuration structs.
	var validateFuncs []func() error

	if c.Envoy != nil {
		validateFuncs = append(validateFuncs, c.Envoy.Validate)
	}
	if c.Gateway != nil {
		validateFuncs = append(validateFuncs, c.Gateway.Validate)
	}
	if c.Tracing != nil {
		validateFuncs = append(validateFuncs, c.Tracing.Validate)
	}

	for _, validate := range validateFuncs {
		if err := validate(); err != nil {
			return err
		}
	}

	return nil
}

func (t *TracingConfig) Validate() error {
	if t.ExtensionService == nil {
		return fmt.Errorf("tracing.extensionService must be defined")
	}

	if t.OverallSampling != nil {
		_, err := strconv.ParseFloat(*t.OverallSampling, 64)
		if err != nil {
			return fmt.Errorf("invalid tracing sampling: %v", err)
		}
	}

	var customTagNames []string

	for _, customTag := range t.CustomTags {
		var fieldCount int
		if customTag.TagName == "" {
			return fmt.Errorf("tracing.customTag.tagName must be defined")
		}

		for _, customTagName := range customTagNames {
			if customTagName == customTag.TagName {
				return fmt.Errorf("tagName %s is duplicate", customTagName)
			}
		}

		if customTag.Literal != "" {
			fieldCount++
		}

		if customTag.RequestHeaderName != "" {
			fieldCount++
		}
		if fieldCount != 1 {
			return fmt.Errorf("must set exactly one of Literal or RequestHeaderName")
		}
		customTagNames = append(customTagNames, customTag.TagName)
	}
	return nil
}

func (d ClusterDNSFamilyType) Validate() error {
	switch d {
	case AutoClusterDNSFamily, IPv4ClusterDNSFamily, IPv6ClusterDNSFamily, AllClusterDNSFamily:
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

	// Envoy TLS configuration
	if e.Listener != nil && e.Listener.TLS != nil {
		return e.Listener.TLS.Validate()
	}

	return nil
}

func ValidateTLSProtocolVersions(minVersion, maxVersion string) error {
	parseVersion := func(version, tip, defVal string) (string, error) {
		switch version {
		case "":
			return defVal, nil
		case "1.2", "1.3":
			return version, nil
		default:
			return "", fmt.Errorf("invalid TLS %s protocol version: %q", tip, version)
		}
	}

	minVer, err := parseVersion(minVersion, "minimum", "1.2")
	if err != nil {
		return err
	}

	maxVer, err := parseVersion(maxVersion, "maximum", "1.3")
	if err != nil {
		return err
	}
	if maxVer < minVer {
		return fmt.Errorf("the minimum TLS protocol version is greater than the maximum TLS protocol version")
	}

	return nil
}

// isValidTLSCipher parses a cipher string and returns true if it is valid.
// We do not support the full syntax defined in the BoringSSL documentation,
// see https://commondatastorage.googleapis.com/chromium-boringssl-docs/ssl.h.html#Cipher-suite-configuration
func isValidTLSCipher(cipherSpec string) bool {
	// Equal-preference group: [cipher1|cipher2|...]
	if strings.HasPrefix(cipherSpec, "[") && strings.HasSuffix(cipherSpec, "]") {
		for _, cipher := range strings.Split(strings.Trim(cipherSpec, "[]"), "|") {
			if _, ok := ValidTLSCiphers[cipher]; !ok {
				return false
			}
		}
		return true
	}

	if _, ok := ValidTLSCiphers[cipherSpec]; !ok {
		return false
	}

	return true
}

// Validate ensures EnvoyTLS configuration is valid.
func (e *EnvoyTLS) Validate() error {
	if err := ValidateTLSProtocolVersions(e.MinimumProtocolVersion, e.MaximumProtocolVersion); err != nil {
		return err
	}

	var invalidCipherSuites []string
	for _, c := range e.CipherSuites {
		if !isValidTLSCipher(c) {
			invalidCipherSuites = append(invalidCipherSuites, c)
		}
	}
	if len(invalidCipherSuites) > 0 {
		return fmt.Errorf("invalid cipher suites %q", invalidCipherSuites)
	}
	return nil
}

// SanitizedCipherSuites returns a deduplicated list of TLS ciphers.
// Order is maintained.
func (e *EnvoyTLS) SanitizedCipherSuites() []string {
	if len(e.CipherSuites) == 0 {
		return DefaultTLSCiphers
	}

	uniqueCiphers := sets.NewString()
	// We also use a list so we can maintain the order.
	validatedCiphers := []string{}
	for _, c := range e.CipherSuites {
		if !uniqueCiphers.Has(c) {
			uniqueCiphers.Insert(c)
			validatedCiphers = append(validatedCiphers, c)
		}
	}
	return validatedCiphers
}

func (f FeatureFlags) Validate() error {
	for _, featureFlag := range f {
		fields := strings.Split(featureFlag, "=")
		if _, found := featureFlagsMap[fields[0]]; !found {
			return fmt.Errorf("invalid contour configuration, unknown feature flag:%s", featureFlag)
		}
	}
	return nil
}

func (f FeatureFlags) IsEndpointSliceEnabled() bool {
	// only when the flag: 'useEndpointSlices=false' is exists, return false
	for _, flag := range f {
		if !strings.HasPrefix(flag, featureFlagUseEndpointSlices) {
			continue
		}
		fields := strings.Split(flag, "=")
		if len(fields) == 2 && strings.ToLower(fields[1]) == "false" {
			return false
		}
	}
	return true
}

// Validate ensures that GatewayRef namespace/name is specified.
func (g *GatewayConfig) Validate() error {
	if g != nil && (g.GatewayRef.Namespace == "" || g.GatewayRef.Name == "") {
		return fmt.Errorf("invalid gateway configuration: gateway ref namespace and name must be specified")
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
	return AccessLogFormatString(e.AccessLogFormatString).Validate()
}

// AccessLogFormatterExtensions returns a list of formatter extension names required by the access log format.
//
// Note: When adding support for new formatter, update the list of extensions here and
// add corresponding configuration in internal/envoy/v3/accesslog.go extensionConfig().
// Currently only one extension exist in Envoy.
func (e *EnvoyLogging) AccessLogFormatterExtensions() []string {
	// Function that finds out if command operator is present in a format string.
	contains := func(format, command string) bool {
		tokens := commandOperatorRegexp.FindAllStringSubmatch(format, -1)
		for _, t := range tokens {
			if t[2] == command {
				return true
			}
		}
		return false
	}

	extensionsMap := make(map[string]bool)
	switch e.AccessLogFormat {
	case EnvoyAccessLog:
		if contains(e.AccessLogFormatString, "REQ_WITHOUT_QUERY") {
			extensionsMap["envoy.formatter.req_without_query"] = true
		}
		if contains(e.AccessLogFormatString, "METADATA") {
			extensionsMap["envoy.formatter.metadata"] = true
		}
	case JSONAccessLog:
		for _, f := range e.AccessLogJSONFields.AsFieldMap() {
			if contains(f, "REQ_WITHOUT_QUERY") {
				extensionsMap["envoy.formatter.req_without_query"] = true
			}
			if contains(f, "METADATA") {
				extensionsMap["envoy.formatter.metadata"] = true
			}
		}
	}

	var extensions []string
	for k := range extensionsMap {
		extensions = append(extensions, k)
	}

	return extensions
}

// endpointsInConfict returns error if different protocol are configured to use single port.
func endpointsInConfict(health *HealthConfig, metrics *MetricsConfig) error {
	if health != nil && metrics != nil && metrics.TLS != nil && health.Address == metrics.Address && health.Port == metrics.Port {
		return fmt.Errorf("cannot use single port for health over HTTP and metrics over HTTPS")
	}
	return nil
}

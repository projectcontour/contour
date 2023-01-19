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

package contourconfig

import (
	"fmt"
	"time"

	"github.com/imdario/mergo"
	contour_api_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/internal/ref"
	"github.com/projectcontour/contour/internal/timeout"
)

// UIntPtr returns a pointer to a uint value.
func UIntPtr(val uint) *uint {
	return &val
}

// UInt32Ptr returns a pointer to a uint32 value.
func UInt32Ptr(val uint32) *uint32 {
	return &val
}

// OverlayOnDefaults overlays the settings in the provided spec onto the
// default settings, and returns the results.
func OverlayOnDefaults(spec contour_api_v1alpha1.ContourConfigurationSpec) (contour_api_v1alpha1.ContourConfigurationSpec, error) {
	res := Defaults()

	if err := mergo.Merge(&res, spec, mergo.WithOverride); err != nil {
		return contour_api_v1alpha1.ContourConfigurationSpec{}, err
	}

	return res, nil
}

// Defaults returns the default settings Contour uses if no user-specified
// configuration is provided.
func Defaults() contour_api_v1alpha1.ContourConfigurationSpec {
	return contour_api_v1alpha1.ContourConfigurationSpec{
		XDSServer: &contour_api_v1alpha1.XDSServerConfig{
			Type:    contour_api_v1alpha1.ContourServerType,
			Address: "0.0.0.0",
			Port:    8001,
			TLS: &contour_api_v1alpha1.TLS{
				CAFile:   "/certs/ca.crt",
				CertFile: "/certs/tls.crt",
				KeyFile:  "/certs/tls.key",
				Insecure: ref.To(false),
			},
		},
		Ingress: &contour_api_v1alpha1.IngressConfig{
			ClassNames:    nil,
			StatusAddress: "",
		},
		Debug: &contour_api_v1alpha1.DebugConfig{
			Address: "127.0.0.1",
			Port:    6060,
		},
		Health: &contour_api_v1alpha1.HealthConfig{
			Address: "0.0.0.0",
			Port:    8000,
		},
		Envoy: &contour_api_v1alpha1.EnvoyConfig{
			Listener: &contour_api_v1alpha1.EnvoyListenerConfig{
				UseProxyProto:              ref.To(false),
				DisableAllowChunkedLength:  ref.To(false),
				DisableMergeSlashes:        ref.To(false),
				ServerHeaderTransformation: contour_api_v1alpha1.OVERWRITE,
				ConnectionBalancer:         "",
				TLS: &contour_api_v1alpha1.EnvoyTLS{
					MinimumProtocolVersion: "1.2",
					CipherSuites:           contour_api_v1alpha1.DefaultTLSCiphers,
				},
			},
			Service: &contour_api_v1alpha1.NamespacedName{
				Namespace: "projectcontour",
				Name:      "envoy",
			},
			HTTPListener: &contour_api_v1alpha1.EnvoyListener{
				Address:   "0.0.0.0",
				Port:      8080,
				AccessLog: "/dev/stdout",
			},
			HTTPSListener: &contour_api_v1alpha1.EnvoyListener{
				Address:   "0.0.0.0",
				Port:      8443,
				AccessLog: "/dev/stdout",
			},
			Health: &contour_api_v1alpha1.HealthConfig{
				Address: "0.0.0.0",
				Port:    8002,
			},
			Metrics: &contour_api_v1alpha1.MetricsConfig{
				Address: "0.0.0.0",
				Port:    8002,
				TLS:     nil,
			},
			ClientCertificate: nil,
			Logging: &contour_api_v1alpha1.EnvoyLogging{
				AccessLogFormat:       contour_api_v1alpha1.EnvoyAccessLog,
				AccessLogFormatString: "",
				AccessLogJSONFields:   nil,
				AccessLogLevel:        contour_api_v1alpha1.LogLevelInfo,
			},
			DefaultHTTPVersions: []contour_api_v1alpha1.HTTPVersionType{
				"HTTP/1.1",
				"HTTP/2",
			},
			Timeouts: &contour_api_v1alpha1.TimeoutParameters{
				RequestTimeout:                nil,
				ConnectionIdleTimeout:         nil,
				StreamIdleTimeout:             nil,
				MaxConnectionDuration:         nil,
				DelayedCloseTimeout:           nil,
				ConnectionShutdownGracePeriod: nil,
				ConnectTimeout:                nil,
			},
			Cluster: &contour_api_v1alpha1.ClusterParameters{
				DNSLookupFamily: contour_api_v1alpha1.AutoClusterDNSFamily,
			},
			Network: &contour_api_v1alpha1.NetworkParameters{
				XffNumTrustedHops: UInt32Ptr(0),
				EnvoyAdminPort:    ref.To(9001),
			},
		},
		Gateway: nil,
		HTTPProxy: &contour_api_v1alpha1.HTTPProxyConfig{
			DisablePermitInsecure: ref.To(false),
			RootNamespaces:        nil,
			FallbackCertificate:   nil,
		},
		EnableExternalNameService: ref.To(false),
		RateLimitService:          nil,
		Policy: &contour_api_v1alpha1.PolicyConfig{
			RequestHeadersPolicy:  &contour_api_v1alpha1.HeadersPolicy{},
			ResponseHeadersPolicy: &contour_api_v1alpha1.HeadersPolicy{},
			ApplyToIngress:        ref.To(false),
		},
		Metrics: &contour_api_v1alpha1.MetricsConfig{
			Address: "0.0.0.0",
			Port:    8000,
			TLS:     nil,
		},
	}
}

type Timeouts struct {
	Request                       timeout.Setting
	ConnectionIdle                timeout.Setting
	StreamIdle                    timeout.Setting
	MaxConnectionDuration         timeout.Setting
	DelayedClose                  timeout.Setting
	ConnectionShutdownGracePeriod timeout.Setting
	ConnectTimeout                time.Duration // Since "infinite" is not valid ConnectTimeout value, use time.Duration instead of timeout.Setting.
}

func ParseTimeoutPolicy(timeoutParameters *contour_api_v1alpha1.TimeoutParameters) (Timeouts, error) {
	var (
		err      error
		timeouts Timeouts
	)

	if timeoutParameters == nil {
		return timeouts, nil
	}

	if timeoutParameters.RequestTimeout != nil {
		timeouts.Request, err = timeout.Parse(*timeoutParameters.RequestTimeout)
		if err != nil {
			return Timeouts{}, fmt.Errorf("failed to parse request timeout: %s", err)
		}
	}
	if timeoutParameters.ConnectionIdleTimeout != nil {
		timeouts.ConnectionIdle, err = timeout.Parse(*timeoutParameters.ConnectionIdleTimeout)
		if err != nil {
			return Timeouts{}, fmt.Errorf("failed to parse connection idle timeout: %s", err)
		}
	}
	if timeoutParameters.StreamIdleTimeout != nil {
		timeouts.StreamIdle, err = timeout.Parse(*timeoutParameters.StreamIdleTimeout)
		if err != nil {
			return Timeouts{}, fmt.Errorf("failed to parse stream idle timeout: %s", err)
		}
	}
	if timeoutParameters.MaxConnectionDuration != nil {
		timeouts.MaxConnectionDuration, err = timeout.Parse(*timeoutParameters.MaxConnectionDuration)
		if err != nil {
			return Timeouts{}, fmt.Errorf("failed to parse max connection duration: %s", err)
		}
	}
	if timeoutParameters.DelayedCloseTimeout != nil {
		timeouts.DelayedClose, err = timeout.Parse(*timeoutParameters.DelayedCloseTimeout)
		if err != nil {
			return Timeouts{}, fmt.Errorf("failed to parse delayed close timeout: %s", err)
		}
	}
	if timeoutParameters.ConnectionShutdownGracePeriod != nil {
		timeouts.ConnectionShutdownGracePeriod, err = timeout.Parse(*timeoutParameters.ConnectionShutdownGracePeriod)
		if err != nil {
			return Timeouts{}, fmt.Errorf("failed to parse connection shutdown grace period: %s", err)
		}
	}
	if timeoutParameters.ConnectTimeout != nil {
		timeouts.ConnectTimeout, err = time.ParseDuration(*timeoutParameters.ConnectTimeout)
		if err != nil {
			return Timeouts{}, fmt.Errorf("failed to parse connect timeout: %s", err)
		}
	}

	return timeouts, nil
}

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

package configmap

import (
	"testing"

	"github.com/projectcontour/contour/internal/provisioner/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
)

func TestDesiredContourConfigmap(t *testing.T) {
	expected := `#
# server:
#   determine which XDS Server implementation to utilize in Contour.
#   xds-server-type: contour
#
# Specify the Gateway API configuration.
gateway:
  gatewayRef:
    namespace: test-ns
    name: test
#
# should contour expect to be running inside a k8s cluster
# incluster: true
#
# path to kubeconfig (if not running inside a k8s cluster)
# kubeconfig: /path/to/.kube/config
#
# Disable RFC-compliant behavior to strip "Content-Length" header if
# "Tranfer-Encoding: chunked" is also set.
# disableAllowChunkedLength: false
# Disable HTTPProxy permitInsecure field
disablePermitInsecure: false
tls:
# minimum TLS version that Contour will negotiate
# minimum-protocol-version: "1.2"
# TLS ciphers to be supported by Envoy TLS listeners when negotiating
# TLS 1.2.
# cipher-suites:
# - '[ECDHE-ECDSA-AES128-GCM-SHA256|ECDHE-ECDSA-CHACHA20-POLY1305]'
# - '[ECDHE-RSA-AES128-GCM-SHA256|ECDHE-RSA-CHACHA20-POLY1305]'
# - 'ECDHE-ECDSA-AES256-GCM-SHA384'
# - 'ECDHE-RSA-AES256-GCM-SHA384'
# Defines the Kubernetes name/namespace matching a secret to use
# as the fallback certificate when requests which don't match the
# SNI defined for a vhost.
  fallback-certificate:
#   name: fallback-secret-name
#   namespace: projectcontour
  envoy-client-certificate:
#   name: envoy-client-cert-secret-name
#   namespace: projectcontour
# The following config shows the defaults for the leader election.
# leaderelection:
#   configmap-name: leader-elect
#   configmap-namespace: projectcontour
####
# ExternalName Services are disabled by default due to CVE-2021-XXXXX
# You can re-enable them by setting this setting to "true".
# This is not recommended without understanding the security implications.
# Please see the advisory at https://github.com/projectcontour/contour/security/advisories/GHSA-5ph6-qq5x-7jwc for the details.
# enableExternalNameService: false
##
### Logging options
# Default setting
accesslog-format: envoy
# To enable JSON logging in Envoy
# accesslog-format: json
# The default fields that will be logged are specified below.
# To customize this list, just add or remove entries.
# The canonical list is available at
# https://godoc.org/github.com/projectcontour/contour/internal/envoy#JSONFields
# json-fields:
#   - "@timestamp"
#   - "authority"
#   - "bytes_received"
#   - "bytes_sent"
#   - "downstream_local_address"
#   - "downstream_remote_address"
#   - "duration"
#   - "method"
#   - "path"
#   - "protocol"
#   - "request_id"
#   - "requested_server_name"
#   - "response_code"
#   - "response_flags"
#   - "uber_trace_id"
#   - "upstream_cluster"
#   - "upstream_host"
#   - "upstream_local_address"
#   - "upstream_service_time"
#   - "user_agent"
#   - "x_forwarded_for"
#
# default-http-versions:
# - "HTTP/2"
# - "HTTP/1.1"
#
# The following shows the default proxy timeout settings.
# timeouts:
#   request-timeout: infinity
#   connection-idle-timeout: 60s
#   stream-idle-timeout: 5m
#   max-connection-duration: infinity
#   delayed-close-timeout: 1s
#   connection-shutdown-grace-period: 5s
#
# Envoy cluster settings.
# cluster:
#   configure the cluster dns lookup family
#   valid options are: auto (default), v4, v6
#   dns-lookup-family: auto
#
# Envoy network settings.
# network:
#   Configure the number of additional ingress proxy hops from the
#   right side of the x-forwarded-for HTTP header to trust.
#   num-trusted-hops: 0
`

	c := &model.Contour{
		ObjectMeta: v1.ObjectMeta{
			Name:      "test",
			Namespace: "test-ns",
		},
	}
	cm, err := desired(configForContour(c))
	require.NoError(t, err)
	require.Equal(t, "contour", cm.Name)
	require.Equal(t, "test-ns", cm.Namespace)
	require.Contains(t, cm.Data, "contour.yaml")
	assert.Equal(t, expected, cm.Data["contour.yaml"])
}

func TestDesiredConfigmapWithConfigurations(t *testing.T) {
	expected := `#
# server:
#   determine which XDS Server implementation to utilize in Contour.
#   xds-server-type: contour
#
# Specify the Gateway API configuration.
gateway:
  gatewayRef:
    namespace: test-ns
    name: test
#
# should contour expect to be running inside a k8s cluster
# incluster: true
#
# path to kubeconfig (if not running inside a k8s cluster)
# kubeconfig: /path/to/.kube/config
#
# Disable RFC-compliant behavior to strip "Content-Length" header if
# "Tranfer-Encoding: chunked" is also set.
# disableAllowChunkedLength: false
# Disable HTTPProxy permitInsecure field
disablePermitInsecure: false
tls:
# minimum TLS version that Contour will negotiate
# minimum-protocol-version: "1.2"
# TLS ciphers to be supported by Envoy TLS listeners when negotiating
# TLS 1.2.
# cipher-suites:
# - '[ECDHE-ECDSA-AES128-GCM-SHA256|ECDHE-ECDSA-CHACHA20-POLY1305]'
# - '[ECDHE-RSA-AES128-GCM-SHA256|ECDHE-RSA-CHACHA20-POLY1305]'
# - 'ECDHE-ECDSA-AES256-GCM-SHA384'
# - 'ECDHE-RSA-AES256-GCM-SHA384'
# Defines the Kubernetes name/namespace matching a secret to use
# as the fallback certificate when requests which don't match the
# SNI defined for a vhost.
  fallback-certificate:
#   name: fallback-secret-name
#   namespace: projectcontour
  envoy-client-certificate:
#   name: envoy-client-cert-secret-name
#   namespace: projectcontour
# The following config shows the defaults for the leader election.
# leaderelection:
#   configmap-name: leader-elect
#   configmap-namespace: projectcontour
####
# ExternalName Services are disabled by default due to CVE-2021-XXXXX
# You can re-enable them by setting this setting to "true".
# This is not recommended without understanding the security implications.
# Please see the advisory at https://github.com/projectcontour/contour/security/advisories/GHSA-5ph6-qq5x-7jwc for the details.
enableExternalNameService: true
##
### Logging options
# Default setting
accesslog-format: envoy
# To enable JSON logging in Envoy
# accesslog-format: json
# The default fields that will be logged are specified below.
# To customize this list, just add or remove entries.
# The canonical list is available at
# https://godoc.org/github.com/projectcontour/contour/internal/envoy#JSONFields
# json-fields:
#   - "@timestamp"
#   - "authority"
#   - "bytes_received"
#   - "bytes_sent"
#   - "downstream_local_address"
#   - "downstream_remote_address"
#   - "duration"
#   - "method"
#   - "path"
#   - "protocol"
#   - "request_id"
#   - "requested_server_name"
#   - "response_code"
#   - "response_flags"
#   - "uber_trace_id"
#   - "upstream_cluster"
#   - "upstream_host"
#   - "upstream_local_address"
#   - "upstream_service_time"
#   - "user_agent"
#   - "x_forwarded_for"
#
# default-http-versions:
# - "HTTP/2"
# - "HTTP/1.1"
#
# The following shows the default proxy timeout settings.
# timeouts:
#   request-timeout: infinity
#   connection-idle-timeout: 60s
#   stream-idle-timeout: 5m
#   max-connection-duration: infinity
#   delayed-close-timeout: 1s
#   connection-shutdown-grace-period: 5s
#
# Envoy cluster settings.
# cluster:
#   configure the cluster dns lookup family
#   valid options are: auto (default), v4, v6
#   dns-lookup-family: auto
#
# Envoy network settings.
# network:
#   Configure the number of additional ingress proxy hops from the
#   right side of the x-forwarded-for HTTP header to trust.
#   num-trusted-hops: 0
`
	c := &model.Contour{
		ObjectMeta: v1.ObjectMeta{
			Name:      "test",
			Namespace: "test-ns",
		},
		Spec: model.ContourSpec{
			EnableExternalNameService: pointer.Bool(true),
		},
	}
	cm, err := desired(configForContour(c))
	require.NoError(t, err)
	require.Contains(t, cm.Data, "contour.yaml")
	assert.Equal(t, expected, cm.Data["contour.yaml"])
}

func TestDesiredContourConfigmapWithGatewayName(t *testing.T) {
	expected := `#
# server:
#   determine which XDS Server implementation to utilize in Contour.
#   xds-server-type: contour
#
# Specify the Gateway API configuration.
gateway:
  gatewayRef:
    namespace: test-ns
    name: test
#
# should contour expect to be running inside a k8s cluster
# incluster: true
#
# path to kubeconfig (if not running inside a k8s cluster)
# kubeconfig: /path/to/.kube/config
#
# Disable RFC-compliant behavior to strip "Content-Length" header if
# "Tranfer-Encoding: chunked" is also set.
# disableAllowChunkedLength: false
# Disable HTTPProxy permitInsecure field
disablePermitInsecure: false
tls:
# minimum TLS version that Contour will negotiate
# minimum-protocol-version: "1.2"
# TLS ciphers to be supported by Envoy TLS listeners when negotiating
# TLS 1.2.
# cipher-suites:
# - '[ECDHE-ECDSA-AES128-GCM-SHA256|ECDHE-ECDSA-CHACHA20-POLY1305]'
# - '[ECDHE-RSA-AES128-GCM-SHA256|ECDHE-RSA-CHACHA20-POLY1305]'
# - 'ECDHE-ECDSA-AES256-GCM-SHA384'
# - 'ECDHE-RSA-AES256-GCM-SHA384'
# Defines the Kubernetes name/namespace matching a secret to use
# as the fallback certificate when requests which don't match the
# SNI defined for a vhost.
  fallback-certificate:
#   name: fallback-secret-name
#   namespace: projectcontour
  envoy-client-certificate:
#   name: envoy-client-cert-secret-name
#   namespace: projectcontour
# The following config shows the defaults for the leader election.
# leaderelection:
#   configmap-name: leader-elect
#   configmap-namespace: projectcontour
####
# ExternalName Services are disabled by default due to CVE-2021-XXXXX
# You can re-enable them by setting this setting to "true".
# This is not recommended without understanding the security implications.
# Please see the advisory at https://github.com/projectcontour/contour/security/advisories/GHSA-5ph6-qq5x-7jwc for the details.
# enableExternalNameService: false
##
### Logging options
# Default setting
accesslog-format: envoy
# To enable JSON logging in Envoy
# accesslog-format: json
# The default fields that will be logged are specified below.
# To customize this list, just add or remove entries.
# The canonical list is available at
# https://godoc.org/github.com/projectcontour/contour/internal/envoy#JSONFields
# json-fields:
#   - "@timestamp"
#   - "authority"
#   - "bytes_received"
#   - "bytes_sent"
#   - "downstream_local_address"
#   - "downstream_remote_address"
#   - "duration"
#   - "method"
#   - "path"
#   - "protocol"
#   - "request_id"
#   - "requested_server_name"
#   - "response_code"
#   - "response_flags"
#   - "uber_trace_id"
#   - "upstream_cluster"
#   - "upstream_host"
#   - "upstream_local_address"
#   - "upstream_service_time"
#   - "user_agent"
#   - "x_forwarded_for"
#
# default-http-versions:
# - "HTTP/2"
# - "HTTP/1.1"
#
# The following shows the default proxy timeout settings.
# timeouts:
#   request-timeout: infinity
#   connection-idle-timeout: 60s
#   stream-idle-timeout: 5m
#   max-connection-duration: infinity
#   delayed-close-timeout: 1s
#   connection-shutdown-grace-period: 5s
#
# Envoy cluster settings.
# cluster:
#   configure the cluster dns lookup family
#   valid options are: auto (default), v4, v6
#   dns-lookup-family: auto
#
# Envoy network settings.
# network:
#   Configure the number of additional ingress proxy hops from the
#   right side of the x-forwarded-for HTTP header to trust.
#   num-trusted-hops: 0
`

	c := &model.Contour{
		ObjectMeta: v1.ObjectMeta{
			Name:      "test",
			Namespace: "test-ns",
		},
	}
	cm, err := desired(configForContour(c))
	require.NoError(t, err)
	require.Equal(t, "contour", cm.Name)
	require.Equal(t, "test-ns", cm.Namespace)
	require.Contains(t, cm.Data, "contour.yaml")
	assert.Equal(t, expected, cm.Data["contour.yaml"])
}

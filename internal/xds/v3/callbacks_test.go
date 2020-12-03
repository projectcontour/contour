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

package v3

import (
	"fmt"
	"testing"

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	envoy_type_v3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"google.golang.org/genproto/googleapis/rpc/code"
	"google.golang.org/genproto/googleapis/rpc/status"
)

func TestLogDiscoveryRequestDetails(t *testing.T) {
	log, logHook := test.NewNullLogger()
	tests := map[string]struct {
		discoveryReq    *envoy_service_discovery_v3.DiscoveryRequest
		expectedLogMsg  string
		expectedLogData logrus.Fields
	}{
		"request with node info": {
			discoveryReq: &envoy_service_discovery_v3.DiscoveryRequest{
				VersionInfo:   "req-version",
				ResponseNonce: "resp-nonce",
				ResourceNames: []string{"some", "resources"},
				TypeUrl:       "some-type-url",
				Node: &envoy_config_core_v3.Node{
					Id: "node-id",
					UserAgentVersionType: &envoy_config_core_v3.Node_UserAgentBuildVersion{
						UserAgentBuildVersion: &envoy_config_core_v3.BuildVersion{
							Version: &envoy_type_v3.SemanticVersion{
								MajorNumber: 9,
								MinorNumber: 8,
								Patch:       7,
							},
						},
					},
				},
			},
			expectedLogMsg: "handling v3 xDS resource request",
			expectedLogData: logrus.Fields{
				"version_info":   "req-version",
				"response_nonce": "resp-nonce",
				"resource_names": []string{"some", "resources"},
				"type_url":       "some-type-url",
				"node_id":        "node-id",
				"node_version":   "v9.8.7",
			},
		},
		"request without node info": {
			discoveryReq: &envoy_service_discovery_v3.DiscoveryRequest{
				VersionInfo:   "req-version",
				ResponseNonce: "resp-nonce",
				ResourceNames: []string{"some", "resources"},
				TypeUrl:       "some-type-url",
			},
			expectedLogMsg: "handling v3 xDS resource request",
			expectedLogData: logrus.Fields{
				"version_info":   "req-version",
				"response_nonce": "resp-nonce",
				"resource_names": []string{"some", "resources"},
				"type_url":       "some-type-url",
			},
		},
		"request with error detail": {
			discoveryReq: &envoy_service_discovery_v3.DiscoveryRequest{
				VersionInfo:   "req-version",
				ResponseNonce: "resp-nonce",
				ErrorDetail: &status.Status{
					Code:    int32(code.Code_INTERNAL),
					Message: "error message from request",
				},
			},
			expectedLogMsg: "error message from request",
			expectedLogData: logrus.Fields{
				"version_info":   "req-version",
				"response_nonce": "resp-nonce",
				"code":           int32(code.Code_INTERNAL),
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			logDiscoveryRequestDetails(log, tc.discoveryReq)
			var expectedLogEntry *logrus.Entry
			for _, le := range logHook.AllEntries() {
				if le.Message == tc.expectedLogMsg {
					expectedLogEntry = le
				}
			}
			assert.NotNil(t, expectedLogEntry, fmt.Sprintf("no log line with expected message %q", tc.expectedLogMsg))
			assert.Equal(t, expectedLogEntry.Data, tc.expectedLogData)
		})
		logHook.Reset()
	}
}

func TestOnStreamRequestCallbackLogs(t *testing.T) {
	log, logHook := test.NewNullLogger()
	callbacks := NewCallbacks(log)
	err := callbacks.OnStreamRequest(999, &envoy_service_discovery_v3.DiscoveryRequest{
		VersionInfo:   "req-version",
		ResponseNonce: "resp-nonce",
		ResourceNames: []string{"some", "resources"},
		TypeUrl:       "some-type-url",
	})
	assert.NoError(t, err)
	assert.NotEmpty(t, logHook.AllEntries())
}

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
	"context"
	"testing"

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	envoy_type_v3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/genproto/googleapis/rpc/code"
	"google.golang.org/genproto/googleapis/rpc/status"
)

func TestLogStreamOpenDetails(t *testing.T) {
	log, logHook := test.NewNullLogger()
	log.SetLevel(logrus.DebugLevel)

	logStreamOpenDetails(log, 66, "some-type")
	assert.Len(t, logHook.AllEntries(), 1)
	entry := logHook.AllEntries()[0]
	assert.Equal(t, "stream opened", entry.Message)
	assert.Equal(t, logrus.Fields{
		"stream_id": int64(66),
		"type_url":  "some-type",
	}, entry.Data)
}

func TestLogStreamClosedDetails(t *testing.T) {
	log, logHook := test.NewNullLogger()
	log.SetLevel(logrus.DebugLevel)

	logStreamClosedDetails(log, 65, nil)
	assert.Len(t, logHook.AllEntries(), 1)
	entry := logHook.AllEntries()[0]
	assert.Equal(t, "stream closed", entry.Message)
	assert.Equal(t, logrus.Fields{
		"stream_id": int64(65),
	}, entry.Data)
	logHook.Reset()

	logStreamClosedDetails(log, 65, &envoy_config_core_v3.Node{Id: "foo"})
	assert.Len(t, logHook.AllEntries(), 1)
	entry = logHook.AllEntries()[0]
	assert.Equal(t, "stream closed", entry.Message)
	assert.Equal(t, logrus.Fields{
		"stream_id": int64(65),
		"node_id":   "foo",
	}, entry.Data)
	logHook.Reset()
}

func TestLogDiscoveryRequestDetails(t *testing.T) {
	log, logHook := test.NewNullLogger()
	log.SetLevel(logrus.DebugLevel)

	tests := map[string]struct {
		discoveryReq    *envoy_service_discovery_v3.DiscoveryRequest
		expectedLogMsg  string
		expectedLogData logrus.Fields
	}{
		"request with node info and node version": {
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
		"request with node info, without node version": {
			discoveryReq: &envoy_service_discovery_v3.DiscoveryRequest{
				VersionInfo:   "req-version",
				ResponseNonce: "resp-nonce",
				ResourceNames: []string{"some", "resources"},
				TypeUrl:       "some-type-url",
				Node: &envoy_config_core_v3.Node{
					Id:                   "node-id",
					UserAgentVersionType: nil,
				},
			},
			expectedLogMsg: "handling v3 xDS resource request",
			expectedLogData: logrus.Fields{
				"version_info":   "req-version",
				"response_nonce": "resp-nonce",
				"resource_names": []string{"some", "resources"},
				"type_url":       "some-type-url",
				"node_id":        "node-id",
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
			var logEntry *logrus.Entry
			for _, le := range logHook.AllEntries() {
				if le.Message == tc.expectedLogMsg {
					logEntry = le
					break
				}
			}
			assert.NotNil(t, logEntry, "no log line with expected message %q", tc.expectedLogMsg)
			assert.Equal(t, tc.expectedLogData, logEntry.Data)
			logHook.Reset()
		})
	}
}

func TestOnStreamRequestCallbackLogs(t *testing.T) {
	log, logHook := test.NewNullLogger()
	log.SetLevel(logrus.DebugLevel)

	callbacks := NewRequestLoggingCallbacks(log)

	err := callbacks.OnStreamOpen(context.TODO(), 999, "a-type")
	require.NoError(t, err)
	assert.NotEmpty(t, logHook.AllEntries())
	logHook.Reset()

	callbacks.OnStreamClosed(999, &envoy_config_core_v3.Node{Id: "envoy-1234"})
	assert.NotEmpty(t, logHook.AllEntries())
	logHook.Reset()

	err = callbacks.OnStreamRequest(999, &envoy_service_discovery_v3.DiscoveryRequest{
		VersionInfo:   "req-version",
		ResponseNonce: "resp-nonce",
		ResourceNames: []string{"some", "resources"},
		TypeUrl:       "some-type-url",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, logHook.AllEntries())
	logHook.Reset()
}

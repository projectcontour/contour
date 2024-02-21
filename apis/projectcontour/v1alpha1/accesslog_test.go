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

package v1alpha1_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	contour_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
)

func TestValidateAccessLogType(t *testing.T) {
	require.Error(t, contour_v1alpha1.AccessLogType("").Validate())
	require.Error(t, contour_v1alpha1.AccessLogType("foo").Validate())

	require.NoError(t, contour_v1alpha1.EnvoyAccessLog.Validate())
	require.NoError(t, contour_v1alpha1.JSONAccessLog.Validate())
}

func TestValidateAccessLogLevel(t *testing.T) {
	require.Error(t, contour_v1alpha1.AccessLogLevel("").Validate())
	require.Error(t, contour_v1alpha1.AccessLogLevel("foo").Validate())

	require.NoError(t, contour_v1alpha1.LogLevelInfo.Validate())
	require.NoError(t, contour_v1alpha1.LogLevelError.Validate())
	require.NoError(t, contour_v1alpha1.LogLevelCritical.Validate())
	require.NoError(t, contour_v1alpha1.LogLevelDisabled.Validate())
}

func TestValidateAccessLogJSONFields(t *testing.T) {
	errorCases := [][]string{
		{"dog", "cat"},
		{"req"},
		{"resp"},
		{"trailer"},
		{"@timestamp", "dog"},
		{"@timestamp", "content-id=%REQ=dog%"},
		{"@timestamp", "content-id=%dog(%"},
		{"@timestamp", "content-id=%REQ()%"},
		{"@timestamp", "content-id=%DOG%"},
		{"@timestamp", "duration=my durations % are %DURATION%.0 and %REQ(:METHOD)%"},
		{"invalid=%REQ%"},
		{"invalid=%TRAILER%"},
		{"invalid=%RESP%"},
		{"invalid=%REQ_WITHOUT_QUERY%"},
		{"invalid=%ENVIRONMENT%"},
		{"@timestamp", "invalid=%START_TIME(%s.%6f):10%"},
	}

	for _, c := range errorCases {
		require.Error(t, contour_v1alpha1.AccessLogJSONFields(c).Validate(), c)
	}

	successCases := [][]string{
		{"@timestamp", "method"},
		{"start_time"},
		{"@timestamp", "response_duration"},
		{"@timestamp", "duration=%DURATION%.0"},
		{"@timestamp", "duration=My duration=%DURATION%.0"},
		{"@timestamp", "duration=%START_TIME(%s.%6f)%"},
		{"@timestamp", "content-id=%REQ(X-CONTENT-ID)%"},
		{"@timestamp", "content-id=%REQ(X-CONTENT-ID):10%"},
		{"@timestamp", "length=%RESP(CONTENT-LENGTH):10%"},
		{"@timestamp", "trailer=%TRAILER(CONTENT-LENGTH):10%"},
		{"@timestamp", "duration=my durations are %DURATION%.0 and method is %REQ(:METHOD)%"},
		{"path=%REQ_WITHOUT_QUERY(X-ENVOY-ORIGINAL-PATH?:PATH)%"},
		{"pod=%ENVIRONMENT(ENVOY_POD_NAME)%"},
		{"dog=pug", "cat=black"},
		{"grpc_status"},
		{"grpc_status_number"},
	}

	for _, c := range successCases {
		require.NoError(t, contour_v1alpha1.AccessLogJSONFields(c).Validate(), c)
	}
}

func TestAccessLogFormatString(t *testing.T) {
	errorCases := []string{
		"%REQ=dog%\n",
		"%dog(%\n",
		"%REQ()%\n",
		"%DOG%\n",
		"my durations % are %DURATION%.0 and %REQ(:METHOD)%\n",
		"%REQ%\n",
		"%TRAILER%\n",
		"%RESP%\n",
		"%REQ_WITHOUT_QUERY%\n",
		"%START_TIME(%s.%6f):10%\n",
		"no newline at the end",
		"%METADATA%\n",
	}

	for _, c := range errorCases {
		require.Error(t, contour_v1alpha1.AccessLogFormatString(c).Validate(), c)
	}

	successCases := []string{
		"",
		"%DURATION%.0\n",
		"My duration %DURATION%.0\n",
		"%START_TIME(%s.%6f)%\n",
		"%START_TIME%\n",
		"%REQ(X-CONTENT-ID)%\n",
		"%REQ(X-CONTENT-ID):10%\n",
		"%RESP(CONTENT-LENGTH):10%\n",
		"%TRAILER(CONTENT-LENGTH):10%\n",
		"my durations are %DURATION%.0 and method is %REQ(:METHOD)%\n",
		"queries %REQ_WITHOUT_QUERY(X-ENVOY-ORIGINAL-PATH?:PATH)% removed\n",
		"just a string\n",
		"%GRPC_STATUS%\n",
		"%GRPC_STATUS_NUMBER%\n",
		"%METADATA(ROUTE:com.test.my_filter:test_key):20%\n",
		"%UPSTREAM_PROTOCOL%\n",
		"%UPSTREAM_PEER_SUBJECT%\n",
		"%UPSTREAM_PEER_ISSUER%\n",
		"%UPSTREAM_TLS_SESSION_ID%\n",
		"%UPSTREAM_TLS_CIPHER%\n",
		"%UPSTREAM_TLS_VERSION%\n",
		"%UPSTREAM_PEER_CERT_V_START%\n",
		"%UPSTREAM_PEER_CERT_V_END%\n",
		"%UPSTREAM_PEER_CERT%\n",
		"%UPSTREAM_FILTER_STATE%\n",
	}

	for _, c := range successCases {
		require.NoError(t, contour_v1alpha1.AccessLogFormatString(c).Validate(), c)
	}
}

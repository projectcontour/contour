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

package debug

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"

	"github.com/projectcontour/contour/internal/httpsvc"
)

func TestServiceLogger(t *testing.T) {
	log := logrus.New()

	// The debug service is normally given an Entry derived from the
	// process-wide logger; the level must be changed on that logger.
	svc := &Service{Service: httpsvc.Service{FieldLogger: log.WithField("context", "debugsvc")}}
	require.Same(t, log, svc.logger())

	svc = &Service{Service: httpsvc.Service{FieldLogger: log}}
	require.Same(t, log, svc.logger())

	// Without a configured logger fall back to the standard logger.
	svc = &Service{}
	require.Same(t, logrus.StandardLogger(), svc.logger())
}

func TestLogLevelEndpoint(t *testing.T) {
	log := logrus.New()
	log.SetOutput(io.Discard)
	log.SetLevel(logrus.InfoLevel)

	mux := http.NewServeMux()
	registerLogLevel(mux, log)

	do := func(method, target, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, target, strings.NewReader(body))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}

	// GET reports the current level.
	rec := do(http.MethodGet, "/debug/loglevel", "")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "info\n", rec.Body.String())

	// PUT with the level in the request body changes the level.
	rec = do(http.MethodPut, "/debug/loglevel", "debug\n")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "debug\n", rec.Body.String())
	require.Equal(t, logrus.DebugLevel, log.GetLevel())

	rec = do(http.MethodGet, "/debug/loglevel", "")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "debug\n", rec.Body.String())

	// PUT with the level as a query parameter changes the level.
	rec = do(http.MethodPut, "/debug/loglevel?level=warning", "")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "warning\n", rec.Body.String())
	require.Equal(t, logrus.WarnLevel, log.GetLevel())

	// POST is accepted as well, and level names are case-insensitive.
	rec = do(http.MethodPost, "/debug/loglevel", "ERROR")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "error\n", rec.Body.String())
	require.Equal(t, logrus.ErrorLevel, log.GetLevel())

	// An unknown level is rejected and leaves the level unchanged.
	rec = do(http.MethodPut, "/debug/loglevel", "bogus")
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "not a valid logrus Level")
	require.Equal(t, logrus.ErrorLevel, log.GetLevel())

	// An empty level is rejected and leaves the level unchanged.
	rec = do(http.MethodPut, "/debug/loglevel", "")
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, logrus.ErrorLevel, log.GetLevel())

	// Other methods are not allowed.
	rec = do(http.MethodDelete, "/debug/loglevel", "debug")
	require.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	require.Equal(t, "GET, PUT, POST", rec.Header().Get("Allow"))
	require.Equal(t, logrus.ErrorLevel, log.GetLevel())
}

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

// Package debug provides http endpoints for healthcheck, metrics,
// and pprof debugging.
package debug // nolint:revive // Ignore var-naming warning about package name conflicting with runtime/debug.

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/pprof"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/httpsvc"
)

// Service serves various http endpoints including /debug/pprof.
type Service struct {
	httpsvc.Service

	Builder *dag.Builder
}

func (svc *Service) NeedLeaderElection() bool {
	return false
}

// Implements controller-runtime Runnable interface.
// When context is done, http server will shutdown.
func (svc *Service) Start(ctx context.Context) error {
	registerProfile(&svc.ServeMux)
	registerDotWriter(&svc.ServeMux, svc.Builder)
	registerLogLevel(&svc.ServeMux, svc.logger())
	return svc.Service.Start(ctx)
}

// logger returns the logger whose level is controlled by the
// /debug/loglevel endpoint. The service is normally configured with an
// Entry derived from the process-wide logger, so the level is changed on
// the Logger backing it; without a configured logger the standard logger
// is used.
func (svc *Service) logger() *logrus.Logger {
	switch log := svc.FieldLogger.(type) {
	case *logrus.Logger:
		return log
	case *logrus.Entry:
		return log.Logger
	default:
		return logrus.StandardLogger()
	}
}

func registerProfile(mux *http.ServeMux) {
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	mux.Handle("/debug/pprof/block", pprof.Handler("block"))
	mux.Handle("/debug/pprof/goroutine", pprof.Handler("goroutine"))
	mux.Handle("/debug/pprof/heap", pprof.Handler("heap"))
	mux.Handle("/debug/pprof/threadcreate", pprof.Handler("threadcreate"))
}

func registerDotWriter(mux *http.ServeMux, builder *dag.Builder) {
	mux.HandleFunc("/debug/dag", func(w http.ResponseWriter, _ *http.Request) {
		dw := &dotWriter{
			Builder: builder,
		}
		dw.writeDot(w)
	})
}

// registerLogLevel registers the /debug/loglevel endpoint, which allows the
// log level to be inspected and changed at runtime without restarting
// Contour. GET returns the current level; PUT or POST sets the level given
// as the "level" query parameter or, failing that, as the request body.
func registerLogLevel(mux *http.ServeMux, log *logrus.Logger) {
	mux.HandleFunc("/debug/loglevel", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
		case http.MethodPut, http.MethodPost:
			level := r.URL.Query().Get("level")
			if level == "" {
				body, err := io.ReadAll(io.LimitReader(r.Body, 64))
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				level = strings.TrimSpace(string(body))
			}

			parsed, err := logrus.ParseLevel(level)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			log.Infof("changing log level from %s to %s", log.GetLevel(), parsed)
			log.SetLevel(parsed)
		default:
			w.Header().Set("Allow", "GET, PUT, POST")
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintln(w, log.GetLevel())
	})
}

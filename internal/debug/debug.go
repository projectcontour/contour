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
package debug

import (
	"net/http"
	"net/http/pprof"

	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/httpsvc"
)

// Service serves various http endpoints including /debug/pprof.
type Service struct {
	httpsvc.Service

	Builder *dag.Builder
}

// Start fulfills the g.Start contract.
// When stop is closed the http server will shutdown.
func (svc *Service) Start(stop <-chan struct{}) error {
	registerProfile(&svc.ServeMux)
	registerDotWriter(&svc.ServeMux, svc.Builder)
	return svc.Service.Start(stop)
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
	mux.HandleFunc("/debug/dag", func(w http.ResponseWriter, r *http.Request) {
		dw := &dotWriter{
			Builder: builder,
		}
		dw.writeDot(w)
	})
}

// Copyright © 2018 Heptio
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
	"context"
	"fmt"
	"net/http"
	"net/http/pprof"
	"time"

	"github.com/heptio/contour/internal/dag"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
)

// Service serves various http endpoints including /debug/pprof.
type Service struct {
	Addr string
	Port int

	logrus.FieldLogger

	*dag.Builder
}

// Start fulfills the g.Start contract.
// When stop is closed the http server will shutdown.
func (svc *Service) Start(stop <-chan struct{}, registry *prometheus.Registry) (err error) {
	defer func() {
		if err != nil {
			svc.WithError(err).Error("terminated with error")
		} else {
			svc.Info("stopped")
		}
	}()
	mux := http.NewServeMux()
	registerProfile(mux)
	registerHealthCheck(mux)
	registerMetrics(mux, registry)

	// register DAG dot writer.
	mux.HandleFunc("/debug/dag", svc.writeDot)

	s := http.Server{
		Addr:           fmt.Sprintf("%s:%d", svc.Addr, svc.Port),
		Handler:        mux,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   5 * time.Minute, // allow for long trace requests
		MaxHeaderBytes: 1 << 11,         // 8kb should be enough for anyone
	}

	go func() {
		// wait for stop signal from group.
		<-stop

		// shutdown the server with 5 seconds grace.
		ctx := context.Background()
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		s.Shutdown(ctx)
	}()

	svc.WithField("address", s.Addr).Info("started")
	return s.ListenAndServe()
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

func registerHealthCheck(mux *http.ServeMux) {
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "OK")
	})
}

func registerMetrics(mux *http.ServeMux, registry *prometheus.Registry) {
	mux.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
}

// Write out a .dot representation of the DAG.
func (svc *Service) writeDot(w http.ResponseWriter, r *http.Request) {
	dw := &dotWriter{
		Builder:     svc.Builder,
		FieldLogger: svc.FieldLogger,
	}
	dw.writeDot(w)
}

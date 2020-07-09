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

// Package httpsvc provides a HTTP/1.x Service which is compatible with the
// workgroup.Group API.
package httpsvc

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"
)

// Service is a HTTP/1.x endpoint which is compatible with the workgroup.Group API.
type Service struct {
	Addr string
	Port int

	logrus.FieldLogger
	http.ServeMux
}

// Start fulfills the g.Start contract.
// When stop is closed the http server will shutdown.
func (svc *Service) Start(stop <-chan struct{}) (err error) {
	defer func() {
		if err != nil {
			svc.WithError(err).Error("terminated HTTP server with error")
		} else {
			svc.Info("stopped HTTP server")
		}
	}()

	s := http.Server{
		Addr:           fmt.Sprintf("%s:%d", svc.Addr, svc.Port),
		Handler:        &svc.ServeMux,
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
		_ = s.Shutdown(ctx) // ignored, will always be a cancelation error
	}()

	svc.WithField("address", s.Addr).Info("started HTTP server")
	return s.ListenAndServe()
}

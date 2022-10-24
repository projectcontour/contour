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
// manager.Runnable API.
package httpsvc

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/sirupsen/logrus"
)

// Service is a HTTP/1.x endpoint which is compatible with the manager.Runnable API.
type Service struct {
	Addr string
	Port int

	// TLS parameters
	CABundle string
	Cert     string
	Key      string

	logrus.FieldLogger
	http.ServeMux
}

func (svc *Service) NeedLeaderElection() bool {
	return false
}

// Implements controller-runtime Runnable interface.
// When context is done, http server will shutdown.
func (svc *Service) Start(ctx context.Context) (err error) {
	defer func() {
		if err != nil {
			svc.WithError(err).Error("terminated HTTP server with error")
		} else {
			svc.Info("stopped HTTP server")
		}
	}()

	// Create TLSConfig if both certificate and key are provided.
	var tlsConfig *tls.Config
	if svc.Cert != "" && svc.Key != "" {
		tlsConfig, err = svc.tlsConfig()
		if err != nil {
			return err
		}
	}

	// If one of the TLS parameters are defined, at least server certificate
	// and key must be defined.
	if (svc.Cert != "" || svc.Key != "" || svc.CABundle != "") &&
		(svc.Cert == "" || svc.Key == "") {
		svc.Fatal("you must supply at least server certificate and key TLS parameters or none of them")
	}

	s := http.Server{
		Addr:              net.JoinHostPort(svc.Addr, strconv.Itoa(svc.Port)),
		Handler:           &svc.ServeMux,
		ReadTimeout:       10 * time.Second,
		ReadHeaderTimeout: 10 * time.Second, // To mitigate Slowloris attacks: https://www.cloudflare.com/learning/ddos/ddos-attack-tools/slowloris/
		WriteTimeout:      5 * time.Minute,  // allow for long trace requests
		MaxHeaderBytes:    1 << 11,          // 8kb should be enough for anyone
		TLSConfig:         tlsConfig,
	}

	go func() {
		// wait for stop signal from group.
		<-ctx.Done()

		// shutdown the server with 5 seconds grace.
		ctx := context.Background()
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		_ = s.Shutdown(ctx) // ignored, will always be a cancellation error
	}()

	if s.TLSConfig != nil {
		svc.WithField("address", s.Addr).Info("started HTTPS server")
		return s.ListenAndServeTLS(svc.Cert, svc.Key)
	}

	svc.WithField("address", s.Addr).Info("started HTTP server")
	return s.ListenAndServe()
}

func (svc *Service) tlsConfig() (*tls.Config, error) {
	// Define a closure that lazily loads certificates and key at TLS handshake
	// to ensure that latest certificates are used in case they have been rotated.
	loadConfig := func() (*tls.Config, error) {
		cert, err := tls.LoadX509KeyPair(svc.Cert, svc.Key)
		if err != nil {
			return nil, err
		}

		clientAuth := tls.NoClientCert
		var certPool *x509.CertPool
		if svc.CABundle != "" {
			clientAuth = tls.RequireAndVerifyClientCert
			ca, err := os.ReadFile(svc.CABundle)
			if err != nil {
				return nil, err
			}

			certPool = x509.NewCertPool()
			if ok := certPool.AppendCertsFromPEM(ca); !ok {
				return nil, fmt.Errorf("unable to append certificate in %s to CA pool", svc.CABundle)
			}
		}

		return &tls.Config{
			Certificates: []tls.Certificate{cert},
			ClientAuth:   clientAuth,
			ClientCAs:    certPool,
			MinVersion:   tls.VersionTLS13,
		}, nil
	}

	// Attempt to load certificates and key to catch configuration errors early.
	if _, err := loadConfig(); err != nil {
		return nil, err
	}

	return &tls.Config{
		MinVersion: tls.VersionTLS12,
		GetConfigForClient: func(*tls.ClientHelloInfo) (*tls.Config, error) {
			return loadConfig()
		},
	}, nil
}

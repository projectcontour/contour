// Copyright © 2020 VMware
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

package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/projectcontour/contour/internal/metrics"

	"github.com/sirupsen/logrus"

	"github.com/projectcontour/contour/internal/workgroup"
	"gopkg.in/alecthomas/kingpin.v2"
)

// handler for /healthz
func (s *shutdownmanagerContext) healthzHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, err := w.Write([]byte("OK"))
	if err != nil {
		s.Error(err)
	}
}

// handles /shutdown
func (s *shutdownmanagerContext) shutdownHandler(w http.ResponseWriter, r *http.Request) {
	prometheusURL := fmt.Sprintf("http://%s:%d%s", s.envoyHost, s.envoyPort, s.prometheusPath)
	envoyAdminURL := fmt.Sprintf("http://%s:%d/healthcheck/fail", s.envoyHost, s.envoyPort)

	// Send shutdown signal to Envoy to start draining connections
	err := shutdownEnvoy(envoyAdminURL)
	if err != nil {
		s.Errorf("Error sending envoy healthcheck fail: %v", err)
	}

	s.Infof("Sent healthcheck fail to Envoy...waiting %s before polling for draining connections", s.checkDelay)
	time.Sleep(s.checkDelay)

	for {
		openConnections, err := getOpenConnections(prometheusURL, s.prometheusStat, s.prometheusValues)
		if err != nil {
			s.Error(err)
		} else {
			if openConnections <= s.minOpenConnections {
				s.Infof("Found %d open connections with min number of %d connections. Shutting down...", openConnections, s.minOpenConnections)
				return
			}
			s.Infof("Found %d open connections with min number of %d connections. Polling again...", openConnections, s.minOpenConnections)
		}
		time.Sleep(s.checkInterval)
	}
}

// shutdownEnvoy sends a POST request to /healthcheck/fail to tell Envoy to start draining connections
func shutdownEnvoy(url string) error {
	resp, err := http.Post(url, "", nil)
	if err != nil {
		return fmt.Errorf("creating POST request for URL %q failed: %s", url, err)
	}

	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("POST request for URL %q returned HTTP status %s", url, resp.Status)
	}
	return nil
}

// getOpenConnections parses a http request to a prometheus endpoint returning the sum of values found
func getOpenConnections(url, prometheusStat string, prometheusValues []string) (int, error) {
	// Make request to Envoy Prometheus endpoint
	resp, err := http.Get(url)
	if err != nil {
		return -1, fmt.Errorf("GET request for URL %q failed: %s", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return -1, fmt.Errorf("GET request for URL %q returned HTTP status %s", url, resp.Status)
	}

	// Parse Prometheus listener stats for open connections
	return metrics.ParseOpenConnections(resp.Body, prometheusStat, prometheusValues)
}

func doShutdownManager(config *shutdownmanagerContext) error {
	var g workgroup.Group

	g.Add(func(stop <-chan struct{}) error {
		config.Info("started envoy shutdown manager")
		defer config.Info("stopped")

		http.HandleFunc("/healthz", config.healthzHandler)
		http.HandleFunc("/shutdown", config.shutdownHandler)
		log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", config.httpServePort), nil))

		return nil
	})

	return g.Run()
}

// registerShutdownManager registers the envoy shutdown sub-command and flags
func registerShutdownManager(cmd *kingpin.CmdClause, log logrus.FieldLogger) (*kingpin.CmdClause, *shutdownmanagerContext) {
	ctx := &shutdownmanagerContext{
		FieldLogger: log,
	}
	shutdownmgr := cmd.Command("shutdown-manager", "Start envoy shutdown-manager.")
	shutdownmgr.Flag("check-interval", "Time to poll Envoy for open connections.").Default("5s").DurationVar(&ctx.checkInterval)
	shutdownmgr.Flag("check-delay", "Time wait before polling Envoy for open connections.").Default("60s").DurationVar(&ctx.checkDelay)
	shutdownmgr.Flag("min-open-connections", "Min number of open connections when polling Envoy.").Default("0").IntVar(&ctx.minOpenConnections)
	shutdownmgr.Flag("serve-port", "Port to serve the http server on.").Default("8090").IntVar(&ctx.httpServePort)
	shutdownmgr.Flag("prometheus-path", "The path to query Envoy's Prometheus HTTP Endpoint.").Default("/stats/prometheus").StringVar(&ctx.prometheusPath)
	shutdownmgr.Flag("prometheus-stat", "Prometheus stat to query.").Default("envoy_http_downstream_cx_active").StringVar(&ctx.prometheusStat)
	shutdownmgr.Flag("prometheus-values", "Prometheus values to look for in prometheus-stat.").Default("ingress_http", "ingress_https").StringsVar(&ctx.prometheusValues)
	shutdownmgr.Flag("envoy-host", "HTTP endpoint for Envoy's stats page.").Default("localhost").StringVar(&ctx.envoyHost)
	shutdownmgr.Flag("envoy-port", "HTTP port for Envoy's stats page.").Default("9001").IntVar(&ctx.envoyPort)

	return shutdownmgr, ctx
}

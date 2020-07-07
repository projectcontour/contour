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

package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/projectcontour/contour/internal/contour"

	"github.com/prometheus/common/expfmt"

	"github.com/sirupsen/logrus"

	"gopkg.in/alecthomas/kingpin.v2"
)

const (
	prometheusURL      = "http://localhost:9001/stats/prometheus"
	healthcheckFailURL = "http://localhost:9001/healthcheck/fail"
	prometheusStat     = "envoy_http_downstream_cx_active"
)

func prometheusLabels() []string {
	return []string{contour.ENVOY_HTTP_LISTENER, contour.ENVOY_HTTPS_LISTENER}
}

type shutdownmanagerContext struct {
	// checkInterval defines time delay between polling Envoy for open connections
	checkInterval time.Duration

	// checkDelay defines time to wait before polling Envoy for open connections
	checkDelay time.Duration

	// minOpenConnections defines the minimum amount of connections
	// that can be open when polling for active connections in Envoy
	minOpenConnections int

	// httpServePort defines what port the shutdown-manager listens on
	httpServePort int

	logrus.FieldLogger
}

func newShutdownManagerContext() *shutdownmanagerContext {
	// Set defaults for parameters which are then overridden via flags, ENV, or ConfigFile
	return &shutdownmanagerContext{
		checkInterval:      5 * time.Second,
		checkDelay:         60 * time.Second,
		minOpenConnections: 0,
		httpServePort:      8090,
	}
}

// handles the /healthz endpoint which is used for the shutdown-manager's liveness probe
func (s *shutdownmanagerContext) healthzHandler(w http.ResponseWriter, r *http.Request) {
	http.StatusText(http.StatusOK)
	if _, err := w.Write([]byte("OK")); err != nil {
		s.Error(err)
	}
}

// shutdownHandler handles the /shutdown endpoint which should be called from a pod preStop hook,
// where it will block pod shutdown until envoy is able to drain connections to below the min-open threshold
func (s *shutdownmanagerContext) shutdownHandler(w http.ResponseWriter, r *http.Request) {
	// Send shutdown signal to Envoy to start draining connections
	s.Infof("failing envoy healthchecks")
	err := shutdownEnvoy()
	if err != nil {
		s.Errorf("error sending envoy healthcheck fail: %v", err)
	}

	s.Infof("waiting %s before polling for draining connections", s.checkDelay)
	time.Sleep(s.checkDelay)

	for {
		openConnections, err := getOpenConnections()
		if err != nil {
			s.Error(err)
		} else {
			if openConnections <= s.minOpenConnections {
				s.WithField("open_connections", openConnections).
					WithField("min_connections", s.minOpenConnections).
					Info("min number of open connections found, shutting down")
				return
			}
			s.WithField("open_connections", openConnections).
				WithField("min_connections", s.minOpenConnections).
				Info("polled open connections")
		}
		time.Sleep(s.checkInterval)
	}
}

// shutdownEnvoy sends a POST request to /healthcheck/fail to tell Envoy to start draining connections
func shutdownEnvoy() error {
	resp, err := http.Post(healthcheckFailURL, "", nil)
	if err != nil {
		return fmt.Errorf("creating healthcheck fail POST request failed: %s", err)
	}

	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("POST for %q returned HTTP status %s", healthcheckFailURL, resp.Status)
	}
	return nil
}

// getOpenConnections parses a http request to a prometheus endpoint returning the sum of values found
func getOpenConnections() (int, error) {
	// Make request to Envoy Prometheus endpoint
	resp, err := http.Get(prometheusURL)
	if err != nil {
		return -1, fmt.Errorf("creating metrics GET request failed: %s", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return -1, fmt.Errorf("GET for %q returned HTTP status %s", prometheusURL, resp.Status)
	}

	// Parse Prometheus listener stats for open connections
	return parseOpenConnections(resp.Body)
}

// parseOpenConnections returns the sum of open connections from a Prometheus HTTP request
func parseOpenConnections(stats io.Reader) (int, error) {
	var parser expfmt.TextParser
	openConnections := 0

	if stats == nil {
		return -1, fmt.Errorf("stats input was nil")
	}

	// Parse Prometheus http response
	metricFamilies, err := parser.TextToMetricFamilies(stats)
	if err != nil {
		return -1, fmt.Errorf("parsing Prometheus text format failed: %v", err)
	}

	// Validate stat exists in output
	if _, ok := metricFamilies[prometheusStat]; !ok {
		return -1, fmt.Errorf("error finding Prometheus stat %q in the request result", prometheusStat)
	}

	// Look up open connections value
	for _, metrics := range metricFamilies[prometheusStat].Metric {
		for _, labels := range metrics.Label {
			for _, item := range prometheusLabels() {
				if item == labels.GetValue() {
					openConnections += int(metrics.Gauge.GetValue())
				}
			}
		}
	}
	return openConnections, nil
}

func doShutdownManager(config *shutdownmanagerContext) {
	config.Info("started envoy shutdown manager")
	defer config.Info("stopped")

	http.HandleFunc("/healthz", config.healthzHandler)
	http.HandleFunc("/shutdown", config.shutdownHandler)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", config.httpServePort), nil))
}

// registerShutdownManager registers the envoy shutdown sub-command and flags
func registerShutdownManager(cmd *kingpin.CmdClause, log logrus.FieldLogger) (*kingpin.CmdClause, *shutdownmanagerContext) {
	ctx := newShutdownManagerContext()
	ctx.FieldLogger = log.WithField("context", "shutdown-manager")

	shutdownmgr := cmd.Command("shutdown-manager", "Start envoy shutdown-manager.")
	shutdownmgr.Flag("check-interval", "Time to poll Envoy for open connections.").DurationVar(&ctx.checkInterval)
	shutdownmgr.Flag("check-delay", "Time wait before polling Envoy for open connections.").Default("60s").DurationVar(&ctx.checkDelay)
	shutdownmgr.Flag("min-open-connections", "Min number of open connections when polling Envoy.").IntVar(&ctx.minOpenConnections)
	shutdownmgr.Flag("serve-port", "Port to serve the http server on.").IntVar(&ctx.httpServePort)
	return shutdownmgr, ctx
}

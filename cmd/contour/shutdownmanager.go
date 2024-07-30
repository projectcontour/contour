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
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/prometheus/common/expfmt"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
)

const (
	// The prometheusURL is used to fetch the envoy metrics. Note that the filter
	// value matches Envoy's raw stat names (i.e. those on the `/stats/` endpoint).
	prometheusURL      = "http://unix/stats/prometheus?filter=^http\\..*\\.downstream_cx_active$"
	healthcheckFailURL = "http://unix/healthcheck/fail"
	prometheusStat     = "envoy_http_downstream_cx_active"
)

// shutdownReadyFile is the default file path used in the /shutdown endpoint.
const shutdownReadyFile = "/admin/ok"

// shutdownReadyCheckInterval is the default polling interval for the file used in the /shutdown endpoint.
const shutdownReadyCheckInterval = time.Second * 1

type shutdownmanagerContext struct {
	// httpServePort defines what port the shutdown-manager listens on
	httpServePort int
	// shutdownReadyFile is the default file path used in the /shutdown endpoint
	shutdownReadyFile string
	// shutdownReadyCheckInterval is the polling interval for the file used in the /shutdown endpoint
	shutdownReadyCheckInterval time.Duration

	logrus.FieldLogger
}

type shutdownContext struct {
	// checkInterval defines time delay between polling Envoy for open connections
	checkInterval time.Duration

	// checkDelay defines time to wait before polling Envoy for open connections
	checkDelay time.Duration

	// drainDelay defines time to wait before draining Envoy connections
	drainDelay time.Duration

	// minOpenConnections defines the minimum amount of connections
	// that can be open when polling for active connections in Envoy
	minOpenConnections int

	// Deprecated: adminPort defines the port for the Envoy admin webpage, being configurable through --admin-port flag
	adminPort int

	// adminAddress defines the address for the Envoy admin webpage, being configurable through --admin-address flag
	adminAddress string

	// shutdownReadyFile defines the name of the file that is used to signal that shutdown is completed.
	shutdownReadyFile string

	logrus.FieldLogger
}

func newShutdownManagerContext() *shutdownmanagerContext {
	// Set defaults for parameters which are then overridden via flags, ENV, or ConfigFile
	return &shutdownmanagerContext{
		httpServePort:              8090,
		shutdownReadyFile:          shutdownReadyFile,
		shutdownReadyCheckInterval: shutdownReadyCheckInterval,
	}
}

func newShutdownContext() *shutdownContext {
	return &shutdownContext{
		checkInterval:      5 * time.Second,
		checkDelay:         0,
		drainDelay:         0,
		minOpenConnections: 0,
	}
}

// healthzHandler handles the /healthz endpoint which is used for the shutdown-manager's liveness probe.
func (s *shutdownmanagerContext) healthzHandler(w http.ResponseWriter, _ *http.Request) {
	if _, err := w.Write([]byte(http.StatusText(http.StatusOK))); err != nil {
		s.WithField("context", "healthzHandler").Error(err)
	}
}

// shutdownReadyHandler handles the /shutdown endpoint which is used by Envoy to determine if it can terminate.
// Once enough connections have drained based upon configuration, a file will be written in
// the shutdown manager's file system. Any HTTP request to /shutdown will use the existence of this
// file to understand if it is safe to terminate. The file-based approach is used since the process in which
// the kubelet calls the shutdown command is different than the HTTP request from Envoy to /shutdown
func (s *shutdownmanagerContext) shutdownReadyHandler(w http.ResponseWriter, r *http.Request) {
	l := s.WithField("context", "shutdownReadyHandler")
	ctx := r.Context()
	for {
		_, err := os.Stat(s.shutdownReadyFile)
		switch {
		case os.IsNotExist(err):
			l.Infof("file %s does not exist; checking again in %v", s.shutdownReadyFile,
				s.shutdownReadyCheckInterval)
		case err == nil:
			l.Infof("detected file %s; sending HTTP response", s.shutdownReadyFile)
			if _, err := w.Write([]byte(http.StatusText(http.StatusOK))); err != nil {
				l.Error(err)
			}
			return
		default:
			l.Errorf("error checking for file: %v", err)
		}

		select {
		case <-time.After(s.shutdownReadyCheckInterval):
		case <-ctx.Done():
			l.Infof("client request cancelled")
			return
		}
	}
}

// shutdownHandler is called from a pod preStop hook, where it will block pod shutdown
// until envoy is able to drain connections to below the min-open threshold.
func (s *shutdownContext) shutdownHandler() {
	s.WithField("context", "shutdownHandler").Infof("waiting %s before draining connections", s.drainDelay)
	time.Sleep(s.drainDelay)

	// Send shutdown signal to Envoy to start draining connections
	s.Infof("failing envoy healthchecks")

	// Retry any failures to shutdownEnvoy(s.adminAddress) in a Backoff time window
	// doing 4 total attempts, multiplying the Duration by the Factor
	// for each iteration.
	err := retry.OnError(wait.Backoff{
		Steps:    4,
		Duration: 200 * time.Millisecond,
		Factor:   5.0,
		Jitter:   0.1,
	}, func(error) bool {
		// Always retry any error.
		return true
	}, func() error {
		s.Infof("attempting to shutdown")
		return shutdownEnvoy(s.adminAddress)
	})
	if err != nil {
		// May be conflict if max retries were hit, or may be something unrelated
		// like permissions or a network error
		s.WithField("context", "shutdownHandler").Errorf("error sending envoy healthcheck fail after 4 attempts: %v", err)
	}

	s.WithField("context", "shutdownHandler").Infof("waiting %s before polling for draining connections", s.checkDelay)
	time.Sleep(s.checkDelay)

	for {
		openConnections, err := getOpenConnections(s.adminAddress)
		if err != nil {
			s.Error(err)
		} else {
			if openConnections <= s.minOpenConnections {
				s.WithField("context", "shutdownHandler").
					WithField("open_connections", openConnections).
					WithField("min_connections", s.minOpenConnections).
					Info("min number of open connections found, shutting down")
				file, err := os.Create(s.shutdownReadyFile)
				if err != nil {
					s.Error(err)
				}
				defer file.Close()
				return
			}
			s.WithField("context", "shutdownHandler").
				WithField("open_connections", openConnections).
				WithField("min_connections", s.minOpenConnections).
				Info("polled open connections")
		}
		time.Sleep(s.checkInterval)
	}
}

// shutdownEnvoy sends a POST request to /healthcheck/fail to tell Envoy to start draining connections
func shutdownEnvoy(adminAddress string) error {
	httpClient := http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", adminAddress)
			},
		},
	}
	/* #nosec */
	resp, err := httpClient.Post(healthcheckFailURL, "", nil)
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
func getOpenConnections(adminAddress string) (int, error) {
	httpClient := http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", adminAddress)
			},
		},
	}

	// Make request to Envoy Prometheus endpoint
	/* #nosec */
	resp, err := httpClient.Get(prometheusURL)
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
			switch labels.GetValue() {
			// don't count connections to these listeners.
			case "admin", "envoy-admin", "stats", "health", "stats-health":
			default:
				openConnections += int(metrics.Gauge.GetValue())
			}
		}
	}
	return openConnections, nil
}

func doShutdownManager(config *shutdownmanagerContext) {
	config.Info("started envoy shutdown manager")

	http.HandleFunc("/healthz", config.healthzHandler)
	http.HandleFunc("/shutdown", config.shutdownReadyHandler)

	// Fails gosec G114: Use of net/http serve function that has no support for setting timeouts
	// nolint:gosec
	if err := http.ListenAndServe(fmt.Sprintf(":%d", config.httpServePort), nil); err != http.ErrServerClosed {
		log.Fatal(err)
	}
	config.Info("stopped")
}

// registerShutdownManager registers the envoy shutdown-manager sub-command and flags
func registerShutdownManager(cmd *kingpin.CmdClause, log logrus.FieldLogger) (*kingpin.CmdClause, *shutdownmanagerContext) {
	ctx := newShutdownManagerContext()
	ctx.FieldLogger = log.WithField("context", "shutdown-manager")

	shutdownmgr := cmd.Command("shutdown-manager", "Start envoy shutdown-manager.")
	shutdownmgr.Flag("ready-file", "File to poll while waiting shutdown to be completed.").Default(shutdownReadyFile).StringVar(&ctx.shutdownReadyFile)
	shutdownmgr.Flag("serve-port", "Port to serve the http server on.").IntVar(&ctx.httpServePort)

	return shutdownmgr, ctx
}

// registerShutdown registers the envoy shutdown sub-command and flags
func registerShutdown(cmd *kingpin.CmdClause, log logrus.FieldLogger) (*kingpin.CmdClause, *shutdownContext) {
	ctx := newShutdownContext()
	ctx.FieldLogger = log.WithField("context", "shutdown")

	shutdown := cmd.Command("shutdown", "Initiate an shutdown sequence which configures Envoy to begin draining connections.")
	shutdown.Flag("admin-address", "Envoy admin interface address.").Default("/admin/admin.sock").StringVar(&ctx.adminAddress)
	shutdown.Flag("admin-port", "DEPRECATED: Envoy admin interface port.").IntVar(&ctx.adminPort)
	shutdown.Flag("check-delay", "Time to wait before polling Envoy for open connections.").Default("0s").DurationVar(&ctx.checkDelay)
	shutdown.Flag("check-interval", "Time to poll Envoy for open connections.").DurationVar(&ctx.checkInterval)
	shutdown.Flag("drain-delay", "Time to wait before draining Envoy connections.").Default("0s").DurationVar(&ctx.drainDelay)
	shutdown.Flag("min-open-connections", "Min number of open connections when polling Envoy.").IntVar(&ctx.minOpenConnections)
	shutdown.Flag("ready-file", "File to write when shutdown is completed.").Default(shutdownReadyFile).StringVar(&ctx.shutdownReadyFile)

	return shutdown, ctx
}

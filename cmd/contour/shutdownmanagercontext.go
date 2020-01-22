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
	"time"

	"github.com/sirupsen/logrus"
)

type shutdownmanagerContext struct {
	// checkInterval defines time delay between polling Envoy for open connections
	checkInterval time.Duration

	// checkDelay defines time to wait before polling Envoy for open connections
	checkDelay time.Duration

	// minOpenConnections defines the minimum amount of connections
	// that can be open when polling for active connections in Envoy
	minOpenConnections int

	// httpServePort defines the port to serve the http server on
	httpServePort int

	// prometheusPath defines the path to query Envoy's Prometheus http Endpoint
	prometheusPath string

	// prometheusStat defines the stat to query for in the /stats/prometheus endpoint
	prometheusStat string

	// prometheusValues defines the values to query for in the prometheusStat
	prometheusValues []string

	envoyHost string
	envoyPort int

	logrus.FieldLogger
}

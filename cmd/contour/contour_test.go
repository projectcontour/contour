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
	"sort"
	"testing"

	"github.com/alecthomas/kingpin/v2"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func assertOptionFlagsAreSorted(t *testing.T, cmd *kingpin.CmdClause) {
	var flags []string

	for _, v := range cmd.Model().FlagGroupModel.Flags {
		flags = append(flags, v.Name)
	}
	assert.Truef(t, sort.StringsAreSorted(flags), "the flags for subcommand %q aren't sorted: %v", cmd.Model().Name, flags)
}

func TestOptionFlagsAreSorted(t *testing.T) {
	app := kingpin.New("contour_option_flags_are_sorted", "Assert contour options are sorted")
	log := logrus.StandardLogger()

	bootstrap, _ := registerBootstrap(app)
	assertOptionFlagsAreSorted(t, bootstrap)

	certgen, _ := registerCertGen(app)
	assertOptionFlagsAreSorted(t, certgen)

	cli, _ := registerCli(app, log)
	assertOptionFlagsAreSorted(t, cli)

	envoyCmd := app.Command("envoy", "Sub-command for envoy actions.")

	sdmShutdown, _ := registerShutdown(envoyCmd, log)
	assertOptionFlagsAreSorted(t, sdmShutdown)

	sdm, _ := registerShutdownManager(envoyCmd, log)
	assertOptionFlagsAreSorted(t, sdm)

	gatewayProvisioner, _ := registerGatewayProvisioner(app)
	assertOptionFlagsAreSorted(t, gatewayProvisioner)

	serve, _ := registerServe(app)
	assertOptionFlagsAreSorted(t, serve)
}

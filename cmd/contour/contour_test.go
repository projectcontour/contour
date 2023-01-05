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

	"github.com/sirupsen/logrus"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
)

func testOptionFalgsAreSortedHelper(t *testing.T, cmd *kingpin.CmdClause) {
	var flags []string

	for _, v := range cmd.Model().FlagGroupModel.Flags {
		flags = append(flags, v.Name)
	}
	if !sort.StringsAreSorted(flags) {
		t.Errorf("the flags for subcommand: '%s' aren't sorted", cmd.Model().Name)

		sort.Strings(flags)
		for _, v := range flags {
			println(v)
		}
	}
}

func TestOptionFlagsAreSorted(t *testing.T) {
	app := kingpin.New("contour_option_flags_are_sorted", "Test contour options are sorted or not")

	bootstrap, _ := registerBootstrap(app)
	testOptionFalgsAreSortedHelper(t, bootstrap)

	certgen, _ := registerCertGen(app)
	testOptionFalgsAreSortedHelper(t, certgen)

	cli, _ := registerCli(app)
	testOptionFalgsAreSortedHelper(t, cli)

	envoyCmd := app.Command("envoy", "Sub-command for envoy actions.")
	log := logrus.StandardLogger()

	sdmShutdown, _ := registerShutdown(envoyCmd, log)
	testOptionFalgsAreSortedHelper(t, sdmShutdown)

	sdm, _ := registerShutdownManager(envoyCmd, log)
	testOptionFalgsAreSortedHelper(t, sdm)

	gatewayProvisioner, _ := registerGatewayProvisioner(app)
	testOptionFalgsAreSortedHelper(t, gatewayProvisioner)

	serve, _ := registerServe(app)
	testOptionFalgsAreSortedHelper(t, serve)
}

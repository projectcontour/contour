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

//go:build e2e

package e2e

import (
	"fmt"
	"io"
	"os/exec"
	"time"

	"github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
)

type Kubectl struct {
	// Command output is written to this writer.
	cmdOutputWriter io.Writer
}

func (k *Kubectl) StartKubectlPortForward(localPort, containerPort int, namespace, object string, additionalArgs ...string) (*gexec.Session, error) {
	args := append([]string{
		"port-forward",
		"-n",
		namespace,
		object,
		fmt.Sprintf("%d:%d", localPort, containerPort),
	}, additionalArgs...)

	session, err := gexec.Start(exec.Command("kubectl", args...), k.cmdOutputWriter, k.cmdOutputWriter) // nolint:gosec
	if err != nil {
		return nil, err
	}
	// Wait until port-forward to be up and running.
	gomega.Eventually(session).Should(gbytes.Say("Forwarding from"))
	return session, nil
}

func (k *Kubectl) StopKubectlPortForward(cmd *gexec.Session) {
	// Default timeout of 1s produces test flakes,
	// a minute should be more than enough to avoid them.
	cmd.Terminate().Wait(time.Minute)
}

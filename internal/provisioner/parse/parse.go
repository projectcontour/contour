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

package parse

import (
	// Import the hash implementations or this package will panic if
	// Contour or Envoy images reference a sha hash. See the following
	// for details: https://github.com/opencontainers/go-digest#usage
	_ "crypto/sha256"
	_ "crypto/sha512"
	"fmt"
	"os/exec"
	"strings"

	"github.com/docker/distribution/reference"
)

// Image parses s, returning and error if s is not a syntactically
// valid image reference. Image does not not handle short digests.
func Image(s string) error {
	_, err := reference.Parse(s)
	if err != nil {
		return fmt.Errorf("failed to parse s %s: %w", s, err)
	}

	return nil
}

// StringInPodExec parses the output of cmd for expectedString executed in the specified
// pod ns/name, returning an error if expectedString was not found.
func StringInPodExec(ns, name, expectedString string, cmd []string) error {
	cmdPath, err := exec.LookPath("kubectl")
	if err != nil {
		return err
	}
	args := []string{"exec", name, fmt.Sprintf("--namespace=%v", ns), "--"}
	args = append(args, cmd...)
	if _, err := lookForString(cmdPath, args, expectedString); err != nil {
		return err
	}
	return nil
}

// lookForString looks for the given string using cmd and args, returning
// true if the string was found.
func lookForString(cmd string, args []string, expectedString string) (bool, error) {
	result, err := runCmd(cmd, args)
	if err != nil {
		return false, err
	}
	if strings.Contains(result, expectedString) {
		return true, nil
	}
	return false, nil
}

// runCmd runs command cmd with arguments args and returns the output
// of the command or an error.
func runCmd(cmd string, args []string) (string, error) {
	for _, arg := range args {
		if strings.HasPrefix(arg, "/") || strings.HasPrefix(arg, ".") {
			return "", fmt.Errorf("invalid argument %q", arg)
		}
	}
	execCmd := exec.Command(cmd, args...)
	result, err := execCmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to run command %q with args %q: %v", cmd, args, err)
	}
	return string(result), nil
}

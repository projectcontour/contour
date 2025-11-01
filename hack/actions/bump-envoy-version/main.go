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

//go:build none

// Updates all references to the Envoy version in the repository and generates a template for changelog entry.
//
// Usage:
//
//	go run ./hack/actions/bump-envoy-version/main.go
//
// By default, the script updates to the latest patch release for the current minor version.
// To target a specific major or minor version:
//
//	go run ./hack/actions/bump-envoy-version/main.go distroless-v1.35
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"regexp"
	"strings"

	"github.com/sirupsen/logrus"
)

var (
	log = logrus.StandardLogger()

	filesToPatch = []string{
		"Makefile", // Envoy version was in Makefile in older code.
		"cmd/contour/gatewayprovisioner.go",
		"examples/contour/03-envoy.yaml",
		"examples/deployment/03-envoy-deployment.yaml",
	}
)

func main() {
	log.SetFormatter(&logrus.TextFormatter{ForceColors: true})

	currentVersion := getCurrentEnvoyVersion()

	// releaseTrack is the Envoy major or minor version to track, e.g. "v1.36".
	var releaseTrack string
	if len(os.Args) < 2 {
		// If no argument is given, derive the release track from the current minor version.
		releaseTrack = currentVersion[:strings.LastIndex(currentVersion, ".")]
	} else {
		releaseTrack = os.Args[1]
	}

	log.Infof("Current Envoy version: %s", currentVersion)

	// Strip "distroless" prefix for GitHub API lookup but use it for file updates.
	var imagePrefix string
	if isDistroless := strings.HasPrefix(currentVersion, "distroless-"); isDistroless {
		releaseTrack = strings.TrimPrefix(releaseTrack, "distroless-")
		imagePrefix = "distroless-"
	}

	latestVersion := getLatestEnvoyVersion(releaseTrack)
	log.Infof("Latest version: %s", latestVersion)

	updateFiles(imagePrefix + latestVersion)
	changelogFile := createChangelogTemplate(latestVersion)

	log.Info("Envoy version update completed")
	log.Info("Update following files manually (in main branch):")
	log.Info("- site/content/resources/compatibility-matrix.md")
	log.Info("- versions.yaml")
	log.Infof("- %s (only needed if doing bump in main branch)", changelogFile)
}

func getCurrentEnvoyVersion() string {
	content, err := os.ReadFile("examples/contour/03-envoy.yaml")
	if err != nil {
		log.WithError(err).Fatal("Failed to determine current version")
	}

	envoyImageRe := regexp.MustCompile(`docker\.io/envoyproxy/envoy:(distroless-)?v([0-9]+\.[0-9]+\.[0-9]+)`)
	matches := envoyImageRe.FindStringSubmatch(string(content))
	if len(matches) < 3 {
		log.Fatal("Failed to match current version in examples/contour/03-envoy.yaml")
	}

	return matches[1] + "v" + matches[2]
}

func getLatestEnvoyVersion(track string) string {
	resp, err := http.Get("https://api.github.com/repos/envoyproxy/envoy/releases")
	if err != nil {
		log.WithError(err).Fatal("Failed to fetch releases from GitHub API")
	}
	defer resp.Body.Close()

	var releases []struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		log.WithError(err).Fatal("Failed to parse releases in GitHub API response")
	}

	prefix := track + "."
	for _, r := range releases {
		if strings.HasPrefix(r.TagName, prefix) {
			return r.TagName
		}
	}
	log.WithField("track", track).Fatal("No release found for track")
	return ""
}

func updateFiles(version string) {
	envoyImageRe := regexp.MustCompile(`docker\.io/envoyproxy/envoy:(distroless-)?v[0-9]+\.[0-9]+\.[0-9]+`)

	for _, file := range filesToPatch {
		content, err := os.ReadFile(file)
		if err != nil {
			log.WithError(err).WithField("file", file).Fatal("Failed to read file")
		}

		updated := envoyImageRe.ReplaceAllString(string(content), "docker.io/envoyproxy/envoy:"+version)

		if updated != string(content) {
			if err := os.WriteFile(file, []byte(updated), 0o600); err != nil {
				log.WithError(err).WithField("file", file).Fatal("Failed to write file")
			}
			log.Infof("Updated file: %s", file)
		}
	}

	log.Info("Running 'make generate' to update generated files")
	cmd := exec.Command("make", "generate")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		log.WithError(err).Fatal("Failed to run 'make generate'")
	}
}

func createChangelogTemplate(version string) string {
	u, err := user.Current()
	if err != nil {
		log.WithError(err).Fatal("Failed to get current user")
	}

	file := fmt.Sprintf("changelogs/unreleased/dddd-%s-small.md", u.Username)
	majorMinor := version[:strings.LastIndex(version, ".")]
	url := fmt.Sprintf("https://www.envoyproxy.io/docs/envoy/%s/version_history/%s/%s", version, majorMinor, version)
	content := fmt.Sprintf("Updates Envoy to %s. See the [Envoy release notes](%s) for more information about the content of the release.\n", version, url)

	if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
		log.WithError(err).Fatal("Failed to write changelog")
	}
	log.Infof("Created changelog template: %s", file)

	return file
}

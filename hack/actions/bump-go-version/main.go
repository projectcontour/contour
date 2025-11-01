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

// Updates all references to the Go version in the repository and creates a template for changelog entry.
//
// Usage:
//
//	go run ./hack/actions/bump-go-version/main.go
//
// By default, the script updates to the latest patch release for the current minor version.
// To target a specific major or minor version:
//
//	go run ./hack/actions/bump-go-version/main.go 1.25
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
		"Makefile",
		".github/workflows/build_daily.yaml",
		".github/workflows/build_tag.yaml",
		".github/workflows/codeql-analysis.yml",
		".github/workflows/prbuild.yaml",
	}
)

func main() {
	log.SetFormatter(&logrus.TextFormatter{ForceColors: true})

	currentVersion := getCurrentGoVersion()

	// releaseTrack is the Go major or minor version to track, e.g. "1.25".
	var releaseTrack string
	if len(os.Args) < 2 {
		// If no argument is given, derive the release track from the current minor version.
		releaseTrack = currentVersion[:strings.LastIndex(currentVersion, ".")]
	} else {
		releaseTrack = os.Args[1]
	}

	log.Infof("Current Go version: %s", currentVersion)

	latestVersion := getLatestGoVersionByReleaseTrack(releaseTrack)
	log.Infof("Latest version: %s", latestVersion)

	latestImageHash := getGolangImageHash(latestVersion)
	log.Infof("Image hash: %s", latestImageHash)

	updateFiles(latestVersion, latestImageHash)
	createChangelogTemplate(latestVersion)

	log.Info("Go version update completed")
}

func getCurrentGoVersion() string {
	content, err := os.ReadFile("Makefile")
	if err != nil {
		log.WithError(err).Fatal("Failed to determine current version")
	}

	buildImageRe := regexp.MustCompile(`BUILD_BASE_IMAGE\s*\?=\s*golang:([0-9.]+)`)
	matches := buildImageRe.FindStringSubmatch(string(content))
	if len(matches) < 2 {
		log.Fatal("Failed to match current version in Makefile")
	}

	return matches[1]
}

func getLatestGoVersionByReleaseTrack(track string) string {
	resp, err := http.Get("https://go.dev/dl/?mode=json&include=all")
	if err != nil {
		log.WithError(err).Fatal("Failed to fetch releases from go.dev API")
	}
	defer resp.Body.Close()

	var releases []struct{ Version string }
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		log.WithError(err).Fatal("Failed to parse releases in go.dev API response")
	}

	prefix := "go" + track
	for _, r := range releases {
		if strings.HasPrefix(r.Version, prefix) {
			return r.Version
		}
	}
	log.WithField("track", track).Fatal("No release found for track")
	return ""
}

func getGolangImageHash(version string) string {
	tag := strings.TrimPrefix(version, "go")
	url := fmt.Sprintf("https://registry.hub.docker.com/v2/repositories/library/golang/tags/%s", tag)

	resp, err := http.Get(url) // #nosec G107: Potential HTTP request made with variable url
	if err != nil {
		log.WithError(err).Fatal("Failed to fetch image hash from Docker Hub")
	}
	defer resp.Body.Close()

	var info struct{ Digest string }
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		log.WithError(err).Fatal("Failed to parse tag info from Docker Hub API response")
	}

	if info.Digest == "" {
		log.WithField("version", version).Fatal("No image found for version")
	}
	return info.Digest
}

func updateFiles(version, hash string) {
	ver := strings.TrimPrefix(version, "go")
	buildImageRegexp := regexp.MustCompile(`(BUILD_BASE_IMAGE\s*\?=\s*golang:)[0-9.]+(@sha256:[a-f0-9]{64})?`)
	goVersionRegexp := regexp.MustCompile(`(GO_VERSION:\s*)[0-9.]+`)

	for _, file := range filesToPatch {
		content, err := os.ReadFile(file)
		if err != nil {
			log.WithError(err).WithField("file", file).Fatal("Failed to read file")
		}

		updated := buildImageRegexp.ReplaceAllString(string(content), fmt.Sprintf("${1}%s@%s", ver, hash))
		updated = goVersionRegexp.ReplaceAllString(updated, "${1}"+ver)

		if updated != string(content) {
			if err := os.WriteFile(file, []byte(updated), 0o600); err != nil {
				log.WithError(err).WithField("file", file).Fatal("Failed to write file")
			}
			log.Infof("Updated file: %s", file)
		}
	}

	log.Info("Running 'go mod tidy' to update module files")
	cmd := exec.Command("go", "mod", "tidy")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		log.WithError(err).Fatal("Failed to run 'go mod tidy'")
	}
}

func createChangelogTemplate(version string) {
	u, err := user.Current()
	if err != nil {
		log.WithError(err).Fatal("Failed to get current user")
	}

	file := fmt.Sprintf("changelogs/unreleased/nnnn-%s-small.md", u.Username)
	parts := strings.SplitN(strings.TrimPrefix(version, "go"), ".", 3)
	url := fmt.Sprintf("https://go.dev/doc/devel/release#go%s.%s.0", parts[0], parts[1])
	content := fmt.Sprintf("Updates Go to %s. See the [Go release notes](%s) for more information about the content of the release.\n", version, url)

	if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
		log.WithError(err).Fatal("Failed to write changelog")
	}
	log.Infof("Created changelog template: %s", file)
}

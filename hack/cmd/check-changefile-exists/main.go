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

// Versioning comment for rerunning jobs.
// check-changefile-exists.go
//
// Checks that the required changelog file exists in the
// changelogs/unreleased directory.
package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/google/go-github/v48/github"
	"github.com/sirupsen/logrus"
	"golang.org/x/oauth2"
)

func main() {
	log := logrus.StandardLogger()
	// Forcing colors makes the output nicer to read,
	// and allows multiline strings to work properly.
	log.SetFormatter(&logrus.TextFormatter{
		ForceColors: true,
	})

	logFriendlyError := func(errorMessage string) {
		log.Fatal(fmt.Sprintf(`
Thanks for your PR.
For a PR to be accepted to Contour, it must have:
- at least one release-note label set
- a file named changelogs/unreleased/PR#-author-category,
  where category matches the release-note/category label you apply.

Error: %s
	
Please see the "Commit message and PR guidelines" section of CONTRIBUTING.md,
or https://github.com/projectcontour/contour/blob/main/design/changelog.md for background.`, errorMessage))
	}

	// We need a GITHUB_TOKEN and PR_NUMBER in the environment.
	// These are set by the Action config file
	// in .github/workflows/prbuild.yaml,
	// under the check-changelog step.
	token, ok := os.LookupEnv("GITHUB_TOKEN")
	if !ok {
		log.Fatal("No GITHUB_TOKEN set, check the Action config.")
	}
	prEnv, ok := os.LookupEnv("PR_NUMBER")
	if !ok {
		log.Fatal("No PR_NUMBER set, check the Action config.")
	}
	pr, err := strconv.Atoi(prEnv)
	if err != nil {
		log.Fatalf("Couldn't convert PR number, %s", err)
	}

	// We've got what we need, set up the Github client to get the
	// labels associated with the PR.
	ctx := context.Background()

	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(ctx, ts)

	client := github.NewClient(tc)

	prDetails, _, err := client.PullRequests.Get(ctx, "projectcontour", "contour", pr)
	if err != nil {
		log.Fatalf("Couldn't get PR details: %s", err)
	}

	if len(prDetails.Labels) == 0 {
		logFriendlyError("No labels set on PR")
	}

	changeLogFileName := func(category string) string {
		return fmt.Sprintf("./changelogs/unreleased/%d-%s-%s.md", pr, *prDetails.User.Login, category)
	}

	// Collect list of changelog files to check for.
	// Labels guaranteed to be unique and exist based on label check action.
	changelogFiles := []string{}
	for _, label := range prDetails.Labels {
		name := *label.Name
		if !strings.HasPrefix(name, "release-note/") {
			continue
		}

		var category string
		labelSplit := strings.Split(name, "/")
		if len(labelSplit) > 1 {
			category = strings.Join(labelSplit[1:], "/")
		}
		switch name {
		case "release-note/none-required":
			// Exit early if no changelog required.
			log.Println("No changelog required.")
			os.Exit(0)
		case "release-note/major", "release-note/minor", "release-note/small", "release-note/docs", "release-note/infra", "release-note/deprecation":
		default:
			logFriendlyError(fmt.Sprintf("Invalid release-note label category: %q", category))
		}

		changelogFiles = append(changelogFiles, changeLogFileName(category))
	}

	for _, f := range changelogFiles {
		changelogFile, err := os.Stat(f)

		if os.IsNotExist(err) {
			logFriendlyError("Missing changelog file at " + f)
		} else if err != nil {
			log.Fatal(err)
		}

		if changelogFile.Size() == 0 {
			logFriendlyError("Empty changelog file at " + f)
		}
	}

	os.Exit(0)
}

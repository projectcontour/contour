//go:build none
// +build none

// Versioning comment for rerunning jobs.
// check-changefile-exists.go
//
// Checks that the required changelog file exists in the
// changelogs/unreleased directory.
//
package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/google/go-github/v39/github"
	"github.com/sirupsen/logrus"
	"golang.org/x/oauth2"
)

func main() {

	log := logrus.StandardLogger()

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

	// No labels. This is most likely what people will see at first, so this message
	// is as friendly as I could make it.
	if len(prDetails.Labels) == 0 {
		log.Fatal(`
Thanks for your PR.
For PRs to be accepted to Contour, it must have:
- at least one release-note label set
- a file named changelogs/unreleased/PR#-author-category,
	where category matches the relase-note/category label you apply`)

	}

	// Try to determine the category of the PR.
	var category string
	for _, label := range prDetails.Labels {
		name := *label.Name
		if strings.HasPrefix(name, "release-note") {
			// In case the old release-note labels stick around, mark them
			// as "major" category.
			if name == "release-note" || name == "release-note-action-required" {
				category = "major"
				break
			}

			// Otherwise, extract the category.
			labelSplit := strings.Split(name, "/")
			if len(labelSplit) > 1 {
				category = labelSplit[1]
			}
		}
	}

	if category == "" {
		log.Fatal(`
Thanks for your PR.
For PRs to be accepted to Contour, it must have:
- at least one release-note label set
- a file named changelogs/unreleased/PR#-author-category,
  where category matches the relase-note/category label you apply.

There are some labels set, but there must be at least one release-note label.`)
	}

	// None required is the escape hatch for small changes.
	if category == "none-required" {
		log.Println("No changelog required.")
		os.Exit(0)
	}

	changelogFile, err := os.Stat(fmt.Sprintf("./changelogs/unreleased/%d-%s-%s.md",
		pr, *prDetails.User.Login, category))

	if os.IsNotExist(err) {
		log.Fatalf(`
Thanks for your PR.
For PRs to be accepted to Contour, it must have:
- at least one release-note label set
- a file named changelogs/unreleased/%d-%s-%s.md with a description of the change.`,
			pr, *prDetails.User.Login, category)
	}

	if changelogFile.Size() == 0 {
		log.Fatalf(`
Thanks for your PR.
For PRs to be accepted to Contour, it must have:
- at least one release-note label set
- a file named changelogs/unreleased/%d-%s-%s.md with a description of the change
- the file must not be empty.`,
			pr, *prDetails.User.Login, category)
	}

	os.Exit(0)
}

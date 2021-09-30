//go:build none
// +build none

// Versioning comment for rerunning jobs.

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/google/go-github/v39/github"
	"golang.org/x/oauth2"
)

func main() {

	token, ok := os.LookupEnv("GITHUB_TOKEN")
	if !ok {
		log.Fatal("No GITHUB_TOKEN set, exiting.")

	}

	prEnv, ok := os.LookupEnv("PR_NUMBER")
	if !ok {
		log.Fatal("No PR_NUMBEr set, exiting.")
	}

	pr, err := strconv.Atoi(prEnv)
	if err != nil {
		log.Fatalf("Couldn't convert PR number, %s", err)
	}

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

	// labelsJSON, err := json.Marshal(prDetails.Labels)
	// if err != nil {
	// 	fmt.Println(err)
	// 	os.Exit(1)
	// }
	if len(prDetails.Labels) == 0 {
		log.Fatal("Must have at least one release-note label set.")
	}

	var category string
	for _, label := range prDetails.Labels {
		name := *label.Name
		if strings.HasPrefix(name, "release-note") {
			if name == "release-note" || name == "release-note-action-required" {
				category = "major"
				break
			}

			labelSplit := strings.Split(name, "/")
			if len(labelSplit) > 1 {
				category = labelSplit[1]
			}
		}
	}

	if category == "" {
		log.Fatal("Labels present, but must have at least one release-note label set.")
	}

	if category == "none-required" {
		log.Println("No changelog required.")
		os.Exit(0)
	}

	log.Printf("PR file should be changelogs/%d-%s-%s.md", pr, *prDetails.User.Login, category)

	changelogFile, err := os.Stat(fmt.Sprintf("./changelogs/unreleased/%d-%s-%s.md", pr, *prDetails.User.Login, category))

	if os.IsNotExist(err) {
		log.Fatalf("changelogs/unreleased/%d-%s-%s.md must exist", pr, *prDetails.User.Login, category)
	}

	if changelogFile.Size() == 0 {
		log.Fatalf("changelogs/unreleased/%d-%s-%s.md must be non-empty", pr, *prDetails.User.Login, category)
	}

	os.Exit(0)
}

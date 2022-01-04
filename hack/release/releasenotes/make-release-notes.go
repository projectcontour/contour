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
// +build none

package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
)

type Entry struct {
	PRNumber string
	Author   string
	Content  string
}

type Data struct {
	Version              string
	Prerelease           bool
	Major                []Entry
	Minor                []Entry
	Small                []Entry
	Docs                 []Entry
	Contributors         []string
	KubernetesMinVersion string
	KubernetesMaxVersion string
}

const unreleasedChangelogDir = "../../../changelogs/unreleased"

const usageString = `USAGE: go run make-release-notes.go CONTOUR_VERSION KUBERNETES_MIN_VERSION KUBERNETES_MAX_VERSION

EXAMPLE: go run make-release-notes.go v1.20.0 1.20 1.22`

func main() {
	if len(os.Args) != 4 {
		fmt.Println(usageString)
		os.Exit(1)
	}

	d := Data{
		Version:              os.Args[1],
		KubernetesMinVersion: os.Args[2],
		KubernetesMaxVersion: os.Args[3],
	}

	if strings.Contains(d.Version, "alpha") || strings.Contains(d.Version, "beta") || strings.Contains(d.Version, "rc") {
		d.Prerelease = true
	}

	dirEntries, err := os.ReadDir(unreleasedChangelogDir)
	checkErr(err)

	for _, dirEntry := range dirEntries {
		if strings.HasSuffix(dirEntry.Name(), "-sample.md") {
			continue
		}

		nameParts := strings.Split(strings.TrimSuffix(dirEntry.Name(), ".md"), "-")

		if len(nameParts) != 3 {
			fmt.Printf("Skipping changelog file with invalid name format %q\n", dirEntry.Name())
			continue
		}

		contents, err := ioutil.ReadFile(filepath.Join(unreleasedChangelogDir, dirEntry.Name()))
		checkErr(err)

		entry := Entry{
			PRNumber: nameParts[0],
			Author:   "@" + nameParts[1],
			Content:  strings.TrimSpace(string(contents)),
		}

		switch strings.ToLower(nameParts[2]) {
		case "major":
			d.Major = append(d.Major, entry)
		case "minor":
			d.Minor = append(d.Minor, entry)
		case "small":
			entry.Content = strings.TrimSpace(entry.Content)
			d.Small = append(d.Small, entry)
		case "docs":
			d.Docs = append(d.Docs, entry)
		default:
			fmt.Printf("Unrecognized category %s\n", nameParts[2])
		}

		d.Contributors = recordContributor(d.Contributors, entry.Author)
	}

	sort.Strings(d.Contributors)

	tmpl, err := template.ParseFiles("release-notes-template.md")
	checkErr(err)

	f, err := os.Create("../../../changelogs/CHANGELOG-" + d.Version + ".md")
	checkErr(err)
	defer f.Close()

	tmpl.Execute(f, d)
}

func checkErr(err error) {
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

// recordContributor adds contributor to contributors if they
// are not already present and are not a maintainer.
func recordContributor(contributors []string, contributor string) []string {
	for _, existing := range contributors {
		if contributor == existing {
			return contributors
		}
	}

	for _, maintainer := range []string{"@skriss", "@stevesloka", "@sunjayBhatia", "@tsaarni", "@youngnick"} {
		if contributor == maintainer {
			return contributors
		}
	}

	return append(contributors, contributor)
}

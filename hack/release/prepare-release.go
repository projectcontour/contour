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

package main

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"golang.org/x/mod/semver"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

func must(err error) {
	if err != nil {
		log.Fatalf("%s", err.Error())
	}
}

func run(cmd []string) {
	// nolint:gosec
	c := exec.Command(cmd[0], cmd[1:]...)

	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr

	if err := c.Run(); err != nil {
		log.Fatal(err)
	}
}

func capture(cmd []string) string {
	out := bytes.Buffer{}
	// nolint:gosec
	c := exec.Command(cmd[0], cmd[1:]...)

	c.Stdin = nil
	c.Stdout = &out
	c.Stderr = &out

	if err := c.Run(); err != nil {
		log.Fatal(err)
	}

	return out.String()
}

func updateMappingForTOC(filePath string, vers string, toc string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	rn := yaml.MustParse(string(data))

	if _, err := rn.Pipe(
		yaml.FieldSetter{Name: vers, StringValue: toc}); err != nil {
		return err
	}

	return os.WriteFile(filePath, []byte(rn.MustString()), 0600)
}

// InsertAfter is like yaml.ElementAppender except it inserts after the named node.
type InsertAfter struct {
	After string
	Node  *yaml.Node
}

// Filter ...
func (a InsertAfter) Filter(rn *yaml.RNode) (*yaml.RNode, error) {
	if err := yaml.ErrorIfInvalid(rn, yaml.SequenceNode); err != nil {
		return nil, err
	}

	content := make([]*yaml.Node, 0)

	for _, node := range rn.YNode().Content {
		content = append(content, node)
		if node.Value == a.After {
			content = append(content, a.Node)
		}
	}

	rn.YNode().Content = content
	return rn, nil
}

// InsertBefore is like yaml.ElementAppender except it inserts before the named node.
type InsertBefore struct {
	Before string
	Node   *yaml.Node
}

// Filter ...
func (a InsertBefore) Filter(rn *yaml.RNode) (*yaml.RNode, error) {
	if err := yaml.ErrorIfInvalid(rn, yaml.SequenceNode); err != nil {
		return nil, err
	}

	content := make([]*yaml.Node, 0)

	for _, node := range rn.YNode().Content {
		if node.Value == a.Before {
			content = append(content, a.Node)
		}
		content = append(content, node)
	}

	rn.YNode().Content = content
	return rn, nil
}

func updateConfigForSite(filePath string, oldVers, vers string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	rn := yaml.MustParse(string(data))

	// Set params.latest_version to vers if it is the latest according
	// to semver.
	latest, err := rn.GetFieldValue("params.latest_version")
	if err != nil {
		return err
	}

	latestVers, ok := latest.(string)
	if !ok {
		return fmt.Errorf("could not convert params.latest version to string: %v", latest)
	}

	if semver.Compare(vers, latestVers) > 0 {
		if _, err := rn.Pipe(
			yaml.Lookup("params"),
			yaml.FieldSetter{Name: "latest_version", StringValue: vers},
		); err != nil {
			return err
		}
	}

	// Insert vers into params.docs_versions in the correct order.
	versNode := yaml.Node{
		Kind:  yaml.ScalarNode,
		Value: vers,
	}

	var insertVersion yaml.Filter
	if oldVers == "main" {
		insertVersion = InsertAfter{
			After: "main",
			Node:  &versNode,
		}
	} else {
		insertVersion = InsertBefore{
			Before: oldVers,
			Node:   &versNode,
		}
	}

	if _, err := rn.Pipe(
		yaml.Lookup("params", "docs_versions"),
		insertVersion,
	); err != nil {
		log.Fatalf("%s", err)
	}

	return os.WriteFile(filePath, []byte(rn.MustString()), 0600)
}

func updateIndexFile(filePath, oldVers, newVers string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	upd := strings.ReplaceAll(string(data), "version: "+oldVers, "version: "+newVers)

	return os.WriteFile(filePath, []byte(upd), 0600)
}

func main() {
	var (
		oldVers     = "main"
		newVers     = ""
		kubeMinVers = ""
		kubeMaxVers = ""
	)

	switch len(os.Args) {
	case 4:
		newVers = os.Args[1]
		kubeMinVers = os.Args[2]
		kubeMaxVers = os.Args[3]
	case 5:
		oldVers = os.Args[1]
		newVers = os.Args[2]
		kubeMinVers = os.Args[3]
		kubeMaxVers = os.Args[4]
	default:
		fmt.Printf("Usage: %s NEWVERS KUBEMINVERS KUBEMAXVERS | OLDVERS NEWVERS KUBEMINVERS KUBEMAXVERS\n", path.Base(os.Args[0]))
		os.Exit(1)
	}

	log.Printf("Verifying repository state ...")

	status := capture([]string{"git", "status", "--short"})
	for _, line := range strings.Split(status, "\n") {
		// See https://git-scm.com/docs/git-status#_short_format.
		if strings.ContainsAny(line, "MADRCU") {
			log.Fatal("uncommitted changes in repository")
		}
	}

	if !isPrereleaseVersion(newVers) {
		log.Printf("Cloning versioned documentation ...")

		// Jekyll hates it when the TOC file name contains a dot.
		tocName := strings.ReplaceAll(fmt.Sprintf("%s-toc", newVers), ".", "-")
		oldTocName := strings.ReplaceAll(fmt.Sprintf("%s-toc", oldVers), ".", "-")

		// Make a versioned copy of the oldVers docs.
		run([]string{"cp", "-r", fmt.Sprintf("site/content/docs/%s", oldVers), fmt.Sprintf("site/content/docs/%s", newVers)})
		run([]string{"git", "add", fmt.Sprintf("site/content/docs/%s", newVers)})

		// Update site/content/docs/<newVers>/_index.md content.
		must(updateIndexFile(fmt.Sprintf("site/content/docs/%s/_index.md", newVers), oldVers, newVers))
		run([]string{"git", "add", fmt.Sprintf("site/content/docs/%s/_index.md", newVers)})

		// Make a versioned TOC for the docs.
		run([]string{"cp", "-r", fmt.Sprintf("site/data/docs/%s.yml", oldTocName), fmt.Sprintf("site/data/docs/%s.yml", tocName)})
		run([]string{"git", "add", fmt.Sprintf("site/data/docs/%s.yml", tocName)})

		// Insert the versioned TOC.
		must(updateMappingForTOC("site/data/docs/toc-mapping.yml", newVers, tocName))
		run([]string{"git", "add", "site/data/docs/toc-mapping.yml"})

		// Insert the versioned docs into the main site layout.
		must(updateConfigForSite("site/config.yaml", oldVers, newVers))
		run([]string{"git", "add", "site/config.yaml"})

		// Now commit everything
		run([]string{"git", "commit", "-s", "-m", fmt.Sprintf("Prepare documentation site for %s release.", newVers)})
	}

	must(generateReleaseNotes(newVers, kubeMinVers, kubeMaxVers))
	run([]string{"git", "add", "changelogs/*"})
	run([]string{"git", "commit", "-s", "-m", fmt.Sprintf("Add changelog for %s release.", newVers)})
}

func isPatchRelease(version string) bool {
	parts := strings.Split(version, ".")
	if len(parts) < 3 {
		panic("invalid version string")
	}

	return !strings.HasPrefix(parts[2], "0")
}

func generateReleaseNotes(version, kubeMinVersion, kubeMaxVersion string) error {
	d := Data{
		Version:              version,
		Prerelease:           isPrereleaseVersion(version),
		KubernetesMinVersion: kubeMinVersion,
		KubernetesMaxVersion: kubeMaxVersion,
	}

	var deletions []string

	// If a patch release, generate an empty changelog to be filled
	// in manually.
	if !isPatchRelease(version) {
		dirEntries, err := os.ReadDir("changelogs/unreleased")
		if err != nil {
			return err
		}

		for _, dirEntry := range dirEntries {
			if strings.HasSuffix(dirEntry.Name(), "-sample.md") {
				continue
			}

			entry, err := parseChangelogFilename(dirEntry.Name())
			if err != nil {
				fmt.Printf("Skipping changelog file: %v\n", err)
				continue
			}

			contents, err := os.ReadFile(filepath.Join("changelogs", "unreleased", dirEntry.Name()))
			if err != nil {
				return fmt.Errorf("error reading file %s: %v", filepath.Join("changelogs", "unreleased", dirEntry.Name()), err)
			}

			entry.Content = strings.TrimSpace(string(contents))

			switch strings.ToLower(entry.Category) {
			case "major":
				d.Major = append(d.Major, entry)
			case "minor":
				d.Minor = append(d.Minor, entry)
			case "small":
				d.Small = append(d.Small, entry)
			case "docs":
				d.Docs = append(d.Docs, entry)
			case "deprecation":
				d.Deprecation = append(d.Deprecation, entry)
			default:
				fmt.Printf("Unrecognized category %q\n", entry.Category)
				continue
			}

			d.Contributors = recordContributor(d.Contributors, entry.Author)

			// If a prerelease, don't delete the individual changelog
			// files since we want to keep them around for the GA release
			// notes.
			if !d.Prerelease {
				deletions = append(deletions, filepath.Join("changelogs", "unreleased", dirEntry.Name()))
			}
		}

		sort.Strings(d.Contributors)
	}

	tmpl, err := template.ParseFiles("hack/release/release-notes-template.md")
	if err != nil {
		return fmt.Errorf("error parsing release notes template: %v", err)
	}

	f, err := os.Create("changelogs/CHANGELOG-" + d.Version + ".md")
	if err != nil {
		return fmt.Errorf("error creating changelog file: %v", err)
	}
	defer f.Close()

	if err := tmpl.Execute(f, d); err != nil {
		return fmt.Errorf("error executing template: %v", err)
	}

	for _, deletion := range deletions {
		if err := os.Remove(deletion); err != nil {
			fmt.Printf("Error deleting changelog file %s: %v. Remove manually.\n", deletion, err)
		}
	}

	return nil
}

func isPrereleaseVersion(version string) bool {
	return strings.Contains(version, "alpha") || strings.Contains(version, "beta") || strings.Contains(version, "rc")
}

func parseChangelogFilename(filename string) (Entry, error) {
	parts := strings.Split(strings.TrimSuffix(filename, ".md"), "-")

	// We may have more than 3 parts if the GitHub username itself
	// contains a '-'.
	if len(parts) < 3 {
		return Entry{}, fmt.Errorf("invalid name format %q", filename)
	}

	return Entry{
		PRNumber: parts[0],
		Author:   "@" + strings.Join(parts[1:len(parts)-1], "-"),
		Category: parts[len(parts)-1],
	}, nil
}

// recordContributor adds contributor to contributors if they
// are not a maintainer and not already present.
func recordContributor(contributors []string, contributor string) []string {
	if _, found := maintainers[contributor]; found {
		return contributors
	}

	for _, existing := range contributors {
		if contributor == existing {
			return contributors
		}
	}

	return append(contributors, contributor)
}

var maintainers = map[string]bool{
	"@skriss":       true,
	"@stevesloka":   true,
	"@sunjayBhatia": true,
	"@tsaarni":      true,
}

type Entry struct {
	PRNumber string
	Author   string
	Content  string
	Category string
}

type Data struct {
	Version              string
	Prerelease           bool
	Major                []Entry
	Minor                []Entry
	Small                []Entry
	Docs                 []Entry
	Deprecation          []Entry
	Contributors         []string
	KubernetesMinVersion string
	KubernetesMaxVersion string
}

//go:build none
// +build none

package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"sigs.k8s.io/kustomize/kyaml/yaml"
)

func must(err error) {
	if err != nil {
		log.Fatalf("%s", err.Error())
	}
}

func run(cmd []string) {
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
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return err
	}

	rn := yaml.MustParse(string(data))

	if _, err := rn.Pipe(
		yaml.FieldSetter{Name: vers, StringValue: toc}); err != nil {
		return err
	}

	return ioutil.WriteFile(filePath, []byte(rn.MustString()), 0644)
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

func updateConfigForSite(filePath string, vers string) error {
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return err
	}

	rn := yaml.MustParse(string(data))

	// Set params.latest_version to the provided version.
	if _, err := rn.Pipe(
		yaml.Lookup("params"),
		yaml.FieldSetter{Name: "latest_version", StringValue: vers},
	); err != nil {
		return err
	}

	versNode := yaml.Node{
		Kind:  yaml.ScalarNode,
		Value: vers,
	}

	// Add the new version to the params.docs_versions array.
	// We insert after the "main" element so that it stays in
	// order.
	if _, err := rn.Pipe(
		yaml.Lookup("params", "docs_versions"),
		InsertAfter{After: "main", Node: &versNode},
	); err != nil {
		log.Fatalf("%s", err)
	}

	return ioutil.WriteFile(filePath, []byte(rn.MustString()), 0644)
}

func updateIndexFile(filePath, oldVers, newVers string) error {
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return err
	}

	upd := strings.ReplaceAll(string(data), "version: "+oldVers, "version: "+newVers)

	return ioutil.WriteFile(filePath, []byte(upd), 0644)
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
	must(updateConfigForSite("site/config.yaml", newVers))
	run([]string{"git", "add", "site/config.yaml"})

	// Now commit everything
	run([]string{"git", "commit", "-s", "-m", fmt.Sprintf("Prepare documentation site for %s release.", newVers)})

	must(generateReleaseNotes(newVers, kubeMinVers, kubeMaxVers))
	run([]string{"git", "add", "changelogs/*"})
	run([]string{"git", "commit", "-s", "-m", fmt.Sprintf("Add changelog for %s release.", newVers)})
}

func generateReleaseNotes(version, kubeMinVersion, kubeMaxVersion string) error {
	d := Data{
		Version:              version,
		KubernetesMinVersion: kubeMinVersion,
		KubernetesMaxVersion: kubeMaxVersion,
	}

	if strings.Contains(d.Version, "alpha") || strings.Contains(d.Version, "beta") || strings.Contains(d.Version, "rc") {
		d.Prerelease = true
	}

	dirEntries, err := os.ReadDir("changelogs/unreleased")
	if err != nil {
		return err
	}

	for _, dirEntry := range dirEntries {
		if strings.HasSuffix(dirEntry.Name(), "-sample.md") {
			continue
		}

		nameParts := strings.Split(strings.TrimSuffix(dirEntry.Name(), ".md"), "-")

		if len(nameParts) != 3 {
			fmt.Printf("Skipping changelog file with invalid name format %q\n", dirEntry.Name())
			continue
		}

		contents, err := ioutil.ReadFile(filepath.Join("changelogs", "unreleased", dirEntry.Name()))
		if err != nil {
			return fmt.Errorf("error reading file %s: %v", filepath.Join("changelogs", "unreleased", dirEntry.Name()), err)
		}

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

	tmpl, err := template.ParseFiles("hack/release/release-notes-template.md")
	if err != nil {
		return fmt.Errorf("error parsing release notes template: %v", err)
	}

	f, err := os.Create("changelogs/CHANGELOG-" + d.Version + ".md")
	if err != nil {
		return fmt.Errorf("error creating changelog file: %v", err)
	}
	defer f.Close()

	return tmpl.Execute(f, d)
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

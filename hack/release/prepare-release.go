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
	"strings"

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

	scopeNode := yaml.MustParse(
		fmt.Sprintf(`
scope:
  path: docs/%s
values:
  version: %s
  layout: "docs"
`, vers, vers))

	rn := yaml.MustParse(string(data))

	// Append the new scope to the "defaults" array.
	if _, err := rn.Pipe(
		yaml.Lookup("defaults"),
		yaml.Append(scopeNode.YNode()),
	); err != nil {
		log.Fatalf("%s", err)
	}

	// Set this version to the "latest" value.
	if _, err := rn.Pipe(
		yaml.FieldSetter{Name: "latest", StringValue: vers}); err != nil {
		return err
	}

	versNode := yaml.Node{
		Kind:  yaml.ScalarNode,
		Value: vers,
	}

	// Insert the new scope to the "defaults" array. We insert
	// after the "master" element so that it stays in order.
	if _, err := rn.Pipe(
		yaml.Lookup("versions"),
		InsertAfter{After: "master", Node: &versNode},
	); err != nil {
		log.Fatalf("%s", err)
	}

	return ioutil.WriteFile(filePath, []byte(rn.MustString()), 0644)
}

func main() {
	if len(os.Args) != 2 {
		fmt.Printf("Usage: %s NEWVERS\n",
			path.Base(os.Args[0]))
		os.Exit(1)
	}

	newVers := os.Args[1]

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

	// Make a versioned copy of the amster docs.
	run([]string{"cp", "-r", "site/docs/master", fmt.Sprintf("site/docs/%s", newVers)})
	run([]string{"git", "add", fmt.Sprintf("site/docs/%s", newVers)})

	// Make a versioned TOC for the docs.
	run([]string{"cp", "-r", "site/_data/master-toc.yml", fmt.Sprintf("site/_data/%s.yml", tocName)})
	run([]string{"git", "add", fmt.Sprintf("site/_data/%s.yml", tocName)})

	// Insert the versioned TOC.
	must(updateMappingForTOC("site/_data/toc-mapping.yml", newVers, tocName))
	run([]string{"git", "add", "site/_data/toc-mapping.yml"})

	// Insert the versioned docs into the main site layout.
	must(updateConfigForSite("site/_config.yml", newVers))
	run([]string{"git", "add", "site/_config.yml"})

	// Now commit everything
	run([]string{"git", "commit", "-s", "-m", fmt.Sprintf("Prepare documentation site for %s release.", newVers)})
}

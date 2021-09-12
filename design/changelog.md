# Add change tracking to Contour workflow

Status: Accepted

## Abstract
This design proposes adding a series of standards around recording change notes for changes as we make them.
The intent is to make the process of building the release changelog faster and easier.

## Background
At the time of writing, creating Contour's release notes takes at least a day, sometimes two.
Whoever is in charge of a given release must carefully check through all the PRs merged since the last release, write up a change summary for each, and get all of that into Markdown, ready to be put into the Github Release.
The Contour team has got feedback on a few occasions that our current release notes are excellent, and we want to keep that up, but it should not take so long.

## Goals
- Make the process of generating release notes easier and more automatable, and ensure it's followed.
- The person authoring the change should include a release note
- Storing the Changelog inside the Repo would be nice

## Non Goals
- Fully automate this, just help with generating release note Markdown is okay

## Background research

As part of pulling this together, I looked into a few other projects that are either similar to Contour or are dependencies.

### Envoy

Envoy has a /docs/root/version_history directory, with a separate file for each version. Each PR is expected to include an update to this file (which is in RST format, like the rest of their docs).

#### Sections:

- Incompatible Behavior Changes: Changes that are expected to cause an incompatibility if applicable; deployment changes are likely required
- Minor Behavior Changes: Changes that may cause incompatibilities for some users, but should not for most
- Bug Fixes: Changes expected to improve the state of the world and are unlikely to have negative effects
- Removed Config or Runtime: Normally occurs at the end of the :ref:`deprecation period <deprecated>`
- New Features
- Deprecated

### Velero

The repo contains a `/changelogs` directory, which itself contains Markdown files that are copies of the release notes.

The `/changelogs/unreleased` directory contains individual text files named PR#-PRauthor with the details of the PR.
Most changelogs appear to be a single line.

There is a small bash script to put these files together into a markdown bulleted list.

### Kubernetes

PRs include a one-line summary of the change, which makes sense given the huge scope of change in any Kubernetes release.

### Sonobuoy

Release notes look to be based on the `git shortlog` command.
The final notes use the short commit hash rather than the PR number.

## High-Level Design
Contour should implement a system taking parts from Envoy and Velero, that stores completed changelogs in a `/changelogs` directory,
and the current unreleased set in a `/changelogs/unreleased` directory, in Markdown format.

Like Velero, the filename will indicate the PR number and author name associated, but also will include a category (similar to the Envoy changelog).
Contour already separates our changelogs into sections, so I think that this is a good fit.

The implementation will include a CI check that verifies that a file is present with the correct details and is not empty.
This will remind us all to include the file and the change notes.

## Detailed Design

We'll do the following:

* Add a `/changelogs` directory and a `/changelogs/unreleased` directory.
* The `unreleased` directory will contains a set of Markdown files with names PR#-author-category.md
* On release, the files are concatenated to make the changelog, which is then edited and saved into `/changelogs` as `CHANGELOG-1.xx.x.md`.
* one extra magic field in the file name - an category descriptor that will allow us to put the markdown into the right place.
* Five categories:
  * Major - major changes and themes for the release. These are the ones that have more than one small paragraph (3-4 lines) of explanation/change notes, and might include examples or other detailed description.
  Breaking changes should always be in here, because we should be explaining them fully when we make them.
  * Minor - smaller changes that have at most one small paragraph of explanation.
  * Small - a single line description, basically the first line of the commit.
  * Docs - These changes go into a `Documentation updates` section.
  * Infra - These ones get put into a separate infrastructure updates section.
* A CI check will ensure that the files are present in the `/changelogs/unreleased` directory.
  * The stretch goal here is to be able to tag PRs with `release-note/<category>` and have the check ensure that you have the right category, and vice versa.
  If this works, then we should also have a `release-note/not-required` for tiny changes.


### Pros
- Clear system.
- Should be easily automatable.
- Should be straightforward to write a CI check as well.

### Cons
- Can't determine PR number before PR is open
  - Hoping to make this a little easier by having the CI check tell you what the name should be.

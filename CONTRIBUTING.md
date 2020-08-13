# Contributing

Thanks for taking the time to join our community and start contributing.
These guidelines will help you get started with the Contour project.
Please note that we require [DCO sign off](#dco-sign-off).  

Read this document for additional website specific guildlines: [Site Contribution Guidelines](/SITE_CONTRIBUTION.md).
Guidelines in this document still apply to website contributions.

If you want to get more insight into how the Contour maintainer team approaches R&D, this [page](https://projectcontour.io/resources/how-we-work/) captures how we work on Contour.

## Building from source

This section describes how to build Contour from source.

### Prerequisites

1. *Install Go*

    Contour requires [Go 1.14][1] or later.
    We also assume that you're familiar with Go's [`GOPATH` workspace][3] convention, and have the appropriate environment variables set.

### Fetch the source

Contour uses [`go modules`][2] for dependency management.

```
go get github.com/projectcontour/contour
```

Go is very particular when it comes to the location of the source code in your `$GOPATH`.
The easiest way to make the `go` tool happy is to rename Contour's remote location to something else, and substitute your fork for `origin`.
For example, to set `origin` to your fork, run this command substituting your GitHub username where appropriate.

```
git remote rename origin upstream
git remote add origin git@github.com:davecheney/contour.git
```

This ensures that the source code on disk remains at `$GOPATH/src/github.com/projectcontour/contour` while the remote repository is configured for your fork.

The remainder of this document assumes your terminal's working directory is `$GOPATH/src/github.com/projectcontour/contour`.

### Building

To build Contour, run:

```
make
```

This uses a `go install` and produces a `contour` binary in your `$GOPATH/bin` directory.


### Running the unit tests

Once you have Contour building, you can run all the unit tests for the project:

```
make check
```

This assumes your working directory is set to `$GOPATH/src/github.com/projectcontour/contour`. 

To run the tests for a single package, change to package directory and run:

```
go test .
```

## Contribution workflow

This section describes the process for contributing a bug fix or new feature.
It follows from the previous section, so if you haven't set up your Go workspace and built Contour from source, do that first.

### Before you submit a pull request

This project operates according to the _talk, then code_ rule.
If you plan to submit a pull request for anything more than a typo or obvious bug fix, first you _should_ [raise an issue][6] to discuss your proposal, before submitting any code.

Depending on the size of the feature you may be expected to first write a design proposal. Follow the [Proposal Process](https://github.com/projectcontour/community/blob/main/GOVERNANCE.md#proposal-process) documented in Contour's Governance.

### Commit message and PR guidelines

- Have a short subject on the first line and a body. The body can be empty.
- Use the imperative mood (ie "If applied, this commit will (subject)" should make sense).
- There must be a DCO line ("Signed-off-by: David Cheney <cheneyd@vmware.com>"), see [DCO Sign Off](#dco-sign-off) below.
- Put a summary of the main area affected by the commit at the start,
with a colon as delimiter. For example 'docs:', 'internal/(packagename):', 'design:' or something similar.
- Do not merge commits that don't relate to the affected issue (e.g. "Updating from PR comments", etc). Should
the need to cherrypick a commit or rollback arise, it should be clear what a specific commit's purpose is.
- If main has moved on, you'll need to rebase before we can merge,
so merging upstream main or rebasing from upstream before opening your
PR will probably save you some time.

Pull requests *must* include a `Fixes #NNNN` or `Updates #NNNN` comment.
Remember that `Fixes` will close the associated issue, and `Updates` will link the PR to it.

#### Commit message template

```
<packagename>: <imperative mood short description>

<longer change description/justification>

Updates #NNNN
Fixes #MMMM

Signed-off-by: Your Name <you@youremail.com>
```

#### Sample commit message

```
internal/contour: Add quux functions

To implement the quux functions from #xxyyz, we need to
florble the greep dots, then ensure that the florble is
warbed.

Fixes #xxyyz

Signed-off-by: Your Name <you@youremail.com>

```

### Merging commits

Maintainers should prefer to merge pull requests with the [Squash and merge](https://help.github.com/en/github/collaborating-with-issues-and-pull-requests/about-pull-request-merges#squash-and-merge-your-pull-request-commits) option.
This option is preferred for a number of reasons.
First, it causes GitHub to insert the pull request number in the commit subject which makes it easier to track which PR changes landed in.
Second, it gives maintainers an opportunity to edit the commit message to conform to Contour standards and general [good practice](https://chris.beams.io/posts/git-commit/).
Finally, a one-to-one correspondence between pull requests and commits makes it easier to manage reverting changes and increases the reliability of bisecting the tree (since CI runs at a pull request granularity).

At a maintainer's discretion, pull requests with multiple commits can be merged with the [Create a merge commit](https://help.github.com/en/github/collaborating-with-issues-and-pull-requests/about-pull-request-merges) option.
Merging pull requests with multiple commits can make sense in cases where a change involves code generation or mechanical changes that can be cleanly separated from semantic changes.
The maintainer should review commit messages for each commit and make sure that each commit builds and passes tests.

### Pre commit CI

Before a change is submitted it should pass all the pre commit CI jobs.
If there are unrelated test failures the change can be merged so long as a reference to an issue that tracks the test failures is provided.

Once a change lands in main it will be built and available at this tag, `docker.io/projectcontour/contour:main`.
You can read more about the available contour images in the [tagging][7] document.

### Build an image

To build an image of your change using Contour's `Dockerfile`, run these commands (replacing the repository host and tag with your own):

```
docker build -t docker.io/davecheney/contour:latest .
docker push docker.io/davecheney/contour:latest
```
or, you can use the make helper, like so:

```
REGISTRY=docker.io/davecheney VERSION=latest make push
```

This will push to `:latest` in `docker.io/davecheney` obviously you'll also need to replace the repo host with your own here too. If you don't specify `VERSION`, `make push` will push to a git hash tag (the output of ` git rev-parse --short=8 --verify HEAD`).

### Verify your change

To verify your change by deploying the image you built, take one of the [deployment manifests][7], edit it to point to your new image, and deploy to your Kubernetes cluster.

## DCO Sign off

All authors to the project retain copyright to their work. However, to ensure
that they are only submitting work that they have rights to, we are requiring
everyone to acknowledge this by signing their work.

Since this signature indicates your rights to the contribution and
certifies the statements below, it must contain your real name and
email address. Various forms of noreply email address must not be used.

Any copyright notices in this repository should specify the authors as "The
project authors".

To sign your work, just add a line like this at the end of your commit message:

```
Signed-off-by: David Cheney <cheneyd@vmware.com>
```

This can easily be done with the `--signoff` option to `git commit`.

By doing this you state that you can certify the following (from [https://developercertificate.org/][5]):

```
Developer Certificate of Origin
Version 1.1

Copyright (C) 2004, 2006 The Linux Foundation and its contributors.
1 Letterman Drive
Suite D4700
San Francisco, CA, 94129

Everyone is permitted to copy and distribute verbatim copies of this
license document, but changing it is not allowed.


Developer's Certificate of Origin 1.1

By making a contribution to this project, I certify that:

(a) The contribution was created in whole or in part by me and I
    have the right to submit it under the open source license
    indicated in the file; or

(b) The contribution is based upon previous work that, to the best
    of my knowledge, is covered under an appropriate open source
    license and I have the right under that license to submit that
    work with modifications, whether created in whole or in part
    by me, under the same open source license (unless I am
    permitted to submit under a different license), as indicated
    in the file; or

(c) The contribution was provided directly to me by some other
    person who certified (a), (b) or (c) and I have not modified
    it.

(d) I understand and agree that this project and the contribution
    are public and that a record of the contribution (including all
    personal information I submit with it, including my sign-off) is
    maintained indefinitely and may be redistributed consistent with
    this project or the open source license(s) involved.
```

[1]: https://golang.org/dl/
[2]: https://github.com/golang/go/wiki/Modules
[3]: https://golang.org/doc/code.html
[4]: https://golang.org/pkg/testing/
[5]: https://developercertificate.org/
[6]: https://github.com/projectcontour/contour/issues/new/choose
[6]: site/_resources/tagging.md
[7]: site/docs/main/deploy-options.md

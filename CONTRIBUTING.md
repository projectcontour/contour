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

1. Install Docker

    The easiest way to experiment with Contour is to build it in a container and run it locally in [kind](https://kind.sigs.k8s.io/) cluster.

    On Mac, you can follow instructions to download and install [Docker Desktop](https://www.docker.com/products/docker-desktop/).

    On Linux, you can follow one of the installation methods provided by [Docker](https://docs.docker.com/desktop/install/linux-install/).

2. Install tools: `git`, `make`, `gcc` (used by `go test -race`), [`kind`](https://kind.sigs.k8s.io/docs/user/quick-start/), `kubectl`, `jq` and `yq`

    On Mac, the easiest, and recommended way, to get `git` and `make` is to install Xcode command line tools. To install Xcode command line tools run:

    ```bash
    xcode-select --install
    ```

    For some of the other tools, the recommended way is to use [homebrew](https://brew.sh/) to install them. Once brew is installed you can run:

    ```bash
    brew install kind kubectl jq yq
    ```

    On Ubuntu Linux, some of the above tools can be downloaded via the package manager:

    ```bash
    apt-get install build-essential git jq
    ```

    Reference the documentation of the various tools for installation instructions for your platform:

    * kubectl: https://kubernetes.io/docs/tasks/tools/install-kubectl-linux/
    * kind: https://kind.sigs.k8s.io/docs/user/quick-start/#installing-from-release-binaries
    * yq: https://github.com/mikefarah/yq?tab=readme-ov-file#install

3. Install Go

    To debug locally and to run unit tests, you will need to install Go.
    Contour generally uses the most recent minor [Go version][1].
    Look in the `Makefile` (search for the `BUILD_BASE_IMAGE` variable) to find the specific version being used.

    For installation instructions for Golang see: https://go.dev/doc/install

    On Mac with homebrew, you can install Golang with:

    ```bash
    brew install go
    ```

4. (Optional for MacOS) [Docker Mac Net Connect](https://github.com/chipmk/docker-mac-net-connect) to connect directly to Docker-for-Mac containers via IP address.

    See installation instructions [here](https://github.com/chipmk/docker-mac-net-connect?tab=readme-ov-file#installation)

### Fetch the source

1. [Fork][3] the projectcontour/contour repository.

2. Create a local clone:

    ```
    git clone git@github.com:YOUR-USERNAME/contour.git
    ```
    or
    ```
    git clone https://github.com:YOUR-USERNAME/contour
    ```

### Running your first build

To build the Contour binary:

```shell
go build ./cmd/contour
```

### Running unit tests

To run all of the unit tests:

```shell
go test ./...
```

To run the tests for a single package, change to the package directory and run:

```shell
go test .
```

### Deploying to a local kind cluster

The simplest way to get up and running is to build Contour in a Docker container and to deploy it to a local Kind cluster.
These commands will launch a Kind cluster and deploy your build of Contour to it.

```shell
make install-contour-working
```

or for Contour Gateway Provisioner:

```shell
make install-provisioner-working
```

You can access Contour in localhost ports 9080 and 9443.

To remove the Kind cluster and all resources, run:

```shell
make cleanup-kind
```

#### MacOS

Both `install-contour-working` and `install-provisioner-working` configure [MetalLB](https://metallb.universe.tf/) to setup a local LoadBalancer Service that can be accessed on the Docker network.
On Linux, you are able to directly reach IPs on the Docker network, but on MacOS the docker network is not directly accessible on the host.

As a workaround to this problem, [Docker Mac Net Connect](https://github.com/chipmk/docker-mac-net-connect) can be installed to setup a tunnel between your host and the Docker Desktop Linux VM (see [here](https://github.com/chipmk/docker-mac-net-connect?tab=readme-ov-file#how-does-it-work) for more info on how this works).
To setup follow the [readme installation instructions](https://github.com/chipmk/docker-mac-net-connect?tab=readme-ov-file#installation).

Once it is setup, you are able to create the kind cluster using the above instructions. If you have issues with connecting to the MetalLB IP try to restart the Docker Engine or make sure there is an HTTProxy deployed on your kind cluster so the Contour listener is created.

### Pre-submit checks

To run all checks locally before submitting a pull request, run:

```shell
make checkall
```

This builds the binary, runs unit tests, runs the linters, and verifies that code generation (`make generate`) has been run and the results have been committed.

See `make help` for more information on other useful `make` targets.

### Local Development/Testing

As an alternative to using `make install-contour-working` from above, you may prefer to be able to test out changes to Contour locally without building images and pushing into clusters.

To accomplish this, Envoy can be run inside a Kubernetes cluster, typically a `kind` cluster.
Then Contour is run on your local machine and Envoy will be configured to look for Contour running on your machine vs running in the cluster.

1. Create a kind cluster

```shell
kind create cluster --config=./examples/kind/kind-expose-port.yaml --name=contour
```

2. Deploy Contour & Deps to cluster:

```shell
kubectl apply -f examples/contour
```

_Note: The Contour Deployment/Service can be deleted if desired since it's not used._

3. Find IP of local machine (e.g. `ifconfig` or similar depending on your environment)

4. Edit Envoy Daemonset & change the xds-server value to your local IP address
```shell
kubectl edit ds envoy -n projectcontour
```

Change `initContainers:` to look like this updating the IP and removing the three envoy cert flags:
```shell
 initContainers:
  - args:
    - bootstrap
    - /config/envoy.json
    - --xds-address=<YOUR_IP_ADDRESS>
    - --xds-port=8001
    - --xds-resource-version=v3
    - --resources-dir=/config/resources
```

5. Change your Contour code.

6. Build & start Contour allowing Envoy to connect and get its configuration.
```shell
make install && contour serve --kubeconfig=$HOME/.kube/config --xds-address=0.0.0.0 --insecure
```

8. Test using the local kind cluster by deploying resources into that cluster. Many of our examples use `local.projectcontour.io` which is configured to point to `127.0.0.1` which allows requests to route to the local kind cluster for easy testing.

7. Make more changes and repeat step #6.

## Contribution workflow

This section describes the process for contributing a bug fix or new feature.
It follows from the previous section, so if you haven't set up your Go workspace and built Contour from source, do that first.

### Before you submit a pull request

This project operates according to the _talk, then code_ rule.
If you plan to submit a pull request for anything more than a typo or obvious bug fix, first you _should_ [raise an issue][6] to discuss your proposal, before submitting any code.

Depending on the size of the feature you may be expected to first write a design proposal. Follow the [Proposal Process](https://github.com/projectcontour/community/blob/master/GOVERNANCE.md#proposal-process) documented in Contour's Governance.

### Issue and PR Triage

#### Project Board

In addition to maintaining the project repositories, project maintainers are responsible for maintaining the [project tracking board](https://github.com/orgs/projectcontour/projects/2).
This board is intended to organize work for the team as well as provide visibility into the status and priority of tracks of work, specific Issues, and PRs.
The board is used in conjunction with Issue and PR labels.

Priority of cards on the board flows from left to right, with the *leftmost* column representing what is currently being worked on.
Within a column, priority flows from top to bottom, with the *topmost* cards having the highest priority.

The leftmost column should represent what is planned for the upcoming release.
The "Investigating" column contains longer term items that may need more information, feedback from Issue reporters, etc.
Further columns represent decreasing priority, with "Prioritized Backlog" containing cards that are coming soon, all the way to the "Unprioritized" column which contains items currently not yet sorted.

**Notes for maintainers and contributors**
- If you are looking for work to pick up:
  - Look to the leftmost columns of the project board
  - *New contributors* can use [this shortcut link](https://github.com/projectcontour/contour/issues?q=is%3Aissue+is%3Aopen+label%3A%22good+first+issue%22+label%3A%22help+wanted%22) to find good beginner Contour issues to work on
- When a new Issue or PR is added, add it to the project board and make a best judgement on relative priority so we have a starting place in triage
- When moving items between columns, please add a comment to the Issue or PR detailing why so we have context in triage sessions

#### Community Meeting Triage

The weekly Contour [community meeting](https://projectcontour.io/community/) provides some time for the maintainer team and community to collaborate on Issue and PR triage.

Community members can add links to specific items they would like to discuss to the [meeting notes](https://hackmd.io/84Xbl4WBTpm7OBhaOAsSiw).
This time will be used for clarification, potential to bump priority in the queue of work items for the team, and the ability for contributors to provide more context to their contributions.

In addition, the meeting will be used to go over untriaged issues, longer-term items, and current progress on items in-flight as needed.

**Procedural notes**
- Once an issue has been discussed, remove and add the appropriate labels and pull the item into the correct column
- After triage, Issues and PRs that are actively being worked on should be assigned to someone. This could be:
  - the person working on the issue or PR or
  - a maintainer who is shepherding a new contributor or large PR
- Issues and PRs that are not being actively worked on should have comments updated with the current context, then be unassigned

### Commit message and PR guidelines

- Have a short subject on the first line and a body. The body can be empty.
- Use the imperative mood (ie "If applied, this commit will (subject)" should make sense).
- There must be a DCO line ("Signed-off-by: David Cheney <cheneyd@vmware.com>"), see [DCO Sign Off](#dco-sign-off) below.
- Do not merge commits that don't relate to the affected issue (e.g. "Updating from PR comments", etc). Should
the need to cherrypick a commit or rollback arise, it should be clear what a specific commit's purpose is.
- Put a summary of the main area affected by the commit at the start,
with a colon as delimiter. For example 'docs:', 'internal/(packagename):', 'design:' or something similar.
- PRs *must* be labelled with a `release-note/category` label, where category is one of
`major`, `minor`, `small`, `docs`, or `infra`, unless the change is really small, in which case
it may have a `release-note/not-required` category. PRs may also include a `release-note/deprecation`
label alone or in addition to the primary label.
- PRs *must* include a file named `changelogs/unreleased/PR#-githubID-category.md`, which is a Markdown
file with a description of the change. Please see `changelogs/unreleased/<category>-sample.md` for
sample changelogs.
- If main has moved on, you'll need to rebase before we can merge,
so merging upstream main or rebasing from upstream before opening your
PR will probably save you some time.
- Pull requests *must* include a `Fixes #NNNN` or `Updates #NNNN` comment.
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

### Code formatting

Contour utilizes [`gofumpt`](https://github.com/mvdan/gofumpt) for strict Golang formatting of the contour codebase.
The `lint` CI job checks this to ensure all commits are formatted as expected.

The `make format` target can be used to run `gofumpt` locally before making a PR.

### Import Aliases

Naming is one of the most difficult things in software engineering.
Contour uses the following general pattern to name imports when referencing internal packages and packages from other projects.

> thing_version: The name+package path of the thing and then the version separated by underscores

Examples:

```
contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
contour_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
xdscache_v3 "github.com/projectcontour/contour/internal/xdscache/v3"
```

Exact patterns for import paths can be found in the `importas` linter settings in `.golangci.yml`

### Pre commit CI

Before a change is submitted it should pass all the pre commit CI jobs.
If there are unrelated test failures the change can be merged so long as a reference to an issue that tracks the test failures is provided.

Once a change lands in main it will be built and available at this tag, `ghcr.io/projectcontour/contour:main`.
You can read more about the available contour images in the [tagging][7] document.

### Build an image

To build an image of your change using Contour's `Dockerfile`, run these commands (replacing the repository host and tag with your own):

```
docker build -t ghcr.io/davecheney/contour:latest .
docker push ghcr.io/davecheney/contour:latest
```
or, you can use the make helper, like so:

```
REGISTRY=ghcr.io/davecheney VERSION=latest make push
```

This will push to `:latest` in `ghcr.io/davecheney` obviously you'll also need to replace the repo host with your own here too. If you don't specify `VERSION`, `make push` will push to a git hash tag (the output of ` git rev-parse --short=8 --verify HEAD`).

### Verify your change

To verify your change by deploying the image you built, take one of the [deployment manifests][7], edit it to point to your new image, and deploy to your Kubernetes cluster.

## Contour testing

This section provides some useful information and guidelines for working with Contour's tests.

### Glossary

#### Config/Data Categories
* **Kubernetes Config**: `HTTPProxy`, `Ingress` or [Gateway API][8] config that Contour watches and converts to Envoy config.
* **DAG**: The internal Contour representation of L7 proxy concepts. Kubernetes config is first converted to DAG objects before being converted to Envoy config.
* **Envoy Config**: Configuration that can be provided to Envoy via xDS. This is Contour's final output, generated directly from the DAG.

#### Test Categories
* **Unit Test**: A Go test for a particular function/package. In some cases, these test more than one package at a time.
* **Feature Test**: A Go test in `internal/featuretests` that tests the translation of Kubernetes config to Envoy config, using a Contour event handler and xDS server.
* **End-To-End (E2E) Test**: A Go test in `test/e2e` that performs a full end-to-end test of Contour running in a cluster. Typically verifies the behavior of HTTP requests given a Kubernetes `HTTPProxy`, `Ingress` or Gateway API config.

### Summary of Major Test Suites

The following table describes the major test suites covering the core Contour processing pipeline (Kubernetes config -> DAG -> Envoy config).
In general, changes to the core processing pipeline should be accompanied by new/updated test cases in each of these test suites.

| Test Suite | Description |
| ---------- | ----------- |
| `internal/dag/builder_test.go` (specifically `TestDAGInsert*` functions) | Tests conversion of Kubernetes config to DAG objects. |
| `internal/dag/status_test.go` | Tests invalid Kubernetes (`HTTPProxy`) configs, verifying their status/conditions. |
| `internal/envoy/v3/*_test.go` | Tests conversion of DAG objects to Envoy config. |
| `internal/xdscache/v3/*_test.go` (specifically the `Test[Cluster\|Listener\|Route\|Secret]Visit` functions) | Tests conversion of Kubernetes config to Envoy config. |
| `internal/featuretests/v3/*_test.go` | Tests conversion of Kubernetes config to Envoy config, using a ~full Contour event handler and xDS server. |
| `test/e2e/[httpproxy\|gateway\|ingress]` | E2E tests with Contour running in a cluster. Verifies behavior of HTTP requests for configured proxies. |

> **Note:**
To compile code under folder `e2e` with VSCode, please add `"go.buildTags": "e2e,conformance"` to VSCode config file `settings.json`. Since tests under it have e2e and conformance build tags.


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
[3]: https://docs.github.com/en/github/getting-started-with-github/fork-a-repo#fork-an-example-repository
[4]: https://golang.org/pkg/testing/
[5]: https://developercertificate.org/
[6]: https://github.com/projectcontour/contour/issues/new/choose
[6]: site/_resources/tagging.md
[7]: site/docs/main/deploy-options.md
[8]: https://gateway-api.sigs.k8s.io/

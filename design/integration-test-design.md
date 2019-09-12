# Contour integration test design

Status: Draft

This document outlines a new testing process and rig for Contour.
The intent is to allow testing of the behavior both of Contour and of Envoy, by actually deploying to a cluster.

## Goals

- Enable full integration testing of Contour and Envoy, when combined with Kubernetes.
- Ensure that the testing rig is extensible enough to allow further additions later (for example performance testing and benchmarking)

## Non Goals

- Enabling performance testing or benchmarking

## Background

As Contour has evolved, it's become clear that our existing testing regime has some holes.
In particular, while our Contour tests prove that we can translate Kubernetes objects to a valid set of messages to Envoy,
we can't always be sure that those messages to Envoy produce the outcome we expect.

Some examples of things that are hard to test currently:

- Validating that session affinity routes requests as configured.
- Validating that changes to already-existing Kubernetes objects produce the expected results
- Validating that the IngressRoute/HTTPProxy CRDs can connect with cert-manager (when we build that functionality)
- The behavior of Envoy (particularly with respect to memory usage) when there are a lot of resources or a lot of resource churn in the cluster

## High-Level Design

The idea behind this is to add an additional layer to our testing regime, so that it will look like this:

- Unit tests (in-repo, run as a gate on PRs)
- Feature tests (currently called `e2e` although that is a misnomer, see [#1436](https://github.com/heptio/contour/issues/1436) (in-repo, run as a gate on PRs)
- Integration tests (run in a testing Kubernetes cluster).

Initially, the new integration tests will be run either manually or on a cron schedule, or possibly after a merge to master. In the future, it would be good to have a facility to run per-branch, but that is not required in the first cut.

At a high level, the intent is that the integration test suite will install a version of Contour, using the example deployment, into an existing cluster containing a large set of Ingress, IngressRoute, and HTTPProxy objects.
It will then run a series of tests to ensure that the objects have produced the expected behaviors in both Contour and Envoy.
It's expected that the set of objects will include both working objects and some broken in an expected way.

## Detailed Design

The following are the steps that will be required to implement this high-level design.

### Add kube cluster

We will find a place to keep a long-lived testing cluster, to run the automated test suite.
The testing cluster should be as close to a vanilla Kubernetes cluster as possible.
Deploying the example requires a `LoadBalancer` Service, so this should run in a cloud provider that can supply LoadBalancers.

This task does not block the rest, as any further tooling should be able to connect to a supplied Kubernetes cluster (using the usual KUBECONFIG mechanisms).

### Design test objects

The test objects should cover all the supported configuration types (at time of writing, Ingress, IngressRoute, and HTTPProxy).
They should also cover a variety of successful configurations, and a variety of common errors.

For example (not an exhaustive list):

- Ingress objects should include the common supported annotations for Contour, as well as both HTTP and HTTPS services. A stretch goal here is cert-manager integration.
- IngressRoute objects should test HTTP and HTTPS services, including TLS termination. It should also test a variety of delegation scenarios, both working and not.
- HTTPProxy objects should test the same things as IngressRoute, plus any additional functionality. (For example, delegation and routing on other keys than path).

An idea to note is that these test manifests may have an annotation (perhaps `testing.projectcontour.io/valid` or similar) that indicates to the automation if the file is part of a test case that should succeed or fail.

These object manifests should be stored in a central location, most likely inside the Contour repository.

### Templating for test objects

The test manifests will need some form of templating, to allow them to be able to run on other clusters than the default testing cluster.

For example, domain names will need to be able to be templated, if for no other reason than whoever is developing changes to the tests needs to be able to test those changes.
Additionally, some templating may allow the possibility of running these tests on branches.
For example, using different `kubernetes.io/ingress.class` names combined with a per-deployment setting of the `--ingress-class-name` flag to Contour might allow multiple test suites to run inside a single cluster. This would need careful evaluation.

### How to run the tests

There are a variety of options for defining and running the test suite.

However, the first subject for evaluation should be to use a similar stack to Kubernetes itself, that is, to use the Kubernetes e2e framework, which uses Ginkgo and Gomega to define a `go test` compatible test suite.

This has the advantages that it:

- uses technology developed for testing Kubernetes things
- produces Go test files, which means less for prospective test-adders to learn
- has a large degree of extensibility.

### How to retrieve and collate results

The outcome of this will largely be decided by [How to run the tests](#how-to-run-the-tests).

However, whatever we choose must be able to indicate a simple pass/fail (for use in CI), as well as convey more in-depth information in the event of a failure.

### How to automate the test run

Once the tool to actually run the tests is chosen, we need to decide how we will automate the test runs.
The early options to evaluate here are:

- Travis (which we currently use for CI)
- Github actions (which we currently use for CD/image deployment)

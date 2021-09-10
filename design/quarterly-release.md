# Change Contour releases to quarterly

Status: Accepted

## Abstract
Move Contour and Contour Operator to a longer release cadence to cushion for release planning, align Contour releases with Contour Operator releases, align with downstream consumers, and improve feature planning. 

Please see [#3634](https://github.com/projectcontour/contour/issues/3634) for more discussion.

## Current issues with monthly release
* Very tough for users to keep up with Contour monthly releases. It’s rare for production deployments to always chase the latest version and more likely to upgrade every 6 months or longer. As part of these changes, we will be investigating how possible enabling jump upgrades is for Contour.
* Release engineering is taxing and we have to spend some time preparing for this every month. The tradeoff here is that we will need to do more patch releases, and that quarterly releases will take more effort to put together than monthly ones, simply because they'll be bigger.
* Since we currently release the Contour Operator 1:1 with Contour, the Operator release adds to the complexity of release engineering.  An issue with a Contour release will thus affect Contour Operator pushing it out and in turn eat into the next release cycle

# Anticipated issues with quarterly release
* More work from backporting changes due to expanding support window
* Larger releases means more change, more risk, and more testing required.
* We will probably need to cut more patch releases, which have done very rarely, and so don't have good processes on.


## Proposal

*  Move from existing model of only supporting the latest release to supporting N-2 model, i.e. 3 minor releases (denoted by W, X, Y) at any given time.  For example, with the release of Contour v1.22, we are responsible for maintaining v1.22, v1.21, and v1.20.
*  The first version to be covered by this schedule will be Contour v1.20. Please see the "Rollout Process" section below for details.
*  The following will be reasons we will consider issuing a patch release:
   * CVE reported for Contour or an upstream component of Contour like Envoy
   * Critical bugs 
   * Feature enhancement requests with enough community support. This will require an exceptional circumstance and a lot of community support.
* If we issue a patch release for the latest minor version, we will also backport it to all supported versions.
* We will also make at least one Release Candidate (RC) build available before each *minor* release, to enable Contour's downstream consumers to test and validate before Contour releases. This RC build will be released at least two weeks before a minor release.

## Rollout Process

The first Contour version covered by the quarterly release cadence will be Contour v1.20, scheduled for late October 2021.

At the time it is released, it will be the only supported version, and versions 1.21 and 1.22 will continue supporting back to Contour 1.20.

When Contour 1.23 releases (nine months later), Contour 1.20 will fall out of support.

The following table illustrates how this will work.

| Version |v1.19 |v1.20|v1.21|v1.22|v1.23|
|---------|--------|-------|-------|-------|-------|
|Q3 2021  | :heavy_check_mark: |
|Q4 2021  | :negative_squared_cross_mark: | :heavy_check_mark: |
|Q1 2022  | :negative_squared_cross_mark: | :heavy_check_mark: |:heavy_check_mark: |
|Q2 2022  | :negative_squared_cross_mark: | :heavy_check_mark: |:heavy_check_mark: |:heavy_check_mark: |
|Q3 2022  | :negative_squared_cross_mark: | :negative_squared_cross_mark: |:heavy_check_mark: |:heavy_check_mark: | :heavy_check_mark: |

## Upstream dependency management

There are some unresolved questions about upstream dependencies and their release cadences, but some best guesses:
* Kubernetes releases three times per year. We will investigate which Contour release is the best one to upgrade our Kubernetes dependencies and update this document at a later date.
* Envoy releases quarterly, we will attempt to ensure that Contour releases harmonize with Envoy releases.
* Go releases every six months, we will most likely upgrade Go soon after it is released.

---
title: Contributing Technical Documentation
layout: page
---

# Contour Technical Documentation Contributing Guide

The purpose of the Working Group is to build up a self-sustaining community around documentation for Contour. We have an initial need to rework the existing documentation based on the recommendations outlined in the [CNCF Tech Docs Review](https://github.com/cncf/techdocs/blob/main/assessments/0001-contour.md), and a continuous need for documentation being added/edited/removed for releases going forward.
This group is open to contributors of all levels, the only requirement is being interested in helping with docs!

## Getting started with in the Working Group

Whenever you’re available to join, come say hi using either method:

- Join the [Contour Office Hours Zoom](https://zoom.us/j/96698475744?pwd=KzVUd3BZSWI2bWIxTmhjZ2d5QVcxUT09) every 1st and 3rd Thursday at 1-2pm ET / 10am-11pm PT and introduce yourself
- On the [Kubernetes Slack workspace](https://slack.k8s.io/), join the [#contour](https://kubernetes.slack.com/messages/contour) channel and introduce yourself

When you introduce yourself, let us know:

- Why you are interested in participating in the working group and what you hope to get out of your time contributing to the Contour docs
- Your prior experience level with our docs toolchain (Git and GitHub, Markdown, Hugo, HTML) and where you need some help
- Which operating system you’re using (for help with setting up your environment)

We can help you help find an open issue to work on or answer any questions you have about writing Contour docs. If you notice something about the docs you'd like to improve, please file an issue and bring it up with the working group. We'd love to hear your ideas.

### Set up your environment

Make sure you have the following installed:

- A GitHub login
- [Git](https://git-scm.com/book/en/v2/Getting-Started-Installing-Git)
- [Hugo](https://gohugo.io/getting-started/installing)
- A good editor

Now you can start editing content:

1. Start by [forking](https://docs.github.com/en/github/getting-started-with-github/quickstart/fork-a-repo) the [contour](https://github.com/projectcontour/contour) repo
1. Within the `contour/site` directory, you will find the entire [projectcontour.io](https://projectcontour.io) website
1. Within the `contour/site/content` directory you will find our docs, our resources, guides and so forth
1. The `docs` directory is divided into `main` which is the latest development docs, and `1.X.Z` where you will find the latest released versioned docs
1. For new content for future versions, it should be created in `main`
1. For edits to older versioned docs, first make the edits to that specific version (spelling errors, broken links etc) and then verify if those changes should also be incorporated in the `main` directory for the latest development docs

### Create a Pull Request with your changes

Please see the [CONTRIBUTING doc](https://github.com/projectcontour/contour/blob/main/CONTRIBUTING.md#contribution-workflow) in the section "Contribution workflow" for more detailed information on how to commit your changes and submit a pull request.

### What if you can’t finish your work?

When you join the working group and get assigned an issue, we'd ask that you try to open a pull requests with your fixes within a few days. If you are unable to finish your assigned issue, please submit a pull request with the content that you were able to create and update the Github issue with the latest information of your progress. This way your work goes to use, you get credit for your a contributions, and we can work with other team members to continue making progress on the issue :)

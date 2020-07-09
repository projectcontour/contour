---
title: Kubernetes Support Matrix
layout: page
---

This page describes the compatibility matrix of Contour and Kubernetes versions.

Contour utilizes [client-go][1] to watch for resources in a Kubernetes cluster.
Since Kubernetes is backwards compatible with clients, older client-go versions will work with many different Kubernetes cluster versions.
The `client-go` package includes a [compatibility matrix][2] as to what Kubernetes API versions are supported with the version of client-go.  

It's important to note that since Contour consumes a small number of quite stable Kubernetes APIs, most Kubernetes versions most likely will not have issues, however, the client-go package does not guarantee compatibility.

---
**NOTE**

If you are using a Kubernetes distribution offered by a public cloud provider, where you don't have the option to upgrade to a more recent and supported Kubernetes version (as per the support matrix outlined below), please talk to us on [Slack][4] or attend a [community meeting][3]. We would like to find a way to satisfy your use case with Contour.

---

## Supported Kubernetes versions

| Contour Version | Kubernetes Version        |
| --------------- | :------------------- |
| master          | 1.18, 1.17, 1.16   |
| 1.6.1           | 1.18, 1.17, 1.16   |
| 1.6.0           | 1.18, 1.17, 1.16   |
| 1.5.1           | 1.18, 1.17, 1.16   |
| 1.5.0           | 1.18, 1.17, 1.16   |
| 1.4.0           | 1.17, 1.16, 1.15   |
| 1.3.0           | 1.17, 1.16, 1.15   |
| 1.2.1           | 1.17, 1.16, 1.15   |
| 1.2.0           | 1.17, 1.16, 1.15   |
| 1.1.0           | 1.15, 1.14, 1.13   |
| 1.0.1           | 1.15, 1.14, 1.13   |
| 1.0.0           | 1.15, 1.14, 1.13   |
{: class="table thead-dark table-bordered"}

[1]: https://github.com/kubernetes/client-go
[2]: https://github.com/kubernetes/client-go#compatibility-matrix
[3]: https://projectcontour.io/community/
[4]: https://kubernetes.slack.com/messages/contour

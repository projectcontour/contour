---
title: Kubernetes Support Matrix
layout: page
---

This page describes the compatibility matrix of Contour and Kubernetes versions.

Contour utilizes [client-go][1] to watch for resources in a Kubernetes cluster.
Since Kubernetes is backwards compatible with clients, older client-go versions will work with many different Kubernetes cluster versions.
The `client-go` package includes a [compatibility matrix][2] as to what Kubernetes API versions are supported with the version of client-go.  

It's important to note that since Contour utilizes v1 CRD API versions, the minimum Kubernetes version is v1.16.
Contour also only consumes a small number of quite stable Kubernetes APIs, most Kubernetes versions most likely will not have issues, however, the client-go package does not guarantee compatibility.

---
**NOTE**

If you are using a Kubernetes distribution offered by a public cloud provider, where you don't have the option to upgrade to a more recent and supported Kubernetes version (as per the support matrix outlined below), please talk to us on [Slack][4] or attend a [community meeting][3]. We would like to find a way to satisfy your use case with Contour.

---

## Supported Kubernetes versions

| Contour Version | Kubernetes Version |
| --------------- | :----------------- |
| main            | 1.19, 1.18, 1.17   |
| 1.11.x          | 1.19, 1.18, 1.17   |
| 1.10.x          | 1.19, 1.18, 1.17   |
| 1.9.x           | 1.18, 1.17, 1.16   |
| 1.8.x           | 1.18, 1.17, 1.16   |
| 1.7.x           | 1.18, 1.17, 1.16   |
| 1.6.x           | 1.18, 1.17, 1.16   |
| 1.5.x           | 1.18, 1.17, 1.16   |
| 1.4.x           | 1.17, 1.16, 1.15   |
| 1.3.x           | 1.17, 1.16, 1.15   |
| 1.2.x           | 1.17, 1.16, 1.15   |
| 1.1.x           | 1.15, 1.14, 1.13   |
| 1.0.x           | 1.15, 1.14, 1.13   |
{: class="table thead-dark table-bordered"}

[1]: https://github.com/kubernetes/client-go
[2]: https://github.com/kubernetes/client-go#compatibility-matrix
[3]: https://projectcontour.io/community/
[4]: https://kubernetes.slack.com/messages/contour

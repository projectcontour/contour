---
title: Kubernetes Support Matrix
layout: page
---

This page describes the compatibility matrix of Contour and Kubernetes versions.

Contour utilizes [client-go][1] to watch for resources in a Kubernetes cluster.
Since Kubernetes is backwards compatible with clients, older client-go versions will work with many different Kubernetes cluster versions.
The `client-go` package includes a [compatibility matrix][2] as to what Kubernetes API versions are supported with the version of client-go.  

## Supported Kubernetes versions

| Kubernetes version | Contour v1.0.0 | Contour v1.0.1 | Contour v1.1.0 |
| ------------ | :-----------: | :-----------: | :----------: | 
| 1.15.x | Supported | Supported | Supported | 
| 1.16.x | Supported | Supported | Supported |
| 1.17.x | Supported | Supported | Supported | 
| 1.18.x | Not Supported<sup>1</sup>  | Not Supported<sup>1</sup> | Not Supported <sup>1</sup> |

### Notes

1. It's important to note that since Contour consumes a small number of quite stable Kubernetes APIs, most Kubernetes versions most likely will not have issues, however, the client-go package does not guarantee compatibility.

[1]: https://github.com/kubernetes/client-go
[2]: https://github.com/kubernetes/client-go#compatibility-matrix

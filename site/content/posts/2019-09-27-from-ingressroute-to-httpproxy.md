---
title: From IngressRoute to HTTPProxy
excerpt: 
author_name: Dave Cheney
author_avatar: /img/contributors/dave-cheney.png
categories: [kubernetes]
tags: ['IngressRoute', 'HTTPProxy', 'ingress', 'Dave Cheney']
date: 2019-09-27
slug: from-ingressroute-to-httpproxy
---

As part of the preparations to deliver Contour 1.0 at KubeCon US, [Contour 1.0.0-beta.1 (available now!)][1] renamed the [IngressRoute][2] CRD to [HTTPProxy][3].
This post explains the path from IngressRoute to HTTPProxy and why the change isn't a revolution but an evolution.

## IngressRoute is dead, long live HTTPProxy

More than a year ago Contour 0.6 introduced a new CRD, IngressRoute.
IngressRoute was our attempt to address the issues preventing Kubernetes developers from utilizing modern web deployment patterns in multi tenant Kubernetes clusters.

Fast forward to July of this year where plans to move Contour out of the _0.whatever_ doldrums were being set in motion.
We knew that stamping a 1.0 release on Contour required us to do the same for IngressRoute, which had at that point been in beta for a period of time that would make a Google product blush.
Bringing IngressRoute, as it was known at the time, to 1.0 status would involve three things.

The first was addressing, which in retrospect seemed like an inspired piece of guerrilla marketing, the fact that I had plastered not just the name of the product but the name of the sponsoring company throughout annotation names, CRD groups, repository image hosting, and namespace objects.
Taking the necessary hit to _rename all the things_ is the focus of the beta.1 and upcoming rc.1 releases.

Once these procedural issues were in hand the second issue was to consider the name _IngressRoute_ itself.
The name, to the best of my recollection chosen without any particular deliberation, was more of a portmanteau of the problems IngressRoute was designed to solve; improving Ingress in multi tenant clusters, and more flexible Routing.

With a year's experience developing and supporting IngressRoute, a few problems with the name had become evident.
As a name, IngressRoute was lengthy.
Abbreviating it introduced confusion with another Kubernetes object--also in beta--which we didn't want to be confused with.
If you wanted to be precise, you had to type, and _say aloud_, the entire thing.
But there are more problems than verbosity.

The original Kubernetes Ingress object was clearly intended to address more than just HTTP routing.
The word Ingress, especially if you talk to the overlay, physical, or software defined networking folks, has nothing to do with layer 7 proxying and load balancing.
Contour defined its mission as an Ingress controller by what Kubernetes users were using the Ingress object for in 2017: HTTP routing, load balancing, and proxying.
Collectively Kubernetes cloud natives might call the configuration for our HTTP proxies _Ingress_, but what were were doing had little to do with ingress and egress traffic as networking vendors define it.
The name _HTTPProxy_ reflects the desire to clarify Contour's role in the crowded Kubernetes networking space.

The final issue is addressing the limitations in the IngressRoute--now HTTPProxy--object which we felt could not be solved in an backwards compatible way once we committed to a v1 of the object.
HTTPProxy brings with it two new concepts--[inclusion][4] and [conditions][5]--which, like the transition from IngressRoute to HTTPProxy, represent the respective evaluations of the delegation model and our limited support for prefix based routing.

The intent of making this change now is to prepare HTTPProxy as a stable CRD for Contour users following the same backwards compatibility goals as Contour 1.0.
With this goal in mind the IngressRoute CRD, having never made it out of beta, should be considered deprecated.
Contour will continue to support the IngressRoute CRD up to the 1.0 release of Contour in November, however no further enhancements or bug fixes will be made over this period unless absolutely necessary.
The plan at this stage is to remove support for the IngressRoute CRD after Contour 1.0 ships.
We've [written a guide][6] to help you transition your IngressRoute objects to HTTPProxy.

The next blog post in this series will delve into how to use inclusion and conditions.
Stay tuned for that. 

## TCP proxying future

The final question that should be answered is, with the focus on layer 7 HTTP proxying, what is the future of Contour's TCP proxying feature?
The short answer is Contour's layer 3/4 TCP proxying feature is not going away.
Despite the cognitive dissonance, we're committed to supporting and enhancing Contour's TCP proxying abilities via the HTTPProxy CRD for the long term.

[1]: {{< param github_url >}}/releases/tag/v1.0.0-beta.1
[2]: {{< param github_url >}}/blob/v1.0.0-beta.1/docs/ingressroute.md
[3]: {{< param github_url >}}/blob/v1.0.0-beta.1/docs/httpproxy.md
[4]: {{< param github_url >}}/blob/v1.0.0-beta.1/docs/httpproxy.md#httpproxy-inclusion
[5]: {{< param github_url >}}/blob/v1.0.0-beta.1/docs/httpproxy.md#conditions
[6]: {% link _guides/ingressroute-to-httpproxy.md %}

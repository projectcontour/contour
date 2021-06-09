---
title: Announcing Contour 1.0&#58; A Proxy for Your Multi-Tenant Future
excerpt: The Journey from 0.1 to 1.0
author_name: Contour Team
#author_avatar: /img/Contour.svg
categories: [kubernetes]
# Tag should match author to drive author pages
tags: ['Contour Team', 'release']
date: 2019-11-01
slug: announcing-contour-1.0
---

_Authored by Dave Cheney, Steve Sloka, Nick Young, and James Peach_

Exactly two years ago, we launched a [new open-source ingress controller][1]. Contour was the first ingress controller to take advantage of the growing popularity of Envoy to create a layer 7 load-balancing solution for Kubernetes users. Contour was also the first Ingress controller to make use of Envoy’s gRPC API, which allowed changes to be streamed directly from Kubernetes.

Today, we are very happy to announce **Contour 1.0**. We’ve come a long way since that first commit two years ago! This blog post takes stock of our journey to Contour 1.0 and describes some of the new features in this milestone release.

## The Journey from 0.1 to 1.0

Since its launch, Contour has held to a monthly release cadence. This pace allowed us to explore the problems that administrators and development teams were struggling with when deploying modern web applications on top of Kubernetes. In the process, we’ve iterated rapidly to introduce a compelling set of features. After two years, we’ve decided it's time to move out of the shadow of perpetual beta and commit to a stable version of Contour and its CRD types.

Contour 1.0 moves the HTTPProxy CRD to version 1 and represents our commitment to evolve that API in a backward-compatible manner.

## From Ingress to IngressRoute to HTTPProxy

The Kubernetes Ingress object has many limitations. Some of those are being addressed as part of SIG-Networking’s work to bring Ingress to v2, but other limitations that prevent Ingress from being used safely inside multi-tenant clusters remain unsolved.

In this spirit, Contour 0.6, released in September of 2018, introduced a new CRD, IngressRoute. IngressRoute was our attempt to address the issues preventing application developers from utilizing modern web development patterns in multi-tenant Kubernetes environments. 

As part of preparations for bringing IngressRoute from beta to v1, it has been renamed HTTPProxy. This new name reflects the desire to clarify Contour's role in the crowded Kubernetes networking space by setting it apart from other networking tools.

HTTPProxy brings with it two new concepts, inclusion and conditions, both of which, like the transition from IngressRoute to HTTPProxy, represent evolutions of the IngressRoute’s  delegation model and limited support for prefix-based matching.

## Decoupled Deployment

Originally, our recommended deployment model for Contour was for the Contour and Envoy containers to share a pod, controlled by a DaemonSet. However, this model linked the lifecycles of Contour and Envoy. You could not, for instance, update Contour’s configuration without having to take an outage of the co-located Envoy instances.

In order to change this model, we needed to make it straightforward to secure your xDS traffic. This is because the TLS certificates and keys used for serving traffic  must be transmitted to Envoy across the xDS connection. To make sure that these can’t leak out, we introduced secure gRPC over TLS and a subcommand, `contour certgen`, that generates a set of self-signed keypairs for you. When you install Contour 1.0 using our example deployment, this is all done automatically for you.

Splitting Contour and Envoy apart also allowed us to enable leader election for Contour. Kubernetes leader election is a standard feature that allows you to use Kubernetes primitives as a distributed locking mechanism, and designate one Contour as the leader. In Contour 1.0, this leadership is what controls which Contour instance can write status updates back to HTTPProxy objects. 

## Notable Features

* Contour 1.0 supports outputting HTTP request logs in a configurable structured JSON format.
* Under certain circumstances, it is now possible to combine TLS pass-through on Port 443 with Port 80 served from the same service. The use case for this feature is that the application on Port 80 can provide a helpful message when the service on Port 443 does not use HTTPS.
* One service per route can be nominated as a mirror. The mirror service will receive a copy of the read traffic sent to any non-mirror service. The mirror traffic is considered read only; any response by the mirror will be discarded.
* Per-route idle timeouts can be configured with the HTTPProxy CRD.

## What’s Next after 1.0?

What does the future hold for Contour? In a word (if this is a word): HTTPProxy. We plan to continue to explore the ways Contour can complement the work of application teams deploying web application workloads on Kubernetes, and the operations teams responsible for supporting those applications in production.

We will continue to follow the progression of discussions about Ingress v1 and v2 in the Kubernetes community, and at this time, we expect to add support for those objects when they become available.

We’re also mindful of the ever-present feature backlog we’ve accrued during the process of delivering Contour 1.0. The backlog will require careful prioritization, and we will have to walk the line between endless configuration knobs and a recognition that no two applications, deployments, or clusters are identical.

## Contributor Shoutouts

We’re immensely grateful for all the community contributions that help make Contour even better! The lifeblood of any open source project is its community.

![Contour 1.0 stats][2]

The sign of a strong community is how users communicate through Slack and GitHub Issues as well as make contributions back to the project. We couldn’t have made it to 1.0 without you. **Thank you!**

[![256dpi](https://avatars2.githubusercontent.com/u/696886?v=4&s=48)](https://github.com/256dpi)
[![aknuds1](https://avatars1.githubusercontent.com/u/281303?v=4&s=48)](https://github.com/aknuds1)
[![alexbrand](https://avatars2.githubusercontent.com/u/545723?v=4&s=48)](https://github.com/alexbrand)
[![alvaroaleman](https://avatars2.githubusercontent.com/u/6496100?v=4&s=48)](https://github.com/alvaroaleman)
[![andrewsykim](https://avatars0.githubusercontent.com/u/12699319?v=4&s=48)](https://github.com/andrewsykim)
[![arminbuerkle](/img/contour-1.0/22750465.png)](https://github.com/arminbuerkle)
[![Atul9](https://avatars1.githubusercontent.com/u/3390330?v=4&s=48)](https://github.com/Atul9)
[![awprice](https://avatars3.githubusercontent.com/u/2804025?v=4&s=48)](https://github.com/awprice)
[![bgagnon](https://avatars2.githubusercontent.com/u/81865?v=4&s=48)](https://github.com/bgagnon)
[![bhudlemeyer](https://avatars1.githubusercontent.com/u/2275490?v=4&s=48)](https://github.com/bhudlemeyer)
[![Bradamant3](https://avatars2.githubusercontent.com/u/6934230?v=4&s=48)](https://github.com/Bradamant3)
[![ceralena](https://avatars3.githubusercontent.com/u/615299?v=4&s=48)](https://github.com/ceralena)
[![cromefire](https://avatars0.githubusercontent.com/u/26320625?v=4&s=48)](https://github.com/cromefire)
[![cw-sakamoto](https://avatars2.githubusercontent.com/u/29860510?v=4&s=48)](https://github.com/cw-sakamoto)
[![davecheney](https://avatars0.githubusercontent.com/u/7171?v=4&s=48)](https://github.com/davecheney)
[![dvdmuckle](https://avatars2.githubusercontent.com/u/8870292?v=4&s=48)](https://github.com/dvdmuckle)
[![DylanGraham](https://avatars1.githubusercontent.com/u/4900511?v=4&s=48)](https://github.com/DylanGraham)
[![embano1](https://avatars0.githubusercontent.com/u/15986659?v=4&s=48)](https://github.com/embano1)
[![emman27](https://avatars0.githubusercontent.com/u/6295583?v=4&s=48)](https://github.com/emman27)
[![ffahri](https://avatars2.githubusercontent.com/u/13694962?v=4&s=48)](https://github.com/ffahri)
[![glerchundi](/img/contour-1.0/2232214.png)](https://github.com/glerchundi)
[![HerrmannHinz](https://avatars0.githubusercontent.com/u/11093419?v=4&s=48)](https://github.com/HerrmannHinz)
[![jbeda](https://avatars2.githubusercontent.com/u/37310?v=4&s=48)](https://github.com/jbeda)
[![jelmersnoeck](https://avatars1.githubusercontent.com/u/815655?v=4&s=48)](https://github.com/jelmersnoeck)
[![jhamilton1](https://avatars1.githubusercontent.com/u/40370921?v=4&s=48)](https://github.com/jhamilton1)
[![johnharris85](https://avatars3.githubusercontent.com/u/746221?v=4&s=48)](https://github.com/johnharris85)
[![jonas](https://avatars2.githubusercontent.com/u/8417?v=4&s=48)](https://github.com/jonas)
[![jonasrosland](https://avatars3.githubusercontent.com/u/1690215?v=4&s=48)](https://github.com/jonasrosland)
[![joonathan](https://avatars0.githubusercontent.com/u/3045?v=4&s=48)](https://github.com/joonathan)
[![josebiro](https://avatars0.githubusercontent.com/u/1455144?v=4&s=48)](https://github.com/josebiro)
[![joshrosso](https://avatars2.githubusercontent.com/u/6200057?v=4&s=48)](https://github.com/joshrosso)
[![jpeach](/img/contour-1.0/9917.png)](https://github.com/jpeach)
[![Lookyan](/img/contour-1.0/1040646.png)](https://github.com/Lookyan)
[![lostllama](https://avatars2.githubusercontent.com/u/9258568?v=4&s=48)](https://github.com/lostllama)
[![lucasreed](https://avatars0.githubusercontent.com/u/6800091?v=4&s=48)](https://github.com/lucasreed)
[![mitsutaka](https://avatars2.githubusercontent.com/u/557782?v=4&s=48)](https://github.com/mitsutaka)
[![msample](https://avatars1.githubusercontent.com/u/4896732?v=4&s=48)](https://github.com/msample)
[![mwhittington21](https://avatars1.githubusercontent.com/u/29389868?v=4&s=48)](https://github.com/mwhittington21)
[![nicolasbernard](https://avatars1.githubusercontent.com/u/15658?v=4&s=48)](https://github.com/nicolasbernard)
[![norrs](https://avatars2.githubusercontent.com/u/272215?v=4&s=48)](https://github.com/norrs)
[![odacremolbap](https://avatars2.githubusercontent.com/u/9891289?v=4&s=48)](https://github.com/odacremolbap)
[![paivagustavo](https://avatars0.githubusercontent.com/u/7898464?v=4&s=48)](https://github.com/paivagustavo)
[![PeteE](https://avatars3.githubusercontent.com/u/89916?v=4&s=48)](https://github.com/PeteE)
[![pims](https://avatars3.githubusercontent.com/u/27320?v=4&s=48)](https://github.com/pims)
[![prasoontelang](https://avatars2.githubusercontent.com/u/2859827?v=4&s=48)](https://github.com/prasoontelang)
[![ramnes](https://avatars2.githubusercontent.com/u/835072?v=4&s=48)](https://github.com/ramnes)
[![rata](https://avatars1.githubusercontent.com/u/70861?v=4&s=48)](https://github.com/rata)
[![rbankston](https://avatars1.githubusercontent.com/u/130836?v=4&s=48)](https://github.com/rbankston)
[![robbiemcmichael](https://avatars2.githubusercontent.com/u/2044464?v=4&s=48)](https://github.com/robbiemcmichael)
[![rochacon](https://avatars2.githubusercontent.com/u/321351?v=4&s=48)](https://github.com/rochacon)
[![rohandvora](https://avatars3.githubusercontent.com/u/8749993?v=4&s=48)](https://github.com/rohandvora)
[![rosskukulinski](https://avatars2.githubusercontent.com/u/2746479?v=4&s=48)](https://github.com/rosskukulinski)
[![rothgar](https://avatars1.githubusercontent.com/u/371796?v=4&s=48)](https://github.com/rothgar)
[![rsyvarth](https://avatars3.githubusercontent.com/u/1712051?v=4&s=48)](https://github.com/rsyvarth)
[![samuela](https://avatars0.githubusercontent.com/u/226872?v=4&s=48)](https://github.com/samuela)
[![SDBrett](https://avatars0.githubusercontent.com/u/25494777?v=4&s=48)](https://github.com/SDBrett)
[![SEJeff](https://avatars1.githubusercontent.com/u/4603?v=4&s=48)](https://github.com/SEJeff)
[![sevein](https://avatars2.githubusercontent.com/u/606459?v=4&s=48)](https://github.com/sevein)
[![shaneog](https://avatars2.githubusercontent.com/u/130415?v=4&s=48)](https://github.com/shaneog)
[![shivanshu21](https://avatars2.githubusercontent.com/u/14923644?v=4&s=48)](https://github.com/shivanshu21)
[![stephenmoloney](https://avatars3.githubusercontent.com/u/12668653?v=4&s=48)](https://github.com/stephenmoloney)
[![stevesloka](https://avatars3.githubusercontent.com/u/1048184?v=4&s=48)](https://github.com/stevesloka)
[![sudeeptoroy](/img/contour-1.0/10099903.png)](https://github.com/sudeeptoroy)
[![tasdikrahman](https://avatars1.githubusercontent.com/u/4672518?v=4&s=48)](https://github.com/tasdikrahman)
[![uablrek](https://avatars1.githubusercontent.com/u/37046727?v=4&s=48)](https://github.com/uablrek)
[![unicell](https://avatars1.githubusercontent.com/u/35352?v=4&s=48)](https://github.com/unicell)
[![vaamarnath](https://avatars1.githubusercontent.com/u/1831109?v=4&s=48)](https://github.com/vaamarnath)
[![varunkumar](https://avatars1.githubusercontent.com/u/509433?v=4&s=48)](https://github.com/varunkumar)
[![vmogilev](https://avatars1.githubusercontent.com/u/1376994?v=4&s=48)](https://github.com/vmogilev)
[![wadeholler](https://avatars1.githubusercontent.com/u/13917666?v=4&s=48)](https://github.com/wadeholler)
[![willmadison](https://avatars2.githubusercontent.com/u/1326766?v=4&s=48)](https://github.com/willmadison)
[![yob](https://avatars2.githubusercontent.com/u/8132?v=4&s=48)](https://github.com/yob)
[![youngnick](https://avatars0.githubusercontent.com/u/9346599?v=4&s=48)](https://github.com/youngnick)
[![yvespp](https://avatars0.githubusercontent.com/u/15231595?v=4&s=48)](https://github.com/yvespp)
[![zhilingc](https://avatars1.githubusercontent.com/u/15104168?v=4&s=48)](https://github.com/zhilingc)
[![zxvdr](/img/contour-1.0/223340.png)](https://github.com/zxvdr)

_**Note**: Stats above were taken on  Oct. 31, 2019._

[1]: {{< param github_url >}}/commit/788feabc67c4da76cd1ae3c9ac1998b43cb0e2f3
[2]: {% link img/contour-1.0/contour-1.0-stats.png %}

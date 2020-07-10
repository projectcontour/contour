---
title: Contour’s landscape, July 2020
excerpt: Contour is now part of the CNCF, and we're taking the opportunity to reevaluate our product philosophy.
author_name: Nick Young
author_avatar: /img/contributors/nick-young.png
categories: [kubernetes]
# Tag should match author to drive author pages
tags: ['Contour Team', 'Nick Young', 'landscape']
---

# Contour’s landscape, June 2020

Hi everyone, my name is Nick Young, and I’ve not-so-recently taken over from Dave Cheney as Tech Lead for the Contour maintainer team. Thanks very much to Dave for getting Contour started and for everything he’s done in his time with the team. His talent and insight are and will be missed.
![Bon Voyage Dave](https://frinkiac.com/gif/S08E03/1306838/1310375.gif?b64lines=Qm9uIFZveWFnZSBEYXZlIQ== "Bon Voyage Dave")

As part of coming onboard as Tech Lead, I’ve been spending some time thinking about where Contour is, what it’s about, and where it’s going. Some of that effort has materialized already in things like the new [Philosophy document](https://projectcontour.io/resources/philosophy/), but I’ve also spent a bunch of time reviewing the project with a different lens than my previous maintainer-only one. In this post, I’d like to talk about some things I’ve found as part of that, what I think they mean for Contour’s mission, what we want to achieve, and what we will be doing differently in the future.

I’m currently aiming for this to be a quarterly retrospective and mission update series. We’ll see how that goes.

## Big News
Contour has been accepted into the CNCF at the “incubating” level!

![Woohoo](https://frinkiac.com/gif/S04E17/1281462/1283464.gif?b64lines=V29vaG9v)

This is a huge testament to the work of the Contour community, and it comes with a few benefits to our community:
- Being part of the CNCF means Contour is more closely associated (and aligned) with our two main dependencies, Envoy and Kubernetes. 
- The CNCF offers our project vendor neutral governance to encourage more companies and contributors to join our community, contribute, and help us deliver on the vision of Contour. 
- We believe the CNCF's networking community can play a big role in shaping the opinion for ingress controllers and Envoy control planes.
- The CNCF offers access to resources (like DevStats, assistance with user and developer outreach, and centralized functions) that help optimize our growth.

This is also a good time for a look back at what we, the Contour project, have done, and where we are going.


## The Past and Present
### The Good

Contour is, by most measures I can think of, a reasonably successful open source project. We have good velocity, we’ve released a stable v1.0, the project is being used in production, and we have a healthy stream of incoming feature requests from users.

When we started, we initially created a custom resource, IngressRoute, which introduced the idea of delegating a URL path, enabling Contour users to split Ingress configuration between teams without risking people inadvertently clobbering each other’s config. This was an improvement to the default Ingress behavior, which had quite a few examples of this creating outages for users.

When we realized that the delegation model was not capable of managing more dimensions than just path, we rebuilt the model into inclusion inside the new HTTPProxy resource, which, as of Contour 1.0, has been marked as GA by being moved to `v1`. The reason we moved to inclusion was to allow the use of other factors than just the URL path to be used for segmenting the URL space - you can now combine path prefix matching with header matching to make routing decisions in HTTPProxy.

Since we are using a similar contract as the majority of the Kubernetes ecosystem, moving to v1 means that we will support the currently existing fields, and only make additive changes to this object from now on. If substantial changes are required, we’ll start that process with a `v2alpha1` version, although this is unlikely for other reasons I’ll go into in a bit. We’ve also removed IngressRoute in favor of this new resource, in order to concentrate our efforts on doing one thing well.

Over time, we’ve also built and maintained a consistent project direction around simplicity and abstracting Envoy pieces away, so that you can use Contour without needing to know a lot about Envoy. This has helped new Contour users get started, but has come at the expense of configurability.

We’ve made great strides as well in increasing the operability and reliability of Contour and Envoy working together. There have been a great deal of optimizations on how Contour sends updates to Envoy that have helped some large installations with high config churn drop their Envoy memory usage dramatically. We’ve also added observability tools to enable more and better metrics, dashboards, and logging.

### The Bad

Deprecating IngressRoute made life hard for the people who were using it. We do provide a tool, ([ir2proxy](https://github.com/projectcontour/ir2proxy)), to help with migrating to HTTPProxy, but the substantial model changes between IngressRoute and HTTPProxy means that we can’t have a fully automated solution, sadly. We won’t be changing the name again without an extremely good reason. In addition, we are confident that we should be able to change HTTPProxy additively to add all the features we can think of right now. So you should use HTTPProxy knowing that Contour will support it for the foreseeable future.

We’ve still got a long way to go with operability. Contour’s current position on command-line arguments and the configuration file can be hard to understand. We will be moving focus to using the configuration file as the first place to configure the Contour service, rather than command-line flags, as we have more options around versioning configuration files than we do for command-line flags.

Lastly, while our stance on simplicity has helped Contour work with a small team, and deliver some well-designed features, it’s meant that we’ve moved too slowly in building out new features and in exposing some Envoy functionality that is actually quite necessary in any proxy.

In understanding what we were building with Contour, we overlooked the extent to which proxies are used as the place to tweak behaviors when you don’t control one end or the other. A great example of this is timeouts, as documented in [contour#2225](https://github.com/projectcontour/contour/issues/2225) and others. In particular, the addition of requestTimeout to HTTPProxy was a user-requested change, but in serving the consumers of HTTPProxy (in our current persona set, the Application Developer), we’ve inadvertently made a problem for the Contour deployment owner (currently the Cluster Administrator), since it’s possible for Application Developers to set the timeout to ‘unlimited’, even when the Cluster Admin may not want to allow that.

We’ve had a similar problem in the past with the `permitInsecure` option for HTTPProxy, and have a `--disable-permit-insecure` flag to stop that flag working, but this doesn’t seem like the right thing for a more general solution.

### The Ugly

Talking to users of Contour, we’ve found that a number of teams have forked Contour and maintain their own patchsets so that they can run it in production. This says to me that we’ve missed something about their use cases, since running a fork is not an insignificant engineering investment, and if that’s considered a better use of engineering resources, then we need to reevaluate how easy it is to bring changes upstream, how we work with people to bring their changes, and what sort of changes we’ll accept. The good side of people forking Contour is that the software is obviously useful to them, and the features that people forked Contour for are a good indicator of just how important those features are for them.

If you’re in the “We forked Contour” camp, or even if you haven’t, we’d love to hear from you. Please come to our [community meetings](https://projectcontour.io/community/), engage with us on [Slack](https://kubernetes.slack.com/messages/contour), or create issues to bring things to our attention.

## The Future

So, while Contour has done a few things really well, there’s also plenty of room for improvement.

Before we get into what we want to do about improving Contour and our community, I’d like to talk a little bit about supportability.

Before I was an open-source maintainer, I spent a long time as a sysadmin (as we used to be called), consuming tools like Contour.
So I understand that, sometimes, you just want a little bit of configurability to help you solve the problem you have, or to help you determine what’s causing it.
And that’s exactly what open source is for, so you can request (or make) the changes you need.
But, as maintainers of a project that we need to be able to maintain indefinitely, it’s also on us to make sure that each new thing we add is maintainable and supportable.
 
We don't want features in Contour that are difficult to use correctly. To say this another way, if Contour takes configuration, we (the maintainers) should be able to tell you what Contour has done with it, and be able to help you help yourself if a feature doesn't work.
 
More concretely, we've found that often, the *code changes* required to enable a feature are much less work than the *design changes* required to make the feature work in a way that makes sense for the future of Contour.

The thing to remember is that when we’re being cautious, it’s because we want this project to be around to help for a long time.
 
With that said, here are some things the Contour team is going to do to try and improve.

### Building greater configurability

In order to recognize that a segment of our users really need more configurability, we are going to allow more configuration of a greater set of settings. In the past we’ve tried to set reasonable defaults, and add configurable settings as a last resort.

We’re going to be building more configurability, and pushing that configurability towards Contour’s configuration file with the aim of exposing whatever complexity we need in there for Contour-level configuration (as opposed to individual-resource level configuration). We’ll be starting with timeouts, putting configuration of the timeouts requested in [contour#2225](https://github.com/projectcontour/contour/issues/2225) into the config file, with some thought put into both how to configure upper and/or lower bounds for those timeouts if necessary, and how to name the timeouts to make sense for people who only know Contour, not Envoy.

Over time, we’ll try to limit the number of command line flags to Contour, with the aim of eventually paring Contour down to just the flags useful for fast local development (such as `--insecure` to not require TLS certs for xDS serving, or `--debug` to enable debug logging).

The corollary to accepting greater configurability is that, whatever we do, we need to support. So, the requirement for these timeout settings will be better documentation of the configuration file, and documentation of where in a request chain the timeouts apply. More generally, I’ll be asking that designs that add open-ended configuration include documentation about why the default value was chosen, and what are reasonable values for at least some use cases.

Other features similar to this include the addition of CORS support to HTTPProxy, and similar tweakables.

### Exposing more Envoy configuration
We’ve had a few requests for exposing more Envoy-specific configuration settings over Contour’s lifetime, and historically, we’ve been reluctant to break Contour’s abstraction in any substantial way, preferring to expose an abstracted version rather than the Envoy config directly. The exception to this is the bootstrap configuration - we currently supply `contour bootstrap` as a way to automatically generate this config, but it’s always been intended that Contour users could generate their own bootstrap configuration and add additional features if they wish. We’ve not done a great job of communicating this ability in the past.

All of this is because Envoy is a fast-moving project that often changes both the implementation of features, or directly removes them. I believe that this problem will only be exacerbated with the rapid rate of change for the xDS APIs, so I’d prefer not to completely give up on abstracting things.

However, it’s increasingly clear that we’re poorly serving those users of Contour who do understand Envoy, so I’m open to talking more about where we draw the line here. For example, we have some requests for us to allow passing Lua filters to Envoy. I don’t think that this is a feature we can easily support in Contour - there’s no way that we could test anything other than “is a basic Lua filter sent to Envoy?”, and we’re not a Lua product, so we can’t really help too much with troubleshooting the Lua filter itself - but if we could find a way to make exactly what we do support clear, I think we could consider it.

Another, more implementable example is configuring tracing, which requires a global Envoy config, and per-route config, but also comes under the next section, as we’re configuring Envoy to talk to a third service.

### Configuring Envoy with third-party services
Another closely related set of requests are those for functionality that requires configuring Envoy to talk to another service, either to retrieve information from it, or send to it. Examples here are external authentication, rate limiting, tracing, and Envoy’s ALS logging.

While each of these needs to be considered on their own, one thing that I think is not negotiable is this: If Contour accepts the configuration for something, then it bears the responsibility for communicating whether that configuration is syntactically valid, accepted by Envoy, and doing what you asked for. 

For each of the examples, we need two things: information on what Contour has configured, and a way to surface that information back to the person who configured it. For some of the features, this is an application-level thing (like ALS and maybe tracing), and we may need to surface the information in Contour’s logs. For other features, this is resource-level config, and should be surfaced back on the resource that configured it (for example in the HTTPProxy status.) I’ve logged [contour#2325](https://github.com/projectcontour/contour/issues/2325) to cover an idea I had for one solution to this.

In short, we would love to have more support for configuring Envoy with other services. Before we can do that, properly integrating any external service into the overall Contour picture requires foundational work to build the model for if and how Contour interacts with these services. This work has started (see [contour#2325](https://github.com/projectcontour/contour/issues/2325) and [contour#2495](https://github.com/projectcontour/contour/issues/2495) for some example issues), and we would love feedback about our approach and the implementation. Getting this foundational work right is tricky and may take some time, but once we have it done, we should be able to get the actual features out much more easily in the future.

### Service-APIs and the future of Ingress
As an Ingress controller, we on the Contour team have been watching the upstream work on Ingress v1 and contributing to the the [Service-APIs subproject of Kubernetes SIG-Network](https://kubernetes-sigs.github.io/service-apis) (which started out of Ingress V2 work, and has a similar forward looking design to HTTPProxy). Contour is committed to migrating Ingress support to Ingress v1 as soon as support windows allow ([contour#2139](https://github.com/projectcontour/contour/issues/2139)), and are actively contributing to the Service APIs subproject, with the aim of bringing Contour support for the new Service APIs objects as soon as they have enough behavior defined to be implementable.


## We want your feedback
Thanks for reading! We’d love to hear from you about these changes to Contour’s direction. Please come to our [community meeting](https://projectcontour.io/community/), join our [Slack channel](https://kubernetes.slack.com/messages/contour), and check out [our roadmap](https://github.com/projectcontour/community/blob/master/ROADMAP.md).

---
title: Contour Adopters
layout: page
---

If you're using Contour and want to add your organization to this list, please
[submit a pull request][1]!

<a href="https://knative.dev" target="_blank"><img alt="knative.dev" src="../../img/adopters/knative.svg" height="50"></a>


<a href="https://www.vmware.com" target="_blank"><img alt="vmware.com" src="../../img/adopters/VMware-logo-grey.jpg" height="50"></a>

<a href="https://flyte.org/" target="_blank"><img alt="flyte.com" src="../../img/adopters/flyte.png" height="50"></a>

<a href="https://gojek.io/"  target="_blank"><img alt="gojek.io" src="../../img/adopters/gojek.svg" height="50"></a>

<a href="https://daocloud.io/" target="_blank"><img alt="daocloud.io" src="../../img/adopters/daocloud.png" height="50"></a>

<a href="https://snapp.ir/" target="_blank"><img alt="snapp.ir" src="../../img/adopters/snappcloud.png" height="50"></a>

<a href="https://bugfender.com/" target="_blank"><img alt="bugfender.com" src="../../img/adopters/bugfender.svg" height="50"></a>

## Success Stories

Below is a list of adopters of Contour in **production environments** that have
publicly shared the details of how they use it.

**Migrating from Openshift Router to Contour in SnappCloud**
SnappCloud is the private cloud infrastructure provider for Snapp, the largest ride-hailing platform in the Middle East. In addition to ride-hailing, Snapp supports a range of services including online doctor consultations, food shopping, and more. Within our infrastructure, we utilize multiple OKD (OpenShift) clusters. We have successfully transitioned from the OpenShift Router Controller to Contour for our ingress needs. To maintain consistent behavior during this migration, we employed the route-to-contour-httpproxy project. This Kubernetes controller is designed for converting OpenShift HAProxy Route to Contour HTTPProxy, incorporating default values of OpenShift Router HAProxy and converting OpenShift-specific annotations to HTTPProxy configurations.

## Solutions built with Contour

Below is a list of solutions where Contour is being used as a component.

**[Knative](https://knative.dev)**
Knative can use Contour to serve all incoming traffic via the `net-contour` ingress Gateway. The [net-contour](https://github.com/knative-sandbox/net-contour) controller enables Contour to satisfy the networking needs of Knative Serving by bridging Knative's KIngress resources to Contour's HTTPProxy resources.

**[VMware](https://tanzu.vmware.com/tanzu)**
All four [VMware Tanzu](https://tanzu.vmware.com/content/blog/simplify-your-approach-to-application-modernization-with-4-simple-editions-for-the-tanzu-portfolio) editions make the best possible use of various open source projects, starting with putting Kubernetes at their core. We’ve included leading projects to provide our customers with flexibility and a range of necessary capabilities, including Harbor (for image registry), Antrea (for container networking), Contour (for ingress control), and Cluster API (for lifecycle management).

**[Flyte](https://flyte.org/)**
Flyte's [sandbox environment](https://docs.flyte.org/en/latest/deployment/sandbox.html#deployment-sandbox) is powered by Contour and this is the default Ingress Controller. Sandbox environment has made it possible for data scientists all over to try out Flyte quickly and without contour that would not have been easy.

**[Gojek](https://gojek.io/)**

Gojek launched in 2010 as a call center for booking motorcycle taxi rides in Indonesia. Today, the startup is a decacorn serving millions of users across Southeast Asia with its mobile wallet, GoPay, and 20+ products on its super app. Want to order dinner? Book a massage? Buy movie tickets? You can do all of that with the Gojek app.

The company’s mission is to solve everyday challenges with technology innovation. To achieve that across multiple markets, the team at Gojek focused on building an infrastructure for speed, reliability, and scale.

Gojek Data Platform team processes more than hundred terabyte data per day. We are fully using kubernetes in our production environment and chose Contour as the ingress for all kubernetes clusters that we have.

**[DaoCloud](https://daocloud.io/)**

DaoCloud is an innovation leader in the cloud-native field. With the competitive expertise of its proprietary intellectual property rights, DaoCloud is committed to creating an open Cloud OS which enables enterprises to easily carry out digital transformation.

DaoCloud build Next Generation Microservices Gateway based on Contour, and also contribute in Contour Community deeply.

**[SnappCloud](https://snapp.ir)**

SnappCloud has developed several solutions to provide a complete self-service and multi-tenant API-GW solution with Contour:

1. [Cerberus](https://github.com/snapp-incubator/Cerberus): Cerberos is a powerful authorization server designed to seamlessly integrate with Contour by implementing the auth_ext interface of Envoy. In the world of modern application deployment and microservices architecture, ensuring secure and controlled access to services is paramount. Cerberos fills this role by providing a dynamic and flexible access control solution tailored to the unique demands of Contour-based applications.

2. [Contour Global Rate Limit Operator](https://github.com/snapp-incubator/contour-global-ratelimit-operator): This project provides a Kubernetes operator that allows users to configure global ratelimits in their HTTPProxy and it configures a RLS service based on [envoyproxy/ratelimit](https://github.com/envoyproxy/ratelimit).

3. [Contour Admission Webhook](https://github.com/snapp-incubator/contour-admission-webhook): This webhook facilitates the validation and mutation of Contour's HTTPProxy resources, ensuring configurations adhere to defined policies and standards. For example, it blocks creation of HTTPProxies with conflicting FQDNs, to prevent a user to invalidate other HTTPProxies in other namespaces.

4. [Contour Console Plugin](https://github.com/snapp-incubator/contour-console-plugin): A plugin based on [Openshift Dynamic Plugins](https://www.redhat.com/blog/dynamic-plugins-now-available) designed to integrate with Openshift consoles, providing a user-friendly interface to manage and visualize Contour resources, and to have a form based creation of HTTPProxies, same as `Route` experience in openshift.

5. [Contour Auth Multi-Tenant](https://github.com/snapp-incubator/contour-auth-multi-tenant): This project is an Envoy-compatible authorization server that builds upon the foundation of [contour-authserver](https://github.com/projectcontour/contour-authserver), enabling multi-tenancy by allowing different tenants to manage their authentication services independently, and referencing their own secrets in the same namespace of HTTPProxy.

At SnappCloud, we are dedicated to enriching the open-source community by developing additional components and plugins, contributing to various projects, and weaving together open-source solutions to create integrated, full-fledged products that rival enterprise solutions. Our commitment is focused on building robust toolchains that enhance and extend the capabilities of the open-source ecosystem.

**[Bugfender](https://bugfender.com)**

Bugfender is a log aggregation platform designed for mobile and web front-end applications, with a strong focus on security and privacy. Its SDK seamlessly integrates into applications, facilitating log transmission to the Bugfender Dashboard. Bugfender streamlines bug identification and reproduction for enhanced user experience.

At Bugfender, we chose Contour as Kubernetes ingress for its high performance. With millions of devices running Bugfender's SDK simultaneously communicating with our backend, Contour proves more efficient in handling TLS connections than its alternatives. This was pivotal for our growth.

## Adding a logo to projectcontour.io

If you would like to add your logo to a future `Adopters of Contour` section
of [projectcontour.io][2], add an SVG or PNG version of your logo to the site/img/adopters
directory in this repo and submit a pull request with your change.
Name the image file something that reflects your company
(e.g., if your company is called Acme, name the image acme.png).
We will follow up and make the change in the [projectcontour.io][2] website.

[1]: https://github.com/projectcontour/contour/pulls
[2]: https://projectcontour.io

# Contour Adopters

If you're using Contour and want to add your organization to this list, please
[submit a pull request][1]!

<a href="https://knative.dev" border="0" target="_blank"><img alt="knative.dev" src="site/img/adopters/knative.svg" height="50"></a>

<a href="https://www.vmware.com" border="0" target="_blank"><img alt="vmware.com" src="site/img/adopters/VMware-logo-grey.jpg" height="50"></a>

<a href="https://flyte.org/" border="0" target="_blank"><img alt="flyte.com" src="site/img/adopters/flyte.png" height="50"></a>&nbsp; &nbsp; &nbsp;

<a href="https://gojek.io/" border="0" target="_blank"><img alt="gojek.io" src="site/img/adopters/gojek.svg" height="50"></a>

<a href="https://daocloud.io/" border="0" target="_blank"><img alt="daocloud.io" src="site/img/adopters/daocloud.png" height="50"></a>

## Success Stories

Below is a list of adopters of Contour in **production environments** that have
publicly shared the details of how they use it.

_Add yours here!_

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

## Adding a logo to projectcontour.io

If you would like to add your logo to a future `Adopters of Contour` section
of [projectcontour.io][2], add an SVG or PNG version of your logo to the site/img/adopters
directory in this repo and submit a pull request with your change.
Name the image file something that reflects your company
(e.g., if your company is called Acme, name the image acme.png).
We will follow up and make the change in the [projectcontour.io][2] website.

[1]: https://github.com/projectcontour/contour/pulls
[2]: https://projectcontour.io

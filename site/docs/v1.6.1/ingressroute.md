# IngressRoute Reference

<div id="toc" class="navigation"></div>

The [Ingress][1] object was added to Kubernetes in version 1.1 to describe properties of a cluster-wide reverse HTTP proxy.
Since that time, the Ingress object has not progressed beyond the beta stage, and its stagnation inspired an [explosion of annotations][2] to express missing properties of HTTP routing.

The goal of the `IngressRoute` Custom Resource Definition (CRD) was to expand upon the functionality of the Ingress API to allow for a richer user experience as well as solve shortcomings in the original design.

<p class="alert-deprecation">
<b>Removal Notice</b><br>
The <code>IngressRoute</code> CRD has been removed from Contour.
Please see the documentation for <a href="{% link docs/{{site.latest}}/httpproxy.md %}"><code>HTTPProxy</code></a>, which is the successor to <code>IngressRoute</code>.
You can also read the <a href="{% link _guides/ingressroute-to-httpproxy.md %}">IngressRoute to HTTPProxy upgrade</a> guide.
</p>

[1]: https://kubernetes.io/docs/concepts/services-networking/ingress/
[2]: https://github.com/kubernetes/ingress-nginx/blob/main/docs/user-guide/nginx-configuration/annotations.md

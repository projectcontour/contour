---
layout: page
title: Resources
description: Contour Resources
id: resources
---
## Resources

{% for resource in site.resources %}
- [{{ resource.title }}]({{ resource.url }})
{% endfor %}

## Presentations

* Contour – High performance ingress controller for Kubernetes (October, 2019)

<iframe width="560" height="315" src="https://www.youtube.com/embed/764YUk-wSa0" frameborder="0" allow="accelerometer; autoplay; encrypted-media; gyroscope; picture-in-picture" allowfullscreen></iframe>

* Contour 101 - Kubernetes Ingress and Blue/Green Deployments (April, 2019)

<iframe width="560" height="315" src="https://www.youtube.com/embed/xUJbTnN3Dmw" frameborder="0" allow="accelerometer; autoplay; encrypted-media; gyroscope; picture-in-picture" allowfullscreen></iframe>

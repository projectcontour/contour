---
layout: page
title: Contour Documentation
description: Contour Documentation / Contour Docs
id: docs
---

## Contour 1.0 Documentation

{% for doc in site.docs_1_0 %}
- [{{ doc.title }}]({{ doc.url }})
{% endfor %}

## Pre Contour 1.0 Documentation

The documentation for older versions of Contour is available on GitHub.

{% for repository in site.github.releases limit:10 %}
{% if repository.prerelease == false %}
- [{{ repository.tag_name }}]({{ site.github.repository_url }}/tree/{{ repository.tag_name }}/docs)
{% endif %}
{% endfor %}

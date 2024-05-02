We are delighted to present version {{ .Version }} of Contour, our layer 7 HTTP reverse proxy for Kubernetes clusters.

A big thank you to everyone who contributed to the release.
{{ if .Prerelease }}
**Please note that this is pre-release software**, and as such we do not recommend installing it in production environments.
Feedback and bug reports are welcome!
{{ end }}

- [Major Changes](#major-changes)
- [Minor Changes](#minor-changes)
- [Other Changes](#other-changes)
- [Docs Changes](#docs-changes)
- [Deprecations/Removals](#deprecation-and-removal-notices)
- [Installing/Upgrading](#installing-and-upgrading)
- [Compatible Kubernetes Versions](#compatible-kubernetes-versions)
- [Community Thanks!](#community-thanks)

# Major Changes
{{ range .Major }}
{{ .Content }}

(#{{ .PRNumber }}, {{ .Author }})
{{ end }}

# Minor Changes
{{ range .Minor }}
{{ .Content }}

(#{{ .PRNumber }}, {{ .Author }})
{{ end }}

# Other Changes
{{ range .Small }}- {{ .Content }} (#{{ .PRNumber }}, {{ .Author }})
{{ end }}

# Docs Changes
{{ range .Docs }}- {{ .Content }} (#{{ .PRNumber }}, {{ .Author }})
{{ end }}

# Deprecation and Removal Notices

{{ range .Deprecation }}
{{ .Content }}

(#{{ .PRNumber }}, {{ .Author }})
{{ end }}

# Installing and Upgrading
{{ if .Prerelease}}
The simplest way to install {{ .Version }} is to apply one of the example configurations:

Standalone Contour:
```bash
kubectl apply -f https://raw.githubusercontent.com/projectcontour/contour/{{ .Version }}/examples/render/contour.yaml
```

Contour Gateway Provisioner:
```bash
kubectl apply -f https://raw.githubusercontent.com/projectcontour/contour/{{ .Version }}/examples/render/contour-gateway-provisioner.yaml
```

Statically provisioned Contour with Gateway API:
```bash
kubectl apply -f https://raw.githubusercontent.com/projectcontour/contour/{{ .Version }}/examples/render/contour-gateway.yaml
```
{{ else }}
For a fresh install of Contour, consult the [getting started documentation](https://projectcontour.io/getting-started/).

To upgrade an existing Contour installation, please consult the [upgrade documentation](https://projectcontour.io/resources/upgrading/).
{{ end }}

# Compatible Kubernetes Versions

Contour {{ .Version }} is tested against Kubernetes {{ .KubernetesMinVersion }} through {{ .KubernetesMaxVersion }}.

# Community Thanks!
We’re immensely grateful for all the community contributions that help make Contour even better! For this release, special thanks go out to the following contributors:

{{ range .Contributors }}- {{ . }}
{{ end}}

# Are you a Contour user? We would love to know!
If you're using Contour and want to add your organization to our adopters list, please visit this [page](https://projectcontour.io/resources/adopters/). If you prefer to keep your organization name anonymous but still give us feedback into your usage and scenarios for Contour, please post on this [GitHub thread](https://github.com/projectcontour/contour/issues/1269).

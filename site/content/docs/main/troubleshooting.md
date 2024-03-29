## Troubleshooting

If you encounter issues, follow the guides below for help. For topics not covered here, you can [file an issue][0], or talk to us on the [#contour channel][1] on Kubernetes Slack.

### [Troubleshooting Common Proxy Errors][2]
A guide on how to investigate common errors with Contour and Envoy.

### [Envoy Administration Access][3]
Review the linked steps to learn how to access the administration interface for your Envoy instance.

### [Contour Debug Logging][4]
Learn how to enable debug logging to diagnose issues between Contour and the Kubernetes API.

### [Envoy Debug Logging][5]
Learn how to enable debug logging to diagnose TLS connection issues.

### [Visualize the Contour Graph][6]
Learn how to visualize Contour's internal object graph in [DOT][9] format, or as a png file.

### [Show Contour xDS Resources][7]
Review the linked steps to view the [xDS][10] resource data exchanged by Contour and Envoy.

### [Profiling Contour][8]
Learn how to profile Contour by using [net/http/pprof][11] handlers. 

### [Envoy container stuck in unready/draining state][12]
Read the linked document if you have Envoy containers stuck in an unready/draining state.

[0]: {{< param github_url >}}/issues
[1]: {{< param slack_url >}}
[2]: /docs/{{< param version >}}/troubleshooting/common-proxy-errors/
[3]: /docs/{{< param version >}}/troubleshooting/envoy-admin-interface/
[4]: /docs/{{< param version >}}/troubleshooting/contour-debug-log/
[5]: /docs/{{< param version >}}/troubleshooting/envoy-debug-log/
[6]: /docs/{{< param version >}}/troubleshooting/contour-graph/
[7]: /docs/{{< param version >}}/troubleshooting/contour-xds-resources/
[8]: /docs/{{< param version >}}/troubleshooting/profiling-contour/
[9]: https://en.wikipedia.org/wiki/Dot
[10]: https://www.envoyproxy.io/docs/envoy/latest/api-docs/xds_protocol
[11]: https://golang.org/pkg/net/http/pprof/
[12]: /docs/{{< param version >}}/troubleshooting/envoy-container-draining/

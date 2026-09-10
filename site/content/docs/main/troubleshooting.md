## Troubleshooting

If you encounter issues, follow the guides below for help. For topics not covered here, you can [file an issue][0], or talk to us on the [#contour channel][1] on Kubernetes Slack.

### [Troubleshooting Common Proxy Errors][2]
A guide on how to investigate common errors with Contour and Envoy.

### [Envoy Administration Access][3]
Review the linked steps to learn how to access the administration interface for your Envoy instance.

### [Troubleshooting with Envoy Statistics][4]
Learn how to use Envoy statistics to follow a request from a downstream listener to a backend Service.

### [Contour Debug Logging][5]
Learn how to enable debug logging to diagnose issues between Contour and the Kubernetes API.

### [Envoy Debug Logging][6]
Learn how to enable debug logging to diagnose TLS connection issues.

### [Visualize the Contour Graph][7]
Learn how to visualize Contour's internal object graph in [DOT][10] format, or as a png file.

### [Show Contour xDS Resources][8]
Review the linked steps to view the [xDS][11] resource data exchanged by Contour and Envoy.

### [Profiling Contour][9]
Learn how to profile Contour by using [net/http/pprof][12] handlers.

### [Envoy container stuck in unready/draining state][13]
Read the linked document if you have Envoy containers stuck in an unready/draining state.

[0]: {{< param github_url >}}/issues
[1]: {{< param slack_url >}}
[2]: /docs/{{< param version >}}/troubleshooting/common-proxy-errors/
[3]: /docs/{{< param version >}}/troubleshooting/envoy-admin-interface/
[4]: /docs/{{< param version >}}/troubleshooting/envoy-statistics/
[5]: /docs/{{< param version >}}/troubleshooting/contour-debug-log/
[6]: /docs/{{< param version >}}/troubleshooting/envoy-debug-log/
[7]: /docs/{{< param version >}}/troubleshooting/contour-graph/
[8]: /docs/{{< param version >}}/troubleshooting/contour-xds-resources/
[9]: /docs/{{< param version >}}/troubleshooting/profiling-contour/
[10]: https://en.wikipedia.org/wiki/Dot
[11]: https://www.envoyproxy.io/docs/envoy/latest/api-docs/xds_protocol
[12]: https://golang.org/pkg/net/http/pprof/
[13]: /docs/{{< param version >}}/troubleshooting/envoy-container-draining/

---
title: Contour / Envoy Resource Limits
---

## Performance Testing Contour / Envoy

- Cluster Specs
  - Kubernetes
    - Version: v1.12.6
    - Nodes:
      - 5 Worker Nodes
        - 2 CPUs Per Node
        - 8 GB RAM Per Node
      - 10 GB Network
  - Contour
    - Single Instance
      - 4 Instances of Envoy running in a Daemonset
      - Each instance of Envoy is running with HostNetwork
    - Cluster Network Bandwidth

Having a good understanding of the available bandwidth is key when it comes to analyzing performance. It will give you a sense of how many requests per second you can expect to push through the network you are working with.

Use iperf3 to figure out the bandwidth available between two of the kubernetes nodes. The following will deploy an iperf3 server on one node, and an iperf3 client on another node:

```bash
[ ID] Interval           Transfer     Bandwidth       Retr
[  4]   0.00-60.00  sec  34.7 GBytes  4.96 Gbits/sec  479             sender
[  4]   0.00-60.00  sec  34.7 GBytes  4.96 Gbits/sec                  receiver
```

## Memory / CPU usage

Verify the Memory & CPU usage with varying numbers of services, IngressRoute resources, and traffic load into the cluster.

<table>
  <tr>
    <td colspan="4">Test Criteria</td>
    <td colspan="2">Contour</td>
    <td colspan="2">Envoy</td>
  </tr>
  <tr>
    <td>#Svc</td>
    <td>#Ing</td>
    <td>RPS</td>
    <td>CC</td>
    <td>Memory (MB)</td>
    <td>CPU% / Core</td>
    <td>Memory (MB)</td>
    <td>CPU% / Core</td>
  </tr>
  <tr>
    <td align="right">0</td>
    <td align="right">0</td>
    <td align="right">0</td>
    <td align="right">0</td>
    <td align="right">10</td>
    <td align="right">0</td>
    <td align="right">15</td>
    <td align="right">0</td>
  </tr>
  <tr>
    <td align="right">5k</td>
    <td align="right">0</td>
    <td align="right">0</td>
    <td align="right">0</td>
    <td align="right">46</td>
    <td align="right">2%</td>
    <td align="right">15</td>
    <td align="right">0%</td>
  </tr>
  <tr>
    <td align="right">10k</td>
    <td align="right">0</td>
    <td align="right">0</td>
    <td align="right">0</td>
    <td align="right">77</td>
    <td align="right">3%</td>
    <td align="right">205</td>
    <td align="right">2%</td>
  </tr>
  <tr>
    <td align="right">0</td>
    <td align="right">5k</td>
    <td align="right">0</td>
    <td align="right">0</td>
    <td align="right">36</td>
    <td align="right">1%</td>
    <td align="right">230</td>
    <td align="right">2%</td>
  </tr>
  <tr>
    <td align="right">0</td>
    <td align="right">10k</td>
    <td align="right">0</td>
    <td align="right">0</td>
    <td align="right">63</td>
    <td align="right">1%</td>
    <td align="right">10</td>
    <td align="right">1%</td>
  </tr>
  <tr>
    <td align="right">5k</td>
    <td align="right">5k</td>
    <td align="right">0</td>
    <td align="right">0</td>
    <td align="right">244</td>
    <td align="right">1%</td>
    <td align="right">221</td>
    <td align="right">1%</td>
  </tr>
  <tr>
    <td align="right">10k</td>
    <td align="right">10k</td>
    <td align="right">0</td>
    <td align="right">0</td>
    <td align="right">2600</td>
    <td align="right">6%</td>
    <td align="right">430</td>
    <td align="right">4%</td>
  </tr>
  <tr>
    <td align="right">0</td>
    <td align="right">0</td>
    <td align="right">30k</td>
    <td align="right">600</td>
    <td align="right">8</td>
    <td align="right">1%</td>
    <td align="right">17</td>
    <td align="right">3%</td>
  </tr>
  <tr>
    <td align="right">0</td>
    <td align="right">0</td>
    <td align="right">100k</td>
    <td align="right">10k</td>
    <td align="right">10</td>
    <td align="right">1%</td>
    <td align="right">118</td>
    <td align="right">14%</td>
  </tr>
  <tr>
    <td align="right">0</td>
    <td align="right">0</td>
    <td align="right">200k</td>
    <td align="right">20k</td>
    <td align="right">9</td>
    <td align="right">1%</td>
    <td align="right">191</td>
    <td align="right">31%</td>
  </tr>
  <tr>
    <td align="right">0</td>
    <td align="right">0</td>
    <td align="right">300k</td>
    <td align="right">30k</td>
    <td align="right">10</td>
    <td align="right">1%</td>
    <td align="right">225</td>
    <td align="right">40%</td>
  </tr>
</table>

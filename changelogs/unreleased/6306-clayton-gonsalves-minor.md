## Add route generation command

Adds a new contour command to generate envoy route configurations based on input kubernetes manifests.

```bash
❯ ./contour routegen --help
INFO[0000] maxprocs: Leaving GOMAXPROCS=10: CPU quota undefined
usage: contour routegen [<flags>] <resources>...

Generate envoy route configuration based on server config and resources


Flags:
  -h, --[no-]help        Show context-sensitive help (also try --help-long and --help-man).
      --log-format=text  Log output format for Contour. Either text or json.
  -c, --config-path=/path/to/file
                         Path to base configuration.
      --ingress-class-name=<name>
                         Contour IngressClass name.
      --output=OUTPUT    File to write route config into (default to `stdout`.

Args:
  <resources>  Set of input resource manifests that make up the envoy route configuration
```

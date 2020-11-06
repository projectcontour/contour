# Websockets

WebSocket support can be enabled on specific routes using the `enableWebsockets` field:

```yaml
# httpproxy-websockets.yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: chat
  namespace: default
spec:
  virtualhost:
    fqdn: chat.example.com
  routes:
  - services:
    - name: chat-app
      port: 80
  - conditions:
    - prefix: /websocket
    enableWebsockets: true # Setting this to true enables websocket for all paths that match /websocket
    services:
    - name: chat-app
      port: 80
```

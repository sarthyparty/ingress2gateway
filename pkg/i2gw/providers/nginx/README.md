# NGINX Ingress Controller Provider

This provider converts [NGINX Inc's Ingress Controller](https://github.com/nginxinc/kubernetes-ingress) resources to Gateway API resources.

**Note**: This provider is specifically for NGINX Inc's commercial Ingress Controller, not the community [ingress-nginx](https://github.com/kubernetes/ingress-nginx) controller. If you're using the community ingress-nginx controller, please use the `ingress-nginx` provider instead.

## Supported Resources

* **Ingress** - Core Kubernetes Ingress resources with NGINX-specific annotations
* **ConfigMap** - NGINX configuration maps referenced by Ingress resources

## Supported Annotations

* `nginx.org/ssl-services` - SSL/TLS backend connections
* `nginx.org/grpc-services` - gRPC backend connections  
* `nginx.org/websocket-services` - WebSocket backend connections
* `nginx.org/proxy-hide-headers` - Hide headers from responses
* `nginx.org/proxy-set-headers` - Set custom headers
* `nginx.org/listen-ports` - Custom HTTP ports
* `nginx.org/listen-ports-ssl` - Custom HTTPS ports
* `nginx.org/path-regex` - Regex path matching
* `nginx.org/rewrites` - URL rewriting
* `nginx.org/redirect-to-https` - SSL/HTTPS redirects

## Usage

```bash
# Convert NGINX Inc Ingress Controller resources from cluster
ingress2gateway print --providers=nginx

# Convert from file
ingress2gateway print --providers=nginx --input-file=nginx-ingress.yaml
```

## Gateway API Mapping

| NGINX Annotation | Gateway API Resource              |
|---|-----------------------------------|
| `nginx.org/ssl-services` | BackendTLSPolicy                  |
| `nginx.org/grpc-services` | GRPCRoute                         |
| `nginx.org/websocket-services` | Informational notification only  |
| `nginx.org/proxy-hide-headers` | HTTPRoute ResponseHeaderModifier  |
| `nginx.org/proxy-set-headers` | HTTPRoute RequestHeaderModifier   |
| `nginx.org/rewrites` | HTTPRoute URLRewrite filter       |
| `nginx.org/listen-ports*` | Gateway custom listeners          |
| `nginx.org/path-regex` | HTTPRoute RegularExpression paths |
| `nginx.org/redirect-to-https` | HTTPRoute RequestRedirect filter  |

## Examples

```yaml
# SSL Backend
annotations:
  nginx.org/ssl-services: "backend-service"

# WebSocket Backend
annotations:
  nginx.org/websocket-services: "websocket-service"

# Header Manipulation  
annotations:
  nginx.org/proxy-set-headers: "X-Custom-Header: custom-value"
  nginx.org/proxy-hide-headers: "Server,X-Powered-By"

# URL Rewriting
annotations:
  nginx.org/rewrites: "api-service=/v1/api"

# Custom Ports
annotations:
  nginx.org/listen-ports: "8080,8081"
  nginx.org/listen-ports-ssl: "8443"

# Regex Paths
annotations:
  nginx.org/path-regex: "true"

# SSL Redirect
annotations:
  nginx.org/redirect-to-https: "true"
```

## Contributing

When adding support for new NGINX Inc Ingress Controller annotations:

1. Add the annotation constant to `annotations/constants.go`
2. Implement the conversion logic in the appropriate `annotations/*.go` file
3. Add comprehensive tests in `annotations/*_test.go`
4. Update this README with the new annotation details

For more information on the provider architecture, see [PROVIDER.md](../../PROVIDER.md).

## References

* [NGINX Inc Kubernetes Ingress Controller](https://github.com/nginxinc/kubernetes-ingress)
* [NGINX Inc Ingress Controller Annotations](https://docs.nginx.com/nginx-ingress-controller/configuration/ingress-resources/advanced-configuration-with-annotations/)
* [Gateway API Documentation](https://gateway-api.sigs.k8s.io/)
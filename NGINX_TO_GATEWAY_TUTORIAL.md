# NGINX Ingress to Gateway API Conversion Tutorial

This tutorial demonstrates how to convert NGINX Ingress Controller resources (Ingress with annotations and VirtualServer CRDs) to Gateway API using the `ingress2gateway` tool.

## Prerequisites

1. Build the `ingress2gateway` tool:
   ```bash
   go build -o ingress2gateway .
   ```

2. The tool supports both NGINX Ingress Controller annotations and custom resources:
   - **Ingress resources** with `nginx.org/*` annotations
   - **VirtualServer** and **VirtualServerRoute** custom resources
   - **GlobalConfiguration** for custom listeners

## Table of Contents

1. [Basic Ingress with Annotations](#1-basic-ingress-with-annotations)
2. [URL Rewriting](#2-url-rewriting)
3. [SSL Redirect](#3-ssl-redirect) 
4. [Header Manipulation](#4-header-manipulation)
5. [VirtualServer Basic Routing](#5-virtualserver-basic-routing)
6. [Traffic Splitting (Canary Deployments)](#6-traffic-splitting-canary-deployments)
7. [gRPC Services](#7-grpc-services)
8. [Custom Listeners with GlobalConfiguration](#8-custom-listeners-with-globalconfiguration)
9. [Advanced Features](#9-advanced-features)

---

## 1. Basic Ingress with Annotations

### NGINX Ingress Resource
```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: test-ssl-redirect
  namespace: default
  annotations:
    nginx.org/redirect-to-https: "true"
spec:
  ingressClassName: nginx
  rules:
  - host: secure.example.com
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: secure-service
            port:
              number: 80
```

### Conversion Command
```bash
./ingress2gateway print --input-file ssl-redirect-example.yaml --providers nginx
```

### Generated Gateway API Resources
```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  annotations:
    gateway.networking.k8s.io/generator: ingress2gateway-dev
  labels:
    app.kubernetes.io/managed-by: ingress2gateway
  name: nginx
  namespace: default
spec:
  gatewayClassName: nginx
  listeners:
  - hostname: secure.example.com
    name: http-80-secure-example-com
    port: 80
    protocol: HTTP
  - hostname: secure.example.com
    name: https-443-secure-example-com
    port: 443
    protocol: HTTPS
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  annotations:
    gateway.networking.k8s.io/generator: ingress2gateway-dev
  labels:
    app.kubernetes.io/managed-by: ingress2gateway
  name: test-ssl-redirect
  namespace: default
spec:
  hostnames:
  - secure.example.com
  parentRefs:
  - name: nginx
    sectionName: http-80-secure-example-com
  - name: nginx
    sectionName: https-443-secure-example-com
  rules:
  - filters:
    - requestRedirect:
        scheme: https
        statusCode: 301
      type: RequestRedirect
    matches:
    - path:
        type: Prefix
        value: /
  - backendRefs:
    - name: secure-service
      port: 80
    matches:
    - path:
        type: Prefix
        value: /
```

**Key Points:**
- The `nginx.org/redirect-to-https` annotation creates a `RequestRedirect` filter
- Two listeners are created: one for HTTP (port 80) and one for HTTPS (port 443)
- The HTTP route includes a redirect rule before the backend rule

---

## 2. URL Rewriting

### NGINX Ingress with Rewrite Annotation
```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: test-rewrite
  namespace: default
  annotations:
    nginx.org/rewrites: "serviceName=web-service rewrite=/api/v1"
spec:
  ingressClassName: nginx
  rules:
  - host: example.com
    http:
      paths:
      - path: /app
        pathType: Prefix
        backend:
          service:
            name: web-service
            port:
              number: 80
```

### Generated HTTPRoute with URL Rewrite
```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  # ... metadata
spec:
  hostnames:
  - example.com
  parentRefs:
  - name: nginx
    sectionName: http-80-example-com
  rules:
  - backendRefs:
    - name: web-service
      port: 80
    filters:
    - type: URLRewrite
      urlRewrite:
        path:
          type: ReplacePrefixMatch
          replacePrefixMatch: /api/v1
    matches:
    - path:
        type: Prefix
        value: /app
```

**Key Points:**
- `nginx.org/rewrites` becomes a `URLRewrite` filter
- Uses `ReplacePrefixMatch` to rewrite `/app` requests to `/api/v1`
- Preserves sub-paths (e.g., `/app/users` → `/api/v1/users`)

---

## 3. SSL Redirect

As shown in Example 1, SSL redirects are automatically handled by creating both HTTP and HTTPS listeners with appropriate redirect filters.

---

## 4. Header Manipulation

### NGINX Ingress with Header Annotations
```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: header-manipulation
  namespace: default
  annotations:
    nginx.org/proxy-hide-headers: "Server,X-Powered-By,X-Custom-Header"
    nginx.org/proxy-set-headers: "X-Custom-Header: custom-value"
spec:
  ingressClassName: nginx
  rules:
  - host: api.example.com
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: api-service
            port:
              number: 8080
```

### Generated HTTPRoute with Header Filters
```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  # ... metadata
spec:
  rules:
  - backendRefs:
    - name: api-service
      port: 8080
    filters:
    - responseHeaderModifier:
        remove:
        - Server
        - X-Powered-By
        - X-Custom-Header
      type: ResponseHeaderModifier
    - requestHeaderModifier:
        set:
        - name: X-Custom-Header
          value: custom-value
      type: RequestHeaderModifier
    matches:
    - path:
        type: Prefix
        value: /
```

---

## 5. VirtualServer Basic Routing

### NGINX VirtualServer
```yaml
apiVersion: k8s.nginx.org/v1
kind: VirtualServer
metadata:
  name: basic-webapp
  namespace: default
spec:
  host: webapp.example.com
  tls:
    secret: webapp-tls
  upstreams:
  - name: webapp-backend
    service: webapp-service
    port: 8080
  - name: api-backend
    service: api-service
    port: 3000
  routes:
  - path: /
    action:
      pass: webapp-backend
  - path: /api
    action:
      pass: api-backend
  - path: /docs
    action:
      redirect:
        url: https://docs.example.com
        code: 301
```

### Generated Gateway and HTTPRoute
```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  labels:
    app.kubernetes.io/managed-by: ingress2gateway
    ingress2gateway.io/source: nginx-virtualserver
  name: nginx
  namespace: default
spec:
  gatewayClassName: nginx
  listeners:
  - hostname: webapp.example.com
    name: http-80-webapp-example-com
    port: 80
    protocol: HTTP
  - hostname: webapp.example.com
    name: https-443-webapp-example-com-webapp-tls
    port: 443
    protocol: HTTPS
    tls:
      certificateRefs:
      - group: ""
        kind: Secret
        name: webapp-tls
      mode: Terminate
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  labels:
    app.kubernetes.io/managed-by: ingress2gateway
    ingress2gateway.io/source: nginx-virtualserver
    ingress2gateway.io/vs-name: basic-webapp
  name: basic-webapp-httproute
  namespace: default
spec:
  hostnames:
  - webapp.example.com
  parentRefs:
  - name: nginx
    sectionName: http-80-webapp-example-com
  - name: nginx
    sectionName: https-443-webapp-example-com-webapp-tls
  rules:
  - backendRefs:
    - name: webapp-service
      port: 8080
    matches:
    - path:
        type: PathPrefix
        value: /
  - backendRefs:
    - name: api-service
      port: 3000
    matches:
    - path:
        type: PathPrefix
        value: /api
  - filters:
    - requestRedirect:
        hostname: docs.example.com
        scheme: https
        statusCode: 301
      type: RequestRedirect
    matches:
    - path:
        type: PathPrefix
        value: /docs
```

**Key Points:**
- VirtualServer `upstreams` define backend services
- Multiple routes are converted to HTTPRoute rules
- TLS configuration creates HTTPS listeners with certificate references
- Redirect actions become `RequestRedirect` filters

---

## 6. Traffic Splitting (Canary Deployments)

### NGINX VirtualServer with Traffic Splitting
```yaml
apiVersion: k8s.nginx.org/v1
kind: VirtualServer
metadata:
  name: traffic-splitting-vs
  namespace: default
spec:
  host: canary.example.com
  tls:
    secret: canary-tls
  upstreams:
  - name: stable-backend
    service: stable-service
    port: 8080
  - name: canary-backend
    service: canary-service
    port: 8080
  routes:
  - path: /app
    splits:
    - weight: 80
      action:
        pass: stable-backend
    - weight: 20
      action:
        pass: canary-backend
```

### Generated HTTPRoute with Weighted Backend References
```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  # ... metadata
spec:
  rules:
  - backendRefs:
    - name: stable-service
      port: 8080
      weight: 80
    - name: canary-service
      port: 8080
      weight: 20
    matches:
    - path:
        type: PathPrefix
        value: /app
```

**Key Points:**
- VirtualServer `splits` become weighted `backendRefs`
- Traffic is automatically distributed based on weights
- Perfect for canary deployments and A/B testing

---

## 7. gRPC Services

### NGINX VirtualServer with gRPC Upstreams
```yaml
apiVersion: k8s.nginx.org/v1
kind: VirtualServer
metadata:
  name: mixed-grpc-vs
  namespace: default
spec:
  host: api.grpc.example.com
  tls:
    secret: grpc-tls
  upstreams:
  - name: grpc-backend
    service: grpc-service
    port: 9090
    type: grpc
  - name: http-backend
    service: http-service
    port: 8080
  routes:
  - path: /api/rest
    action:
      pass: http-backend
  - path: /api.UserService
    action:
      pass: grpc-backend
```

### Generated HTTPRoute and GRPCRoute
The tool automatically generates both resources:

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  # ... metadata for HTTP traffic
spec:
  rules:
  - backendRefs:
    - name: http-service
      port: 8080
    matches:
    - path:
        type: PathPrefix
        value: /api/rest
---
apiVersion: gateway.networking.k8s.io/v1
kind: GRPCRoute
metadata:
  # ... metadata for gRPC traffic
spec:
  rules:
  - backendRefs:
    - name: grpc-service
      port: 9090
    matches:
    - method:
        service: api.UserService
```

**Key Points:**
- `type: grpc` upstreams automatically create GRPCRoute resources
- Mixed HTTP/gRPC services generate both HTTPRoute and GRPCRoute
- gRPC method matching is properly converted

---

## 8. Custom Listeners with GlobalConfiguration

### NGINX VirtualServer with Custom Listeners
```yaml
apiVersion: k8s.nginx.org/v1
kind: VirtualServer
metadata:
  name: custom-listeners-vs
  namespace: default
spec:
  host: custom.example.com
  tls:
    secret: custom-tls
  listener:
    http: custom-http-8080
    https: custom-https-8443
  upstreams:
  - name: app-backend
    service: app-service
    port: 8080
  routes:
  - path: /app
    action:
      pass: app-backend
---
apiVersion: k8s.nginx.org/v1
kind: GlobalConfiguration
metadata:
  name: nginx-configuration
  namespace: nginx-ingress
spec:
  listeners:
  - name: custom-http-8080
    port: 8080
    protocol: HTTP
  - name: custom-https-8443
    port: 8443
    protocol: HTTPS
```

### Conversion Command with GlobalConfiguration
```bash
./ingress2gateway print \
  --input-file custom-listeners.yaml \
  --providers nginx \
  --nginx-global-configuration nginx-configuration
```

### Generated Gateway with Custom Ports
```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  # ... metadata
spec:
  gatewayClassName: nginx
  listeners:
  - hostname: custom.example.com
    name: http-8080-custom-example-com
    port: 8080  # Custom port from GlobalConfiguration
    protocol: HTTP
  - hostname: custom.example.com
    name: https-8443-custom-example-com-custom-tls
    port: 8443  # Custom port from GlobalConfiguration
    protocol: HTTPS
    tls:
      certificateRefs:
      - group: ""
        kind: Secret
        name: custom-tls
      mode: Terminate
```

**Key Points:**
- GlobalConfiguration defines custom listener ports
- Must specify `--nginx-global-configuration` flag during conversion
- Custom ports (8080/8443) replace defaults (80/443)

---

## 9. Advanced Features

### Conversion Command Options

```bash
# Basic conversion
./ingress2gateway print --input-file input.yaml --providers nginx

# With GlobalConfiguration
./ingress2gateway print \
  --input-file input.yaml \
  --providers nginx \
  --nginx-global-configuration nginx-configuration

# Output formats
./ingress2gateway print --input-file input.yaml --providers nginx --output json
./ingress2gateway print --input-file input.yaml --providers nginx --output kyaml

# Specific namespace
./ingress2gateway print \
  --input-file input.yaml \
  --providers nginx \
  --namespace production

# All namespaces
./ingress2gateway print \
  --input-file input.yaml \
  --providers nginx \
  --all-namespaces
```

### Supported NGINX Features

✅ **Fully Supported:**
- Basic routing and path matching
- SSL/TLS termination and redirects
- URL rewriting with `ReplacePrefixMatch`
- Header manipulation (set/remove)
- Traffic splitting and weighted backends
- gRPC service routing
- Custom listeners via GlobalConfiguration
- VirtualServerRoute references

⚠️ **Partially Supported (with warnings):**
- Complex regex patterns (converted to simple matching)
- Advanced authentication policies
- Rate limiting and DOS protection
- Custom snippets and configuration

❌ **Not Supported:**
- NGINX-specific configurations that don't translate to Gateway API
- Complex lua scripting
- Advanced caching configurations

### Troubleshooting

1. **Missing GlobalConfiguration:**
   ```
   Error: Custom listeners require --nginx-global-configuration flag
   ```
   Solution: Include the GlobalConfiguration flag when using custom listeners.

2. **Unsupported Features:**
   The tool will generate warnings for unsupported features but continue conversion.

3. **Multiple Providers:**
   ```bash
   # Convert multiple provider resources
   ./ingress2gateway print \
     --input-file mixed-resources.yaml \
     --providers nginx,istio,kong
   ```

---

## Conclusion

The `ingress2gateway` tool provides comprehensive conversion from NGINX Ingress Controller resources to Gateway API, supporting:

- **Ingress resources** with NGINX annotations
- **VirtualServer/VirtualServerRoute** custom resources  
- **GlobalConfiguration** for advanced setups
- **Mixed HTTP/gRPC** traffic routing
- **Advanced traffic management** features

The generated Gateway API resources are production-ready and maintain the same traffic routing behavior as the original NGINX configurations.

For more examples, see the test fixtures in `pkg/i2gw/providers/nginx/fixtures/`.
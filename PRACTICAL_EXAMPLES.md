# NGINX to Gateway API: Practical Conversion Examples

This document provides real-world examples with complete before/after configurations for migrating from NGINX Ingress Controller to Gateway API.

## Quick Start

```bash
# Build the conversion tool
go build -o ingress2gateway .

# Convert NGINX resources to Gateway API
./ingress2gateway print --input-file your-nginx-config.yaml --providers nginx
```

## Example 1: Microservices API Gateway with SSL and Header Manipulation

### Before: NGINX Ingress with Annotations
```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: api-gateway
  namespace: production
  annotations:
    nginx.org/redirect-to-https: "true"
    nginx.org/proxy-hide-headers: "Server,X-Powered-By,X-Internal-Version"
    nginx.org/proxy-set-headers: "X-Request-ID: ${request_id}, X-Forwarded-Proto: https"
    nginx.org/rewrites: "serviceName=user-api rewrite=/v2/users, serviceName=order-api rewrite=/v2/orders"
spec:
  ingressClassName: nginx
  tls:
  - hosts:
    - api.mycompany.com
    secretName: api-tls-cert
  rules:
  - host: api.mycompany.com
    http:
      paths:
      - path: /users
        pathType: Prefix
        backend:
          service:
            name: user-api
            port:
              number: 8080
      - path: /orders
        pathType: Prefix
        backend:
          service:
            name: order-api
            port:
              number: 8080
      - path: /health
        pathType: Prefix
        backend:
          service:
            name: health-service
            port:
              number: 3000
```

### After: Gateway API Resources
```bash
./ingress2gateway print --input-file api-gateway.yaml --providers nginx
```

**Generated Output:**
```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  annotations:
    gateway.networking.k8s.io/generator: ingress2gateway
  name: nginx
  namespace: production
spec:
  gatewayClassName: nginx
  listeners:
  - hostname: api.mycompany.com
    name: api-mycompany-com-http
    port: 80
    protocol: HTTP
  - hostname: api.mycompany.com
    name: api-mycompany-com-https
    port: 443
    protocol: HTTPS
    tls:
      certificateRefs:
      - group: ""
        kind: Secret
        name: api-tls-cert
      mode: Terminate
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  annotations:
    gateway.networking.k8s.io/generator: ingress2gateway
  name: api-gateway-api-mycompany-com
  namespace: production
spec:
  hostnames:
  - api.mycompany.com
  parentRefs:
  - name: nginx
  rules:
  # SSL Redirect rule
  - filters:
    - requestRedirect:
        scheme: https
        statusCode: 301
      type: RequestRedirect
    matches:
    - path:
        type: PathPrefix
        value: /
  # User API with URL rewrite and header manipulation
  - backendRefs:
    - name: user-api
      port: 8080
    filters:
    - responseHeaderModifier:
        remove:
        - Server
        - X-Powered-By
        - X-Internal-Version
      type: ResponseHeaderModifier
    - requestHeaderModifier:
        set:
        - name: X-Request-ID
          value: ${request_id}
        - name: X-Forwarded-Proto
          value: https
      type: RequestHeaderModifier
    - urlRewrite:
        path:
          type: ReplacePrefixMatch
          replacePrefixMatch: /v2/users
      type: URLRewrite
    matches:
    - path:
        type: PathPrefix
        value: /users
  # Order API with URL rewrite and header manipulation
  - backendRefs:
    - name: order-api
      port: 8080
    filters:
    - responseHeaderModifier:
        remove:
        - Server
        - X-Powered-By
        - X-Internal-Version
      type: ResponseHeaderModifier
    - requestHeaderModifier:
        set:
        - name: X-Request-ID
          value: ${request_id}
        - name: X-Forwarded-Proto
          value: https
      type: RequestHeaderModifier
    - urlRewrite:
        path:
          type: ReplacePrefixMatch
          replacePrefixMatch: /v2/orders
      type: URLRewrite
    matches:
    - path:
        type: PathPrefix
        value: /orders
  # Health endpoint (no rewrite needed)
  - backendRefs:
    - name: health-service
      port: 3000
    filters:
    - responseHeaderModifier:
        remove:
        - Server
        - X-Powered-By
        - X-Internal-Version
      type: ResponseHeaderModifier
    - requestHeaderModifier:
        set:
        - name: X-Request-ID
          value: ${request_id}
        - name: X-Forwarded-Proto
          value: https
      type: RequestHeaderModifier
    matches:
    - path:
        type: PathPrefix
        value: /health
```

**Migration Benefits:**
- ✅ Automatic SSL redirect handling
- ✅ Clean separation of Gateway (infrastructure) and HTTPRoute (application routing)
- ✅ Header manipulation preserved
- ✅ URL rewriting with path preservation
- ✅ Consistent security headers across all routes

---

## Example 2: Canary Deployment with VirtualServer

### Before: NGINX VirtualServer for Blue/Green Deployment
```yaml
apiVersion: k8s.nginx.org/v1
kind: VirtualServer
metadata:
  name: ecommerce-app
  namespace: ecommerce
spec:
  host: shop.example.com
  tls:
    secret: shop-tls-cert
  upstreams:
  - name: frontend-stable
    service: frontend-v1
    port: 3000
  - name: frontend-canary
    service: frontend-v2
    port: 3000
  - name: api-stable
    service: api-v1
    port: 8080
  - name: api-canary
    service: api-v2
    port: 8080
  routes:
  # Frontend with 90/10 split
  - path: /
    splits:
    - weight: 90
      action:
        pass: frontend-stable
    - weight: 10
      action:
        proxy:
          upstream: frontend-canary
          requestHeaders:
            set:
            - name: X-Canary-Version
              value: "v2.0"
            - name: X-Feature-Flags
              value: "new-checkout,improved-search"

  # API with more aggressive canary (80/20)
  - path: /api
    splits:
    - weight: 80
      action:
        pass: api-stable
    - weight: 20
      action:
        proxy:
          upstream: api-canary
          requestHeaders:
            set:
            - name: X-API-Version
              value: "2.0"
          responseHeaders:
            add:
            - name: X-Response-Source
              value: "canary-v2"
              always: true

  # Static assets always go to stable
  - path: /static
    action:
      pass: frontend-stable

  # Health check endpoint
  - path: /health
    action:
      pass: api-stable
```

### After: Gateway API with Weighted Traffic Splitting
```bash
./ingress2gateway print --input-file ecommerce-canary.yaml --providers nginx
```

**Generated Output:**
```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  annotations:
    gateway.networking.k8s.io/generator: ingress2gateway
  labels:
    app.kubernetes.io/managed-by: ingress2gateway
    ingress2gateway.io/source: nginx-virtualserver
  name: nginx
  namespace: ecommerce
spec:
  gatewayClassName: nginx
  listeners:
  - hostname: shop.example.com
    name: http-80-shop-example-com
    port: 80
    protocol: HTTP
  - hostname: shop.example.com
    name: https-443-shop-example-com-shop-tls-cert
    port: 443
    protocol: HTTPS
    tls:
      certificateRefs:
      - group: ""
        kind: Secret
        name: shop-tls-cert
      mode: Terminate
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  annotations:
    gateway.networking.k8s.io/generator: ingress2gateway
  labels:
    app.kubernetes.io/managed-by: ingress2gateway
    ingress2gateway.io/source: nginx-virtualserver
    ingress2gateway.io/vs-name: ecommerce-app
  name: ecommerce-app-httproute
  namespace: ecommerce
spec:
  hostnames:
  - shop.example.com
  parentRefs:
  - name: nginx
    sectionName: http-80-shop-example-com
  - name: nginx
    sectionName: https-443-shop-example-com-shop-tls-cert
  rules:
  # Frontend with 90/10 weighted split and canary headers
  - backendRefs:
    - name: frontend-v1
      port: 3000
      weight: 90
    - filters:
      - requestHeaderModifier:
          set:
          - name: X-Canary-Version
            value: "v2.0"
          - name: X-Feature-Flags
            value: "new-checkout,improved-search"
        type: RequestHeaderModifier
      name: frontend-v2
      port: 3000
      weight: 10
    matches:
    - path:
        type: PathPrefix
        value: /
  # API with 80/20 split and request/response header modification
  - backendRefs:
    - name: api-v1
      port: 8080
      weight: 80
    - filters:
      - requestHeaderModifier:
          set:
          - name: X-API-Version
            value: "2.0"
        type: RequestHeaderModifier
      - responseHeaderModifier:
          set:
          - name: X-Response-Source
            value: "canary-v2"
        type: ResponseHeaderModifier
      name: api-v2
      port: 8080
      weight: 20
    matches:
    - path:
        type: PathPrefix
        value: /api
  # Static assets to stable version only
  - backendRefs:
    - name: frontend-v1
      port: 3000
    matches:
    - path:
        type: PathPrefix
        value: /static
  # Health check to stable API
  - backendRefs:
    - name: api-v1
      port: 8080
    matches:
    - path:
        type: PathPrefix
        value: /health
```

**Migration Benefits:**
- ✅ Built-in weighted traffic splitting without custom configuration
- ✅ Per-backend header modification (canary version gets special headers)
- ✅ Clear separation of traffic policies from infrastructure
- ✅ Native Gateway API support for blue/green deployments
- ✅ Easier to manage and observe with standard Gateway API tooling

---

## Example 3: gRPC Services with Mixed HTTP/gRPC Traffic

### Before: NGINX VirtualServer with gRPC Upstreams
```yaml
apiVersion: k8s.nginx.org/v1
kind: VirtualServer
metadata:
  name: grpc-services
  namespace: microservices
spec:
  host: grpc.api.example.com
  tls:
    secret: grpc-api-tls
  upstreams:
  # gRPC services
  - name: user-grpc
    service: user-service-grpc
    port: 9090
    type: grpc
  - name: order-grpc
    service: order-service-grpc
    port: 9091
    type: grpc
  # HTTP services
  - name: health-http
    service: health-service
    port: 8080
  - name: metrics-http
    service: metrics-service
    port: 9100
  routes:
  # gRPC User Service
  - path: /user.UserService
    action:
      pass: user-grpc
  # gRPC Order Service with authentication
  - path: /order.OrderService
    matches:
    - conditions:
      - header: authorization
        value: "Bearer .*"
      action:
        pass: order-grpc
    # Fallback for unauthenticated requests
    action:
      return:
        code: 401
        text: "Authentication required"
  # HTTP health checks
  - path: /health
    action:
      pass: health-http
  # HTTP metrics endpoint
  - path: /metrics
    action:
      pass: metrics-http
```

### After: Separate HTTPRoute and GRPCRoute Resources
```bash
./ingress2gateway print --input-file grpc-services.yaml --providers nginx
```

**Generated Output:**
```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  annotations:
    gateway.networking.k8s.io/generator: ingress2gateway
  labels:
    app.kubernetes.io/managed-by: ingress2gateway
    ingress2gateway.io/source: nginx-virtualserver
  name: nginx
  namespace: microservices
spec:
  gatewayClassName: nginx
  listeners:
  - hostname: grpc.api.example.com
    name: http-80-grpc-api-example-com
    port: 80
    protocol: HTTP
  - hostname: grpc.api.example.com
    name: https-443-grpc-api-example-com-grpc-api-tls
    port: 443
    protocol: HTTPS
    tls:
      certificateRefs:
      - group: ""
        kind: Secret
        name: grpc-api-tls
      mode: Terminate
---
# HTTP routes for REST endpoints
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  annotations:
    gateway.networking.k8s.io/generator: ingress2gateway
  labels:
    app.kubernetes.io/managed-by: ingress2gateway
    ingress2gateway.io/source: nginx-virtualserver
    ingress2gateway.io/vs-name: grpc-services
  name: grpc-services-httproute
  namespace: microservices
spec:
  hostnames:
  - grpc.api.example.com
  parentRefs:
  - name: nginx
    sectionName: http-80-grpc-api-example-com
  - name: nginx
    sectionName: https-443-grpc-api-example-com-grpc-api-tls
  rules:
  - backendRefs:
    - name: health-service
      port: 8080
    matches:
    - path:
        type: PathPrefix
        value: /health
  - backendRefs:
    - name: metrics-service
      port: 9100
    matches:
    - path:
        type: PathPrefix
        value: /metrics
---
# gRPC routes for service-to-service communication
apiVersion: gateway.networking.k8s.io/v1
kind: GRPCRoute
metadata:
  annotations:
    gateway.networking.k8s.io/generator: ingress2gateway
  labels:
    app.kubernetes.io/managed-by: ingress2gateway
    ingress2gateway.io/source: nginx-virtualserver
    ingress2gateway.io/vs-name: grpc-services
  name: grpc-services-grpcroute
  namespace: microservices
spec:
  hostnames:
  - grpc.api.example.com
  parentRefs:
  - name: nginx
    sectionName: http-80-grpc-api-example-com
  - name: nginx
    sectionName: https-443-grpc-api-example-com-grpc-api-tls
  rules:
  # User gRPC service
  - backendRefs:
    - name: user-service-grpc
      port: 9090
    matches:
    - method:
        service: user.UserService
  # Order gRPC service with auth header requirement
  - backendRefs:
    - name: order-service-grpc
      port: 9091
    matches:
    - headers:
      - name: authorization
        type: RegularExpression
        value: "Bearer .*"
      method:
        service: order.OrderService
```

**Migration Benefits:**
- ✅ Automatic separation of HTTP and gRPC traffic into appropriate route types
- ✅ Native gRPC method matching instead of path-based routing
- ✅ Better observability and debugging for gRPC vs HTTP traffic
- ✅ Header-based matching for authentication requirements
- ✅ Protocol-aware routing policies

---

## Example 4: Custom Port Configuration with GlobalConfiguration

### Before: NGINX with Custom Ports
```yaml
apiVersion: k8s.nginx.org/v1
kind: VirtualServer
metadata:
  name: custom-ports-app
  namespace: infrastructure
spec:
  host: internal.company.com
  tls:
    secret: internal-tls
  listener:
    http: custom-http-8080
    https: custom-https-8443
  upstreams:
  - name: internal-api
    service: internal-api-service
    port: 3000
  - name: admin-panel
    service: admin-service
    port: 4000
  routes:
  - path: /api
    action:
      pass: internal-api
  - path: /admin
    action:
      pass: admin-panel
---
apiVersion: k8s.nginx.org/v1
kind: GlobalConfiguration
metadata:
  name: nginx-config
  namespace: nginx-ingress
spec:
  listeners:
  - name: custom-http-8080
    port: 8080
    protocol: HTTP
  - name: custom-https-8443
    port: 8443
    protocol: HTTPS
  - name: management-9090
    port: 9090
    protocol: HTTP
```

### After: Gateway with Custom Ports
```bash
./ingress2gateway print \
  --input-file custom-ports.yaml \
  --providers nginx \
  --nginx-global-configuration nginx-config
```

**Generated Output:**
```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  annotations:
    gateway.networking.k8s.io/generator: ingress2gateway
  labels:
    app.kubernetes.io/managed-by: ingress2gateway
    ingress2gateway.io/source: nginx-virtualserver
  name: nginx
  namespace: infrastructure
spec:
  gatewayClassName: nginx
  listeners:
  # Custom HTTP port from GlobalConfiguration
  - hostname: internal.company.com
    name: http-8080-internal-company-com
    port: 8080  # Custom port instead of default 80
    protocol: HTTP
  # Custom HTTPS port from GlobalConfiguration  
  - hostname: internal.company.com
    name: https-8443-internal-company-com-internal-tls
    port: 8443  # Custom port instead of default 443
    protocol: HTTPS
    tls:
      certificateRefs:
      - group: ""
        kind: Secret
        name: internal-tls
      mode: Terminate
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  annotations:
    gateway.networking.k8s.io/generator: ingress2gateway
  labels:
    app.kubernetes.io/managed-by: ingress2gateway
    ingress2gateway.io/source: nginx-virtualserver
    ingress2gateway.io/vs-name: custom-ports-app
  name: custom-ports-app-httproute
  namespace: infrastructure
spec:
  hostnames:
  - internal.company.com
  parentRefs:
  - name: nginx
    sectionName: http-8080-internal-company-com
  - name: nginx
    sectionName: https-8443-internal-company-com-internal-tls
  rules:
  - backendRefs:
    - name: internal-api-service
      port: 3000
    matches:
    - path:
        type: PathPrefix
        value: /api
  - backendRefs:
    - name: admin-service
      port: 4000
    matches:
    - path:
        type: PathPrefix
        value: /admin
```

**Migration Benefits:**
- ✅ Preserves custom port configurations from GlobalConfiguration
- ✅ Non-standard ports properly configured in Gateway listeners
- ✅ Clear dependency on GlobalConfiguration resource
- ✅ Infrastructure-level port management separate from application routing

---

## Migration Checklist

### Pre-Migration
- [ ] Inventory all NGINX Ingress resources and annotations
- [ ] Identify custom GlobalConfiguration settings
- [ ] Test conversion with `--providers nginx` flag
- [ ] Review generated resources for accuracy
- [ ] Check for unsupported feature warnings

### During Migration
- [ ] Use `--nginx-global-configuration` flag for custom listeners
- [ ] Validate TLS certificate references
- [ ] Verify weighted traffic splitting configurations
- [ ] Test gRPC service conversions separately
- [ ] Check header manipulation filters

### Post-Migration
- [ ] Apply Gateway and HTTPRoute/GRPCRoute resources
- [ ] Verify traffic routing behavior
- [ ] Monitor for any functionality differences
- [ ] Update monitoring and alerting for new resource types
- [ ] Train team on Gateway API concepts and troubleshooting

---

## Common Migration Patterns

1. **Annotations → Filters:** All `nginx.org/*` annotations become HTTPRoute filters
2. **VirtualServer → Gateway + HTTPRoute:** Infrastructure and routing concerns separated  
3. **Traffic Splits → Weighted BackendRefs:** Native load balancing without custom configs
4. **gRPC Detection → Separate Routes:** Automatic HTTP vs gRPC route generation
5. **GlobalConfiguration → Custom Listeners:** Port customization preserved

The `ingress2gateway` tool handles these patterns automatically, providing a smooth migration path from NGINX Ingress Controller to Gateway API standards.
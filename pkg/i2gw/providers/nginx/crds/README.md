# NGINX Ingress Controller CRDs → Gateway API Conversion

This provider converts NGINX Ingress Controller Custom Resource Definitions (CRDs) to Gateway API resources.

## Supported Resources

### VirtualServer & VirtualServerRoute
Converts Layer 7 HTTP/HTTPS routing to Gateway API resources.

**Input:** `VirtualServer`, `VirtualServerRoute`  
**Output:** `Gateway`, `HTTPRoute`, `GRPCRoute`

#### ✅ Supported Features

| VirtualServer Feature | Gateway API Mapping | Notes |
|----------------------|---------------------|--------|
| **Basic Routing** | HTTPRoute rules | Path-based routing with prefix/exact/regex matching |
| **Upstreams** | BackendRefs | Service references with ports and weights |
| **TLS Termination** | Gateway TLS config | Certificate refs and TLS mode |
| **TLS Redirect** | HTTPRoute redirect filter | HTTP→HTTPS redirects |
| **Hostname** | Gateway listener hostname | SNI-based routing |
| **Custom Listeners** | Gateway listeners | GlobalConfiguration integration |
| **Path Rewriting** | URLRewrite filter | Path modification |
| **Traffic Splitting** | Weighted BackendRefs | Load balancing between services |
| **Header Modification** | Header filters | Request/response header manipulation |
| **Redirects** | RequestRedirect filter | URL redirects with status codes |
| **Match Conditions** | HTTPRoute matches | Header/query parameter matching |
| **gRPC Services** | GRPCRoute | Service/method matching for gRPC traffic |

#### Route Processing

**Match Flattening**: VirtualServer routes with multiple match conditions are flattened into separate Gateway API rules.
```yaml
# VirtualServer: Single route with multiple matches
routes:
- path: /api
  matches:
  - headers: [auth]
  - query: [version=v2]

# Gateway API: Becomes 2 separate HTTPRoute rules
rules:
- matches: [path: /api, headers: [auth]]
- matches: [path: /api, query: [version=v2]]
```

**HTTP vs gRPC Detection**: A VirtualServer route becomes **gRPC** if it targets an upstream with `type: grpc` or has `grpc` field defined. Otherwise it's **HTTP**. Single VirtualServer can generate both `HTTPRoute` and `GRPCRoute` resources.

#### ⚠️ Limitations
- Return actions → Warning (no direct Gateway API equivalent)
- Rate limiting → Not supported
- Advanced proxy features → Limited support
- Client settings → Not supported


### GlobalConfiguration
Defines custom listeners and global settings.

**Features:**
- Custom HTTP/HTTPS listeners
- Port and protocol mapping
- Integration with VirtualServer

## Gateway Integration

### Shared Gateway Pattern
- **One Gateway per namespace** consolidates all listeners
- **Mixed protocols** supported (HTTP, HTTPS)
- **Automatic merging** of VirtualServer listeners

### Listener Naming Convention
```
http-{port}-{hostname}           # HTTP listeners
https-{port}-{hostname}-{secret} # HTTPS listeners  
```

## Usage Examples

### Basic VirtualServer Conversion
```bash
# Convert VirtualServer to HTTPRoute
ingress2gateway print --providers=nginx --input-file=virtualserver.yaml
```

### With GlobalConfiguration
```bash
# Include GlobalConfiguration for custom listeners
ingress2gateway print --providers=nginx \
  --nginx-global-configuration=nginx-configuration \
  --input-file=resources.yaml
```

## Resource Mapping Summary

| NGINX Resource | Gateway API Output | Use Case |
|----------------|-------------------|----------|
| `VirtualServer` | `Gateway` + `HTTPRoute` | Web applications, APIs |
| `VirtualServer` (gRPC) | `Gateway` + `GRPCRoute` | gRPC services |
| `GlobalConfiguration` | `Gateway` listeners | Custom ports/protocols |
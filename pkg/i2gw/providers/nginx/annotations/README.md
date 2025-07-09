# NGINX Ingress Annotations

This directory contains the implementation of [NGINX Inc's Ingress Controller](https://github.com/nginxinc/kubernetes-ingress) annotations for the ingress2gateway conversion tool.

**Note**: This is specifically for NGINX Inc's commercial Ingress Controller, not the community [ingress-nginx](https://github.com/kubernetes/ingress-nginx) controller.

## Structure

- **`constants.go`** - All annotation constants and schema definitions
- **`backend_protocol.go`** - Backend protocol annotations (`ssl-services`, `grpc-services`, `websocket-services`)
- **`header_manipulation.go`** - Header manipulation annotations (`hide-headers`, `proxy-set-headers`, etc.)
- **`listen_ports.go`** - Custom port listeners (`listen-ports`, `listen-ports-ssl`)
- **`path_matching.go`** - Path regex matching (`path-regex`)
- **`path_rewrite.go`** - URL rewriting (`rewrites`)
- **`ssl_redirect.go`** - SSL/HTTPS redirects (`redirect-to-https`)

## Exported Functions

Each annotation file exports a main feature function:

- `BackendProtocolFeature` - Processes backend protocol annotations
- `HeaderManipulationFeature` - Processes header manipulation annotations  
- `ListenPortsFeature` - Processes custom port listener annotations
- `PathRegexFeature` - Processes path regex annotations
- `RewriteTargetFeature` - Processes URL rewrite annotations
- `SSLRedirectFeature` - Processes SSL redirect annotations

## Testing

Each annotation implementation includes comprehensive unit tests:

- `*_test.go` files contain feature-specific tests
- `*_helpers_test.go` files contain shared test utilities
- Tests cover various annotation formats, edge cases, and error conditions

## Adding New Annotations

To add a new NGINX annotation:

1. Add the annotation constant to `constants.go`
2. Create the feature implementation file (e.g., `my_feature.go`)
3. Export the main feature function (e.g., `MyFeature`)
4. Add comprehensive tests in `my_feature_test.go`
5. Register the feature function in `../converter.go`

## Integration

These annotation handlers are integrated into the main NGINX provider via `../converter.go`, which registers all feature parsers with the conversion pipeline.
# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability in Silk, please report it responsibly:

1. **Do not** open a public issue
2. Email the maintainers directly with details
3. Include steps to reproduce if possible

We will acknowledge receipt within 48 hours and provide a fix timeline.

## Scope

Silk is a desktop UI framework. Security considerations include:
- **Code generation**: Generated Go files should not contain injection vulnerabilities
- **File I/O**: Design file loading should validate input
- **CGO**: Cairo/GLFW bindings should handle malformed data safely

## Supported Versions

| Version | Supported |
|---------|-----------|
| 2.x     | ✅        |
| < 2.0   | ❌        |

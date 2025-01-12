# Security Policy

## Best Practices

- Keep API keys secure and never commit them
- Use environment variables for sensitive credentials
- Enable request timeouts
- Implement rate limiting
- Use HTTPS for API endpoints

## Known Security Considerations

- API keys and embeddings handling
- Secure database connections for vector storage
- TLS/HTTPS for API requests
- LLM-specific security implications

## Reporting a Vulnerability

### For Critical Vulnerabilities

Email us at <hello@shaharialab.com> with:

- Vulnerability description
- Steps to reproduce
- Impact assessment
- Suggested fix (if available)

You'll receive a response within 48 hours.

### For Non-Critical Issues

Create a GitHub issue with label `security`:

- Clearly mark non-sensitive vulnerabilities
- Follow issue template guidelines
- Avoid including sensitive data or credentials

## Security Updates

- Security patches in minor version updates
- Critical fixes released ASAP
- Updates documented in CHANGELOG.md
- Security advisories for confirmed vulnerabilities

## Dependency Security

- Regular dependency updates
- Dependabot alerts enabled
- Dependency integrity via `go mod verify`
- CI/CD vulnerability scanning

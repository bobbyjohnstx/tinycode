# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability, please report it responsibly:

1. **Do NOT** open a public GitHub issue
2. Use GitHub's [private vulnerability reporting](https://github.com/bobbyjohnstx/tinycode/security/advisories/new)
3. Include:
   - Description of the vulnerability
   - Steps to reproduce
   - Impact assessment
   - Suggested fix (if any)

## Response Timeline

- **Acknowledgment**: Within 48 hours
- **Assessment**: Within 1 week
- **Fix**: Depending on severity, typically within 2 weeks for critical issues

## Supported Versions

| Version | Supported |
|---------|-----------|
| 2.0.x   | Yes       |
| 1.x     | No        |
| 0.x     | No        |

## Security Best Practices

- Set `TINYCODE_AUTH_TOKEN` when exposing the server on a network (or rely on the auto-generated token logged at startup)
- Never commit API keys or secrets to the repository
- Review MCP server configurations — they execute with your user permissions

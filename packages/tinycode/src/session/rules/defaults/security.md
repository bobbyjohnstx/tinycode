# Security Rules

## Mandatory Security Checks

> **Scope:** Applies to commits touching application or service code. Docs-only and config-only repos can skip items that are N/A.

Before each commit:
- [ ] No hardcoded secrets (API keys, passwords, tokens)
- [ ] All user inputs validated
- [ ] SQL injection prevention (parameterized queries)
- [ ] XSS prevention (sanitized HTML)
- [ ] CSRF protection enabled
- [ ] Authentication/authorization verified
- [ ] Rate limiting on public/auth-sensitive endpoints (or N/A for internal-only services)
- [ ] Error messages don't leak sensitive data

## Secret Management

```typescript
// NEVER: Hardcoded secrets
const apiKey = "sk-proj-xxxxx"

// ALWAYS: Environment variables
const apiKey = process.env.API_KEY
if (!apiKey) throw new Error('API_KEY not configured')
```

## Security Response Protocol

If security issue found:
1. STOP immediately
2. Run a dedicated security review pass (e.g. with a `security-reviewer` agent if available)
3. Fix CRITICAL issues before continuing
4. Rotate any exposed secrets
5. Review entire codebase for similar issues

## [CUSTOMIZE] Project-Specific Security

Add project-specific security requirements here:
- Authentication method
- Authorization rules
- Data encryption requirements
- Compliance requirements (GDPR, HIPAA, etc.)

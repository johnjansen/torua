# Security Policy

## Supported Versions

We release patches for security vulnerabilities in the following versions:

| Version | Supported          |
| ------- | ------------------ |
| 0.x.x   | :white_check_mark: |

## Reporting a Vulnerability

We take the security of Torua seriously. If you believe you have found a security vulnerability, please report it to us as described below.

### Please do NOT:
- Open a public GitHub issue for security vulnerabilities
- Post about the vulnerability in public forums before it's fixed
- Exploit the vulnerability for any purpose other than testing

### Please DO:
- Email us directly at security@torua.dev with details
- Include the following information:
  - Type of vulnerability (e.g., remote code execution, SQL injection, cross-site scripting, etc.)
  - Full paths of source file(s) related to the vulnerability
  - Location of the affected source code (tag/branch/commit or direct URL)
  - Step-by-step instructions to reproduce the issue
  - Proof-of-concept or exploit code (if possible)
  - Impact of the issue, including how an attacker might exploit it

### What to Expect

1. **Acknowledgment**: We will acknowledge receipt of your vulnerability report within 48 hours
2. **Initial Assessment**: Within 7 days, we will provide an initial assessment and expected timeline
3. **Updates**: We will keep you informed about the progress of fixing the vulnerability
4. **Fix & Disclosure**: Once fixed, we will:
   - Release a security patch
   - Publish a security advisory
   - Credit you for the discovery (unless you prefer to remain anonymous)

## Security Best Practices for Deployment

### Network Security
- Always use TLS/HTTPS in production environments
- Implement proper firewall rules to restrict access to coordinator and node ports
- Use private networks for inter-node communication when possible

### Authentication & Authorization
- Implement authentication before exposing Torua to public networks
- Use API keys or tokens for client access
- Implement rate limiting to prevent abuse

### Data Security
- Encrypt sensitive data at rest
- Use encrypted connections for all network communication
- Regularly backup your data
- Implement proper access controls

### Operational Security
- Keep Torua and all dependencies up to date
- Monitor logs for suspicious activity
- Implement alerting for security events
- Regular security audits of your deployment

## Known Security Considerations

### Current Limitations
1. **No built-in authentication**: Torua currently does not include authentication mechanisms
2. **No encryption at rest**: Data stored in nodes is not encrypted by default
3. **No TLS by default**: Communication between components is not encrypted by default

### Recommended Mitigations
- Deploy behind a reverse proxy with authentication (e.g., nginx with basic auth)
- Use network-level encryption (VPN, private networks)
- Implement application-level authentication in front of Torua
- Use filesystem encryption for data directories

## Security Features Roadmap

- [ ] Built-in TLS support for all communications
- [ ] Authentication and authorization framework
- [ ] Encrypted storage backend
- [ ] Audit logging
- [ ] Role-based access control (RBAC)
- [ ] API key management
- [ ] Rate limiting and DDoS protection

## Compliance

While Torua is designed with security in mind, it has not yet been audited for compliance with specific standards (HIPAA, PCI-DSS, etc.). Users requiring compliance should:

1. Conduct their own security assessment
2. Implement additional security controls as needed
3. Consult with security professionals
4. Consider enterprise support options (when available)

## Contact

- Security issues: security@torua.dev
- General questions: [GitHub Discussions](https://github.com/johnjansen/torua/discussions)
- Bug reports: [GitHub Issues](https://github.com/johnjansen/torua/issues)

Thank you for helping keep Torua and its users safe!
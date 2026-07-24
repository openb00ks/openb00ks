# Security Policy

Open B00KS is bookkeeping software that handles financial data, so we take
security reports seriously and appreciate responsible disclosure.

## Supported versions

Open B00KS is pre-1.0 and evolving. Security fixes are applied to the `main`
branch and the most recent tagged release. Older releases are not patched;
please upgrade to the latest release before reporting.

## Reporting a vulnerability

**Please do not open a public issue for security problems.**

Report privately through GitHub's built-in flow:

1. Go to the repository's **Security** tab → **Report a vulnerability**
   (GitHub Private Vulnerability Reporting).
2. Describe the issue with enough detail to reproduce it: affected version or
   commit, component (API, worker, web, chart), impact, and a proof of concept
   if you have one.

If you cannot use GitHub's reporting flow, email **security@spectrumlabs.tech**
with the same information.

## What to expect

- **Acknowledgement** within 3 business days.
- **An initial assessment** (severity and whether we can reproduce it) within
  10 business days.
- **Coordinated disclosure:** we will keep you updated on remediation and agree
  on a disclosure timeline with you. Please give us a reasonable window to ship
  a fix before any public disclosure.
- **Credit:** with your permission, we are happy to credit you in the release
  notes for the fix.

## Scope

In scope: the Open B00KS application code in this repository (API, worker,
ops-scheduler, web frontend) and the Helm chart.

Out of scope: vulnerabilities in third-party dependencies (report those
upstream), and issues that require a pre-compromised host or physical access.
Self-hosted deployments are the operator's responsibility to secure; this
policy covers defects in the software itself.

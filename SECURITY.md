# Security Policy for Umono

Umono is developed by a single developer, Yunus Emre Bulut. I take its security seriously: the safety of its users' data and infrastructure is a top priority.

## Supported Versions

Umono is currently in active development. To maintain a fast development pace, **only the latest minor release receives security updates.** Once a new minor version is released, the previous minor versions will no longer receive security patches. Users are strongly encouraged to always upgrade to the latest version.

| Version | Supported |
| :--- | :--- |
| **Latest Minor Release (Current)** | ✅ Supported |
| **Older Versions** | ❌ Not Supported (Please upgrade) |

## Dependency Scanning

Known vulnerabilities in Umono's dependencies and in the Go standard library are scanned with [`govulncheck`](https://go.dev/doc/security/vuln/) four times a day, on both `main` and the latest release. The same scan runs on every pull request and every push to `main`, and fails if a vulnerability affects the code.

A vulnerability affects Umono when its code calls the vulnerable function. Such a vulnerability is fixed within **24 hours** after a fixed version of the dependency is released. If no fixed version exists yet, the 24 hours start when it is released.

## Reporting a Vulnerability

If you discover a security vulnerability in Umono, I appreciate your help in disclosing it to me in a responsible manner.

**🚨 Please DO NOT report security vulnerabilities through public GitHub issues or discussions.**

Instead, please report them privately via email to: **yunusemrebulut123@gmail.com**

Please include the following information in your report to help me understand and resolve the issue quickly:
* **Description:** A brief description of the vulnerability.
* **Steps to Reproduce:** Detailed steps to reproduce the issue.
* **Impact:** The potential impact of the vulnerability (e.g., what an attacker could achieve).
* **Suggested Mitigation:** Any potential solutions or ideas on how to fix it (optional but highly appreciated).

### Response Timeline
* I will acknowledge receipt of your vulnerability report within **48 hours**.
* I will investigate the issue and determine if it is a valid vulnerability.
* I strive to keep you informed about my progress and will notify you when a patch is released.

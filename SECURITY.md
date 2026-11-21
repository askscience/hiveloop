# Security policy

## Reporting a vulnerability

Please **do not open a public issue** for security problems. Use GitHub's private
[security advisories](../../security/advisories/new) for this repository instead, with:

- what the problem is and what an attacker could do with it;
- steps to reproduce (a request, a page, a configuration);
- the version or commit you tested.

You will get an answer as soon as possible; fixes are released before details are made public.

## Scope and design notes

- Hiveloop expects to run behind HTTPS (reverse proxy on the same host). It trusts
  `X-Forwarded-For` only from the local machine.
- State-changing requests require the `X-Hiveloop` header (CSRF protection) and a session cookie
  that is `HttpOnly`, `SameSite=Lax` and `Secure` by default.
- Logins, sign-ups, chat messages and assistant requests are rate limited.
- The assistant acts with the permissions of the person using it; web pages it reads are treated
  as untrusted data and it cannot reach private network addresses.
- Uploaded images are decoded on the server and resized or re-encoded when needed; anything that is not a real JPEG, PNG or WebP image is rejected.

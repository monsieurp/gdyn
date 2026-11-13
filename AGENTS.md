# Global rules

## Working rules

- Do not write code or files unless the user asks.
- Be honest, not agreeable. Say when you do not know.
- Go one step at a time and wait for the user.
- Replies: commands or code first, prose one or two lines.
- No Oxford comma. No comma before "and" that joins clauses.
- Never commit unless asked. Never push. No attribution lines.

## Commits

A repo's own commit rules (for example `docs/commits.md`) win
over these.

- One logical change per commit. Layout-only changes go in
  their own commit.
- Subject: `prefix: imperative summary`, 72 chars max.
  Prefix = Go/Python package or dir, or `all:`, `build:`,
  `docs:`.
- Lowercase after the colon. No trailing period.
- Blank line, then body wrapped at 72. Body says why in one
  to three sentences. Skip it when the subject says it all.
- Lowercase everywhere, except code identifiers and acronyms.
- Issue reference last, on its own line: `fixes #12`.
- Write in simple-english + caveman style.

Example:

```
livedns: send token only in bearer header

the client sent the PAT in both x-api-key and `Authorization`. on
403 it retried with `Apikey`, so a bad token cost two calls. send
bearer only and fail on the first 403.

fixes #36
```

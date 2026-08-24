# Skill: access-request

Non-authoritative guidance when recall hits a deny or empty authorized set.

## When

After at most three retrieval rounds (search/get/graph/brief) without sufficient cited evidence, or when the packet decision is deny / empty with a clear need.

## How

1. Call `context.request_access` with `resource_id`, `purpose`, and a short `justification`.
2. Do not retry the same unauthorized fetch in a loop.
3. Inform the operator/user that access is pending — skills cannot approve requests.

## Never

- Use this skill to escalate privileges or spoof another principal.
- Treat a filed access request as temporary grant of visibility.

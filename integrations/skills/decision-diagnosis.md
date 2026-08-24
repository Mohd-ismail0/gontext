# Skill: decision-diagnosis

Non-authoritative guidance for explaining allow/deny outcomes.

## When

A retrieval returned empty, truncated, or denied and the operator needs a reason code.

## How

1. Capture `audit_id` / `decision` / `reason_code` from the ContextPacket or MCP error payload.
2. Use REST diagnose endpoints (`context/diagnose/decision/{auditId}` or `cf diagnose decision`) — not skill inventiveness.
3. Map common codes to next actions: wrong org → fix tenant; purpose mismatch → restate purpose; missing relation → `context.request_access` or AuthZ tuple ops via operators.

## Never

- Interpret diagnosis as permission to disclose denied fields.
- Bypass OpenFGA by trusting client-side role claims from this skill.

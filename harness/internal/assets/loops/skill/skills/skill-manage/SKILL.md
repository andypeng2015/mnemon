---
name: skill-manage
description: Submit approved skill lifecycle and content changes to Local Mnemon.
---

# skill-manage

Use this skill only after a proposal has been approved by the user or by an
explicit host policy.

## Boundary

This skill submits approved skill declarations to Local Mnemon. It does not edit
host skill directories or canonical files directly. New active skills become
host-visible after Local Mnemon accepts the declaration and the host projection
refreshes.

## Allowed MVP Operations

- submit an approved active skill declaration
- submit approved `SKILL.md` content drafted by `skill-author`
- submit a replacement declaration for an existing skill
- submit lifecycle status changes: `active`, `stale`, or `archived`
- submit metadata or usage notes needed by the lifecycle

## Procedure

1. Read the approved proposal and confirm the intended operation.
2. Check `MNEMON_SKILL_LOOP_PROTECTED_SKILLS`; do not modify protected skills
   unless the approval explicitly covers the exception.
3. Keep skill ids hyphen-case: lowercase letters, numbers, and `-`. Preserve a
   non-conforming id only when an external host compatibility boundary requires
   it.
4. Submit the smallest approved declaration through Local Mnemon:

<!-- mnemon:payload-contract -->

5. Do not edit the host skill surface directly. Let Local Mnemon and Prime
   regenerate mirrors.
6. Record the submitted declaration in the proposal or usage log when useful.

## Safety

If the proposal is ambiguous, risky, or conflicts with current repository state,
stop and ask for approval instead of guessing.

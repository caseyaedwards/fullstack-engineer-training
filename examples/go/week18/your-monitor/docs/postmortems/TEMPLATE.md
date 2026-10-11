<!-- region: template -->
# YYYY-MM-DD: One-Line Title That Names the Symptom

**Status:** draft, or final once you have reviewed it a day later.
**Severity:** page or ticket, and which SLO.
**Budget spent:** minutes of late checks, or failed requests, and the share
of the 30-day budget.

## Summary

Three sentences: what users saw, for how long, and what ended it.

## Impact

- What failed, from the outside: the status page, the checks, the alerts.
- From when to when, in UTC.
- The numbers: failed requests, late checks, budget spent.

## Timeline

All times UTC. One line per event: what happened, or what you saw, did and
decided.

| Time | Event |
| --- | --- |
| 00:00 | The trigger. |
| 00:00 | The page rings. |
| 00:00 | Mitigation starts. |
| 00:00 | The alert resolves. |

## Detection

How you found out, and how long after the start. Did the right alert fire?
Did anything fire too late, too often, or not at all?

## Causes

What made it happen, as a chain of conditions, each one needed. Write about
systems and decisions, never about a person: "deploy.yml applies a change
without a canary", not "I forgot to test".

## What Went Well

## What Went Badly

## Where You Got Lucky

## Actions

Each action fixes a cause above, has an issue link, and a date.

| Action | Kind | Issue | Due |
| --- | --- | --- | --- |
| | prevent, detect or mitigate | | |
<!-- endregion: template -->

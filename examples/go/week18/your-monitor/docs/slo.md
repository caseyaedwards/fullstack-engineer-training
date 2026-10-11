# Service Level Objectives

<!-- region: slos -->
Both SLOs cover a rolling window of 30 days. `observability/rules/slo.rules.yml`
computes them; `docs/runbook.md` says what to do when one burns.

## Status Page Availability

- **SLI:** the share of `GET` requests to `/api/` routes that the api answers
  with a status under 500.
- **Valid events:** every `GET /api/...` the api answers, including the two
  probes that blackbox exporter sends every 15 seconds: one through the
  public URL, one through the web Service. A request that matches no route
  carries `route="unmatched"` and is not a valid event. A `429` from the
  rate limit never reaches the request counter.
- **Target:** 99.5%.
- **Error budget:** 0.5% of requests. The probes alone send 8 a minute,
  345,600 in 30 days: the budget is 1,728 failed probes, or 3 hours and
  36 minutes of total outage.
- **Not covered:** requests that never reach the api cost no budget: a dead
  VPS, an nginx `502` with no api pod left, an expired certificate.
  `StatusPageDown` pages for those. The alternative SLI
  `1 - avg_over_time(probe_success[30d])` counts them, and sees nothing
  but the probes.

## Check Freshness

- **SLI:** the share of 30-second steps in which checks run on schedule: the
  oldest waiting job has waited 50 seconds or less, and at least one check
  finished in the last 2 minutes.
- **Why 50 seconds:** a due monitor waits up to 5 seconds for the next
  scheduler round, and a check takes up to 5 seconds, the worker
  `-timeout`. With 50 seconds in the queue, every monitor is checked
  within its interval plus 60 seconds.
- **Needs the self-check:** a monitor of `https://<your-domain>/healthz`
  with an interval of 30 seconds. Without a monitor checked every 60 seconds
  or more often, a 2-minute window can hold no finished check, the step
  reads late, and the fast burn pages with nothing wrong.
- **Target:** 99%.
- **Error budget:** 1% of 30 days: 7 hours and 12 minutes of late checks.

## Budget Policy

- Budget left: ship features as usual.
- Budget spent: stop feature work. Spend the next week on the cause of the
  biggest burn in the postmortems, until the 30-day budget recovers.
<!-- endregion: slos -->

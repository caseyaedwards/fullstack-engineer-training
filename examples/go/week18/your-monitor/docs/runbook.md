# Runbook

One section per alert, named after the alert. The `runbook_url` of each
rule in `observability/rules/slo.rules.yml` links to its section.

Open Prometheus and Alertmanager from your laptop first:

```sh
kubectl -n monitoring port-forward svc/prometheus 9090 &
kubectl -n monitoring port-forward svc/alertmanager 9093 &
```

<!-- region: fast-burn -->
## ErrorBudgetFastBurn

**Means:** the SLO named in the `slo` label spends its budget at 14.4 times
the sustainable rate or more, over both the last hour and the last
5 minutes. At that rate, the whole 30-day budget lasts 50 hours or less.

**First five minutes:**

1. Find what changed: `kubectl -n your-monitor rollout history deployment/api`
   and the latest run of `deploy.yml` on GitHub.
2. Look at the pods: `kubectl -n your-monitor get pods`. Look for
   `CrashLoopBackOff`, `Pending` and restarts.
3. For `slo="availability"`, read the api errors:
   `kubectl -n your-monitor logs deployment/api --since=15m | grep '"status":5'`.
4. For `slo="freshness"`, compare `monitor_jobs_oldest_seconds` with
   `sum(rate(monitor_checks_total[5m]))` in Prometheus: an age that climbs
   means stuck workers; no checks at all means no worker or no database.
5. Check Postgres: `kubectl -n your-monitor get statefulset postgres` must
   show `1/1`.

**Mitigate:**

- A deploy in the last hour: `kubectl -n your-monitor rollout undo deployment/<name>`.
- Postgres at `0/1`: `kubectl -n your-monitor scale statefulset postgres --replicas=1`.
- Workers missing: `kubectl -n your-monitor rollout restart deployment/worker`.
- The disk of the VPS is full: `df -h /` over SSH, then delete old images
  with `sudo k3s crictl rmi --prune`.
<!-- endregion: fast-burn -->

## ErrorBudgetSlowBurn

**Means:** the SLO spends its budget at 6 times the sustainable rate or
more, over the last 6 hours and the last 30 minutes. The budget lasts
5 days at that rate. Something fails part of the time: one bad pod of
two, a slow query that times out, a site check that hangs the workers.

**Do:** the steps of ErrorBudgetFastBurn, then look for the share: compare
`sum by (pod) (rate(http_requests_total{code=~"5.."}[30m]))` across the api
pods, and the `err` field in the logs of each.

## ErrorBudgetTicket

**Means:** the SLO spent more than it earned over the last 3 days and the
last 6 hours. No page: the budget lasts 30 days or less at this rate.

**Do:** within one working day, open an issue with the query that shows the
errors, and plan the fix for this week.

## StatusPageDown

**Means:** blackbox exporter has failed to load the status page api through
the web pod for 2 minutes, or Prometheus has no probe result.

**First five minutes:**

1. `kubectl -n your-monitor get pods -l app.kubernetes.io/name=web` and the
   same for `api`.
2. `kubectl -n monitoring get pods`: when blackbox exporter is down, the
   status page is fine and the probe is the problem.
3. Read the failed probe: `probe_http_status_code` in Prometheus, or
   `kubectl -n monitoring logs deployment/blackbox`.

**Mitigate:** as for ErrorBudgetFastBurn.

## Watchdog

**Means:** nothing. It always fires, and Alertmanager pings the heartbeat
check with it every 5 minutes. When the heartbeat service emails you that
pings stopped, Prometheus, Alertmanager or the whole VPS is down:
`kubectl -n monitoring get pods`, then SSH to the VPS.

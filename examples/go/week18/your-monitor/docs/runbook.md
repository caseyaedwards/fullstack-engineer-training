# Runbook

One section per alert, named after the alert. The `runbook_url` of each
rule in `observability/rules/slo.rules.yml` links to its section.

Open the SSH tunnel of Week 16, then open Prometheus and Alertmanager from
your laptop. Stop Compose first with `make down` if it runs: it holds the
same two ports.

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
3. For `slo="availability"`, read the api errors of every api pod:
   `kubectl -n your-monitor logs -l app.kubernetes.io/name=api --prefix --since=15m --tail=-1 | grep '"status":5'`.
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
one of its two paths for 2 minutes, or Prometheus has no probe result.

**First five minutes:**

1. `kubectl -n your-monitor get pods -l app.kubernetes.io/name=web` and the
   same for `api`.
2. Compare the two `instance` labels of the alert. Only the public URL
   failing points at DNS, the certificate, Traefik, or the path from the
   cluster to its own public address:
   `curl -sSv https://status.example.com/api/monitors`. A curl from your
   laptop takes a different path from the probe: a pass there clears DNS
   and the certificate, not that path. Both failing points at the web or
   api pods.
3. `kubectl -n monitoring get pods`: when blackbox exporter is down, the
   status page is fine and the probe is the problem.
4. Read the failed probe: `probe_http_status_code` in Prometheus, or
   `kubectl -n monitoring logs deployment/blackbox`.

**Mitigate:** as for ErrorBudgetFastBurn.

## CertificateExpiresSoon

**Means:** the certificate that the public probe sees expires within
14 days. cert-manager renews 30 days before expiry, so every renewal for
more than two weeks has failed.

**Do:** within one working day, read why:
`kubectl -n your-monitor describe certificate` and
`kubectl -n cert-manager logs deployment/cert-manager --since=24h`. A
failed HTTP-01 challenge usually means port 80 does not reach Traefik:
check the firewall of Week 16.

## Backups

**Means:** healthchecks.io messages you about one of two checks, not
Prometheus.

- **pg-backup:** the backup CronJob pinged `/fail`, or sent no ping by
  05:17 UTC. List the runs with `kubectl -n your-monitor get jobs`, then
  read the newest: `kubectl -n your-monitor logs job/<name>`. A
  `backup: FAILED:` line names the step. Run a backup now with
  `kubectl -n your-monitor create job --from=cronjob/pg-backup pg-backup-manual`.
- **restore-drill:** no drill passed in the last 8 days. Run the restore
  drill of Week 17 on your laptop. It pings the check when it passes.

## Watchdog

**Means:** nothing. It always fires, and Alertmanager pings the heartbeat
check with it every 5 minutes. When healthchecks.io tells you that the
pings of the `vps` check stopped, Prometheus, Alertmanager or the whole
VPS is down:
`kubectl -n monitoring get pods`, then SSH to the VPS.

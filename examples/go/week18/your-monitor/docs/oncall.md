# On Call

<!-- region: oncall -->
## Rotation

- **Who:** you, every week. When a second person joins, alternate weeks and
  hand over on Monday at 09:00.
- **Hours:** Monday to Friday 07:00 to 22:00, weekends 09:00 to 22:00, in
  the time zone of `time_intervals` in `observability/alertmanager.yml`.
  Outside these hours, pages wait in Alertmanager and ring when the hours
  start, if they still fire.
- **Response:** acknowledge a page within 15 minutes of it ringing. Away
  from a laptop for more than an hour of on-call time: add a silence for
  that time and accept the budget it costs.
- **Channel:** pages ring in the Telegram chat of your bot. Tickets arrive in
  the same chat without a sound.

## When a Page Rings

1. Open the runbook link in the message.
2. Start an incident note in `docs/postmortems/` from `TEMPLATE.md`, with
   the time the page rang.
3. Mitigate first: restore the service, then look for the cause.
4. Write each action and its time in the timeline as you go.
5. When the alert resolves, finish the postmortem within two days.

## Monday Review

- Read the budget left: `slo:error_budget:remaining` in Prometheus.
- Read every ticket of the week, and open an issue for each one you keep.
- Check the heartbeat service: it lists the last ping from Alertmanager.
<!-- endregion: oncall -->

export type Phase = 'foundations' | 'build' | 'harden' | 'run';

export type Week = { n: number; slug: string; title: string; phase: Phase };

export const phases: readonly { id: Phase; label: string }[] = [
  { id: 'foundations', label: 'Foundations (0–5)' },
  { id: 'build', label: 'Build the app (6–10)' },
  { id: 'harden', label: 'Harden it (11–14)' },
  { id: 'run', label: 'Run it for real (15–18)' },
];

export const weeks: readonly Week[] = [
  { n: 0, slug: '00-setup', title: 'Setup', phase: 'foundations' },
  { n: 1, slug: '01-go-basics', title: 'Go basics', phase: 'foundations' },
  { n: 2, slug: '02-go-packages-testing', title: 'Go: packages, interfaces, testing', phase: 'foundations' },
  { n: 3, slug: '03-linux-processes', title: 'Linux and processes', phase: 'foundations' },
  { n: 4, slug: '04-networking', title: 'Networking', phase: 'foundations' },
  { n: 5, slug: '05-concurrency', title: 'Concurrency', phase: 'foundations' },
  { n: 6, slug: '06-architecture-repo', title: 'Frontend vs backend, repo skeleton', phase: 'build' },
  { n: 7, slug: '07-backend-api', title: 'Backend: HTTP API', phase: 'build' },
  { n: 8, slug: '08-backend-postgres', title: 'Backend: Postgres', phase: 'build' },
  { n: 9, slug: '09-jobs-alerts', title: 'Background jobs and alerts', phase: 'build' },
  { n: 10, slug: '10-frontend', title: 'Frontend', phase: 'build' },
  { n: 11, slug: '11-security-auth', title: 'Security and auth', phase: 'harden' },
  { n: 12, slug: '12-debugging', title: 'Debugging', phase: 'harden' },
  { n: 13, slug: '13-observability', title: 'Observability', phase: 'harden' },
  { n: 14, slug: '14-ci-cd', title: 'CI/CD', phase: 'harden' },
  { n: 15, slug: '15-kubernetes-local', title: 'Kubernetes on the laptop', phase: 'run' },
  { n: 16, slug: '16-going-live', title: 'Going live', phase: 'run' },
  { n: 17, slug: '17-day-2-traffic', title: 'Day 2 and traffic', phase: 'run' },
  { n: 18, slug: '18-sre', title: 'SRE', phase: 'run' },
];

/** AI-use ladder (see the AI-assisted development track). Keyed by the `aiStage` frontmatter value. */
export const aiStages = {
  1: 'Explain only',
  2: 'Review everything',
  3: 'Pair partner',
} as const;

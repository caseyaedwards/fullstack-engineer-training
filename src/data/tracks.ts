export type TrackGroup = 'foundations' | 'building' | 'shipping' | 'running' | 'working';

export const trackGroups: readonly { id: TrackGroup; label: string }[] = [
  { id: 'foundations', label: 'Foundations' },
  { id: 'building', label: 'Building' },
  { id: 'shipping', label: 'Shipping' },
  { id: 'running', label: 'Running' },
  { id: 'working', label: 'Working' },
];

export const tracks = [
  { slug: 'programming', title: 'Core programming concepts', group: 'foundations' },
  { slug: 'operating-systems', title: 'Operating systems', group: 'foundations' },
  { slug: 'networking', title: 'Networking', group: 'foundations' },
  { slug: 'dns', title: 'DNS', group: 'foundations' },
  { slug: 'time-ntp', title: 'Time and NTP', group: 'foundations' },
  { slug: 'storage', title: 'Storage', group: 'foundations' },
  { slug: 'databases', title: 'Databases', group: 'building' },
  { slug: 'messaging', title: 'Messaging and streaming', group: 'building' },
  { slug: 'system-design', title: 'System and software design', group: 'building' },
  { slug: 'testing', title: 'Testing', group: 'building' },
  { slug: 'ci-cd', title: 'CI/CD', group: 'shipping' },
  { slug: 'automation', title: 'Automation', group: 'shipping' },
  { slug: 'containers', title: 'Containers', group: 'shipping' },
  { slug: 'kubernetes', title: 'Kubernetes', group: 'shipping' },
  { slug: 'observability', title: 'Observability', group: 'running' },
  { slug: 'load-balancing', title: 'Load balancing', group: 'running' },
  { slug: 'backups', title: 'Backups', group: 'running' },
  { slug: 'tls-pki', title: 'TLS and PKI', group: 'running' },
  { slug: 'iam', title: 'IAM', group: 'running' },
  { slug: 'security', title: 'Security', group: 'running' },
  { slug: 'sre', title: 'SRE and incident response', group: 'running' },
  { slug: 'ai-assisted-dev', title: 'AI-assisted development', group: 'working' },
  { slug: 'git', title: 'Git', group: 'working' },
  { slug: 'documentation', title: 'Documentation', group: 'working' },
  { slug: 'work-tracking', title: 'Ticket and work tracking', group: 'working' },
] as const satisfies readonly { slug: string; title: string; group: TrackGroup }[];

export type TrackSlug = (typeof tracks)[number]['slug'];
export type Track = { slug: TrackSlug; title: string; group: TrackGroup };

/** Non-empty tuple of slugs, for z.enum(). */
export const trackSlugs = tracks.map((t) => t.slug) as [TrackSlug, ...TrackSlug[]];

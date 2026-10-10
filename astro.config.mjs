// @ts-check
import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';
import starlightLinksValidator from 'starlight-links-validator';
import { weeks, phases } from './src/data/weeks.ts';
import { tracks, trackGroups } from './src/data/tracks.ts';

const site = process.env.SITE_URL ?? 'http://localhost:4321';
const base = process.env.BASE_PATH ?? '/';

// Built from the registries so the sidebar can't drift from the pages. Slug items
// get the site base applied by Starlight. The hidden dev/ fixture page is not listed.
const sidebar = [
  { label: 'Start Here', slug: 'start-here' },
  {
    label: 'Weeks',
    items: phases.map((p) => ({
      label: p.label,
      items: weeks
        .filter((w) => w.phase === p.id)
        .map((w) => ({ label: `${w.n}. ${w.title}`, slug: `weeks/${w.slug}` })),
    })),
  },
  {
    label: 'Tracks',
    items: trackGroups.map((g) => ({
      label: g.label,
      items: tracks
        .filter((t) => t.group === g.id)
        .map((t) => ({ label: t.title, slug: `tracks/${t.group}/${t.slug}` })),
    })),
  },
];

export default defineConfig({
  site,
  base,
  integrations: [
    starlight({
      title: 'Full-Stack Engineer Path',
      sidebar,
      plugins: [starlightLinksValidator()],
      customCss: ['./src/styles/diagrams.css', './src/styles/custom.css'],
    }),
  ],
});

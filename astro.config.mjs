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
      customCss: ['./src/styles/theme.css', './src/styles/diagrams.css', './src/styles/custom.css'],
      // Gruvbox syntax colours. The first theme is the default (dark); Starlight's dark
      // mode switch shows each theme in the matching site mode. useStarlightUiThemeColors
      // draws the frames from the site tokens (src/styles/theme.css): the editor sits on
      // bg0 (#282828 dark, #fbf1c7 light) and the title bar on the page colour.
      // Expressive Code lifts token colours to minSyntaxHighlightingColorContrast
      // against a fixed reference background (#23262f dark, #f6f7f9 light) rather than
      // the frame. The Gruvbox frames are slightly lighter (dark) and darker (light)
      // than those, so 5.5:1 there keeps every token above 4.5:1 on the frame while
      // leaving most Gruvbox colours untouched (it lifts the comment gray and the reds).
      expressiveCode: {
        themes: ['gruvbox-dark-hard', 'gruvbox-light-hard'],
        useStarlightDarkModeSwitch: true,
        useStarlightUiThemeColors: true,
        minSyntaxHighlightingColorContrast: 5.5,
      },
    }),
  ],
});

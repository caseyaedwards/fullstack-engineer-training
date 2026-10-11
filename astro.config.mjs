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
      favicon: '/favicon.svg',
      sidebar,
      plugins: [starlightLinksValidator()],
      customCss: [
        './src/styles/theme.css',
        './src/styles/diagrams.css',
        './src/styles/custom.css',
        './src/styles/width.css',
      ],
      // Adds the page width select beside the theme select.
      components: { ThemeSelect: './src/components/ThemeSelect.astro' },
      // Applies the stored page width before first paint (see src/styles/width.css).
      head: [
        {
          tag: 'script',
          content:
            "(function(){var w='medium';try{var s=localStorage.getItem('fsp-width');if(s==='slim'||s==='wide'||s==='full')w=s}catch(e){}document.documentElement.setAttribute('data-width',w)})()",
        },
      ],
      // Syntax colours: Shiki's vitesse themes, whose muted greens, browns and
      // tans sit with the ivory and clay palette. The first theme is the default
      // (dark); Starlight's dark mode switch shows each theme in the matching site
      // mode. useStarlightUiThemeColors draws the frames from the site tokens
      // (src/styles/theme.css): the editor sits on #262521 (dark) or paper #ffffff
      // (light). Expressive Code lifts token colours to
      // minSyntaxHighlightingColorContrast against a fixed reference background
      // (#23262f dark, #f6f7f9 light) rather than the frame; 5.5:1 there keeps
      // every token above 4.5:1 on the frame.
      expressiveCode: {
        themes: ['vitesse-dark', 'vitesse-light'],
        useStarlightDarkModeSwitch: true,
        useStarlightUiThemeColors: true,
        minSyntaxHighlightingColorContrast: 5.5,
        // Keep tabs: a Makefile recipe copied with spaces fails with
        // "missing separator". CSS sets how wide a tab looks.
        tabWidth: 0,
      },
    }),
  ],
});

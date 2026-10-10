import { getCollection } from 'astro:content';
import { tracks, type TrackSlug } from '../data/tracks';
import { weeks } from '../data/weeks';

/** Site base with exactly one trailing slash. BASE_URL may or may not have one, depending on BASE_PATH. */
const base = () => import.meta.env.BASE_URL.replace(/\/?$/, '/');

export function weekHref(n: number): string {
  const week = weeks.find((w) => w.n === n);
  if (!week) throw new Error(`weekHref: unknown week ${n}`);
  return `${base()}weeks/${week.slug}/`;
}

export function trackHref(slug: TrackSlug): string {
  const track = tracks.find((t) => t.slug === slug);
  if (!track) throw new Error(`trackHref: unknown track "${slug}"`);
  return `${base()}tracks/${track.group}/${track.slug}/`;
}

/** Week numbers (ascending) whose frontmatter `tracks` list names this track. */
export async function weeksForTrack(slug: TrackSlug): Promise<number[]> {
  const docs = await getCollection('docs');
  return docs
    .filter((d) => d.data.week !== undefined && d.data.tracks?.includes(slug))
    .map((d) => d.data.week as number)
    .sort((a, b) => a - b);
}

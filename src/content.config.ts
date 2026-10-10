import { defineCollection } from 'astro:content';
import { z } from 'astro/zod';
import { docsLoader } from '@astrojs/starlight/loaders';
import { docsSchema } from '@astrojs/starlight/schema';
import { trackSlugs } from './data/tracks';

// Zod's default enum message omits the offending value; name it so a typo is findable.
const trackSlug = z.enum(trackSlugs, {
  error: (issue) => `Unknown track slug ${JSON.stringify(issue.input)}. Expected one of: ${trackSlugs.join(', ')}`,
});

const weekFields = ['phase', 'hours', 'aiStage', 'tracks'] as const;

const extra = z.object({
  /** Week pages: 0 to 18. Its presence makes the other week fields required. */
  week: z.number().int().min(0).max(18).optional(),
  phase: z.enum(['foundations', 'build', 'harden', 'run']).optional(),
  /** Time estimate as [low, high] hours, e.g. [5, 8]. */
  hours: z
    .tuple([z.number().positive(), z.number().positive()])
    .refine(([low, high]) => low <= high, {
      error: (issue) => `hours must be a range [low, high] with low <= high, got ${JSON.stringify(issue.input)}`,
    })
    .optional(),
  aiStage: z.union([z.literal(1), z.literal(2), z.literal(3)]).optional(),
  tracks: z.array(trackSlug).min(1).optional(),
  /** Track pages. */
  track: trackSlug.optional(),
});

export const collections = {
  docs: defineCollection({
    loader: docsLoader(),
    schema: (ctx) =>
      docsSchema({ extend: extra })(ctx)
        .superRefine((data, refine) => {
          if (data.week === undefined) return;
          for (const field of weekFields) {
            if (data[field] === undefined) {
              refine.addIssue({
                code: 'custom',
                path: [field],
                message: 'Required on week pages (week is set)',
              });
            }
          }
        }),
  }),
};

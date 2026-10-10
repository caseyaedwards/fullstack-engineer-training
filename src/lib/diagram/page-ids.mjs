// Tracks diagram ids per rendered page so a repeated id fails the build
// (duplicate ids would break anchors, SVG id references and walkthroughs).
// Keyed on the page's Request object, which Astro creates once per page render.

/** @type {WeakMap<object, Set<string>>} */
const seen = new WeakMap();

/**
 * @param {object} page  per-page key (Astro.request)
 * @param {string} id
 */
export function claimId(page, id) {
  let ids = seen.get(page);
  if (!ids) seen.set(page, (ids = new Set()));
  if (ids.has(id)) {
    const where = /** @type {any} */ (page)?.url ? ` on ${new URL(/** @type {any} */ (page).url).pathname}` : '';
    throw new Error(`Diagram id "${id}" is used more than once${where}`);
  }
  ids.add(id);
}

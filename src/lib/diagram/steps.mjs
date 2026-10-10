// Validates walkthrough steps against the keys a rendered diagram exposes, so a
// typo or an unusable key fails the build instead of silently dimming everything.

/**
 * @param {string} id  diagram id, for error messages
 * @param {{ hl: string[] }[]} steps
 * @param {Set<string>} keys  data-k keys from renderD2
 */
export function checkSteps(id, steps, keys) {
  for (const [i, step] of steps.entries()) {
    for (const k of step.hl) {
      // data-hl is a space-separated list, so a key with whitespace can never match.
      if (/\s/.test(k)) {
        throw new Error(
          `Diagram "${id}" step ${i + 1}: highlight key "${k}" contains whitespace. ` +
            `Give the shape a short key and a label instead, e.g. "lb: load balancer".`,
        );
      }
      if (!keys.has(k)) {
        throw new Error(
          `Diagram "${id}" step ${i + 1}: unknown highlight key "${k}". Known keys: ${[...keys].join(', ')}`,
        );
      }
    }
  }
}

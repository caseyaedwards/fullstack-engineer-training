// Checklist progress, kept in localStorage as one JSON array of ticked item ids per
// week (`fsp:done:week-NN`). Storage can be missing or throw (private windows, blocked
// site data), so every access is wrapped and the session falls back to a Map.

const memory = new Map<number, string[]>();

const key = (week: number) => `fsp:done:week-${String(week).padStart(2, '0')}`;

export function loadDone(week: number): string[] {
  try {
    const parsed: unknown = JSON.parse(localStorage.getItem(key(week)) ?? 'null');
    if (Array.isArray(parsed)) return [...new Set(parsed.filter((id): id is string => typeof id === 'string'))];
  } catch {
    // Storage unavailable or the value is not JSON: use the session copy below.
  }
  return memory.get(week) ?? [];
}

export function saveDone(week: number, ids: string[]): void {
  memory.set(week, ids);
  try {
    localStorage.setItem(key(week), JSON.stringify(ids));
  } catch {
    // Kept in memory for this page view only.
  }
}

/**
 * True when `total` distinct items are ticked. Pass `current` (the week's present item ids) to
 * count only ids that still exist, so items removed or renamed since they were ticked don't count.
 */
export function weekComplete(week: number, total: number, current?: string[]): boolean {
  const ticked = current ? loadDone(week).filter((id) => current.includes(id)) : loadDone(week);
  return total > 0 && ticked.length >= total;
}

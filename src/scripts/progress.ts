// Client side of <DoneWhen> and <WeekTimeline>: restore ticks, persist changes, mark done weeks.
import { loadDone, saveDone, weekComplete } from '../lib/progress';

document.querySelectorAll<HTMLElement>('.done-when').forEach((list) => {
  const week = Number(list.dataset.week);
  const boxes = Array.from(list.querySelectorAll<HTMLInputElement>('input'));
  const done = loadDone(week);
  boxes.forEach((b) => (b.checked = done.includes(b.value)));
  list.onchange = () => saveDone(week, boxes.filter((b) => b.checked).map((b) => b.value));
});

const nodes = document.querySelectorAll<HTMLElement>('.week-timeline li[data-week]');
const mark = () =>
  nodes.forEach((li) => {
    const ids: string[] = JSON.parse(li.dataset.ids ?? '[]');
    li.toggleAttribute('data-complete', weekComplete(Number(li.dataset.week), ids.length, ids));
  });
mark();
if (nodes.length) {
  // Back/forward cache restores the page without re-running scripts; progress may have changed since.
  addEventListener('pageshow', (e) => e.persisted && mark());
  // A week ticked in another tab. A null key means that tab cleared all of storage.
  addEventListener('storage', (e) => (e.key === null || e.key.startsWith('fsp:done:')) && mark());
}

// Client side of <DoneWhen> and <WeekTimeline>: restore ticks, persist changes, mark done weeks.
import { loadDone, saveDone, weekComplete } from '../lib/progress';

document.querySelectorAll<HTMLElement>('.done-when').forEach((list) => {
  const week = Number(list.dataset.week);
  const boxes = Array.from(list.querySelectorAll<HTMLInputElement>('input'));
  const done = loadDone(week);
  boxes.forEach((b) => (b.checked = done.includes(b.value)));
  list.onchange = () => saveDone(week, boxes.filter((b) => b.checked).map((b) => b.value));
});

document.querySelectorAll<HTMLElement>('.week-timeline li[data-week]').forEach((li) => {
  li.toggleAttribute('data-complete', weekComplete(Number(li.dataset.week), Number(li.dataset.total)));
});

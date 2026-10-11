// Progressive enhancement for <Diagram>:
// - a diagram too wide for its box scrolls sideways, so its box becomes focusable
//   and keyboard users can scroll it with the arrow keys;
// - <Diagram steps> swaps the <ol class="dg-steps"> for Prev/Next/Play controls that
//   highlight each step's data-k elements and scroll them into view inside the box.
document.querySelectorAll<HTMLElement>('figure.dg').forEach((fig) => {
  const box = fig.querySelector<HTMLElement>('.dg-svg');
  if (box) {
    const label = `${fig.querySelector('figcaption')?.textContent ?? 'Diagram'} (scrolls sideways)`;
    const focusable = () => {
      const scrolls = box.scrollWidth > box.clientWidth;
      if (scrolls) {
        box.tabIndex = 0;
        box.setAttribute('role', 'region');
        box.setAttribute('aria-label', label);
      } else {
        box.removeAttribute('tabindex');
        box.removeAttribute('role');
        box.removeAttribute('aria-label');
      }
    };
    focusable();
    new ResizeObserver(focusable).observe(box);
  }

  const ol = fig.querySelector<HTMLElement>('ol.dg-steps');
  const steps = ol ? (Array.from(ol.children) as HTMLElement[]) : [];
  if (!ol || !steps.length) return;
  const els = Array.from(fig.querySelectorAll<SVGElement>('[data-k]'));
  const nav = document.createElement('div');
  nav.className = 'dg-nav';
  nav.setAttribute('role', 'group');
  nav.setAttribute('aria-label', 'Diagram steps');
  const btn = (label: string, d: number) => {
    const b = document.createElement('button');
    b.type = 'button';
    b.textContent = label;
    b.onclick = () => go(i + d);
    return b;
  };
  const [prev, next] = [btn('Prev', -1), btn('Next', 1)];
  const play = document.createElement('button');
  play.type = 'button';
  play.className = 'dg-play';
  play.textContent = 'Play';
  const status = document.createElement('span');
  status.setAttribute('aria-live', 'polite');
  nav.append(prev, status, next, play);
  nav.onkeydown = (e) => {
    const d = e.key === 'ArrowLeft' ? -1 : e.key === 'ArrowRight' ? 1 : 0;
    if (d) (e.preventDefault(), go(i + d));
  };

  // Play advances one step per STEP_MS, long enough to read a caption, and stops
  // on the last step. Any step change by the reader (go) stops it too.
  const STEP_MS = 2500;
  let timer: ReturnType<typeof setInterval> | undefined;
  const stop = () => {
    clearInterval(timer);
    timer = undefined;
    play.textContent = 'Play';
  };
  const go = (n: number) => {
    stop();
    show(n);
  };
  play.onclick = () => {
    if (timer !== undefined) return stop();
    if (i === steps.length - 1) show(0);
    play.textContent = 'Pause';
    timer = setInterval(() => {
      show(i + 1);
      if (i === steps.length - 1) stop();
    }, STEP_MS);
  };

  // Scroll the box sideways (never the page) so the elements in focus are in view:
  // all of them when they fit, otherwise their left edge.
  const reveal = (lit: SVGElement[]) => {
    if (!box || !lit.length || box.scrollWidth <= box.clientWidth) return;
    const b = box.getBoundingClientRect();
    const rects = lit.map((el) => el.getBoundingClientRect());
    const left = Math.min(...rects.map((r) => r.left)) - b.left;
    const right = Math.max(...rects.map((r) => r.right)) - b.left;
    const pad = 8;
    if (left < pad || right - left + 2 * pad > b.width) box.scrollLeft += left - pad;
    else if (right > b.width - pad) box.scrollLeft += right - b.width + pad;
  };

  let i = 0;
  let before: string[] = [];
  function show(n: number) {
    i = Math.max(0, Math.min(steps.length - 1, n));
    const hl = (steps[i].dataset.hl ?? '').split(/\s+/);
    const lit: SVGElement[] = [];
    for (const el of els) {
      const on = hl.includes(el.dataset.k!);
      el.classList.toggle('dg-hl', on);
      el.classList.toggle('dg-dim', !on);
      if (on) lit.push(el);
    }
    // Focus on the shapes this step adds (the new actor), else on everything lit.
    const added = lit.filter((el) => !before.includes(el.dataset.k!) && !/->|<-|--/.test(el.dataset.k!));
    reveal(added.length ? added : lit);
    before = hl;
    status.textContent = `${i + 1}/${steps.length}: ${steps[i].textContent}`;
    const active = document.activeElement;
    prev.disabled = i === 0;
    next.disabled = i === steps.length - 1;
    // A disabled button drops focus (and arrow keys with it): hand it to the other one.
    if (active === prev && prev.disabled) next.focus();
    else if (active === next && next.disabled) prev.focus();
  }
  ol.hidden = true;
  ol.after(nav);
  show(0);
});

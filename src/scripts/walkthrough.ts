// Progressive enhancement for <Diagram steps>: swap the <ol class="dg-steps">
// for Prev/Next controls that highlight each step's data-k elements.
document.querySelectorAll<HTMLElement>('figure.dg').forEach((fig) => {
  const ol = fig.querySelector<HTMLElement>('ol.dg-steps');
  const steps = ol ? Array.from(ol.children) as HTMLElement[] : [];
  if (!ol || !steps.length) return;
  const els = Array.from(fig.querySelectorAll<SVGElement>('[data-k]'));
  const nav = document.createElement('div');
  nav.className = 'dg-nav';
  const btn = (label: string, d: number) => {
    const b = document.createElement('button');
    b.type = 'button';
    b.textContent = label;
    b.onclick = () => show(i + d);
    return b;
  };
  const [prev, next] = [btn('Prev', -1), btn('Next', 1)];
  const status = document.createElement('span');
  status.setAttribute('aria-live', 'polite');
  nav.append(prev, status, next);
  nav.onkeydown = (e) => {
    const d = e.key === 'ArrowLeft' ? -1 : e.key === 'ArrowRight' ? 1 : 0;
    if (d) (e.preventDefault(), show(i + d));
  };
  let i = 0;
  function show(n: number) {
    i = Math.max(0, Math.min(steps.length - 1, n));
    const hl = (steps[i].dataset.hl ?? '').split(/\s+/);
    for (const el of els) {
      const on = hl.includes(el.dataset.k!);
      el.classList.toggle('dg-hl', on);
      el.classList.toggle('dg-dim', !on);
    }
    status.textContent = `${i + 1}/${steps.length}: ${steps[i].textContent}`;
    prev.disabled = i === 0;
    next.disabled = i === steps.length - 1;
  }
  ol.hidden = true;
  ol.after(nav);
  show(0);
});

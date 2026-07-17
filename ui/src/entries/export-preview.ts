// The Export page's live preview: as the filters change it re-fetches the same
// preview fragment the page rendered on first paint and swaps it into place. Not
// a React island — the second plain-TS one (daily-scroll is the first) — because
// it only moves server-rendered HTML into the page, formatting and parsing
// nothing.
//
// The count and every sample row come from the server: the fragment at
// /export/preview is the same Go partial that drew the first paint, so the
// preview can never drift from the download it previews, and no money or date
// formatting lives in here. The island reads the live filter values straight off
// the form (its single source of truth), debounces a burst of keystrokes into one
// request, and keeps one fetch in flight — a newer edit aborts an older one.

const DEBOUNCE_MS = 250;
const FILTERS = ['from', 'to', 'category', 'payer'] as const;

const mount = document.querySelector<HTMLElement>('[data-island="export-preview"]');
const form = mount?.closest('form') ?? document.querySelector<HTMLFormElement>('#export-form');

if (mount && form) {
  // Split the mount into a slot holding the last good preview and a hint line
  // for soft failures — so a mid-edit bad range or a dropped network leaves the
  // last preview (and the Download button) untouched instead of clearing it. The
  // slot is seeded with the server's first paint, so nothing fetches until an
  // edit.
  const slot = document.createElement('div');
  slot.innerHTML = mount.innerHTML;
  const hint = document.createElement('p');
  hint.className = 'text-xs text-amber-600 mt-2 hidden';
  mount.replaceChildren(slot, hint);

  const showHint = (text: string) => {
    hint.textContent = text;
    hint.classList.remove('hidden');
  };
  const clearHint = () => {
    hint.textContent = '';
    hint.classList.add('hidden');
  };

  const query = () => {
    const params = new URLSearchParams();
    for (const name of FILTERS) {
      const field = form.elements.namedItem(name) as HTMLInputElement | null;
      params.set(name, field?.value ?? '');
    }
    return params.toString();
  };

  let controller: AbortController | null = null;
  let timer: number | undefined;

  const update = async () => {
    controller?.abort(); // supersede any in-flight fetch; its catch sees the abort and bows out
    controller = new AbortController();
    const { signal } = controller;

    try {
      const res = await fetch(`/export/preview?${query()}`, { signal });
      // A bad or inverted range typed mid-edit is a soft 400: keep the last good
      // preview and the Download button, just nudge the dates. It can never 503 —
      // the preview does no rate work.
      if (res.status === 400) {
        showHint('Check the dates');
        return;
      }
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      slot.innerHTML = await res.text();
      clearHint();
    } catch {
      if (signal.aborted) return; // a newer edit owns the outcome now
      // Network or 5xx: there is no armed observer to disarm — the next edit
      // retries naturally — so leave the last preview and never block Download.
      showHint('Preview unavailable');
    }
  };

  const schedule = () => {
    clearTimeout(timer);
    timer = window.setTimeout(update, DEBOUNCE_MS);
  };

  // Every control in the form is one of the four filters (the presets are links
  // and the Custom toggle is a button, neither of which emits input/change), so
  // any input or change is a filter edit worth previewing.
  form.addEventListener('input', schedule);
  form.addEventListener('change', schedule);
}

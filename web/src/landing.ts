/** Landing page shown at the root URL, when there is no share fragment to open. */

import hljs from 'highlight.js/lib/core';
import bash from 'highlight.js/lib/languages/bash';
import powershell from 'highlight.js/lib/languages/powershell';

hljs.registerLanguage('bash', bash);
hljs.registerLanguage('powershell', powershell);

const INSTALL_COMMANDS = {
  unix: 'curl -fsSL https://punchpage.pages.dev/install.sh | sh',
  windows: 'irm https://punchpage.pages.dev/install.ps1 | iex'
} as const;

type Platform = keyof typeof INSTALL_COMMANDS;

const COPIED_RESET_MS = 1600;

/** Best-effort Windows detection, preferring the modern hints API. */
function detectPlatform(): Platform {
  const hinted = (navigator as Navigator & {userAgentData?: {platform?: string}}).userAgentData?.platform;
  const source = hinted || navigator.userAgent || '';
  return /win/i.test(source) ? 'windows' : 'unix';
}

/** Hides the status card and renders the marketing page. */
export function showLanding(): void {
  const status = document.querySelector('#status') as HTMLElement | null;
  const landing = document.querySelector('#landing') as HTMLElement | null;
  if (status) status.style.display = 'none';
  if (!landing) return;
  landing.hidden = false;
  document.body.classList.add('landing-mode');
  document.title = 'PunchPage: like ngrok, but peer-to-peer';

  // A share link pasted into the address bar of an already-open landing page is a
  // same-document navigation, so reload to let main.ts pick the fragment up.
  window.addEventListener('hashchange', () => location.reload(), {once: true});

  wireScrollLinks();
  wireCopyButton('#copy-prompt', '#agent-prompt');
  spinGlobe();

  const command = document.querySelector('#install-command') as HTMLElement | null;
  const unixTab = document.querySelector('#tab-unix') as HTMLButtonElement | null;
  const windowsTab = document.querySelector('#tab-windows') as HTMLButtonElement | null;
  const copyButton = document.querySelector('#copy-command') as HTMLButtonElement | null;
  const hint = document.querySelector('#install-hint') as HTMLElement | null;
  if (!command || !unixTab || !windowsTab || !copyButton) return;

  let platform: Platform = detectPlatform();
  let copiedTimer: ReturnType<typeof setTimeout> | undefined;

  function select(next: Platform): void {
    platform = next;
    const highlighted = hljs.highlight(INSTALL_COMMANDS[next], {
      language: next === 'windows' ? 'powershell' : 'bash'
    }).value;
    // hljs leaves URLs untokenized, and they are the part worth drawing the eye to.
    command!.innerHTML = highlighted.replace(
      /https?:\/\/[^\s<"]+/g,
      url => `<span class="hljs-link">${url}</span>`
    );
    unixTab!.setAttribute('aria-pressed', String(next === 'unix'));
    windowsTab!.setAttribute('aria-pressed', String(next === 'windows'));
    if (hint) hint.hidden = next !== 'windows';
  }

  unixTab.addEventListener('click', () => select('unix'));
  windowsTab.addEventListener('click', () => select('windows'));
  copyButton.addEventListener('click', () => {
    const text = INSTALL_COMMANDS[platform];
    void copyText(text, command).then(ok => {
      copiedTimer = markCopied(copyButton!, ok, copiedTimer);
    });
  });

  select(platform);
}

/* ------------------------------------------------------------------ globe */

const GLOBE = {
  cx: 140,
  cy: 140,
  r: 92,
  points: 820,
  tilt: (-20 * Math.PI) / 180,   // north pole tipped toward the viewer
  light: [-0.48, 0.56, 0.68],    // upper left, slightly in front
  seconds: 78,                   // one revolution
  fps: 30,
  bands: 7                       // brightness steps, one <path> each
} as const;

/**
 * Turns the hero globe. The markup ships a static frame of the same sphere, so
 * this only takes over when motion is welcome; it swaps the ~390 individual
 * dots for a few paths and rewrites those instead, which keeps a frame to a
 * handful of attribute writes rather than a couple of thousand.
 */
function spinGlobe(): void {
  const land = document.querySelector('.art-land') as SVGGElement | null;
  if (!land) return;
  if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) return;

  const {cx, cy, r, points, tilt, seconds, fps, bands} = GLOBE;
  const [lx, ly, lz] = GLOBE.light;
  const lightLen = Math.hypot(lx, ly, lz);
  const golden = Math.PI * (3 - Math.sqrt(5));

  // unit sphere, Fibonacci spaced: even coverage with no ring banding
  const base = new Float64Array(points * 3);
  for (let i = 0; i < points; i++) {
    const y = 1 - (2 * (i + 0.5)) / points;
    const rad = Math.sqrt(Math.max(0, 1 - y * y));
    const th = i * golden;
    base[i * 3] = Math.cos(th) * rad;
    base[i * 3 + 1] = y;
    base[i * 3 + 2] = Math.sin(th) * rad;
  }

  land.textContent = '';
  const paths: SVGPathElement[] = [];
  for (let b = 0; b < bands; b++) {
    const path = document.createElementNS('http://www.w3.org/2000/svg', 'path');
    path.setAttribute('opacity', (0.07 + (0.63 * b) / (bands - 1)).toFixed(3));
    land.appendChild(path);
    paths.push(path);
  }

  const cosT = Math.cos(tilt);
  const sinT = Math.sin(tilt);
  const buffers: string[] = new Array<string>(bands).fill('');

  function draw(angle: number): void {
    buffers.fill('');
    const cosA = Math.cos(angle);
    const sinA = Math.sin(angle);

    for (let i = 0; i < base.length; i += 3) {
      const x0 = base[i] as number;
      const y0 = base[i + 1] as number;
      const z0 = base[i + 2] as number;

      const x = x0 * cosA + z0 * sinA;          // spin about the globe's axis
      const zs = -x0 * sinA + z0 * cosA;
      const y = y0 * cosT - zs * sinT;          // then tip toward the viewer
      const z = y0 * sinT + zs * cosT;
      if (z <= 0.05) continue;                  // facing away

      const sx = cx + r * x;
      const sy = cy - r * y;
      const dot = 0.45 + Math.sqrt(z);          // foreshortened near the limb
      const lambert = Math.max(0, (x * lx + y * ly + z * lz) / lightLen);
      const band = Math.min(bands - 1, Math.round(Math.pow(lambert, 1.15) * (bands - 1)));

      // a filled circle as two arcs, cheaper than one element per dot
      buffers[band] +=
        `M${sx.toFixed(1)} ${sy.toFixed(1)}m-${dot.toFixed(2)} 0` +
        `a${dot.toFixed(2)} ${dot.toFixed(2)} 0 1 0 ${(dot * 2).toFixed(2)} 0` +
        `a${dot.toFixed(2)} ${dot.toFixed(2)} 0 1 0 -${(dot * 2).toFixed(2)} 0`;
    }

    for (let b = 0; b < bands; b++) paths[b]?.setAttribute('d', buffers[b] ?? '');
  }

  let running = true;
  let last = 0;
  const frame = 1000 / fps;
  const start = performance.now();

  function tick(now: number): void {
    if (!running) return;
    if (now - last >= frame) {
      last = now;
      draw(((now - start) / (seconds * 1000)) * Math.PI * 2);
    }
    requestAnimationFrame(tick);
  }

  function setRunning(next: boolean): void {
    if (next === running) return;
    running = next;
    if (next) requestAnimationFrame(tick);
  }

  document.addEventListener('visibilitychange', () => setRunning(!document.hidden));
  const art = land.closest('svg');
  if (art && 'IntersectionObserver' in window) {
    new IntersectionObserver(
      entries => setRunning(!document.hidden && entries.some(e => e.isIntersecting))
    ).observe(art);
  }

  draw(0);
  requestAnimationFrame(tick);
}

/**
 * Scrolls in-page anchors without touching `location.hash` — a fragment here
 * would look like a (broken) share link to main.ts on the next page load.
 */
function wireScrollLinks(): void {
  const prefersReducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
  for (const link of document.querySelectorAll<HTMLAnchorElement>('#landing a[data-scroll]')) {
    link.addEventListener('click', event => {
      const target = document.querySelector(link.getAttribute('href') || '');
      if (!target) return;
      event.preventDefault();
      target.scrollIntoView({behavior: prefersReducedMotion ? 'auto' : 'smooth', block: 'start'});
    });
  }
}

/** Wires a copy button to a static text element, mirroring the install button's feedback. */
function wireCopyButton(buttonSelector: string, sourceSelector: string): void {
  const button = document.querySelector(buttonSelector) as HTMLButtonElement | null;
  const source = document.querySelector(sourceSelector) as HTMLElement | null;
  if (!button || !source) return;

  let timer: ReturnType<typeof setTimeout> | undefined;
  button.addEventListener('click', () => {
    void copyText(source.textContent || '', source).then(ok => {
      timer = markCopied(button, ok, timer);
    });
  });
}

/** Flips a copy button into its checkmark state, or hints at manual copy on failure. */
function markCopied(
  button: HTMLButtonElement,
  ok: boolean,
  timer: ReturnType<typeof setTimeout> | undefined
): ReturnType<typeof setTimeout> | undefined {
  button.classList.toggle('copied', ok);
  button.title = ok ? 'Copied' : 'Press ⌘/Ctrl+C to copy the selected text';
  clearTimeout(timer);
  if (!ok) return undefined;
  return setTimeout(() => {
    button.classList.remove('copied');
    button.title = 'Copy';
  }, COPIED_RESET_MS);
}

/** Copies text via the async clipboard API, falling back to selecting the source element. */
async function copyText(text: string, fallback: Element | null): Promise<boolean> {
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text);
      return true;
    }
  } catch { /* denied or unavailable — fall through to selection */ }
  const selection = window.getSelection();
  if (fallback && selection) {
    const range = document.createRange();
    range.selectNodeContents(fallback);
    selection.removeAllRanges();
    selection.addRange(range);
  }
  return false;
}

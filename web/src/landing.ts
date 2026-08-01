/** Landing page shown at the root URL, when there is no share fragment to open. */

import hljs from 'highlight.js/lib/core';
import bash from 'highlight.js/lib/languages/bash';
import powershell from 'highlight.js/lib/languages/powershell';

hljs.registerLanguage('bash', bash);
hljs.registerLanguage('powershell', powershell);

const INSTALL_COMMANDS = {
  unix: 'curl -fsSL https://punchpage.pages.dev/install.sh | sh',
  windows: 'powershell -c "irm https://punchpage.pages.dev/install.ps1 | iex"'
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
  document.title = 'PunchPage — like ngrok, but peer-to-peer';

  // A share link pasted into the address bar of an already-open landing page is a
  // same-document navigation, so reload to let main.ts pick the fragment up.
  window.addEventListener('hashchange', () => location.reload(), {once: true});

  wireScrollLinks();
  wireCopyButton('#copy-prompt', '#agent-prompt');

  const command = document.querySelector('#install-command') as HTMLElement | null;
  const unixTab = document.querySelector('#tab-unix') as HTMLButtonElement | null;
  const windowsTab = document.querySelector('#tab-windows') as HTMLButtonElement | null;
  const copyButton = document.querySelector('#copy-command') as HTMLButtonElement | null;
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

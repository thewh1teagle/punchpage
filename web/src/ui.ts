/** View layer: status/error display and the tunneled-site iframe. */

const statusEl = document.querySelector('#status') as HTMLElement;
const statusText = document.querySelector('#status-text') as HTMLElement;

/** The iframe that hosts the tunneled site. */
export const site = document.querySelector('#site') as HTMLIFrameElement;

/** Shows a normal progress message with the loading indicator. */
export function setStatus(text: string): void {
  statusEl.classList.remove('error');
  statusText.textContent = text;
}

/** Shows a failure message with error styling (no spinner). */
export function showError(text: string): void {
  statusEl.classList.add('error');
  statusText.textContent = text;
}

/** Hides the status screen and reveals the site iframe full-viewport. */
export function showSite(): void {
  statusEl.style.display = 'none';
  site.style.display = 'block';
}

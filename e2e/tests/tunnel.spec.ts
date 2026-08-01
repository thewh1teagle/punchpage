import {spawn, type ChildProcess} from 'node:child_process';
import {expect, test, type Browser, type Frame} from '@playwright/test';

const ROOT = new URL('../..', import.meta.url).pathname;
const FIXTURE_PORT = 8213;
const CLIENT_PORT = 4188;

const children: ChildProcess[] = [];

function run(command: string, args: string[], cwd = ROOT): ChildProcess {
  const child = spawn(command, args, {cwd, stdio: ['ignore', 'pipe', 'pipe']});
  children.push(child);
  return child;
}

/** Resolves once a line of the child's output matches the pattern. */
function waitForLine(child: ChildProcess, pattern: RegExp, label: string): Promise<string> {
  return new Promise((resolve, reject) => {
    let buffer = '';
    const onData = (chunk: Buffer) => {
      // Vite styles its output with ANSI escapes; strip them so patterns match.
      buffer += chunk.toString().replace(/\x1b\[[0-9;]*m/g, '');
      const match = buffer.match(pattern);
      if (match) resolve(match[0]);
    };
    child.stdout?.on('data', onData);
    child.stderr?.on('data', onData);
    child.on('exit', code => reject(new Error(`${label} exited early (code ${code}):\n${buffer}`)));
    setTimeout(() => reject(new Error(`${label} did not become ready:\n${buffer}`)), 60000);
  });
}

test.afterAll(() => {
  for (const child of children) child.kill('SIGTERM');
});

/** Starts the fixture, the static client, and a punch host; returns the link. */
async function startStack(fixturePort: number, clientPort: number): Promise<string> {
  const fixture = run('go', ['run', './e2e/fixture', '--port', String(fixturePort)]);
  // Serve the client under /punchpage/ to mirror the production path layout.
  const client = run('node', ['serve.cjs', `${ROOT}web/dist`, String(clientPort)], `${ROOT}e2e`);
  await Promise.all([
    waitForLine(fixture, /fixture listening/, 'fixture server'),
    waitForLine(client, /client serving/, 'client server')
  ]);
  const host = run('go', [
    'run', './cmd/punch',
    String(fixturePort),
    '--page', `http://127.0.0.1:${clientPort}/punchpage/`
  ]);
  const link = new RegExp(`http://127\\.0\\.0\\.1:${clientPort}/punchpage/#r=\\S+`);
  return waitForLine(host, link, 'punchpage host');
}

test('tunnels fetch, redirect, large, upload, cookie, sse, and websocket', async ({page}) => {
  const shareLink = await startStack(FIXTURE_PORT, CLIENT_PORT);
  await page.goto(shareLink);
  const site = page.frameLocator('#site');
  await expect(site.locator('#result')).toHaveText('ALL CHECKS PASSED', {timeout: 90000});
});

test('keeps each viewer\'s cookies to itself', async ({browser}) => {
  // Its own ports: the first test's servers are still up until afterAll.
  const shareLink = await startStack(FIXTURE_PORT + 1, CLIENT_PORT + 1);

  // Separate contexts, so the two viewers share nothing in the browser either.
  const first = await openViewer(browser, shareLink);
  const second = await openViewer(browser, shareLink);

  expect(await tunnelled(first, 'api/login')).toBe('logged in');
  expect(await tunnelled(first, 'api/whoami')).toBe('secret');
  // The host builds one bridge, and so one cookie jar, per viewer.
  expect(await tunnelled(second, 'api/whoami')).toBe('');
});

/** Opens the share link and resolves the frame running the tunnelled site. */
async function openViewer(browser: Browser, shareLink: string): Promise<Frame> {
  const page = await (await browser.newContext()).newPage();
  await page.goto(shareLink);
  await expect(page.frameLocator('#site').locator('#result')).toBeVisible({timeout: 90000});
  const frame = page.frames().find(candidate => candidate.url().includes('__punchpage__'));
  if (!frame) throw new Error('the tunnelled frame never appeared');
  return frame;
}

/** Fetches a path from inside the tunnelled frame, so it travels the tunnel. */
function tunnelled(frame: Frame, path: string): Promise<string> {
  return frame.evaluate(async target => (await fetch(target)).text(), path);
}

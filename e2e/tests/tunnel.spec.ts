import {spawn, type ChildProcess} from 'node:child_process';
import {expect, test} from '@playwright/test';

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

test('tunnels fetch, redirect, large, upload, cookie, and websocket', async ({page}) => {
  const fixture = run('go', ['run', './e2e/fixture', '--port', String(FIXTURE_PORT)]);
  const client = run('pnpm', ['exec', 'vite', 'preview', '--port', String(CLIENT_PORT), '--strictPort', '--host', '127.0.0.1'], `${ROOT}web`);
  await Promise.all([
    waitForLine(fixture, /fixture listening/, 'fixture server'),
    waitForLine(client, /127\.0\.0\.1:4188/, 'client preview')
  ]);

  const host = run('go', [
    'run', './cmd/punchpage',
    '--target', `http://127.0.0.1:${FIXTURE_PORT}`,
    '--page', `http://127.0.0.1:${CLIENT_PORT}/`
  ]);
  const shareLink = await waitForLine(host, /http:\/\/127\.0\.0\.1:4188\/#r=\S+/, 'punchpage host');

  await page.goto(shareLink);
  const site = page.frameLocator('#site');
  await expect(site.locator('#result')).toHaveText('ALL CHECKS PASSED', {timeout: 90000});
});

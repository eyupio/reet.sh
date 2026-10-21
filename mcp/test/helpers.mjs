import { spawn, execFile } from 'node:child_process';
import { promisify } from 'node:util';
import { mkdtemp, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { createInterface } from 'node:readline';
import { Client } from '@modelcontextprotocol/client';
import { StdioClientTransport } from '@modelcontextprotocol/client/stdio';

export const root = fileURLToPath(new URL('..', import.meta.url));

export async function connect(url, token = 'fixture-token') {
  const transport = new StdioClientTransport({
    command: process.execPath, args: [join(root, 'dist/index.js')],
    env: { REET_URL: url, REET_API_TOKEN: token }, stderr: 'pipe',
  });
  const client = new Client({ name: 'reet-protocol-test', version: '1.0.0' });
  await client.connect(transport);
  return client;
}

export async function fixture(mode = 'token') {
  const dir = await mkdtemp(join(tmpdir(), 'reet-mcp-build-'));
  const binary = join(dir, 'fixture');
  try {
    await promisify(execFile)(process.env.REET_GO ?? 'go', ['build', '-o', binary, './mcp/test/fixture'], {
      cwd: join(root, '..'), timeout: 180_000,
    });
    const child = spawn(binary, [mode], { stdio: ['ignore', 'pipe', 'pipe'] });
    const url = await new Promise((resolve, reject) => {
      const lines = createInterface({ input: child.stdout });
      const timer = setTimeout(() => { child.kill(); reject(new Error('Fixture startup timed out')); }, 15_000);
      child.once('exit', code => { clearTimeout(timer); reject(new Error(`Fixture exited: ${code}`)); });
      child.once('error', reject);
      lines.once('line', line => { clearTimeout(timer); resolve(line); });
    });
    return {
      url,
      async close() {
        await new Promise(resolve => { child.once('exit', resolve); child.kill(); });
        await rm(dir, { recursive: true, force: true });
      },
    };
  } catch (error) {
    await rm(dir, { recursive: true, force: true });
    throw error;
  }
}

export async function call(client, name, args = {}) {
  const result = await client.callTool({ name: `reet_${name}`, arguments: args });
  if (result.isError) throw new Error(result.content[0].text);
  return result.structuredContent.data;
}

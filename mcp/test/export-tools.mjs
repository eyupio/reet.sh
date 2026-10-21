// Export the stdio tool contract for embedding in the hosted Go server.
import { readFile, writeFile } from 'node:fs/promises';
import { connect } from './helpers.mjs';

const client = await connect('http://127.0.0.1:1', ''); // Discovery makes no HTTP API calls.
try {
  const { tools } = await client.listTools();
  const path = new URL('../../internal/api/mcp_tools.json', import.meta.url);
  const generated = `${JSON.stringify(tools, null, 2)}\n`;
  if (process.argv.includes('--check')) {
    if (await readFile(path, 'utf8') !== generated) {
      throw new Error('Hosted tool contract is stale. Run npm run export-tools and commit internal/api/mcp_tools.json.');
    }
  } else {
    await writeFile(path, generated);
  }
} finally { await client.close(); }

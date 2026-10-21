#!/usr/bin/env node
import { StdioServerTransport } from '@modelcontextprotocol/server/stdio';
import { Client, StreamableHTTPClientTransport } from '@modelcontextprotocol/client';
import { object, ReetClient } from './client.js';
import { createServer } from './server.js';

if (process.argv.includes('--help') || process.argv.includes('-h')) {
  console.log('reet-mcp: MCP over stdio\nREET_URL: instance URL (default http://127.0.0.1:8080)\nREET_API_TOKEN: scoped MCP bearer token (recommended); general API/legacy tokens also supported. Omit for REET_AUTH=none.');
} else {
  try {
    const url = process.env.REET_URL ?? 'http://127.0.0.1:8080';
    const token = process.env.REET_API_TOKEN;
    const api = new ReetClient(url, token);
    let server;
    if (token?.startsWith('reet_mcp_')) {
      const remote = new Client({ name: 'reet-stdio-bridge', version: '0.1.0' });
      const endpoint = url.replace(/\/$/, '').endsWith('/mcp') ? url.replace(/\/$/, '') : `${url.replace(/\/$/, '')}/mcp`;
      await remote.connect(new StreamableHTTPClientTransport(new URL(endpoint), { requestInit: { headers: { Authorization: `Bearer ${token}` } } }));
      const { tools } = await remote.listTools();
      server = createServer(api, {
        allowedTools: new Set(tools.map(tool => tool.name)),
        async call(name, args) {
          const result = await remote.callTool({ name, arguments: args });
          if (result.isError) {
            const content = result.content.find(item => item.type === 'text');
            throw new Error(content?.type === 'text' ? content.text : 'The hosted MCP call failed.');
          }
          return object(object(result.structuredContent).data);
        },
      });
    } else {
      server = createServer(api);
    }
    await server.connect(new StdioServerTransport());
  } catch (error) {
    console.error(error instanceof Error ? error.message : 'Failed to start reet-mcp.');
    process.exitCode = 1;
  }
}

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { Client, StreamableHTTPClientTransport, auth } from '@modelcontextprotocol/client';
import { fixture, call } from './helpers.mjs';

test('SDK OAuth discovery, public registration, PKCE, consent, token exchange and refresh just connect', async () => {
  const api = await fixture('accounts');
  let clientInformation, tokens, verifier, authorizationURL, discovery;
  const provider = {
    redirectUrl: 'http://127.0.0.1:9000/callback',
    clientMetadata: { client_name: 'SDK desktop client', redirect_uris: ['http://127.0.0.1:9000/callback'], token_endpoint_auth_method: 'none', grant_types: ['authorization_code', 'refresh_token'], response_types: ['code'] },
    clientInformation: () => clientInformation,
    saveClientInformation: value => { clientInformation = value; },
    tokens: () => tokens,
    saveTokens: value => { tokens = value; },
    state: () => 'sdk-oauth-state',
    redirectToAuthorization: url => { authorizationURL = url; },
    saveCodeVerifier: value => { verifier = value; },
    codeVerifier: () => verifier,
    discoveryState: () => discovery,
    saveDiscoveryState: value => { discovery = value; },
  };
  const client = new Client({ name: 'oauth-client-test', version: '1.0.0' });
  try {
    await auth(provider, { serverUrl: `${api.url}/mcp` });
    assert.ok(authorizationURL);
    assert.equal(authorizationURL.searchParams.get('code_challenge_method'), 'S256');
    const requestScope = authorizationURL.searchParams.get('scope');
    assert.ok(requestScope.includes('mcp:read') && requestScope.includes('mcp:write'));
    const response = await fetch(`${api.url}/api/v2/mcp/authorization`, {
      method: 'POST', headers: { 'Content-Type': 'application/json', Cookie: 'reet_session=fixture-session' },
      body: JSON.stringify({ query: authorizationURL.searchParams.toString(), approve: true, scope: 'write' }),
    });
    assert.equal(response.status, 200);
    const callback = new URL((await response.json()).redirect);
    assert.equal(callback.searchParams.get('state'), 'sdk-oauth-state');
    await auth(provider, { serverUrl: `${api.url}/mcp`, authorizationCode: callback.searchParams.get('code'), iss: callback.searchParams.get('iss') });
    assert.ok(tokens.access_token && tokens.refresh_token);
    await client.connect(new StreamableHTTPClientTransport(new URL(`${api.url}/mcp`), { authProvider: provider }));
    assert.equal((await client.listTools()).tools.length, 19);
    assert.equal((await call(client, 'list_skills', { limit: 1 })).total_count, 6);
    const oldRefresh = tokens.refresh_token;
    await auth(provider, { serverUrl: `${api.url}/mcp` });
    assert.notEqual(tokens.refresh_token, oldRefresh);
    assert.equal((await call(client, 'list_skills', { limit: 1 })).total_count, 6);
  } finally { await client.close(); await api.close(); }
});

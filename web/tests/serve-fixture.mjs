// Serves one component fixture on the local network, so it can be opened on a real phone or tablet:
//   npm run fixture:questions           (then open the printed address on the device)
// The fixture is a test page (tests/fixtures/*.html). It answers nothing from a real server: the page scripts its own stream and only
// records the answers you send. It is a Vite dev server, reachable by anyone on the same network while it runs; stop it with Ctrl+C.
import { createServer } from 'vite'
import { networkInterfaces } from 'node:os'
import { fileURLToPath } from 'node:url'

const page = process.argv[2] || 'question-card'
const root = fileURLToPath(new URL('../', import.meta.url))
const server = await createServer({
  root, configFile: `${root}vite.config.ts`, logLevel: 'error', clearScreen: false,
  optimizeDeps: { noDiscovery: true, include: [] },
  server: { host: '0.0.0.0', port: Number(process.env.PORT) || 5180, strictPort: true, hmr: false, watch: null, allowedHosts: true },
})
await server.listen()
const { port } = server.httpServer.address()
const addresses = Object.values(networkInterfaces()).flat().filter(a => a && a.family === 'IPv4' && !a.internal).map(a => a.address)
console.log(`Fixture "${page}" is running. Open one of these on your device (same network):`)
for (const a of addresses.length ? addresses : ['127.0.0.1']) console.log(`  http://${a}:${port}/tests/fixtures/${page}.html`)
console.log('Add ?theme=light for the light theme, ?tool=claude for Claude Code wording. Ctrl+C to stop.')

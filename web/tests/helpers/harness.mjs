// A Vite dev server for component fixtures under web/tests/fixtures (a page that mounts one component, with the app's
// stylesheets), for a component that no page shows yet. Nothing here is built into the app. The API is answered by the
// test's own page.route, as on the built app; the server only compiles and serves the fixture's modules.
import { createServer } from 'vite'
import { fileURLToPath } from 'node:url'

export async function startHarness() {
  const root = fileURLToPath(new URL('../../', import.meta.url))
  const server = await createServer({
    root, configFile: `${root}vite.config.ts`, logLevel: 'error', clearScreen: false,
    // No discovery: a dependency found mid-test would make Vite reload the page.
    optimizeDeps: { noDiscovery: true, include: [] },
    server: { host: '127.0.0.1', port: 0, hmr: false, watch: null },
  })
  await server.listen()
  const { port } = server.httpServer.address()
  return { url: `http://127.0.0.1:${port}`, close: () => server.close() }
}

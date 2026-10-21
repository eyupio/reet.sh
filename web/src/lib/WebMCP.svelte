<script lang="ts">
  import { onMount } from 'svelte'
  import ConfirmDialog from './ConfirmDialog.svelte'
  import { callBrowserMCP, getBrowserMCPTools, getMCPAccess, type MCPTool } from './api'
  import { browserModelContext } from './webmcp'
  let pending = $state<{ title: string; body: string; details: string; danger: boolean; resolve: (allow: boolean) => void } | null>(null)

  function ask(tool: MCPTool, args: Record<string, unknown>): Promise<boolean> {
    if (pending) return Promise.resolve(false)
    return new Promise(resolve => {
      pending = { title: tool.title ?? tool.name, body: 'Your browser assistant wants to change your shelf. Review the proposed changes before allowing this request.', details: JSON.stringify(args, null, 2), danger: tool.annotations.destructiveHint ?? false, resolve }
    })
  }
  function answer(allow: boolean) { const current = pending; pending = null; current?.resolve(allow) }

  onMount(() => {
    const context = browserModelContext()
    if (!context) return
    const registered = new Set<string>()
    let generation = 0
    let disposed = false
    function clear() {
      for (const name of registered) { try { context!.unregisterTool(name) } catch { /* already removed by the browser */ } }
      registered.clear()
      answer(false)
    }
    async function refresh() {
      const version = ++generation
      clear()
      try {
        const access = await getMCPAccess()
        if (disposed || version !== generation || !access.enabled || !access.browserEnabled) return
        const { tools } = await getBrowserMCPTools()
        if (disposed || version !== generation) return
        for (const tool of tools) {
          context!.registerTool({
            name: tool.name, description: tool.description, inputSchema: tool.inputSchema,
            annotations: { readOnlyHint: tool.annotations.readOnlyHint },
            execute: async (args, client) => {
              if (disposed || version !== generation) throw new Error('Browser access changed. Retry after the page updates.')
              if (!tool.annotations.readOnlyHint) {
                const confirm = () => ask(tool, args)
                const allowed = client?.requestUserInteraction ? await client.requestUserInteraction(confirm) : await confirm()
                if (!allowed || disposed || version !== generation) return { isError: true, content: [{ type: 'text', text: 'The user cancelled this change.' }] }
              }
              const result = await callBrowserMCP(tool.name, args)
              if (!tool.annotations.readOnlyHint) window.dispatchEvent(new Event('reet-shelf-changed'))
              return result
            },
          })
          registered.add(tool.name)
        }
      } catch { clear() }
    }
    const refreshAccess = () => { void refresh() }
    const focusRefresh = () => { if (!pending) refreshAccess() }
    // Recheck on focus and visibility changes so another tab's revocation is reflected promptly.
    const visible = () => { if (!document.hidden) focusRefresh() }
    window.addEventListener('reet-mcp-changed', refreshAccess)
    window.addEventListener('focus', focusRefresh)
    document.addEventListener('visibilitychange', visible)
    void refresh()
    return () => {
      disposed = true; generation++; clear()
      window.removeEventListener('reet-mcp-changed', refreshAccess)
      window.removeEventListener('focus', focusRefresh)
      document.removeEventListener('visibilitychange', visible)
    }
  })
</script>

{#if pending}<ConfirmDialog title={pending.title} body={pending.body} details={pending.details} confirmLabel="Allow this change" danger={pending.danger} onconfirm={() => answer(true)} oncancel={() => answer(false)} />{/if}

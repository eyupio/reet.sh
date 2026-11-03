<script lang="ts">
  import Toast from './Toast.svelte'

  let { message, resetKey = 0, tone = 'ok', actionLabel, onaction, ondismiss }: {
    message: string
    resetKey?: number
    tone?: 'ok' | 'bad'
    actionLabel?: string
    onaction?: () => void | Promise<void>
    ondismiss: () => void
  } = $props()
</script>

{#if message}
  {#key `${resetKey}:${message}`}
    <Toast {message} {tone} {actionLabel} {onaction} {ondismiss} />
  {/key}
{/if}

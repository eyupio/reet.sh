<script lang="ts">
  let { id, value = $bindable(''), autocomplete = 'current-password', placeholder = '', required = true, minlength, describedby }: {
    id: string
    value?: string
    autocomplete?: 'current-password' | 'new-password' | 'off'
    placeholder?: string
    required?: boolean
    minlength?: number
    describedby?: string
  } = $props()
  let visible = $state(false)
</script>

<div class="password-field">
  <input {id} bind:value type={visible ? 'text' : 'password'} {autocomplete} {placeholder} {required} {minlength} aria-describedby={describedby} />
  <button type="button" aria-controls={id} aria-pressed={visible} aria-label={visible ? 'Hide password' : 'Show password'} onclick={() => (visible = !visible)}>{visible ? 'Hide' : 'Show'}</button>
</div>

<style>
  .password-field { position: relative; min-width: 0; }
  .password-field input { width: 100%; padding-right: 68px; }
  button { position: absolute; right: 1px; top: 0; bottom: 0; min-width: 56px; min-height: 44px; border: 0; border-radius: 6px; background: transparent; color: var(--muted); font-size: 0.875rem; font-weight: 600; cursor: pointer; }
  button:hover { background: var(--tint); color: var(--ink); }
</style>

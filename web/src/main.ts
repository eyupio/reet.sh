import { mount } from 'svelte'
import './app.css'
import './dialog-shell.css'
import App from './App.svelte'
import { installCombobox } from './lib/combobox'

installCombobox()

const target = document.getElementById('app')!

// Remove the static crawler fallback from index.html before mounting the app.
target.replaceChildren()

// The boot shell in index.html holds the page's shape until the app knows what it is drawing. App.svelte sets
// `data-ready` once it does, which fades the shell; then it, and its inline styles, are removed. A reader who prefers
// reduced motion gets no fade, so the shell goes on the next frame instead of waiting for a transition that never runs.
const boot = document.getElementById('boot')
const bootStyle = document.getElementById('seo-fallback-style')
const reduceMotion = matchMedia('(prefers-reduced-motion: reduce)').matches

function dismissBoot() {
  if (!boot?.isConnected) {
    bootStyle?.remove()
    return
  }
  const remove = () => {
    boot.remove()
    bootStyle?.remove()
  }
  if (reduceMotion) {
    requestAnimationFrame(remove)
    return
  }
  boot.addEventListener('transitionend', remove, { once: true })
  // A transition that never fires (a hidden tab, a browser that batched the change) must still clear the shell.
  setTimeout(remove, 400)
}

function watchReady() {
  if (document.documentElement.dataset.ready) {
    dismissBoot()
    return
  }
  const observer = new MutationObserver(() => {
    if (!document.documentElement.dataset.ready) return
    observer.disconnect()
    dismissBoot()
  })
  observer.observe(document.documentElement, { attributes: true, attributeFilter: ['data-ready'] })
}

export default mount(App, { target })
watchReady()

import { mount } from 'svelte'
import './app.css'
import './dialog-shell.css'
import App from './App.svelte'
import { installCombobox } from './lib/combobox'

installCombobox()

const target = document.getElementById('app')!

// Remove the static crawler fallback from index.html before mounting the app.
target.replaceChildren()
document.getElementById('seo-fallback-style')?.remove()

export default mount(App, { target })

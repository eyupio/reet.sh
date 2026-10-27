import { mount } from 'svelte'
import '../../src/app.css'
import '../../src/dialog-shell.css'
import Harness from './AddProjectHarness.svelte'

// ?theme=light|dark picks the palette (dark, the designed one, by default).
document.documentElement.dataset.theme = new URLSearchParams(location.search).get('theme') === 'light' ? 'light' : 'dark'
mount(Harness, { target: document.getElementById('app')! })

import { mount } from 'svelte'
import '../../src/app.css'
import GafferArtworkHarness from './GafferArtworkHarness.svelte'
document.documentElement.dataset.theme = new URLSearchParams(location.search).get('theme') === 'light' ? 'light' : 'dark'
mount(GafferArtworkHarness, { target: document.getElementById('app')! })

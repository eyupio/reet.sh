// Tiny shared state for the concept mockups: filtering, sorting, toggles. Rendering lives in each concept.
;(function () {
  const skills = window.REET_SKILLS.map((s) => ({ ...s }))
  const state = { tab: 'all', q: '', sort: 'recent' }
  const listeners = []

  const esc = (v) =>
    String(v).replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[c])
  const fmt = (n) => (n >= 1e6 ? (n / 1e6).toFixed(1) + 'M' : n >= 1e3 ? (n / 1e3).toFixed(1) + 'K' : String(n))
  const flagOf = { liked: 'liked', bookmarked: 'bookmarked', used: 'used' }

  function counts() {
    return {
      all: skills.length,
      liked: skills.filter((s) => s.liked).length,
      bookmarked: skills.filter((s) => s.bookmarked).length,
      used: skills.filter((s) => s.used).length,
    }
  }
  function view() {
    const q = state.q.trim().toLowerCase()
    return skills
      .filter((s) => state.tab === 'all' || s[flagOf[state.tab]])
      .filter((s) => !q || (s.name + ' ' + s.source + ' ' + s.description + ' ' + s.notes).toLowerCase().includes(q))
      .sort((a, b) =>
        state.sort === 'name' ? a.name.localeCompare(b.name) : state.sort === 'installs' ? b.installs - a.installs : b.updatedAt - a.updatedAt,
      )
  }
  const emit = () => listeners.forEach((fn) => fn())

  // Re-rendering replaces the DOM, so put keyboard focus back on the control that had it (matched by data-fk).
  function keepFocus(render) {
    const fk = document.activeElement && document.activeElement.dataset ? document.activeElement.dataset.fk : null
    const caret = fk && document.activeElement.selectionStart != null ? document.activeElement.selectionStart : null
    render()
    if (!fk) return
    const el = document.querySelector('[data-fk="' + CSS.escape(fk) + '"]')
    if (el) {
      el.focus()
      if (caret != null && el.setSelectionRange) try { el.setSelectionRange(caret, caret) } catch (e) { /* not a text input */ }
    }
  }

  window.Shelf = {
    skills, state, esc, fmt, counts, view, keepFocus,
    onChange: (fn) => listeners.push(fn),
    set(patch) { Object.assign(state, patch); emit() },
    toggle(id, key) {
      const s = skills.find((x) => x.id === id)
      if (s) { s[key] = !s[key]; s.updatedAt = Math.floor(Date.now() / 1000); emit() }
    },
    setNotes(id, text) {
      const s = skills.find((x) => x.id === id)
      if (s) { s.notes = text; emit() }
    },
    add(text) {
      const slug = text.trim().replace(/^https?:\/\/(www\.)?skills\.sh\//, '').replace(/\/$/, '')
      if (!slug) return null
      const name = slug.split('/').pop()
      const s = { id: slug, source: slug.split('/').slice(0, 2).join('/'), slug: name, sourceType: 'github', url: 'https://www.skills.sh/' + slug, name, description: '', notes: '', installs: 0, liked: false, bookmarked: false, used: false, updatedAt: Math.floor(Date.now() / 1000) }
      skills.unshift(s)
      emit()
      return s
    },
  }
})()

/* Presentation enhancements only. Reading, links and the complete tour work without JS. */
(() => {
  const drawer = document.getElementById("__drawer");
  const menu = document.querySelector("[data-docs-menu]");
  const searchToggle = document.getElementById("__search");
  const query = document.querySelector('[data-md-component="search-query"]');
  let searchOrigin;
  // Material 9.8 observes keyup. Also handle paste, speech and touch input.
  query?.addEventListener("input", () =>
    query.dispatchEvent(new KeyboardEvent("keyup", { bubbles: true })),
  );
  const setToggle = (toggle, checked) => {
    if (!toggle) return;
    toggle.checked = checked;
    toggle.dispatchEvent(new Event("change", { bubbles: true }));
  };
  document.querySelectorAll("[data-docs-search]").forEach((button) => {
    button.addEventListener("click", () => {
      searchOrigin = button;
      setToggle(searchToggle, true);
      query?.focus();
    });
  });
  document.querySelectorAll("[data-search-shortcut]").forEach((label) => {
    label.textContent = /Mac|iPhone|iPad/.test(navigator.platform)
      ? "⌘ K"
      : "Ctrl K";
  });
  menu?.addEventListener("click", (event) => {
    event.preventDefault();
    setToggle(drawer, !drawer.checked);
  });
  menu?.addEventListener("keydown", (event) => {
    if (event.key === "Enter" || event.key === " ") {
      event.preventDefault();
      menu.click();
    }
  });
  drawer?.addEventListener("change", () =>
    menu?.setAttribute("aria-expanded", String(drawer.checked)),
  );
  document.addEventListener(
    "keydown",
    (event) => {
      if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === "k") {
        event.preventDefault();
        searchOrigin = document.activeElement;
        setToggle(searchToggle, !searchToggle.checked);
        if (searchToggle.checked) query?.focus();
      }
      if (event.key === "Escape" && searchToggle?.checked) {
        event.preventDefault();
        setToggle(searchToggle, false);
        searchOrigin?.focus();
      } else if (event.key === "Escape" && drawer?.checked) {
        setToggle(drawer, false);
        menu?.focus();
      }
    },
    true,
  );
  const theme = document.querySelector("[data-theme-toggle]");
  const updateThemeLabel = () =>
    theme?.setAttribute(
      "aria-label",
      `Switch to ${document.body.dataset.mdColorScheme === "slate" ? "light" : "dark"} theme`,
    );
  updateThemeLabel();
  new MutationObserver(updateThemeLabel).observe(document.body, {
    attributes: true,
    attributeFilter: ["data-md-color-scheme"],
  });
  theme?.addEventListener("click", () => {
    const nextScheme =
      document.body.dataset.mdColorScheme === "slate" ? "default" : "slate";
    document
      .querySelector(`input[data-md-color-scheme="${nextScheme}"]`)
      ?.click();
  });

  const tabs = [...document.querySelectorAll("[data-tour]")];
  if (tabs.length) {
    document.querySelector(".tour-tabs").setAttribute("role", "tablist");
    const select = (tab, focus = false) => {
      tabs.forEach((item) => {
        const active = item === tab;
        item.setAttribute("aria-selected", String(active));
        item.tabIndex = active ? 0 : -1;
        document.getElementById(`tour-${item.dataset.tour}`).hidden = !active;
      });
      if (focus) tab.focus();
    };
    tabs.forEach((tab, index) => {
      tab.id = `tab-${tab.dataset.tour}`;
      tab.setAttribute("role", "tab");
      tab.setAttribute("aria-controls", `tour-${tab.dataset.tour}`);
      const panel = document.getElementById(`tour-${tab.dataset.tour}`);
      panel.setAttribute("role", "tabpanel");
      panel.setAttribute("aria-labelledby", tab.id);
      tab.addEventListener("click", (event) => {
        event.preventDefault();
        select(tab);
      });
      tab.addEventListener("keydown", (event) => {
        const next = {
          ArrowRight: (index + 1) % tabs.length,
          ArrowLeft: (index + tabs.length - 1) % tabs.length,
          Home: 0,
          End: tabs.length - 1,
        }[event.key];
        if (next !== undefined) {
          event.preventDefault();
          select(tabs[next], true);
        }
      });
    });
    const selectHash = () => {
      const tab = tabs.find((item) => item.hash === location.hash);
      if (tab) select(tab);
    };
    select(tabs.find((tab) => tab.hash === location.hash) || tabs[0]);
    window.addEventListener("hashchange", selectHash);
  }

  const filters = [...document.querySelectorAll("[data-filter]")];
  const applyFilter = (category) => {
    let count = 0;
    filters.forEach((button) => {
      const active = button.dataset.filter === category;
      button.classList.toggle("active", active);
      button.setAttribute("aria-pressed", String(active));
    });
    document.querySelectorAll("[data-category]").forEach((card) => {
      card.hidden = category !== "all" && card.dataset.category !== category;
      if (!card.hidden) count++;
    });
    const status = document.getElementById("filter-count");
    if (status)
      status.textContent = `${count} ${count === 1 ? "guide" : "guides"}`;
  };
  filters.forEach((button) =>
    button.addEventListener("click", () => {
      applyFilter(button.dataset.filter);
      const url = new URL(location.href);
      if (button.dataset.filter === "all") url.searchParams.delete("topic");
      else url.searchParams.set("topic", button.dataset.filter);
      history.replaceState(null, "", url);
    }),
  );
  if (filters.length) {
    document.querySelector(".filter-bar").hidden = false;
    const topic = new URL(location.href).searchParams.get("topic");
    applyFilter(
      filters.some((button) => button.dataset.filter === topic) ? topic : "all",
    );
  }

  let toastTimer;
  const toast = (message) => {
    let node = document.querySelector(".toast");
    if (!node) {
      node = document.createElement("div");
      node.className = "toast";
      node.setAttribute("role", "status");
      document.body.append(node);
    }
    node.textContent = message;
    clearTimeout(toastTimer);
    toastTimer = setTimeout(() => node.remove(), 2600);
  };
  document.querySelectorAll("[data-copy]").forEach((button) =>
    button.addEventListener("click", async () => {
      try {
        await navigator.clipboard.writeText(button.dataset.copy);
        toast("Copied. Ready for your terminal.");
      } catch {
        toast("Clipboard unavailable. Select and copy the command.");
      }
    }),
  );
})();

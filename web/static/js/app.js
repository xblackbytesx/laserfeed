// Highlight the active sidebar nav link based on the current URL.
// Runs on initial load and after every HTMX navigation.
(function () {
  function updateActiveNav() {
    var path = location.pathname;
    document.querySelectorAll('.sidebar-nav .nav-item').forEach(function (link) {
      var href = link.getAttribute('href');
      var active = href === '/' ? path === '/' : path.startsWith(href);
      link.classList.toggle('active', active);
    });
  }
  document.addEventListener('htmx:afterSettle', updateActiveNav);
  updateActiveNav();
})();

// Auto-populate slug from channel name
(function () {
  function toSlug(s) {
    return s.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '');
  }

  // Shoelace fires 'sl-input' on its own elements
  document.addEventListener('sl-input', (e) => {
    const nameInput = document.getElementById('channel-name-input');
    const slugInput = document.getElementById('channel-slug-input');
    if (e.target === nameInput && slugInput) {
      // Only auto-fill if the user hasn't manually edited the slug
      if (!slugInput.dataset.userEdited) {
        slugInput.value = toSlug(nameInput.value);
      }
    }
    if (e.target === slugInput) {
      slugInput.dataset.userEdited = '1';
    }
  });
})();

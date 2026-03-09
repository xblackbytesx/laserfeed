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

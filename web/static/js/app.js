// Set the X-CSRF-Token header on every HTMX request from the meta tag.
// Replaces the previous hx-headers JSON-concat in base.templ — that pattern
// would silently break if a token ever contained a quote or backslash.
document.addEventListener('htmx:configRequest', function (e) {
  var meta = document.querySelector('meta[name="csrf-token"]');
  if (meta) {
    e.detail.headers['X-CSRF-Token'] = meta.getAttribute('content');
  }
});

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

// Auto-populate slug from channel name.
(function () {
  function toSlug(s) {
    return s.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '');
  }
  document.addEventListener('sl-input', function (e) {
    var nameInput = document.getElementById('channel-name-input');
    var slugInput = document.getElementById('channel-slug-input');
    if (e.target === nameInput && slugInput) {
      if (!slugInput.dataset.userEdited) {
        slugInput.value = toSlug(nameInput.value);
      }
    }
    if (e.target === slugInput) {
      slugInput.dataset.userEdited = '1';
    }
  });
})();

// Delegated click handler for declarative buttons:
//   data-dialog-open="dialog-id"  → call sl-dialog.show()
//   data-dialog-close="dialog-id" → call sl-dialog.hide()
//   data-stop-propagation         → e.stopPropagation()
// Delegation on document survives HTMX swaps without re-binding.
document.addEventListener('click', function (e) {
  var target = e.target instanceof Element ? e.target : null;
  if (!target) return;

  var open = target.closest('[data-dialog-open]');
  if (open) {
    var openDialog = document.getElementById(open.getAttribute('data-dialog-open'));
    if (openDialog && typeof openDialog.show === 'function') {
      openDialog.show();
    }
    return;
  }

  var close = target.closest('[data-dialog-close]');
  if (close) {
    var closeDialog = document.getElementById(close.getAttribute('data-dialog-close'));
    if (closeDialog && typeof closeDialog.hide === 'function') {
      closeDialog.hide();
    }
    return;
  }

  if (target.closest('[data-stop-propagation]')) {
    e.stopPropagation();
  }
});

// Native confirm() before destructive form submissions, declared via
// `<form data-confirm="message">`. Cancel aborts the submit.
document.addEventListener('submit', function (e) {
  var form = e.target;
  if (form && form.getAttribute) {
    var msg = form.getAttribute('data-confirm');
    if (msg && !window.confirm(msg)) {
      e.preventDefault();
    }
  }
});

// `<form data-reset-on-success>` clears its inputs after a successful HTMX POST.
// Replaces hx-on::after-request="this.reset()" so we don't need
// 'unsafe-eval' in our CSP.
document.addEventListener('htmx:afterRequest', function (e) {
  var elt = e.detail && e.detail.elt;
  if (!elt || !elt.matches || !elt.matches('form[data-reset-on-success]')) return;
  var xhr = e.detail.xhr;
  if (xhr && xhr.status >= 200 && xhr.status < 300 && typeof elt.reset === 'function') {
    elt.reset();
  }
});

// Settings page: toggle visibility of the URL field / built-in picker based on
// the selected image mode. No-op on every other page (guarded on element id).
(function () {
  function syncImageModeSections() {
    var sel = document.getElementById('image-mode-select');
    if (!sel) return;
    var custom = document.getElementById('custom-placeholder-section');
    var builtin = document.getElementById('builtin-placeholder-section');
    if (custom) custom.style.display = sel.value === 'placeholder' ? '' : 'none';
    if (builtin) builtin.style.display = sel.value === 'builtin' ? '' : 'none';
    if (!sel.dataset.bound) {
      sel.addEventListener('sl-change', syncImageModeSections);
      sel.dataset.bound = '1';
    }
  }
  document.addEventListener('htmx:afterSettle', syncImageModeSections);
  syncImageModeSections();
})();

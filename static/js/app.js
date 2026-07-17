/* ── app.js — Task Manager Frontend ─────────────────── */

// ── Helpers ────────────────────────────────────────────
const $ = (sel, ctx = document) => ctx.querySelector(sel);
const $$ = (sel, ctx = document) => [...ctx.querySelectorAll(sel)];

function showAlert(message, type = 'info', container = null) {
  const icons = { success: '✓', error: '✕', info: 'ℹ' };
  const el = document.createElement('div');
  el.className = `alert alert-${type}`;
  el.innerHTML = `<span>${icons[type] || 'ℹ'}</span><span>${message}</span>`;
  const target = container || $('#alert-zone') || document.body;
  target.prepend(el);
  setTimeout(() => el.remove(), 4000);
}

// ── Active nav link ─────────────────────────────────────
function setActiveNav() {
  const path = window.location.pathname;
  $$('.nav-link, .sidebar-link').forEach(link => {
    const href = link.getAttribute('href');
    if (href && href !== '#' && path.startsWith(href)) {
      link.classList.add('active');
    }
  });
}

// ── Tab switching ───────────────────────────────────────
function initTabs() {
  $$('.tabs').forEach(tabGroup => {
    const tabs = $$('.tab', tabGroup);
    tabs.forEach(tab => {
      tab.addEventListener('click', () => {
        const target = tab.dataset.tab;
        // deactivate all
        tabs.forEach(t => t.classList.remove('active'));
        $$('.tab-panel').forEach(p => p.classList.remove('active'));
        // activate
        tab.classList.add('active');
        const panel = $(`#${target}`);
        if (panel) panel.classList.add('active');
      });
    });
  });
}

// ── Modals ──────────────────────────────────────────────
function openModal(id) {
  const overlay = $(`#${id}`);
  if (overlay) overlay.classList.add('open');
}
function closeModal(id) {
  const overlay = $(`#${id}`);
  if (overlay) overlay.classList.remove('open');
}
function initModals() {
  // open triggers
  $$('[data-modal-open]').forEach(btn => {
    btn.addEventListener('click', () => openModal(btn.dataset.modalOpen));
  });
  // close triggers
  $$('[data-modal-close], .modal-close').forEach(btn => {
    btn.addEventListener('click', () => {
      const overlay = btn.closest('.modal-overlay');
      if (overlay) overlay.classList.remove('open');
    });
  });
  // click outside
  $$('.modal-overlay').forEach(overlay => {
    overlay.addEventListener('click', e => {
      if (e.target === overlay) overlay.classList.remove('open');
    });
  });
}

// ── Toggle switches ─────────────────────────────────────
function initToggles() {
  $$('.toggle').forEach(toggle => {
    toggle.addEventListener('click', () => {
      toggle.classList.toggle('on');
      const input = toggle.previousElementSibling;
      if (input && input.type === 'checkbox') {
        input.checked = toggle.classList.contains('on');
      }
    });
  });
}

// ── Task checkboxes ─────────────────────────────────────
function initTaskCheckboxes() {
  $$('.task-checkbox').forEach(cb => {
    cb.addEventListener('click', e => {
      e.stopPropagation();
      cb.classList.toggle('checked');
      const item = cb.closest('.task-item');
      if (item) item.classList.toggle('done');
    });
  });
}

// ── View toggle (list / kanban) ─────────────────────────
function initViewToggle() {
  const listView = $('#view-list');
  const kanbanView = $('#view-kanban');
  const listBtn = $('#btn-list-view');
  const kanbanBtn = $('#btn-kanban-view');
  if (!listView || !kanbanView) return;

  listBtn?.addEventListener('click', () => {
    listView.style.display = '';
    kanbanView.style.display = 'none';
    listBtn.classList.add('active');
    kanbanBtn?.classList.remove('active');
  });
  kanbanBtn?.addEventListener('click', () => {
    listView.style.display = 'none';
    kanbanView.style.display = '';
    kanbanBtn.classList.add('active');
    listBtn?.classList.remove('active');
  });
}

// ── Task filter pills ───────────────────────────────────
function initTaskFilters() {
  $$('.filter-btn[data-filter]').forEach(btn => {
    btn.addEventListener('click', () => {
      $$('.filter-btn[data-filter]').forEach(b => b.classList.remove('active'));
      btn.classList.add('active');
      const filter = btn.dataset.filter;
      $$('.task-item').forEach(item => {
        if (filter === 'all') {
          item.style.display = '';
        } else {
          const status = item.dataset.status || '';
          item.style.display = status === filter ? '' : 'none';
        }
      });
    });
  });
}

// ── Search filter ───────────────────────────────────────
function initSearch() {
  const searchInput = $('#task-search');
  if (!searchInput) return;
  searchInput.addEventListener('input', () => {
    const q = searchInput.value.toLowerCase();
    $$('.task-item').forEach(item => {
      const title = item.querySelector('.task-title')?.textContent.toLowerCase() || '';
      item.style.display = title.includes(q) ? '' : 'none';
    });
  });
}

// ── File Upload drag & drop ─────────────────────────────
function initFileUpload() {
  $$('.upload-zone').forEach(zone => {
    const input = zone.querySelector('input[type=file]');
    const fileList = zone.parentElement?.querySelector('.file-list');

    zone.addEventListener('dragover', e => { e.preventDefault(); zone.classList.add('dragover'); });
    zone.addEventListener('dragleave', () => zone.classList.remove('dragover'));
    zone.addEventListener('drop', e => {
      e.preventDefault();
      zone.classList.remove('dragover');
      if (input) {
        // simulate file selection
        handleFiles(e.dataTransfer.files, fileList);
      }
    });

    input?.addEventListener('change', () => handleFiles(input.files, fileList));
  });
}

function handleFiles(files, listEl) {
  if (!listEl) return;
  [...files].forEach(file => {
    const icons = { 'image/': '🖼️', 'application/pdf': '📄', 'text/': '📝', 'video/': '🎬' };
    let icon = '📎';
    for (const [prefix, emoji] of Object.entries(icons)) {
      if (file.type.startsWith(prefix)) { icon = emoji; break; }
    }
    const size = file.size > 1048576
      ? `${(file.size / 1048576).toFixed(1)} MB`
      : `${(file.size / 1024).toFixed(0)} KB`;

    const item = document.createElement('div');
    item.className = 'file-item';
    item.innerHTML = `
      <span class="file-icon">${icon}</span>
      <span class="file-name">${file.name}</span>
      <span class="file-size">${size}</span>
      <button class="file-remove" title="Remove">✕</button>
      <div class="progress-bar" style="grid-column:1/-1"><div class="progress-fill" style="width:0%"></div></div>
    `;
    listEl.appendChild(item);
    item.querySelector('.file-remove').addEventListener('click', () => item.remove());

    // Simulate upload progress (frontend demo only)
    const fill = item.querySelector('.progress-fill');
    let pct = 0;
    const iv = setInterval(() => {
      pct += Math.random() * 15;
      if (pct >= 100) { pct = 100; clearInterval(iv); }
      fill.style.width = `${pct}%`;
    }, 120);
  });
}

// ── Auth form validation ────────────────────────────────
function initAuthForms() {
  const loginForm = $('#login-form');
  const registerForm = $('#register-form');

  if (loginForm) {
    loginForm.addEventListener('submit', e => {
      e.preventDefault(); // remove this line in real app — let Go handle POST
      const email = loginForm.querySelector('[name=email]')?.value;
      if (!email) {
        showAlert('Email is required.', 'error', loginForm);
        return;
      }
      // In real app: form submits POST /login → Go handles it
      showAlert('Form is ready to POST to /login', 'info', loginForm);
    });
  }

  if (registerForm) {
    registerForm.addEventListener('submit', e => {
      e.preventDefault();
      const pw = registerForm.querySelector('[name=password]')?.value || '';
      const pw2 = registerForm.querySelector('[name=password_confirm]')?.value || '';
      if (pw !== pw2) {
        showAlert('Passwords do not match.', 'error', registerForm);
        return;
      }
      showAlert('Form is ready to POST to /register', 'info', registerForm);
    });
  }
}

// ── Task form ───────────────────────────────────────────
function initTaskForm() {
  const form = $('#task-form');
  if (!form) return;
  form.addEventListener('submit', e => {
    e.preventDefault();
    const title = form.querySelector('[name=title]')?.value.trim();
    if (!title) {
      showAlert('Task title is required.', 'error');
      return;
    }
    // In real app: POST /tasks → Go inserts into DB
    showAlert('Ready to POST to /tasks', 'info');
    closeModal('task-modal');
  });
}

// ── Character counter ───────────────────────────────────
function initCharCounters() {
  $$('[data-maxlength]').forEach(el => {
    const max = parseInt(el.dataset.maxlength);
    const counter = document.createElement('span');
    counter.className = 'form-hint';
    counter.textContent = `0 / ${max}`;
    el.parentElement.appendChild(counter);
    el.addEventListener('input', () => {
      const len = el.value.length;
      counter.textContent = `${len} / ${max}`;
      counter.style.color = len > max * .9 ? 'var(--accent2)' : '';
    });
  });
}

// ── Init ────────────────────────────────────────────────
document.addEventListener('DOMContentLoaded', () => {
  setActiveNav();
  initTabs();
  initModals();
  initToggles();
  initTaskCheckboxes();
  initViewToggle();
  initTaskFilters();
  initSearch();
  initFileUpload();
  initAuthForms();
  initTaskForm();
  initCharCounters();
});

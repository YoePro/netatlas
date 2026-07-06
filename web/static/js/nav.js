// nav.js — Navigation setup
(function() {
  const NAV_ITEMS = [
    {href: '/html/index.html',   id: 'dashboard', label: 'Dashboard',    icon: 'grid'},
    {href: '/html/map.html',     id: 'map',       label: 'Network Map',  icon: 'share2'},
    {href: '/html/sensors.html', id: 'sensors',   label: 'Sensors',      icon: 'radio'},
    {type: 'divider'},
    {href: '#alerts',    id: 'alerts',   label: 'Alerts',       icon: 'bell',    badge: '3'},
    {href: '#search',    id: 'search',   label: 'Search',       icon: 'search'},
    {href: '#timeline',  id: 'timeline', label: 'Timeline',     icon: 'clock'},
    {href: '#inventory', id: 'inventory',label: 'Inventory',    icon: 'layers'},
    {href: '#domains',   id: 'domains',  label: 'Domains',      icon: 'globe'},
    {type: 'divider'},
    {href: '#reports',   id: 'reports',  label: 'Reports',      icon: 'bar-chart'},
    {href: '#settings',  id: 'settings', label: 'Settings',     icon: 'settings'},
  ];

  const ICONS = {
    'grid':      '<polyline points="3 3 10 3 10 10 3 10 3 3"/><polyline points="14 3 21 3 21 10 14 10 14 3"/><polyline points="14 14 21 14 21 21 14 21 14 14"/><polyline points="3 14 10 14 10 21 3 21 3 14"/>',
    'share2':    '<circle cx="18" cy="5" r="3"/><circle cx="6" cy="12" r="3"/><circle cx="18" cy="19" r="3"/><line x1="8.59" y1="13.51" x2="15.42" y2="17.49"/><line x1="15.41" y1="6.51" x2="8.59" y2="10.49"/>',
    'radio':     '<circle cx="12" cy="12" r="2"/><path d="M16.24 7.76a6 6 0 0 1 0 8.49m-8.48-.01a6 6 0 0 1 0-8.49m11.31-2.82a10 10 0 0 1 0 14.14m-14.14 0a10 10 0 0 1 0-14.14"/>',
    'bell':      '<path d="M18 8A6 6 0 0 0 6 8c0 7-3 9-3 9h18s-3-2-3-9"/><path d="M13.73 21a2 2 0 0 1-3.46 0"/>',
    'search':    '<circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/>',
    'clock':     '<circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/>',
    'layers':    '<polygon points="12 2 2 7 12 12 22 7 12 2"/><polyline points="2 17 12 22 22 17"/><polyline points="2 12 12 17 22 12"/>',
    'globe':     '<circle cx="12" cy="12" r="10"/><line x1="2" y1="12" x2="22" y2="12"/><path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"/>',
    'bar-chart': '<line x1="18" y1="20" x2="18" y2="10"/><line x1="12" y1="20" x2="12" y2="4"/><line x1="6" y1="20" x2="6" y2="14"/>',
    'settings':  '<circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/>',
    'log-out':   '<path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/><polyline points="16 17 21 12 16 7"/><line x1="21" y1="12" x2="9" y2="12"/>',
  };

  function icon(name) {
    return `<svg class="nav-item-icon" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">${ICONS[name] || ''}</svg>`;
  }

  function buildNav(activeId) {
    const navEl = document.getElementById('main-nav');
    if (!navEl) return;

    let html = `
      <a class="nav-logo" href="/html/index.html">
        <svg class="nav-logo-mark" width="28" height="28" viewBox="0 0 28 28" fill="none">
          <rect width="28" height="28" rx="6" fill="#2563EB"/>
          <circle cx="14" cy="14" r="3" fill="white"/>
          <circle cx="6"  cy="8"  r="2" fill="white" opacity=".7"/>
          <circle cx="22" cy="8"  r="2" fill="white" opacity=".7"/>
          <circle cx="6"  cy="20" r="2" fill="white" opacity=".7"/>
          <circle cx="22" cy="20" r="2" fill="white" opacity=".7"/>
          <line x1="14" y1="14" x2="6"  y2="8"  stroke="white" stroke-width="1" opacity=".45"/>
          <line x1="14" y1="14" x2="22" y2="8"  stroke="white" stroke-width="1" opacity=".45"/>
          <line x1="14" y1="14" x2="6"  y2="20" stroke="white" stroke-width="1" opacity=".45"/>
          <line x1="14" y1="14" x2="22" y2="20" stroke="white" stroke-width="1" opacity=".45"/>
        </svg>
        <div>
          <div class="nav-logo-text">NetAtlas</div>
          <div class="nav-logo-sub">Intelligence</div>
        </div>
      </a>
      <div class="nav-items">`;

    NAV_ITEMS.forEach(item => {
      if (item.type === 'divider') {
        html += `<div class="nav-divider"></div>`;
        return;
      }
      const active = item.id === activeId ? ' active' : '';
      const badge  = item.badge ? `<span class="nav-item-badge">${item.badge}</span>` : '';
      html += `<a class="nav-item${active}" href="${item.href}">${icon(item.icon)}<span>${item.label}</span>${badge}</a>`;
    });

    html += `</div>
      <div class="nav-footer">
        <a class="nav-user" href="#profile">
          <div class="nav-user-avatar">AD</div>
          <div>
            <div class="nav-user-name">Admin</div>
            <div class="nav-user-role">Operator</div>
          </div>
        </a>
      </div>`;

    navEl.innerHTML = html;
  }

  // Theme toggle in topbar
  function bindThemeToggle() {
    const btn = document.getElementById('theme-toggle-btn');
    if (!btn) return;
    const sunSvg  = btn.querySelector('.icon-sun');
    const moonSvg = btn.querySelector('.icon-moon');
    function sync(theme) {
      if (sunSvg)  sunSvg.style.display  = theme === 'dark' ? 'block' : 'none';
      if (moonSvg) moonSvg.style.display = theme === 'dark' ? 'none' : 'block';
    }
    sync(NATheme.getTheme());
    btn.addEventListener('click', () => {
      NATheme.toggle();
      sync(NATheme.getTheme());
    });
  }

  // Status bar clock
  function startClock() {
    const el = document.getElementById('statusbar-time');
    if (!el) return;
    function tick() {
      el.textContent = new Date().toLocaleTimeString('en-GB', {hour12: false});
    }
    tick();
    setInterval(tick, 1000);
  }

  window.NANav = {buildNav, bindThemeToggle, startClock, ICONS, icon};
})();

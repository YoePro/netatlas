// sensors.js
(function() {
  NANav.buildNav('sensors');
  NANav.bindThemeToggle();
  NANav.startClock();

  let allSensors = [];
  let sortCol = 'name';
  let sortDir = 'asc';
  let viewMode = 'table';

  function fmt(n) {
    if (n >= 1e6) return (n/1e6).toFixed(1) + 'M';
    if (n >= 1e3) return (n/1e3).toFixed(1) + 'K';
    return String(n);
  }

  function statusBadge(s) {
    const cl = s === 'online' ? 'green' : s === 'warning' ? 'amber' : 'red';
    return `<span class="badge badge-${cl}"><span class="dot"></span>${s}</span>`;
  }

  function uptimeBar(pct) {
    const cl = pct >= 99 ? 'green' : pct >= 95 ? 'amber' : 'red';
    return `<div style="display:flex;align-items:center;gap:6px;">
      <div class="progress-bar" style="width:60px"><div class="progress-fill ${cl}" style="width:${pct}%"></div></div>
      <span class="t-xs text-muted">${pct}%</span>
    </div>`;
  }

  function renderTable(sensors) {
    const tbody = document.getElementById('sensors-tbody');
    tbody.innerHTML = sensors.map(s => {
      const latency = s.latency != null ? `${s.latency} ms` : '—';
      const sources = s.sources.map(src => `<span class="tag">${src}</span>`).join(' ');
      const detailUrl = `/html/sensor.html?id=${encodeURIComponent(s.id)}`;
      return `<tr class="clickable" onclick="window.location='${detailUrl}'">
        <td>
          <div style="font-weight:500;display:flex;align-items:center;gap:7px;">
            <div class="status-dot ${s.status === 'online' ? 'online' : s.status === 'warning' ? 'warning' : 'offline'}"></div>
            ${s.name}
          </div>
        </td>
        <td class="muted">${s.location}</td>
        <td>${statusBadge(s.status)}</td>
        <td class="mono muted">${s.version}</td>
        <td class="muted">${s.lastSeen}</td>
        <td style="font-variant-numeric:tabular-nums">${fmt(s.events)}</td>
        <td class="muted">${latency}</td>
        <td>${sources}</td>
        <td>${uptimeBar(s.uptime)}</td>
      </tr>`;
    }).join('') || '<tr><td colspan="9"><div class="empty-state"><div class="empty-state-title">No sensors found</div></div></td></tr>';
    document.getElementById('sensor-count').textContent = `${sensors.length} sensor${sensors.length !== 1 ? 's' : ''}`;
  }

  function renderCards(sensors) {
    const grid = document.getElementById('card-view');
    grid.innerHTML = sensors.map(s => {
      const statusClass = s.status === 'online' ? 'green' : s.status === 'warning' ? 'amber' : 'red';
      const detailUrl = `/html/sensor.html?id=${encodeURIComponent(s.id)}`;
      return `<div class="card" style="cursor:pointer" onclick="window.location='${detailUrl}'">
        <div class="card-header">
          <div class="card-title" style="gap:8px;">
            <div class="status-dot ${s.status === 'online' ? 'online' : s.status === 'warning' ? 'warning' : 'offline'}" style="margin-right:2px;"></div>
            ${s.name}
          </div>
          ${statusBadge(s.status)}
        </div>
        <div class="card-body" style="display:flex;flex-direction:column;gap:10px;">
          <div class="flex items-center gap-6">
            <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="var(--text-muted)" stroke-width="2">
              <path d="M21 10c0 7-9 13-9 13s-9-6-9-13a9 9 0 0 1 18 0z"/>
              <circle cx="12" cy="10" r="3"/>
            </svg>
            <span class="t-sm text-secondary">${s.location}</span>
          </div>
          <div class="flex items-center gap-6">
            <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="var(--text-muted)" stroke-width="2">
              <circle cx="12" cy="12" r="10"/>
              <polyline points="12 6 12 12 16 14"/>
            </svg>
            <span class="t-sm text-secondary">${s.lastSeen}</span>
          </div>
          <div class="divider" style="margin:0"></div>
          <div style="display:grid;grid-template-columns:1fr 1fr;gap:12px;">
            <div>
              <div class="t-xs text-muted" style="margin-bottom:3px">Events</div>
              <div class="fw-600">${fmt(s.events)}</div>
            </div>
            <div>
              <div class="t-xs text-muted" style="margin-bottom:3px">Latency</div>
              <div class="fw-600">${s.latency != null ? s.latency + ' ms' : '—'}</div>
            </div>
            <div>
              <div class="t-xs text-muted" style="margin-bottom:3px">Version</div>
              <div class="mono t-sm">${s.version}</div>
            </div>
            <div>
              <div class="t-xs text-muted" style="margin-bottom:3px">Sources</div>
              <div>${s.sources.map(src => `<span class="tag">${src}</span>`).join(' ')}</div>
            </div>
          </div>
          ${uptimeBar(s.uptime)}
        </div>
      </div>`;
    }).join('');
  }

  function sort(sensors) {
    return [...sensors].sort((a, b) => {
      let va = a[sortCol], vb = b[sortCol];
      if (typeof va === 'string') va = va.toLowerCase();
      if (typeof vb === 'string') vb = vb.toLowerCase();
      if (va < vb) return sortDir === 'asc' ? -1 : 1;
      if (va > vb) return sortDir === 'asc' ?  1 : -1;
      return 0;
    });
  }

  function filter(sensors, q) {
    if (!q) return sensors;
    const lq = q.toLowerCase();
    return sensors.filter(s =>
      s.name.toLowerCase().includes(lq) ||
      s.location.toLowerCase().includes(lq) ||
      s.status.toLowerCase().includes(lq) ||
      s.sources.some(src => src.toLowerCase().includes(lq))
    );
  }

  function renderAll() {
    const q = document.getElementById('sensor-filter').value;
    const data = sort(filter(allSensors, q));
    if (viewMode === 'table') renderTable(data);
    else renderCards(data);
  }

  function updateKPIs(sensors) {
    const online  = sensors.filter(s => s.status === 'online').length;
    const warning = sensors.filter(s => s.status === 'warning').length;
    const offline = sensors.filter(s => s.status === 'offline').length;
    const total   = sensors.reduce((s, x) => s + x.events, 0);

    document.getElementById('kpi-online').textContent  = online;
    document.getElementById('kpi-warning').textContent = warning;
    document.getElementById('kpi-offline').textContent = offline;
    document.getElementById('kpi-total-events').textContent = fmt(total);

    const online_ = sensors.filter(s => s.status === 'online').length;
    document.getElementById('sb-sensors').textContent =
      `${online_} of ${sensors.length} sensors online`;
  }

  // Sort headers
  document.querySelectorAll('th.sortable').forEach(th => {
    th.addEventListener('click', () => {
      const col = th.dataset.col;
      if (sortCol === col) {
        sortDir = sortDir === 'asc' ? 'desc' : 'asc';
      } else {
        sortCol = col; sortDir = 'asc';
      }
      document.querySelectorAll('th.sortable').forEach(t => {
        t.classList.remove('asc','desc');
      });
      th.classList.add(sortDir);
      renderAll();
    });
  });

  // Filter input
  document.getElementById('sensor-filter').addEventListener('input', renderAll);

  // View toggle
  document.getElementById('btn-table-view').addEventListener('click', () => {
    viewMode = 'table';
    document.getElementById('table-view').style.display = '';
    document.getElementById('card-view').style.display  = 'none';
    document.getElementById('btn-table-view').className = 'btn btn-secondary btn-sm';
    document.getElementById('btn-card-view').className  = 'btn btn-ghost btn-sm';
    renderAll();
  });
  document.getElementById('btn-card-view').addEventListener('click', () => {
    viewMode = 'card';
    document.getElementById('table-view').style.display = 'none';
    document.getElementById('card-view').style.display  = '';
    document.getElementById('btn-table-view').className = 'btn btn-ghost btn-sm';
    document.getElementById('btn-card-view').className  = 'btn btn-secondary btn-sm';
    renderAll();
  });

  async function load() {
    const sensors = await NAAPI.getSensors();
    allSensors = sensors;
    updateKPIs(sensors);
    renderAll();
  }

  load();
})();

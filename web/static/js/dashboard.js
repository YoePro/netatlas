// dashboard.js
(function() {
  NANav.buildNav('dashboard');
  NANav.bindThemeToggle();
  NANav.startClock();

  function fmt(n) {
    if (n >= 1e6) return (n/1e6).toFixed(1) + 'M';
    if (n >= 1e3) return (n/1e3).toFixed(1) + 'K';
    return String(n);
  }

  function deltaEl(el, value, suffix) {
    const abs = Math.abs(value).toFixed(1);
    if (value > 0) {
      el.className = 'stat-card-delta up';
      el.textContent = `↑ ${abs}${suffix || '%'} vs last hour`;
    } else if (value < 0) {
      el.className = 'stat-card-delta down';
      el.textContent = `↓ ${abs}${suffix || '%'} vs last hour`;
    } else {
      el.className = 'stat-card-delta same';
      el.textContent = 'No change vs last hour';
    }
  }

  // Draw the timeline chart on canvas
  function drawTimeline(data) {
    const canvas = document.getElementById('timeline-chart');
    if (!canvas) return;

    const dpr   = window.devicePixelRatio || 1;
    const rect  = canvas.parentElement.getBoundingClientRect();
    const W     = rect.width;
    const H     = 120;
    canvas.width  = W * dpr;
    canvas.height = H * dpr;
    canvas.style.width  = W + 'px';
    canvas.style.height = H + 'px';

    const ctx = canvas.getContext('2d');
    ctx.scale(dpr, dpr);

    const isDark   = document.documentElement.getAttribute('data-theme') === 'dark';
    const textCol  = isDark ? '#4F5D75' : '#9CA3AF';
    const lineCol  = isDark ? '#263045' : '#E2E8F0';
    const fillTop  = isDark ? 'rgba(59,130,246,.3)' : 'rgba(37,99,235,.15)';
    const fillBot  = isDark ? 'rgba(59,130,246,.0)' : 'rgba(37,99,235,.0)';
    const strokeC  = isDark ? '#3B82F6' : '#2563EB';
    const alertC   = isDark ? '#EF4444' : '#DC2626';

    const values = data.map(d => d.queries);
    const maxV   = Math.max(...values) * 1.1;
    const PAD_L  = 48, PAD_R = 16, PAD_T = 12, PAD_B = 28;
    const plotW  = W - PAD_L - PAD_R;
    const plotH  = H - PAD_T - PAD_B;

    function xOf(i)  { return PAD_L + (i / (data.length - 1)) * plotW; }
    function yOf(v)  { return PAD_T + plotH - (v / maxV) * plotH; }

    ctx.clearRect(0, 0, W, H);

    // Grid lines & y labels
    const yTicks = 4;
    for (let i = 0; i <= yTicks; i++) {
      const v  = (maxV * i) / yTicks;
      const y  = yOf(v);
      ctx.beginPath();
      ctx.moveTo(PAD_L, y); ctx.lineTo(W - PAD_R, y);
      ctx.strokeStyle = lineCol;
      ctx.lineWidth   = 1;
      ctx.stroke();
      ctx.fillStyle  = textCol;
      ctx.font       = `11px Inter, system-ui, sans-serif`;
      ctx.textAlign  = 'right';
      ctx.fillText(fmt(v), PAD_L - 5, y + 4);
    }

    // X labels (every 10 min)
    ctx.textAlign = 'center';
    for (let i = 0; i < data.length; i += 10) {
      const x = xOf(i);
      const d = new Date(data[i].t);
      ctx.fillStyle = textCol;
      ctx.fillText(d.getHours().toString().padStart(2,'0') + ':' + d.getMinutes().toString().padStart(2,'0'),
                   x, H - 6);
    }

    // Filled area
    const grad = ctx.createLinearGradient(0, PAD_T, 0, PAD_T + plotH);
    grad.addColorStop(0, fillTop);
    grad.addColorStop(1, fillBot);

    ctx.beginPath();
    ctx.moveTo(xOf(0), yOf(values[0]));
    for (let i = 1; i < data.length; i++) {
      ctx.lineTo(xOf(i), yOf(values[i]));
    }
    ctx.lineTo(xOf(data.length - 1), PAD_T + plotH);
    ctx.lineTo(xOf(0), PAD_T + plotH);
    ctx.closePath();
    ctx.fillStyle = grad;
    ctx.fill();

    // Line
    ctx.beginPath();
    ctx.moveTo(xOf(0), yOf(values[0]));
    for (let i = 1; i < data.length; i++) {
      ctx.lineTo(xOf(i), yOf(values[i]));
    }
    ctx.strokeStyle = strokeC;
    ctx.lineWidth   = 1.5;
    ctx.stroke();

    // Alert markers
    data.forEach((d, i) => {
      if (d.alerts > 0) {
        const x = xOf(i);
        const y = yOf(values[i]);
        ctx.beginPath();
        ctx.arc(x, y - 6, 3, 0, Math.PI * 2);
        ctx.fillStyle = alertC;
        ctx.fill();
      }
    });
  }

  function renderSensors(sensors) {
    const el = document.getElementById('sensor-status-list');
    el.innerHTML = sensors.map(s => {
      const statusClass = s.status === 'online' ? 'online' : s.status === 'warning' ? 'warning' : 'offline';
      const badgeClass  = s.status === 'online' ? 'badge-green' : s.status === 'warning' ? 'badge-amber' : 'badge-red';
      return `<div class="activity-item clickable" onclick="window.location='/html/sensor.html?id=${s.id}'">
        <div class="activity-dot-col">
          <div class="status-dot ${statusClass}"></div>
        </div>
        <div class="activity-body" style="display:flex;align-items:center;gap:8px;">
          <div style="flex:1;min-width:0;">
            <div class="activity-title">${s.name}</div>
            <div class="activity-meta"><span>${s.location}</span><span>${s.lastSeen}</span></div>
          </div>
          <span class="badge ${badgeClass}">${s.status}</span>
        </div>
      </div>`;
    }).join('');
  }

  function renderTopTalkers(talkers) {
    const tbody = document.getElementById('top-talkers-body');
    const max   = talkers[0] ? talkers[0].queries : 1;
    tbody.innerHTML = talkers.map(t => {
      const pct  = Math.round((t.queries / max) * 100);
      return `<tr class="clickable" onclick="window.location='#host=${t.host}'">
        <td>
          <div style="font-weight:500;">${t.name || t.host}</div>
          <div class="t-xs text-muted mono">${t.host}</div>
        </td>
        <td>
          <div style="font-variant-numeric:tabular-nums">${fmt(t.queries)}</div>
          <div class="progress-bar" style="margin-top:4px;width:80px">
            <div class="progress-fill blue" style="width:${pct}%"></div>
          </div>
        </td>
        <td class="muted">${t.bytes}</td>
      </tr>`;
    }).join('');
  }

  function renderNewDomains(domains) {
    const el = document.getElementById('new-domains-list');
    const catColors = {
      internal:'blue', cdn:'muted', telemetry:'amber',
      advertising:'red', security:'green', development:'purple'
    };
    el.innerHTML = domains.map((d, i) => {
      const col = catColors[d.category] || 'muted';
      const isLast = i === domains.length - 1;
      return `<div class="activity-item clickable" onclick="window.location='#domain=${d.domain}'">
        <div class="activity-dot-col">
          <div class="activity-dot" style="background:var(--${col === 'muted' ? 'text-muted' : col})"></div>
          ${isLast ? '' : '<div class="activity-line"></div>'}
        </div>
        <div class="activity-body" style="display:flex;align-items:center;gap:8px;">
          <div style="flex:1;min-width:0;">
            <div class="activity-title">${d.domain}</div>
            <div class="activity-meta"><span>${d.first_seen}</span><span>${d.clients} client${d.clients>1?'s':''}</span></div>
          </div>
          <span class="badge badge-${col}">${d.category}</span>
        </div>
      </div>`;
    }).join('');
  }

  function renderDiscoveries(domains) {
    const tbody = document.getElementById('discoveries-body');
    const catColors = {
      internal:'blue', cdn:'muted', telemetry:'amber',
      advertising:'red', security:'green', development:'purple'
    };
    tbody.innerHTML = domains.map(d => {
      const col = catColors[d.category] || 'muted';
      return `<tr class="clickable" onclick="window.location='#domain=${d.domain}'">
        <td><span style="font-weight:500">${d.domain}</span></td>
        <td><span class="tag">domain</span></td>
        <td class="muted">${d.first_seen}</td>
        <td>${d.clients}</td>
        <td><span class="badge badge-${col}">${d.category}</span></td>
        <td class="muted">DNS</td>
      </tr>`;
    }).join('');
  }

  async function load() {
    const [stats, sensors] = await Promise.all([
      NAAPI.getStats(),
      NAAPI.getSensors()
    ]);

    // KPIs
    document.getElementById('kpi-sensors').textContent = stats.activeSensors;
    document.getElementById('kpi-sensors-sub').textContent = `of ${stats.totalSensors} total`;

    document.getElementById('kpi-dns').textContent = fmt(stats.dnsRequests.value);
    deltaEl(document.getElementById('kpi-dns-delta'), stats.dnsRequests.delta);

    document.getElementById('kpi-devices').textContent = stats.newDevices.value;
    deltaEl(document.getElementById('kpi-devices-delta'), stats.newDevices.delta, '');

    document.getElementById('kpi-alerts').textContent = stats.alerts.value;
    const alertDeltaEl = document.getElementById('kpi-alerts-delta');
    alertDeltaEl.className = 'stat-card-delta ' + (stats.alerts.delta > 0 ? 'down' : 'up');
    alertDeltaEl.textContent = stats.alerts.delta > 0
      ? `↑ ${stats.alerts.delta} new this hour`
      : 'No new alerts';

    drawTimeline(stats.timeline);
    renderSensors(sensors.slice(0, 5));
    renderTopTalkers(stats.topTalkers);
    renderNewDomains(stats.recentDomains);
    renderDiscoveries(stats.recentDomains);
  }

  load();

  // Re-draw chart on theme change
  document.addEventListener('na:theme', async () => {
    const stats = await NAAPI.getStats();
    drawTimeline(stats.timeline);
  });

  // Re-draw on resize
  let resizeTimer;
  window.addEventListener('resize', () => {
    clearTimeout(resizeTimer);
    resizeTimer = setTimeout(async () => {
      const stats = await NAAPI.getStats();
      drawTimeline(stats.timeline);
    }, 200);
  });
})();

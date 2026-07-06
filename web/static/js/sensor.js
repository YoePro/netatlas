// sensor.js — Single sensor dashboard
(function() {
  NANav.buildNav('sensors');
  NANav.bindThemeToggle();
  NANav.startClock();

  const params = new URLSearchParams(window.location.search);
  const sensorId = params.get('id') || 's1';

  function fmt(n) {
    if (n == null) return '—';
    if (n >= 1e6) return (n/1e6).toFixed(1) + 'M';
    if (n >= 1e3) return (n/1e3).toFixed(1) + 'K';
    return String(n);
  }

  function drawTimeline(data) {
    const canvas = document.getElementById('sensor-timeline');
    if (!canvas) return;
    const dpr   = window.devicePixelRatio || 1;
    const W     = canvas.parentElement.clientWidth;
    const H     = 100;
    canvas.width  = W * dpr;
    canvas.height = H * dpr;
    canvas.style.width  = W + 'px';
    canvas.style.height = H + 'px';
    const ctx = canvas.getContext('2d');
    ctx.scale(dpr, dpr);

    const isDark   = document.documentElement.getAttribute('data-theme') === 'dark';
    const textCol  = isDark ? '#4F5D75' : '#9CA3AF';
    const lineCol  = isDark ? '#263045' : '#E2E8F0';
    const strokeC  = isDark ? '#F59E0B' : '#D97706';
    const fillTop  = isDark ? 'rgba(245,158,11,.25)' : 'rgba(217,119,6,.12)';
    const fillBot  = 'rgba(0,0,0,0)';
    const labelBg  = isDark ? 'rgba(13,17,23,.85)' : 'rgba(248,249,250,.9)';

    const values = data.map(d => d.queries);
    const maxV   = Math.max(...values) * 1.1 || 1;
    const PAD_L  = 44, PAD_R = 12, PAD_T = 8, PAD_B = 22;
    const plotW  = W - PAD_L - PAD_R;
    const plotH  = H - PAD_T - PAD_B;

    function xOf(i) { return PAD_L + (i / (data.length - 1)) * plotW; }
    function yOf(v) { return PAD_T + plotH - (v / maxV) * plotH; }

    ctx.clearRect(0, 0, W, H);

    // Grid
    for (let i = 0; i <= 3; i++) {
      const v = (maxV * i) / 3;
      const y = yOf(v);
      ctx.beginPath(); ctx.moveTo(PAD_L, y); ctx.lineTo(W - PAD_R, y);
      ctx.strokeStyle = lineCol; ctx.lineWidth = 1; ctx.stroke();
      ctx.fillStyle = textCol; ctx.font = '10px Inter,system-ui'; ctx.textAlign = 'right';
      ctx.fillText(fmt(v), PAD_L - 4, y + 3);
    }

    // X labels
    ctx.textAlign = 'center';
    for (let i = 0; i < data.length; i += 10) {
      const d = new Date(data[i].t);
      ctx.fillStyle = textCol;
      ctx.font = '10px Inter,system-ui';
      ctx.fillText(`${String(d.getHours()).padStart(2,'0')}:${String(d.getMinutes()).padStart(2,'0')}`,
                   xOf(i), H - 5);
    }

    // Fill
    const grad = ctx.createLinearGradient(0, PAD_T, 0, PAD_T + plotH);
    grad.addColorStop(0, fillTop);
    grad.addColorStop(1, fillBot);
    ctx.beginPath();
    ctx.moveTo(xOf(0), yOf(values[0]));
    for (let i = 1; i < data.length; i++) ctx.lineTo(xOf(i), yOf(values[i]));
    ctx.lineTo(xOf(data.length-1), PAD_T + plotH);
    ctx.lineTo(xOf(0), PAD_T + plotH);
    ctx.closePath(); ctx.fillStyle = grad; ctx.fill();

    // Line
    ctx.beginPath();
    ctx.moveTo(xOf(0), yOf(values[0]));
    for (let i = 1; i < data.length; i++) ctx.lineTo(xOf(i), yOf(values[i]));
    ctx.strokeStyle = strokeC; ctx.lineWidth = 1.5; ctx.stroke();
  }

  function healthColor(pct) {
    if (pct > 80) return 'red';
    if (pct > 60) return 'amber';
    return 'green';
  }

  function renderSensor(s) {
    document.getElementById('sensor-content').style.display = '';
    document.title = `NetAtlas — ${s.name}`;
    document.getElementById('breadcrumb-name').textContent = s.name;
    document.getElementById('sensor-name').textContent = s.name;
    document.getElementById('sensor-meta').textContent =
      `${s.location} · v${s.version} · ${s.sources.join(', ')}`;

    const statusClass = s.status === 'online' ? 'badge-green' : s.status === 'warning' ? 'badge-amber' : 'badge-red';
    document.getElementById('sensor-status-badge').className = `badge ${statusClass}`;
    document.getElementById('sensor-status-badge').textContent = s.status;

    // KPIs
    document.getElementById('s-events').textContent   = fmt(s.events);
    document.getElementById('s-events-all').textContent = fmt(s.events * 30);
    document.getElementById('s-latency').textContent  = s.latency != null ? `${s.latency} ms` : '—';
    document.getElementById('s-uptime').textContent   = `${s.uptime}%`;

    const uptimeCard = document.getElementById('s-uptime-card');
    uptimeCard.className = 'stat-card ' + (s.uptime >= 99 ? 'green' : s.uptime >= 95 ? 'amber' : 'red');

    document.getElementById('s-health').textContent  = s.status;
    document.getElementById('s-last-seen').textContent = `Last seen: ${s.lastSeen}`;
    const statusCard = document.getElementById('s-status-card');
    statusCard.className = 'stat-card ' + (s.status === 'online' ? 'green' : s.status === 'warning' ? 'amber' : 'red');

    // System health
    const cpuCol  = healthColor(s.cpu);
    const memCol  = healthColor(s.memory);
    const diskCol = healthColor(s.disk);
    document.getElementById('cpu-bar').style.width   = s.cpu + '%';
    document.getElementById('cpu-bar').className     = `progress-fill ${cpuCol}`;
    document.getElementById('cpu-val').textContent   = s.cpu + '%';
    document.getElementById('mem-bar').style.width   = s.memory + '%';
    document.getElementById('mem-bar').className     = `progress-fill ${memCol}`;
    document.getElementById('mem-val').textContent   = s.memory + '%';
    document.getElementById('disk-bar').style.width  = s.disk + '%';
    document.getElementById('disk-bar').className    = `progress-fill ${diskCol}`;
    document.getElementById('disk-val').textContent  = s.disk + '%';

    document.getElementById('sources-wrap').innerHTML =
      s.sources.map(src => `<span class="tag">${src}</span>`).join('');

    // Config
    const cfgRows = Object.entries(s.config || {}).map(([k, v]) =>
      `<tr style="border-bottom:1px solid var(--border)">
        <td style="padding:8px 0;color:var(--text-secondary);font-size:12px;width:130px">${k.replace(/_/g,' ')}</td>
        <td style="padding:8px 0;font-family:var(--font-mono);font-size:12px">${v}</td>
      </tr>`).join('');
    document.getElementById('config-tbody').innerHTML = cfgRows;

    // Errors
    const errColors = {warn:'amber', info:'blue', error:'red'};
    document.getElementById('errors-list').innerHTML =
      (s.recentErrors || []).map((e, i) => {
        const col = errColors[e.level] || 'muted';
        const isLast = i === s.recentErrors.length - 1;
        return `<div class="activity-item">
          <div class="activity-dot-col">
            <div class="activity-dot" style="background:var(--${col==='muted'?'text-muted':col})"></div>
            ${isLast?'':'<div class="activity-line"></div>'}
          </div>
          <div class="activity-body">
            <div class="activity-title">${e.msg}</div>
            <div class="activity-meta">
              <span class="badge badge-${col}" style="font-size:10px">${e.level}</span>
              <span class="mono">${e.time}</span>
            </div>
          </div>
        </div>`;
      }).join('');

    // Top domains table
    const maxQ = (s.topDomains && s.topDomains[0]) ? s.topDomains[0].queries : 1;
    document.getElementById('domains-tbody').innerHTML =
      (s.topDomains || []).map(d => {
        const pct = Math.round((d.queries / maxQ) * 100);
        return `<tr class="clickable">
          <td style="font-weight:500">${d.domain}</td>
          <td style="font-variant-numeric:tabular-nums">${d.queries.toLocaleString()}</td>
          <td><div class="progress-bar"><div class="progress-fill blue" style="width:${pct}%"></div></div></td>
          <td class="muted">${d.pct}%</td>
        </tr>`;
      }).join('');

    // Timeline
    drawTimeline(s.timeline);

    // Statusbar
    const dotEl = document.getElementById('sb-dot');
    dotEl.style.background = s.status === 'online' ? 'var(--green)' : s.status === 'warning' ? 'var(--amber)' : 'var(--red)';
    document.getElementById('sb-sensor-name').textContent = s.name;
    document.getElementById('sb-events-stat').textContent = `${fmt(s.events)} events`;
  }

  async function load() {
    const s = await NAAPI.getSensor(sensorId);
    renderSensor(s);
  }

  load();

  document.addEventListener('na:theme', load);

  let resizeTimer;
  window.addEventListener('resize', () => {
    clearTimeout(resizeTimer);
    resizeTimer = setTimeout(load, 200);
  });
})();

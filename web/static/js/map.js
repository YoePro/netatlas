// map.js — Force-directed network graph
(function() {
  NANav.buildNav('map');
  NANav.bindThemeToggle();
  NANav.startClock();

  const wrap    = document.getElementById('map-canvas-wrap');
  const canvas  = document.getElementById('map-canvas');
  const ctx     = canvas.getContext('2d');
  const tooltip = document.getElementById('map-tooltip');

  // ── State ─────────────────────────────────────────────────────
  let allNodes = [], allEdges = [];
  let nodes = [], edges = [];
  let transform = {x: 0, y: 0, scale: 1};
  let selectedNode = null;
  let hoveredNode  = null;
  let dragging     = null;
  let panning      = null;
  let animFrame;
  let renderQueued = false;
  let visibleTypes = new Set(['client','sensor','domain','server','internet']);

  const NODE_RADIUS = {client:10, sensor:14, domain:9, server:11, internet:16};
  const NODE_COLORS = {
    client:   {fill:'#3B82F6', stroke:'#1D4ED8'},
    sensor:   {fill:'#F59E0B', stroke:'#B45309'},
    domain:   {fill:'#A78BFA', stroke:'#6D28D9'},
    server:   {fill:'#22C55E', stroke:'#15803D'},
    internet: {fill:'#6B7280', stroke:'#374151'},
  };

  function getColors() {
    const dark = document.documentElement.getAttribute('data-theme') === 'dark';
    return {
      bg:        dark ? '#0D1117' : '#F0F2F5',
      edgeBase:  dark ? 'rgba(255,255,255,.08)' : 'rgba(0,0,0,.06)',
      edgeSel:   dark ? 'rgba(59,130,246,.5)' : 'rgba(37,99,235,.4)',
      labelBg:   dark ? 'rgba(13,17,23,.85)' : 'rgba(248,249,250,.9)',
      labelText: dark ? '#CBD5E1' : '#374151',
      nodeSel:   dark ? '#FFFFFF' : '#000000',
      gridLine:  dark ? 'rgba(255,255,255,.03)' : 'rgba(0,0,0,.03)',
    };
  }

  // ── Canvas sizing ──────────────────────────────────────────────
  function resize() {
    const dpr = window.devicePixelRatio || 1;
    let W = wrap.clientWidth;
    let H = wrap.clientHeight;
    if (W < 50) W = Math.max(320, window.innerWidth - 240);
    if (H < 50) H = Math.max(320, window.innerHeight - 120);
    wrap.style.minHeight = H + 'px';
    canvas.width  = W * dpr;
    canvas.height = H * dpr;
    canvas.style.width  = W + 'px';
    canvas.style.height = H + 'px';
    ctx.setTransform(1, 0, 0, 1, 0, 0);
    ctx.scale(dpr, dpr);
  }

  function W() { return canvas.clientWidth || wrap.clientWidth || Math.max(320, window.innerWidth - 240); }
  function H() { return canvas.clientHeight || wrap.clientHeight || Math.max(320, window.innerHeight - 120); }

  // ── Force simulation ──────────────────────────────────────────
  const SIM = {
    repulsion:  3000,
    attraction: 0.04,
    gravity:    0.015,
    damping:    0.82,
    maxV:       12,
  };
  const FORCE_NODE_LIMIT = 180;
  const RENDER_NODE_LIMIT = 500;
  const RENDER_EDGE_LIMIT = 1000;

  function initPositions() {
    const cx = W() / 2, cy = H() / 2;
    nodes.forEach((n, i) => {
      if (n.x === undefined) {
        const a = (i / nodes.length) * Math.PI * 2;
        const r = 180 + Math.random() * 120;
        n.x  = cx + Math.cos(a) * r;
        n.y  = cy + Math.sin(a) * r;
      }
      n.vx = n.vx || 0;
      n.vy = n.vy || 0;
      n.pinned = n.pinned || false;
    });
  }

  function stepSim() {
    const cx = W() / 2, cy = H() / 2;

    // Reset forces
    nodes.forEach(n => { n.fx = 0; n.fy = 0; });

    // Repulsion (Barnes-Hut approximation omitted — small graphs)
    for (let i = 0; i < nodes.length; i++) {
      for (let j = i + 1; j < nodes.length; j++) {
        const dx = nodes[i].x - nodes[j].x;
        const dy = nodes[i].y - nodes[j].y;
        const d2 = dx*dx + dy*dy + 1;
        const f  = SIM.repulsion / d2;
        nodes[i].fx += f * dx / Math.sqrt(d2);
        nodes[i].fy += f * dy / Math.sqrt(d2);
        nodes[j].fx -= f * dx / Math.sqrt(d2);
        nodes[j].fy -= f * dy / Math.sqrt(d2);
      }
    }

    // Attraction along edges
    edges.forEach(e => {
      const a = nodeMap[e.source];
      const b = nodeMap[e.target];
      if (!a || !b) return;
      const dx = b.x - a.x;
      const dy = b.y - a.y;
      const d  = Math.sqrt(dx*dx + dy*dy) + .1;
      const targetLen = 120;
      const f  = (d - targetLen) * SIM.attraction;
      a.fx += f * dx / d;
      a.fy += f * dy / d;
      b.fx -= f * dx / d;
      b.fy -= f * dy / d;
    });

    // Gravity toward center
    nodes.forEach(n => {
      n.fx += (cx - n.x) * SIM.gravity;
      n.fy += (cy - n.y) * SIM.gravity;
    });

    // Integrate
    nodes.forEach(n => {
      if (n.pinned || n === dragging) return;
      n.vx = (n.vx + n.fx) * SIM.damping;
      n.vy = (n.vy + n.fy) * SIM.damping;
      // Clamp velocity
      const speed = Math.sqrt(n.vx*n.vx + n.vy*n.vy);
      if (speed > SIM.maxV) { n.vx = n.vx/speed*SIM.maxV; n.vy = n.vy/speed*SIM.maxV; }
      n.x += n.vx;
      n.y += n.vy;
    });
  }

  function totalKE() {
    return nodes.reduce((s,n) => s + n.vx*n.vx + n.vy*n.vy, 0);
  }

  let simRunning = true;
  let simSteps   = 0;

  function requestRender() {
    if (renderQueued) return;
    renderQueued = true;
    animFrame = requestAnimationFrame(loop);
  }

  function startSimulation() {
    simRunning = nodes.length > 0 && nodes.length <= FORCE_NODE_LIMIT;
    simSteps = 0;
    requestRender();
  }

  function loop() {
    renderQueued = false;
    if (simRunning && simSteps < 600 && nodes.length <= FORCE_NODE_LIMIT) {
      stepSim();
      simSteps++;
      if (totalKE() < 0.1 && simSteps > 60) simRunning = false;
    } else {
      simRunning = false;
    }
    render();
    if (simRunning || dragging || panning) {
      requestRender();
    } else {
      animFrame = null;
    }
  }

  // ── Hierarchical layout ───────────────────────────────────────
  function applyHierarchical() {
    const layers = ['client','sensor','domain','server','internet'];
    const byLayer = {};
    layers.forEach(l => { byLayer[l] = []; });
    nodes.forEach(n => {
      const l = n.type in byLayer ? n.type : 'domain';
      byLayer[l].push(n);
    });
    const cx = W() / 2;
    const layerY = [H()*.15, H()*.3, H()*.5, H()*.7, H()*.85];
    layers.forEach((l, li) => {
      const group = byLayer[l];
      const totalW = (group.length - 1) * 140;
      group.forEach((n, i) => {
        n.x = cx - totalW/2 + i * 140;
        n.y = layerY[li];
        n.vx = 0; n.vy = 0;
      });
    });
    startSimulation();
  }

  // ── Render ────────────────────────────────────────────────────
  function worldToScreen(x, y) {
    return {
      x: x * transform.scale + transform.x,
      y: y * transform.scale + transform.y
    };
  }
  function screenToWorld(x, y) {
    return {
      x: (x - transform.x) / transform.scale,
      y: (y - transform.y) / transform.scale
    };
  }

  function render() {
    const W_ = W(), H_ = H();
    const C  = getColors();
    ctx.clearRect(0, 0, W_, H_);

    // Background
    ctx.fillStyle = C.bg;
    ctx.fillRect(0, 0, W_, H_);

    if (!nodes.length) {
      ctx.fillStyle = C.labelText;
      ctx.font = '14px Inter, system-ui';
      ctx.textAlign = 'center';
      ctx.fillText('No graph data to display', W_ / 2, H_ / 2 - 8);
      ctx.font = '12px Inter, system-ui';
      ctx.fillStyle = C.labelText;
      ctx.fillText('Check /api/graph or sensor filters', W_ / 2, H_ / 2 + 14);
      return;
    }

    // Grid dots
    const gs = Math.max(16, 40 * transform.scale);
    if (W_ * H_ < 2500000) {
      const ox = transform.x % gs;
      const oy = transform.y % gs;
      ctx.fillStyle = C.gridLine;
      for (let gx = ox; gx < W_; gx += gs) {
        for (let gy = oy; gy < H_; gy += gs) {
          ctx.beginPath();
          ctx.arc(gx, gy, 1, 0, Math.PI*2);
          ctx.fill();
        }
      }
    }

    ctx.save();
    ctx.translate(transform.x, transform.y);
    ctx.scale(transform.scale, transform.scale);

    const renderEdges = edges.length > RENDER_EDGE_LIMIT ? edges.slice(0, RENDER_EDGE_LIMIT) : edges;
    const renderNodes = nodes.length > RENDER_NODE_LIMIT ? nodes.slice(0, RENDER_NODE_LIMIT) : nodes;

    // Draw edges
    renderEdges.forEach(e => {
      const a = nodeMap[e.source];
      const b = nodeMap[e.target];
      if (!a || !b) return;
      const isSel = selectedNode && (selectedNode.id === a.id || selectedNode.id === b.id);
      ctx.beginPath();
      ctx.moveTo(a.x, a.y);
      ctx.lineTo(b.x, b.y);
      ctx.strokeStyle = isSel ? C.edgeSel : C.edgeBase;
      ctx.lineWidth   = isSel ? 1.5 / transform.scale : 1 / transform.scale;
      ctx.stroke();

      // Edge count label on selected
      if (isSel && e.count && transform.scale > .6) {
        const mx = (a.x + b.x) / 2;
        const my = (a.y + b.y) / 2;
        ctx.fillStyle   = C.labelBg;
        const text = String(e.count);
        const tw   = ctx.measureText(text).width;
        ctx.fillRect(mx - tw/2 - 3, my - 8, tw + 6, 14);
        ctx.fillStyle = C.labelText;
        ctx.font      = `${10/transform.scale}px Inter, system-ui`;
        ctx.textAlign = 'center';
        ctx.fillText(text, mx, my + 3);
      }
    });

    // Draw nodes
    renderNodes.forEach(n => {
      const r  = (NODE_RADIUS[n.type] || 10);
      const cl = NODE_COLORS[n.type] || NODE_COLORS.client;
      const isSel   = selectedNode && selectedNode.id === n.id;
      const isHover = hoveredNode  && hoveredNode.id  === n.id;

      // Outer ring for selected
      if (isSel) {
        ctx.beginPath();
        ctx.arc(n.x, n.y, r + 5, 0, Math.PI*2);
        ctx.strokeStyle = cl.fill;
        ctx.lineWidth   = 2 / transform.scale;
        ctx.globalAlpha = .4;
        ctx.stroke();
        ctx.globalAlpha = 1;
      }

      // Hover ring
      if (isHover && !isSel) {
        ctx.beginPath();
        ctx.arc(n.x, n.y, r + 3, 0, Math.PI*2);
        ctx.fillStyle = cl.fill + '25';
        ctx.fill();
      }

      // Node body
      ctx.beginPath();
      ctx.arc(n.x, n.y, r, 0, Math.PI*2);
      ctx.fillStyle = cl.fill;
      ctx.fill();
      ctx.strokeStyle = isSel ? '#FFFFFF' : cl.stroke;
      ctx.lineWidth   = (isSel ? 2 : 1.5) / transform.scale;
      ctx.stroke();

      // Label (when zoomed in enough)
      if ((nodes.length <= 120 && transform.scale > .45) || isSel || isHover) {
        const label = n.label || n.id;
        const fs    = Math.min(11, 11 / transform.scale);
        ctx.font     = `${fs}px Inter, system-ui`;
        ctx.textAlign = 'center';
        const tw = ctx.measureText(label).width;
        const lx = n.x;
        const ly = n.y + r + fs + 3;
        ctx.fillStyle = C.labelBg;
        ctx.fillRect(lx - tw/2 - 3, ly - fs, tw + 6, fs + 4);
        ctx.fillStyle = C.labelText;
        ctx.fillText(label, lx, ly);
      }

      // Pinned indicator
      if (n.pinned) {
        ctx.beginPath();
        ctx.arc(n.x + r - 2, n.y - r + 2, 3, 0, Math.PI*2);
        ctx.fillStyle = '#FFFFFF';
        ctx.fill();
      }
    });

    ctx.restore();
  }

  // ── Node picking ──────────────────────────────────────────────
  function pickNode(sx, sy) {
    const w = screenToWorld(sx, sy);
    for (let i = nodes.length - 1; i >= 0; i--) {
      const n = nodes[i];
      const r = (NODE_RADIUS[n.type] || 10) + 4;
      const dx = n.x - w.x, dy = n.y - w.y;
      if (dx*dx + dy*dy < r*r) return n;
    }
    return null;
  }

  // ── Fit view ──────────────────────────────────────────────────
  function fitView() {
    if (!nodes.length) return;
    let minX = Infinity, maxX = -Infinity, minY = Infinity, maxY = -Infinity;
    nodes.forEach(n => {
      const r = NODE_RADIUS[n.type] || 10;
      minX = Math.min(minX, n.x - r);
      maxX = Math.max(maxX, n.x + r);
      minY = Math.min(minY, n.y - r);
      maxY = Math.max(maxY, n.y + r);
    });
    const pad = 60;
    const fw  = W() - pad*2, fh = H() - pad*2;
    const gw  = maxX - minX || 1, gh = maxY - minY || 1;
    const sc  = Math.min(fw / gw, fh / gh, 2);
    transform.scale = sc;
    transform.x = pad + (fw - gw * sc) / 2 - minX * sc;
    transform.y = pad + (fh - gh * sc) / 2 - minY * sc;
    requestRender();
  }

  // ── Interaction ───────────────────────────────────────────────
  function canvasXY(e) {
    const r = canvas.getBoundingClientRect();
    const t = e.touches ? e.touches[0] : e;
    return {x: t.clientX - r.left, y: t.clientY - r.top};
  }

  canvas.addEventListener('mousedown', e => {
    const {x, y} = canvasXY(e);
    const hit = pickNode(x, y);
    if (hit) {
      dragging = hit;
      wrap.classList.add('grabbing');
      requestRender();
    } else {
      panning = {sx: e.clientX, sy: e.clientY, tx: transform.x, ty: transform.y};
      wrap.classList.add('grabbing');
      requestRender();
    }
  });

  window.addEventListener('mousemove', e => {
    const {x, y} = canvasXY(e);
    if (dragging) {
      const w = screenToWorld(x, y);
      dragging.x = w.x; dragging.y = w.y;
      dragging.vx = 0; dragging.vy = 0;
      startSimulation();
    } else if (panning) {
      transform.x = panning.tx + (e.clientX - panning.sx);
      transform.y = panning.ty + (e.clientY - panning.sy);
      requestRender();
    } else {
      const hit = pickNode(x, y);
      if (hit !== hoveredNode) {
        hoveredNode = hit;
        if (hit) {
          tooltip.style.display = 'block';
          tooltip.innerHTML = buildTooltip(hit);
        } else {
          tooltip.style.display = 'none';
        }
        requestRender();
      }
      if (hit) {
        tooltip.style.left = (e.clientX + 12) + 'px';
        tooltip.style.top  = (e.clientY - 10) + 'px';
      }
    }
  });

  window.addEventListener('mouseup', e => {
    if (dragging) {
      dragging.pinned = true;
    }
    dragging = null;
    panning  = null;
    wrap.classList.remove('grabbing');
    requestRender();
  });

  canvas.addEventListener('click', e => {
    if (Math.abs(e.movementX) > 3 || Math.abs(e.movementY) > 3) return;
    const {x, y} = canvasXY(e);
    const hit = pickNode(x, y);
    if (hit) {
      selectedNode = hit;
      openInspector(hit);
    } else {
      selectedNode = null;
      closeInspector();
    }
    requestRender();
  });

  canvas.addEventListener('dblclick', e => {
    const {x, y} = canvasXY(e);
    const hit = pickNode(x, y);
    if (hit) {
      hit.pinned = false;
      startSimulation();
    }
  });

  canvas.addEventListener('wheel', e => {
    e.preventDefault();
    const {x, y} = canvasXY(e);
    const factor = e.deltaY > 0 ? .88 : 1.14;
    const newScale = Math.min(Math.max(transform.scale * factor, .08), 5);
    transform.x = x - (x - transform.x) * (newScale / transform.scale);
    transform.y = y - (y - transform.y) * (newScale / transform.scale);
    transform.scale = newScale;
    requestRender();
  }, {passive: false});

  // ── Inspector ─────────────────────────────────────────────────
  function openInspector(n) {
    const insp  = document.getElementById('node-inspector');
    const title = document.getElementById('inspector-title');
    const body  = document.getElementById('inspector-body');
    insp.classList.add('open');
    title.textContent = n.label || n.id;
    body.innerHTML = buildInspectorFields(n);
  }

  function closeInspector() {
    document.getElementById('node-inspector').classList.remove('open');
  }

  document.getElementById('inspector-close').addEventListener('click', () => {
    selectedNode = null;
    closeInspector();
  });

  function field(label, value, mono) {
    return `<div class="inspector-field">
      <div class="inspector-field-label">${label}</div>
      <div class="inspector-field-value${mono?' mono':''}">${value}</div>
    </div>`;
  }

  function esc(value) {
    return String(value == null ? '' : value)
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;');
  }

  function buildInspectorFields(n) {
    const typeColors = {client:'blue', sensor:'amber', domain:'purple', server:'green', internet:'muted'};
    const col = typeColors[n.type] || 'muted';
    let html = `<span class="badge badge-${col}" style="margin-bottom:14px">${n.type}</span>`;
    html += field('ID', n.id, true);
    if (n.label && n.label !== n.id) html += field('Label', n.label);
    if (n.ip)       html += field('IP Address', n.ip, true);
    if (n.hostname) html += field('Hostname', n.hostname);
    if (n.service)  html += field('Service', n.service);
    if (n.tld)      html += field('TLD', '.' + n.tld);
    if (n.category) html += field('Category', n.category);
    if (n.status)   html += field('Status', `<span class="badge badge-${n.status==='online'?'green':'red'}">${n.status}</span>`);
    if (n.queries)  html += field('DNS Queries', n.queries.toLocaleString());
    if (n.type === 'client' && n.ip) {
      html += buildClientEditor(n);
    }

    // Connected edges
    const conns = edges.filter(e => e.source === n.id || e.target === n.id);
    if (conns.length) {
      html += '<div class="divider"></div>';
      html += `<div class="inspector-field-label" style="font-size:11px;font-weight:600;text-transform:uppercase;letter-spacing:.06em;color:var(--text-muted);margin-bottom:8px">${conns.length} Connection${conns.length>1?'s':''}</div>`;
      html += '<div style="display:flex;flex-direction:column;gap:4px;">';
      conns.slice(0, 8).forEach(e => {
        const peer = e.source === n.id ? nodeMap[e.target] : nodeMap[e.source];
        if (!peer) return;
        const typeColors2 = {client:'blue', sensor:'amber', domain:'purple', server:'green', internet:'muted'};
        const pc = typeColors2[peer.type] || 'muted';
        html += `<div style="display:flex;align-items:center;gap:6px;font-size:12px;padding:4px 6px;border-radius:4px;background:var(--bg-surface-2)">
          <span class="badge badge-${pc}" style="font-size:10px">${peer.type}</span>
          <span class="truncate" style="color:var(--text-secondary)">${peer.label || peer.id}</span>
          <span style="margin-left:auto;color:var(--text-muted);font-size:11px">${e.type}</span>
        </div>`;
      });
      if (conns.length > 8) {
        html += `<div style="font-size:11px;color:var(--text-muted);padding:2px 6px">+${conns.length-8} more</div>`;
      }
      html += '</div>';
    }

    html += `<div class="divider"></div>
      <div style="display:flex;gap:8px;flex-wrap:wrap;">
        <button class="btn btn-secondary btn-sm" onclick="NAMap.focusNode('${n.id}')">Focus</button>
        <button class="btn btn-ghost btn-sm" onclick="NAMap.pinNode('${n.id}')">Pin / Unpin</button>
      </div>`;

    return html;
  }

  function buildClientEditor(n) {
    const type = n.deviceType || '';
    const options = ['', 'computer', 'phone', 'tablet', 'iot', 'container', 'server', 'network', 'other'];
    const optionHTML = options.map(value => {
      const label = value ? value : 'Unclassified';
      return `<option value="${esc(value)}"${value === type ? ' selected' : ''}>${esc(label)}</option>`;
    }).join('');
    return `<div class="divider"></div>
      <div class="inspector-field-label" style="font-size:11px;font-weight:600;text-transform:uppercase;letter-spacing:.06em;color:var(--text-muted);margin-bottom:8px">Client Metadata</div>
      <div style="display:flex;flex-direction:column;gap:8px;">
        <input id="client-manual-name" class="input" placeholder="Working name" value="${esc(n.manualName || '')}">
        <input id="client-hostname" class="input" placeholder="Hostname" value="${esc(n.hostname || '')}">
        <select id="client-device-type" class="input">${optionHTML}</select>
        <textarea id="client-notes" class="input" rows="3" placeholder="Notes">${esc(n.notes || '')}</textarea>
        <label style="display:flex;align-items:center;gap:6px;font-size:12px;color:var(--text-secondary)">
          <input id="client-resolve-dns" type="checkbox"> Lookup DNS name
        </label>
        ${n.dnsName ? field('DNS name', esc(n.dnsName)) : ''}
        ${n.displaySource ? field('Display source', esc(n.displaySource)) : ''}
        <div style="display:flex;gap:8px;align-items:center;flex-wrap:wrap;">
          <button class="btn btn-secondary btn-sm" onclick="NAMap.saveClient('${esc(n.id)}')">Save</button>
          <span id="client-save-status" style="font-size:12px;color:var(--text-muted)"></span>
        </div>
      </div>`;
  }

  function buildTooltip(n) {
    const typeColors = {client:'#3B82F6', sensor:'#F59E0B', domain:'#A78BFA', server:'#22C55E', internet:'#6B7280'};
    const c = typeColors[n.type] || '#9CA3AF';
    let t = `<strong style="color:${c}">${n.label || n.id}</strong>`;
    if (n.ip)       t += `<br><span style="opacity:.7">${n.ip}</span>`;
    if (n.hostname) t += `<br><span style="opacity:.7">${n.hostname}</span>`;
    if (n.queries)  t += `<br>${n.queries.toLocaleString()} queries`;
    return t;
  }

  // ── Node map ──────────────────────────────────────────────────
  let nodeMap = {};

  function applyFilter() {
    nodes = allNodes.filter(n => visibleTypes.has(n.type));
    const visibleIds = new Set(nodes.map(n => n.id));
    edges = allEdges.filter(e => visibleIds.has(e.source) && visibleIds.has(e.target));
    nodeMap = {};
    nodes.forEach(n => { nodeMap[n.id] = n; });
    document.getElementById('stat-nodes').textContent = nodes.length;
    document.getElementById('stat-edges').textContent = edges.length;
    requestRender();
  }

  // ── Data loading ──────────────────────────────────────────────
  async function load() {
    document.getElementById('map-status').textContent = 'Loading graph data…';
    let data;
    try {
      data = await NAAPI.getGraph();
    } catch (err) {
      console.error('Failed to load graph', err);
      data = {nodes: [], edges: []};
      document.getElementById('map-status').textContent = 'Graph load failed';
    }

    allNodes = data.nodes.map(n => ({...n, x: undefined, vx: 0, vy: 0, pinned: false}));
    allEdges = data.edges;

    applyFilter();
    initPositions();
    if (nodes.length > FORCE_NODE_LIMIT) {
      applyHierarchical();
      simRunning = false;
      requestRender();
    } else {
      startSimulation();
    }

    if (nodes.length || edges.length) {
      document.getElementById('map-status').textContent =
        `${nodes.length} nodes · ${edges.length} edges`;
    }

    setTimeout(fitView, 200);
  }

  // ── Controls ──────────────────────────────────────────────────
  document.getElementById('btn-fit').addEventListener('click', fitView);
  document.getElementById('btn-fit-icon').addEventListener('click', fitView);
  document.getElementById('btn-zoom-in').addEventListener('click', () => {
    const cx = W()/2, cy = H()/2;
    const ns = Math.min(transform.scale * 1.3, 5);
    transform.x = cx - (cx - transform.x) * (ns / transform.scale);
    transform.y = cy - (cy - transform.y) * (ns / transform.scale);
    transform.scale = ns;
    requestRender();
  });
  document.getElementById('btn-zoom-out').addEventListener('click', () => {
    const cx = W()/2, cy = H()/2;
    const ns = Math.max(transform.scale / 1.3, .08);
    transform.x = cx - (cx - transform.x) * (ns / transform.scale);
    transform.y = cy - (cy - transform.y) * (ns / transform.scale);
    transform.scale = ns;
    requestRender();
  });
  document.getElementById('btn-layout-hierarchical').addEventListener('click', () => {
    applyHierarchical();
  });
  document.getElementById('btn-layout-force').addEventListener('click', () => {
    nodes.forEach(n => { n.x = undefined; n.vx = 0; n.vy = 0; n.pinned = false; });
    initPositions();
    startSimulation();
    if (!simRunning) {
      document.getElementById('map-status').textContent =
        `${nodes.length} nodes · ${edges.length} edges · force layout disabled for large graph`;
      requestRender();
    }
  });
  document.getElementById('btn-reset-filter').addEventListener('click', () => {
    visibleTypes = new Set(['client','sensor','domain','server','internet']);
    document.querySelectorAll('.node-filter').forEach(cb => { cb.checked = true; });
    applyFilter();
    initPositions();
    startSimulation();
  });

  document.querySelectorAll('.node-filter').forEach(cb => {
    cb.addEventListener('change', () => {
      const type = cb.dataset.type;
      if (cb.checked) visibleTypes.add(type);
      else            visibleTypes.delete(type);
      applyFilter();
      startSimulation();
    });
  });

  // Search
  document.getElementById('map-search-input').addEventListener('input', function() {
    const q = this.value.trim().toLowerCase();
    if (!q) {
      nodes.forEach(n => { n._dim = false; });
      requestRender();
      return;
    }
    nodes.forEach(n => {
      const match = (n.label||'').toLowerCase().includes(q)
        || (n.ip||'').toLowerCase().includes(q)
        || (n.hostname||'').toLowerCase().includes(q);
      n._dim = !match;
    });
    requestRender();
  });

  // ── Public API ────────────────────────────────────────────────
  window.NAMap = {
    focusNode(id) {
      const n = nodeMap[id];
      if (!n) return;
      const s = {x: W()/2 - n.x * transform.scale, y: H()/2 - n.y * transform.scale};
      transform.x = s.x; transform.y = s.y;
      requestRender();
    },
    pinNode(id) {
      const n = nodeMap[id];
      if (n) { n.pinned = !n.pinned; openInspector(n); requestRender(); }
    },
    async saveClient(id) {
      const n = nodeMap[id];
      const status = document.getElementById('client-save-status');
      if (!n || !n.ip) return;
      const payload = {
        manual_name: document.getElementById('client-manual-name')?.value || '',
        hostname: document.getElementById('client-hostname')?.value || '',
        device_type: document.getElementById('client-device-type')?.value || '',
        notes: document.getElementById('client-notes')?.value || '',
        resolve_dns: Boolean(document.getElementById('client-resolve-dns')?.checked)
      };
      if (status) status.textContent = 'Saving...';
      try {
        const detail = await NAAPI.updateClient(n.ip, payload);
        Object.assign(n, {
          label: detail.label || payload.manual_name || payload.hostname || n.ip,
          manualName: detail.manualName || payload.manual_name,
          hostname: detail.hostname || payload.hostname,
          dnsName: detail.dnsName || n.dnsName,
          deviceType: detail.deviceType || payload.device_type,
          notes: detail.notes || payload.notes,
          displaySource: detail.displaySource || 'manual'
        });
        const all = allNodes.find(node => node.id === id);
        if (all) Object.assign(all, n);
        if (status) status.textContent = 'Saved';
        openInspector(n);
        requestRender();
      } catch (err) {
        if (status) status.textContent = 'Save failed';
      }
    }
  };

  // ── Theme reactivity ──────────────────────────────────────────
  document.addEventListener('na:theme', requestRender);

  // ── Init ──────────────────────────────────────────────────────
  window.addEventListener('resize', () => {
    resize();
    fitView();
  });

  resize();
  load();
  requestRender();
})();

// api.js — API client with mock fallback
(function() {
  const BASE = '';
  const TIMEOUT_MS = 4000;

  async function withTimeout(work) {
    const ctrl = new AbortController();
    const timer = setTimeout(() => ctrl.abort(), TIMEOUT_MS);
    try {
      return await work(ctrl.signal);
    } finally {
      clearTimeout(timer);
    }
  }

  async function get(path) {
    return withTimeout(async signal => {
      const res = await fetch(BASE + path, {
        headers: {'Accept': 'application/json'},
        signal
      });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      return res.json();
    });
  }

  async function post(path, body) {
    return withTimeout(async signal => {
      const res = await fetch(BASE + path, {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify(body),
        signal
      });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      return res.json();
    });
  }

  // Mock data used when the real API is unavailable
  const MOCK = {
    '/api/graph': {
      nodes: [
        {id:'n1',  label:'dns-resolver-01', type:'sensor',  ip:'10.0.0.10', status:'online',  queries:142830},
        {id:'n2',  label:'dns-resolver-02', type:'sensor',  ip:'10.0.0.11', status:'online',  queries:98210},
        {id:'n3',  label:'192.168.1.42',    type:'client',  ip:'192.168.1.42', hostname:'laptop-alice'},
        {id:'n4',  label:'192.168.1.55',    type:'client',  ip:'192.168.1.55', hostname:'desktop-bob'},
        {id:'n5',  label:'192.168.1.71',    type:'client',  ip:'192.168.1.71', hostname:'phone-carol'},
        {id:'n6',  label:'192.168.2.10',    type:'client',  ip:'192.168.2.10', hostname:'srv-app01'},
        {id:'n7',  label:'github.com',      type:'domain',  tld:'com',  category:'development'},
        {id:'n8',  label:'microsoft.com',   type:'domain',  tld:'com',  category:'productivity'},
        {id:'n9',  label:'cloudfront.net',  type:'domain',  tld:'net',  category:'cdn'},
        {id:'n10', label:'api.example.org', type:'domain',  tld:'org',  category:'internal'},
        {id:'n11', label:'192.0.2.1',       type:'server',  ip:'192.0.2.1',  service:'HTTPS'},
        {id:'n12', label:'198.51.100.5',    type:'server',  ip:'198.51.100.5', service:'HTTPS'},
        {id:'n13', label:'203.0.113.10',    type:'server',  ip:'203.0.113.10', service:'CDN'},
        {id:'n14', label:'Internet',        type:'internet', label:'Internet Gateway'},
        {id:'n15', label:'192.168.1.88',    type:'client',  ip:'192.168.1.88', hostname:'iot-cam01'},
        {id:'n16', label:'ads.tracker.io',  type:'domain',  tld:'io',   category:'advertising'},
        {id:'n17', label:'192.168.3.20',    type:'client',  ip:'192.168.3.20', hostname:'workstation-dave'},
        {id:'n18', label:'analytics.corp',  type:'domain',  tld:'corp', category:'internal'},
      ],
      edges: [
        {source:'n3', target:'n1', type:'query', count:1820},
        {source:'n4', target:'n1', type:'query', count:942},
        {source:'n5', target:'n2', type:'query', count:3201},
        {source:'n6', target:'n1', type:'query', count:510},
        {source:'n15',target:'n2', type:'query', count:88},
        {source:'n17',target:'n2', type:'query', count:1204},
        {source:'n1', target:'n7',  type:'resolve', count:420},
        {source:'n1', target:'n8',  type:'resolve', count:380},
        {source:'n1', target:'n9',  type:'resolve', count:290},
        {source:'n1', target:'n10', type:'resolve', count:180},
        {source:'n2', target:'n7',  type:'resolve', count:310},
        {source:'n2', target:'n16', type:'resolve', count:95},
        {source:'n2', target:'n18', type:'resolve', count:210},
        {source:'n7', target:'n11', type:'resolves_to', count:420},
        {source:'n8', target:'n12', type:'resolves_to', count:380},
        {source:'n9', target:'n13', type:'resolves_to', count:290},
        {source:'n11',target:'n14', type:'upstream', count:420},
        {source:'n12',target:'n14', type:'upstream', count:380},
        {source:'n13',target:'n14', type:'upstream', count:290},
      ]
    },
    '/api/stats': {
      activeSensors: 4,
      totalSensors: 5,
      dnsRequests: {value: 241040, delta: 8.3},
      newDevices:  {value: 12, delta: 2},
      newDomains:  {value: 47, delta: -3},
      alerts:      {value: 3, delta: 1},
      topTalkers: [
        {host:'192.168.1.42', name:'laptop-alice',   queries:1820, bytes:'2.4 MB'},
        {host:'192.168.1.55', name:'desktop-bob',    queries:942,  bytes:'1.1 MB'},
        {host:'192.168.3.20', name:'workstation-dave',queries:1204,bytes:'1.8 MB'},
        {host:'192.168.1.71', name:'phone-carol',    queries:3201, bytes:'4.2 MB'},
        {host:'192.168.1.88', name:'iot-cam01',      queries:88,   bytes:'0.1 MB'},
      ],
      recentDomains: [
        {domain:'api.example.org', first_seen:'2 min ago', category:'internal', clients:3},
        {domain:'cdn-eu.fastly.net', first_seen:'8 min ago', category:'cdn', clients:7},
        {domain:'telemetry.ms.com', first_seen:'14 min ago', category:'telemetry', clients:12},
        {domain:'metrics.corp.local', first_seen:'22 min ago', category:'internal', clients:1},
        {domain:'update.avast.com', first_seen:'31 min ago', category:'security', clients:2},
      ],
      timeline: generateTimeline()
    },
    '/api/sensors': [
      {id:'s1', name:'dns-resolver-01', location:'DC-A Rack 3',  version:'2.4.1', status:'online',  lastSeen:'just now',  sources:['DNS'],         events:142830, latency:1.2, uptime:99.8},
      {id:'s2', name:'dns-resolver-02', location:'DC-B Rack 7',  version:'2.4.1', status:'online',  lastSeen:'2 min ago', sources:['DNS'],         events:98210,  latency:1.8, uptime:99.1},
      {id:'s3', name:'dhcp-monitor-01', location:'DC-A Rack 1',  version:'1.1.0', status:'warning', lastSeen:'4 min ago', sources:['DHCP'],        events:4210,   latency:12.4,uptime:97.3},
      {id:'s4', name:'fw-edge-01',      location:'Edge Router',   version:'1.0.0', status:'online',  lastSeen:'1 min ago', sources:['Firewall'],    events:892000, latency:0.9, uptime:100},
      {id:'s5', name:'vpn-gateway-01',  location:'DMZ',           version:'0.9.2', status:'offline', lastSeen:'1 hr ago',  sources:['VPN'],         events:0,      latency:null,uptime:0},
    ]
  };

  function generateTimeline() {
    const data = [];
    const now = Date.now();
    for (let i = 59; i >= 0; i--) {
      data.push({
        t: now - i * 60000,
        queries: Math.floor(Math.random() * 2000 + 2000 + Math.sin(i/10)*500),
        alerts: Math.random() > .92 ? Math.floor(Math.random() * 3 + 1) : 0
      });
    }
    return data;
  }

  async function getGraph() {
    try { return normalizeGraph(await get('/api/graph')); }
    catch(_) { return normalizeGraph(MOCK['/api/graph']); }
  }

  async function getStats() {
    try { return await get('/api/stats'); }
    catch(_) { return MOCK['/api/stats']; }
  }

  async function getSensors() {
    try { return await get('/api/sensors'); }
    catch(_) { return MOCK['/api/sensors']; }
  }

  async function getSensor(id) {
    try { return await get(`/api/sensors/${encodeURIComponent(id)}`); }
    catch(_) {
      let s;
      try {
        const sensors = await getSensors();
        s = sensors.find(x => x.id === id);
      } catch (_) {}
      s = s || MOCK['/api/sensors'].find(x => x.id === id) || MOCK['/api/sensors'][0];
      return {
        ...s,
        cpu: s.cpu ?? 0, memory: s.memory ?? 0, disk: s.disk ?? 0,
        config: {
          sensor_id: s.id,
          source: (s.sources || []).join(', '),
          location: s.location || '',
          version: s.version || ''
        },
        recentErrors: [],
        topDomains: [],
        timeline: generateTimeline()
      };
    }
  }

  async function getClient(ip) {
    return get(`/api/clients/${encodeURIComponent(ip)}`);
  }

  async function updateClient(ip, data) {
    return post(`/api/clients/${encodeURIComponent(ip)}`, data);
  }

  function normalizeGraph(data) {
    const graph = data && typeof data === 'object' ? data : {};
    const nodes = Array.isArray(graph.nodes) ? graph.nodes : [];
    const edges = Array.isArray(graph.edges) ? graph.edges : [];
    const nodeIds = new Set();
    const normalizedNodes = [];
    nodes.forEach((node, index) => {
      if (!node || typeof node !== 'object') return;
      const id = String(node.id || `node-${index}`);
      if (nodeIds.has(id)) return;
      nodeIds.add(id);
      normalizedNodes.push({
        ...node,
        id,
        type: node.type || 'client',
        label: node.label || node.ip || id
      });
    });
    const normalizedEdges = [];
    edges.forEach(edge => {
      if (!edge || typeof edge !== 'object') return;
      const source = String(edge.source || '');
      const target = String(edge.target || '');
      if (!nodeIds.has(source) || !nodeIds.has(target)) return;
      normalizedEdges.push({
        ...edge,
        source,
        target,
        type: edge.type || 'link',
        count: Number(edge.count || 1)
      });
    });
    return {nodes: normalizedNodes, edges: normalizedEdges};
  }

  window.NAAPI = {get, post, getGraph, getStats, getSensors, getSensor, getClient, updateClient, MOCK};
})();

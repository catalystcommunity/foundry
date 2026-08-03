const roles = ["management", "openbao", "dns", "zot", "cluster-control-plane", "cluster-worker"];
let currentConfig = null;
let reviewedConfig = null;

const byId = (id) => document.getElementById(id);

async function request(path, options = {}) {
  const response = await fetch(path, {
    credentials: "same-origin",
    headers: { "Content-Type": "application/json", ...(options.headers || {}) },
    ...options,
  });
  const body = await response.json().catch(() => ({}));
  if (!response.ok) throw new Error(body.error || `Request failed (${response.status})`);
  return body;
}

async function authenticate(token) {
  await request("/api/v1/session", { method: "POST", body: JSON.stringify({ token }) });
  history.replaceState(null, "", location.pathname);
}

async function start() {
  const fragment = new URLSearchParams(location.hash.slice(1));
  const fragmentToken = fragment.get("token");
  if (fragmentToken) {
    try { await authenticate(fragmentToken); } catch (error) { showLogin(error.message); return; }
  }
  try {
    currentConfig = await request("/api/v1/config");
  } catch (_) {
    showLogin("");
    return;
  }
  byId("login").hidden = true;
  byId("app").hidden = false;
  populateWizard(currentConfig);
  await loadState();
}

function showLogin(message) {
  byId("app").hidden = true;
  byId("login").hidden = false;
  byId("login-error").textContent = message;
}

byId("login-form").addEventListener("submit", async (event) => {
  event.preventDefault();
  try {
    await authenticate(byId("access-token").value);
    location.reload();
  } catch (error) { byId("login-error").textContent = error.message; }
});

document.querySelectorAll(".nav-button").forEach((button) => button.addEventListener("click", () => {
  document.querySelectorAll(".nav-button").forEach((item) => item.classList.toggle("active", item === button));
  document.querySelectorAll(".view").forEach((view) => { view.hidden = view.id !== `${button.dataset.view}-view`; });
}));

async function loadState() {
  try {
    const state = await request("/api/v1/state");
    renderGraph(byId("state-graph"), state);
    byId("network-guidance").textContent = state.guidance;
    renderHealth(state.nodes);
  } catch (error) { byId("connection-state").textContent = error.message; }
}

byId("refresh-state").addEventListener("click", loadState);

function populateWizard(config) {
  byId("cluster-name").value = config.cluster_name || "";
  byId("primary-domain").value = config.primary_domain || "";
  byId("gateway").value = config.gateway || "";
  byId("netmask").value = config.netmask || "255.255.255.0";
  byId("vip").value = config.vip || "";
  byId("allow-cgnat").checked = Boolean(config.allow_cgnat_vip);
  byId("management-port").value = config.management_port || 9080;
  byId("management-image").value = config.management_image || "ghcr.io/catalystcommunity/foundry";
  byId("hosts").replaceChildren();
  (config.hosts || []).forEach(addHostEditor);
  if (!(config.hosts || []).length) addHostEditor();
  refreshManagementHosts(config.management_host || "");
}

function addHostEditor(host = {}) {
  const editor = byId("host-template").content.firstElementChild.cloneNode(true);
  ["hostname", "address", "user", "port"].forEach((field) => {
    const input = editor.querySelector(`[data-field="${field}"]`);
    input.value = host[field] || (field === "user" ? "root" : field === "port" ? 22 : "");
    input.addEventListener("input", invalidatePlan);
  });
  editor.dataset.state = host.state || "";
  const roleBox = editor.querySelector(".roles");
  roles.forEach((role) => {
    const label = document.createElement("label");
    label.className = "role-chip";
    const input = document.createElement("input");
    input.type = "checkbox";
    input.value = role;
    input.checked = (host.roles || []).includes(role);
    input.addEventListener("change", invalidatePlan);
    label.append(input, document.createTextNode(role));
    roleBox.append(label);
  });
  editor.querySelector(".remove-host").addEventListener("click", () => {
    editor.remove(); refreshManagementHosts(); invalidatePlan();
  });
  editor.querySelector('[data-field="hostname"]').addEventListener("input", () => refreshManagementHosts());
  byId("hosts").append(editor);
}

byId("add-host").addEventListener("click", () => { addHostEditor(); refreshManagementHosts(); invalidatePlan(); });
byId("wizard-form").addEventListener("input", invalidatePlan);

function refreshManagementHosts(selected = byId("management-host").value) {
  const select = byId("management-host");
  const names = [...document.querySelectorAll('.host-editor [data-field="hostname"]')].map((input) => input.value.trim()).filter(Boolean);
  select.replaceChildren(new Option("Do not install a manager", ""));
  names.forEach((name) => select.add(new Option(name, name)));
  select.value = names.includes(selected) ? selected : "";
}

function readWizard() {
  return {
    cluster_name: byId("cluster-name").value.trim(),
    primary_domain: byId("primary-domain").value.trim(),
    gateway: byId("gateway").value.trim(),
    netmask: byId("netmask").value.trim(),
    vip: byId("vip").value.trim(),
    allow_cgnat_vip: byId("allow-cgnat").checked,
    management_host: byId("management-host").value,
    management_port: Number(byId("management-port").value),
    management_image: byId("management-image").value.trim(),
    hosts: [...document.querySelectorAll(".host-editor")].map((editor) => ({
      hostname: editor.querySelector('[data-field="hostname"]').value.trim(),
      address: editor.querySelector('[data-field="address"]').value.trim(),
      user: editor.querySelector('[data-field="user"]').value.trim(),
      port: Number(editor.querySelector('[data-field="port"]').value),
      roles: [...editor.querySelectorAll('.roles input:checked')].map((input) => input.value),
      state: editor.dataset.state || "",
    })),
  };
}

function invalidatePlan() {
  reviewedConfig = null;
  byId("apply").disabled = true;
  byId("plan-summary").hidden = true;
  byId("apply-message").textContent = "Review the plan before you apply it.";
}

byId("review-plan").addEventListener("click", async () => {
  if (!byId("wizard-form").reportValidity()) return;
  try {
    const draft = readWizard();
    const plan = await request("/api/v1/plan", { method: "POST", body: JSON.stringify(draft) });
    reviewedConfig = draft;
    renderGraph(byId("preview-graph"), plan.topology);
    const summary = byId("plan-summary");
    summary.replaceChildren();
    const title = document.createElement("strong"); title.textContent = "Apply plan";
    const list = document.createElement("ul");
    plan.summary.forEach((item) => { const row = document.createElement("li"); row.textContent = item; list.append(row); });
    summary.append(title, list); summary.hidden = false;
    byId("apply").disabled = false;
    byId("apply-message").textContent = "The plan is valid and ready to apply.";
  } catch (error) { byId("apply-message").textContent = error.message; }
});

byId("apply").addEventListener("click", async () => {
  if (!reviewedConfig) return;
  byId("apply").disabled = true;
  try {
    const job = await request("/api/v1/apply", { method: "POST", body: JSON.stringify({ config: reviewedConfig, confirm: true }) });
    byId("apply-message").textContent = job.message;
    pollJob(job.id);
  } catch (error) { byId("apply-message").textContent = error.message; byId("apply").disabled = false; }
});

async function pollJob(id) {
  try {
    const job = await request(`/api/v1/jobs/${encodeURIComponent(id)}`);
    byId("apply-message").textContent = job.message;
    if (["queued", "running"].includes(job.state)) return setTimeout(() => pollJob(id), 1000);
    if (job.state === "complete") { currentConfig = reviewedConfig; await loadState(); }
  } catch (error) { byId("apply-message").textContent = error.message; }
}

function renderHealth(nodes) {
  const container = byId("health-list"); container.replaceChildren();
  nodes.filter((node) => node.kind === "host").forEach((node) => {
    const card = document.createElement("article"); card.className = "health-card";
    const title = document.createElement("strong"); title.textContent = node.label;
    const status = document.createElement("div"); status.className = "status"; status.textContent = `${node.health_label} · ${(node.roles || []).join(", ") || "No roles"}`;
    card.append(title, status); container.append(card);
  });
}

function renderGraph(container, model) {
  container.replaceChildren();
  const svgNS = "http://www.w3.org/2000/svg";
  const hostNodes = model.nodes.filter((node) => node.kind === "host");
  const width = Math.max(860, hostNodes.length * 185 + 90), height = 440;
  const positions = new Map([["internet", { x: width / 2, y: 55 }], ["router", { x: width / 2, y: 150 }], ["vip", { x: width / 2, y: 245 }]]);
  hostNodes.forEach((node, index) => positions.set(node.id, { x: ((index + 1) * width) / (hostNodes.length + 1), y: 365 }));
  const svg = document.createElementNS(svgNS, "svg"); svg.setAttribute("viewBox", `0 0 ${width} ${height}`); svg.setAttribute("role", "img");
  const title = document.createElementNS(svgNS, "title"); title.textContent = "Network topology showing the internet, router, VIP, and Foundry hosts"; svg.append(title);

  model.edges.forEach((edge) => {
    const from = positions.get(edge.from), to = positions.get(edge.to); if (!from || !to) return;
    const line = document.createElementNS(svgNS, "line");
    line.setAttribute("x1", from.x); line.setAttribute("y1", from.y); line.setAttribute("x2", to.x); line.setAttribute("y2", to.y);
    line.setAttribute("class", `graph-edge ${edge.kind}`); svg.append(line);
  });
  model.nodes.forEach((node) => {
    const point = positions.get(node.id); if (!point) return;
    const group = document.createElementNS(svgNS, "g"); group.setAttribute("class", `graph-node ${node.kind}`);
    if (node.kind === "internet") {
      const cloud = document.createElementNS(svgNS, "path"); cloud.setAttribute("d", `M ${point.x-52} ${point.y+12} C ${point.x-68} ${point.y-8}, ${point.x-44} ${point.y-28}, ${point.x-24} ${point.y-20} C ${point.x-12} ${point.y-45}, ${point.x+28} ${point.y-38}, ${point.x+31} ${point.y-14} C ${point.x+61} ${point.y-18}, ${point.x+70} ${point.y+16}, ${point.x+45} ${point.y+25} L ${point.x-38} ${point.y+25} C ${point.x-55} ${point.y+25}, ${point.x-64} ${point.y+20}, ${point.x-52} ${point.y+12}`); group.append(cloud);
    } else if (node.kind === "vip") {
      const circle = document.createElementNS(svgNS, "circle"); circle.setAttribute("cx", point.x); circle.setAttribute("cy", point.y); circle.setAttribute("r", 51); group.append(circle);
    } else {
      const rect = document.createElementNS(svgNS, "rect"); rect.setAttribute("x", point.x - 76); rect.setAttribute("y", point.y - 36); rect.setAttribute("width", 152); rect.setAttribute("height", 72); rect.setAttribute("rx", 11); group.append(rect);
    }
    const nodeTitle = document.createElementNS(svgNS, "text"); nodeTitle.setAttribute("x", point.x); nodeTitle.setAttribute("y", point.y - 3); nodeTitle.setAttribute("class", "node-title"); nodeTitle.textContent = node.label; group.append(nodeTitle);
    const detail = document.createElementNS(svgNS, "text"); detail.setAttribute("x", point.x); detail.setAttribute("y", point.y + 17); detail.setAttribute("class", "node-detail"); detail.textContent = node.address || node.health_label; group.append(detail);
    const dot = document.createElementNS(svgNS, "circle"); dot.setAttribute("cx", point.x + (node.kind === "vip" ? 36 : 61)); dot.setAttribute("cy", point.y - (node.kind === "vip" ? 32 : 23)); dot.setAttribute("r", 5); dot.setAttribute("class", `health-dot ${node.health}`); group.append(dot);
    svg.append(group);
  });
  container.append(svg);
}

start();

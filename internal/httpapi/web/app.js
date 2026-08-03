const canvas = document.querySelector('#galaxy');
const ctx = canvas.getContext('2d');
let stars = [];
let pointer = { x: innerWidth * .62, y: innerHeight * .4 };
const state = { project: '', userId: '', roles: '', token: '' };

function resize() {
  canvas.width = innerWidth * devicePixelRatio; canvas.height = innerHeight * devicePixelRatio;
  canvas.style.width = `${innerWidth}px`; canvas.style.height = `${innerHeight}px`;
  ctx.setTransform(devicePixelRatio, 0, 0, devicePixelRatio, 0, 0);
  stars = Array.from({ length: Math.min(260, Math.floor(innerWidth / 5)) }, () => ({ x: Math.random() * innerWidth, y: Math.random() * innerHeight, z: Math.random() * 1.2 + .1, hue: Math.random() > .82 ? 47 : 184 }));
}
function draw(t) {
  ctx.clearRect(0, 0, innerWidth, innerHeight);
  const glow = ctx.createRadialGradient(innerWidth * .62, innerHeight * .42, 20, innerWidth * .62, innerHeight * .42, Math.max(innerWidth, innerHeight) * .7);
  glow.addColorStop(0, 'rgba(22, 91, 118, .20)'); glow.addColorStop(.5, 'rgba(10, 31, 57, .08)'); glow.addColorStop(1, 'rgba(4, 7, 18, 0)'); ctx.fillStyle = glow; ctx.fillRect(0, 0, innerWidth, innerHeight);
  stars.forEach((s, i) => { const pullX = (pointer.x - s.x) * .00025 * s.z, pullY = (pointer.y - s.y) * .00025 * s.z; s.x += Math.sin(t / 3000 + i) * .13 * s.z + pullX; s.y += Math.cos(t / 3700 + i) * .1 * s.z + pullY; if (s.x < -3 || s.x > innerWidth + 3 || s.y < -3 || s.y > innerHeight + 3) { s.x = Math.random() * innerWidth; s.y = Math.random() * innerHeight; } ctx.fillStyle = `hsla(${s.hue}, 90%, 80%, ${.22 + (Math.sin(t / 700 + i) + 1) * .17})`; ctx.beginPath(); ctx.arc(s.x, s.y, s.z * 1.25, 0, Math.PI * 2); ctx.fill(); });
  requestAnimationFrame(draw);
}
addEventListener('resize', resize); addEventListener('pointermove', e => pointer = { x: e.clientX, y: e.clientY }); resize(); requestAnimationFrame(draw);

const landing = document.querySelector('#landing'); const workspace = document.querySelector('#workspace'); const dialog = document.querySelector('#connection-dialog');
function enterWorkspace() { landing.classList.add('hidden'); workspace.classList.remove('hidden'); window.scrollTo(0, 0); dialog.showModal(); }
function apiHeaders() { const headers = {}; if (state.token) headers.Authorization = `Bearer ${state.token}`; if (state.userId) headers['X-User-ID'] = state.userId; if (state.roles) headers['X-User-Roles'] = state.roles; return headers; }
async function api(path) { const response = await fetch(path, { headers: apiHeaders() }); const body = await response.json().catch(() => ({})); if (!response.ok) throw new Error(body.error || `请求失败 (${response.status})`); return body; }
function setText(id, value) { document.querySelector(`#${id}`).textContent = value; }
function renderMessage(id, message) { const target = document.querySelector(`#${id}`); target.replaceChildren(); const item = document.createElement('p'); item.textContent = message; target.append(item); }
function formatNumber(value) { return new Intl.NumberFormat('zh-CN').format(value || 0); }
function setConnectionStatus(message, failed = false) { const panel = document.querySelector('#status-panel'); panel.textContent = message; panel.classList.toggle('error', failed); }

function renderMetrics(metrics) {
  const rate = metrics.total ? `${Math.round(metrics.succeeded / metrics.total * 100)}%` : '—';
  setText('metric-total', formatNumber(metrics.total)); setText('metric-success', rate); setText('metric-latency', metrics.total ? `${Math.round(metrics.average_duration_ms)} ms` : '—'); setText('metric-tokens', formatNumber(metrics.total_tokens));
  const details = document.querySelector('#metrics-detail'); details.replaceChildren();
  [['成功', metrics.succeeded], ['失败', metrics.errors], ['拒答', metrics.refused], ['成本', `$${Number(metrics.total_cost_usd || 0).toFixed(4)}`]].forEach(([label, value]) => { const row = document.createElement('div'); const key = document.createElement('span'); const data = document.createElement('strong'); key.textContent = label; data.textContent = value; row.append(key, data); details.append(row); });
}
function renderTasks(tasks) {
  const list = document.querySelector('#tasks-list'); list.replaceChildren();
  if (!tasks.length) return renderMessage('tasks-list', '该项目暂无 Agent 任务。');
  tasks.slice(0, 4).forEach(task => { const row = document.createElement('li'); const dot = document.createElement('span'); dot.className = `feed-dot ${task.status === 'completed' ? 'cyan' : task.status === 'failed' ? 'pink' : 'yellow'}`; const copy = document.createElement('p'); const name = document.createElement('b'); const sub = document.createElement('small'); name.textContent = task.kind || 'Agent 任务'; sub.textContent = `${task.status || 'unknown'} · ${task.id}`; copy.append(name, sub); row.append(dot, copy); list.append(row); });
}
function renderRuns(runs) {
  const list = document.querySelector('#runs-list'); list.replaceChildren();
  if (!runs.length) return renderMessage('runs-list', '暂无评测运行。');
  runs.slice(0, 3).forEach(run => { const row = document.createElement('p'); const title = document.createElement('b'); const meta = document.createElement('small'); title.textContent = run.suite_name || run.suite_id; meta.textContent = `${run.status} · ${run.gate_status} · ${Math.round((run.pass_rate || 0) * 100)}%`; row.append(title, meta); list.append(row); });
}
async function refreshDashboard() {
  if (!state.project) return;
  setConnectionStatus('正在从 API 加载真实数据…');
  const results = await Promise.allSettled([api(`/api/v1/observability/answer-metrics?project=${encodeURIComponent(state.project)}`), api(`/api/v1/agent-tasks?project=${encodeURIComponent(state.project)}&limit=8`), api('/api/v1/evaluations/runs?limit=5')]);
  const errors = [];
  if (results[0].status === 'fulfilled') renderMetrics(results[0].value); else errors.push(`问答指标：${results[0].reason.message}`);
  if (results[1].status === 'fulfilled') renderTasks(results[1].value.tasks || []); else errors.push(`Agent 任务：${results[1].reason.message}`);
  if (results[2].status === 'fulfilled') renderRuns(results[2].value.runs || []); else errors.push(`评测运行：${results[2].reason.message}`);
  const allReady = errors.length === 0; setConnectionStatus(allReady ? `已加载项目 ${state.project} 的实时数据。` : errors.join('；'), !allReady);
  setText('connection-note', allReady ? `数据源：${state.project} · 已使用当前身份请求 API。` : '连接失败：请检查项目权限与身份配置。');
}
function openConfig() { dialog.showModal(); }
document.querySelectorAll('[data-enter]').forEach(button => button.addEventListener('click', enterWorkspace));
document.querySelector('[data-home]').addEventListener('click', event => { event.preventDefault(); workspace.classList.add('hidden'); landing.classList.remove('hidden'); });
document.querySelectorAll('[data-action="configure"]').forEach(button => button.addEventListener('click', openConfig));
document.querySelectorAll('[data-action="refresh"]').forEach(button => button.addEventListener('click', () => refreshDashboard().catch(error => setConnectionStatus(error.message, true))));
document.querySelectorAll('[data-page]').forEach(button => button.addEventListener('click', () => activatePage(button.dataset.page)));
document.querySelectorAll('[data-page-target]').forEach(button => button.addEventListener('click', () => activatePage(button.dataset.pageTarget)));
function activatePage(name) { document.querySelectorAll('[data-page]').forEach(item => item.classList.toggle('active', item.dataset.page === name)); document.querySelectorAll('.page, .placeholder-page').forEach(page => page.classList.toggle('active-page', page.id === name)); document.querySelector('#page-title').textContent = document.querySelector(`[data-page="${name}"]`)?.textContent.trim() || '知识中枢'; }
document.querySelector('#connection-form').addEventListener('submit', event => { event.preventDefault(); const form = new FormData(event.currentTarget); state.project = String(form.get('project')).trim(); state.userId = String(form.get('userId')).trim(); state.roles = String(form.get('roles')).trim(); state.token = String(form.get('token')).trim(); if (!state.userId && !state.token) { setText('connection-error', '请填写 Dev 用户 ID 或 Bearer Token。'); return; } setText('connection-error', ''); setText('user-name', state.userId || 'Token 用户'); setText('user-avatar', (state.userId || 'TK').slice(0, 2).toUpperCase()); dialog.close(); refreshDashboard().catch(error => setConnectionStatus(error.message, true)); });

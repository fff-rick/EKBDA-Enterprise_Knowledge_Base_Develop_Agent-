const canvas = document.querySelector('#galaxy');
const ctx = canvas.getContext('2d');
let stars = [];
let pointer = { x: innerWidth * .62, y: innerHeight * .4 };

function resize() {
  canvas.width = innerWidth * devicePixelRatio;
  canvas.height = innerHeight * devicePixelRatio;
  canvas.style.width = `${innerWidth}px`;
  canvas.style.height = `${innerHeight}px`;
  ctx.setTransform(devicePixelRatio, 0, 0, devicePixelRatio, 0, 0);
  stars = Array.from({ length: Math.min(260, Math.floor(innerWidth / 5)) }, () => ({
    x: Math.random() * innerWidth, y: Math.random() * innerHeight,
    z: Math.random() * 1.2 + .1, hue: Math.random() > .82 ? 47 : 184
  }));
}
function draw(t) {
  ctx.clearRect(0, 0, innerWidth, innerHeight);
  const glow = ctx.createRadialGradient(innerWidth * .62, innerHeight * .42, 20, innerWidth * .62, innerHeight * .42, Math.max(innerWidth, innerHeight) * .7);
  glow.addColorStop(0, 'rgba(22, 91, 118, .20)'); glow.addColorStop(.5, 'rgba(10, 31, 57, .08)'); glow.addColorStop(1, 'rgba(4, 7, 18, 0)'); ctx.fillStyle = glow; ctx.fillRect(0, 0, innerWidth, innerHeight);
  stars.forEach((s, i) => {
    const pullX = (pointer.x - s.x) * .00025 * s.z, pullY = (pointer.y - s.y) * .00025 * s.z;
    s.x += Math.sin(t / 3000 + i) * .13 * s.z + pullX; s.y += Math.cos(t / 3700 + i) * .1 * s.z + pullY;
    if (s.x < -3 || s.x > innerWidth + 3 || s.y < -3 || s.y > innerHeight + 3) { s.x = Math.random() * innerWidth; s.y = Math.random() * innerHeight; }
    const a = .22 + (Math.sin(t / 700 + i) + 1) * .17;
    ctx.fillStyle = `hsla(${s.hue}, 90%, 80%, ${a})`; ctx.beginPath(); ctx.arc(s.x, s.y, s.z * 1.25, 0, Math.PI * 2); ctx.fill();
  });
  requestAnimationFrame(draw);
}
addEventListener('resize', resize); addEventListener('pointermove', e => pointer = { x: e.clientX, y: e.clientY }); resize(); requestAnimationFrame(draw);

const landing = document.querySelector('#landing'); const workspace = document.querySelector('#workspace');
function enterWorkspace() { landing.classList.add('hidden'); workspace.classList.remove('hidden'); window.scrollTo(0, 0); }
document.querySelectorAll('[data-enter]').forEach(button => button.addEventListener('click', enterWorkspace));
document.querySelector('[data-home]').addEventListener('click', event => { event.preventDefault(); workspace.classList.add('hidden'); landing.classList.remove('hidden'); });
document.querySelectorAll('[data-page]').forEach(button => button.addEventListener('click', () => {
  document.querySelectorAll('[data-page]').forEach(item => item.classList.remove('active')); button.classList.add('active');
  document.querySelectorAll('.page, .placeholder-page').forEach(page => page.classList.remove('active-page'));
  document.querySelector(`#${button.dataset.page}`).classList.add('active-page');
  document.querySelector('#page-title').textContent = button.textContent.replace(/\d/g, '').trim();
}));

document.documentElement.classList.remove('no-js');

const reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)');

const nav = document.querySelector('.nav');
const updateNav = () => nav.classList.toggle('is-scrolled', window.scrollY > 24);
updateNav();
window.addEventListener('scroll', updateNav, { passive: true });

const reveals = document.querySelectorAll('.reveal');
if ('IntersectionObserver' in window) {
  const observer = new IntersectionObserver(
    entries => {
      for (const entry of entries) {
        if (!entry.isIntersecting) continue;
        entry.target.classList.add('is-visible');
        observer.unobserve(entry.target);
      }
    },
    { threshold: 0.15 }
  );
  reveals.forEach(el => observer.observe(el));
} else {
  reveals.forEach(el => el.classList.add('is-visible'));
}

const demo = document.getElementById('demo');
const replay = document.querySelector('.replay');
const STEP_DELAYS = [300, 1100, 1900, 2900, 3900];
let demoTimers = [];

const playDemo = () => {
  demoTimers.forEach(clearTimeout);
  const steps = [...demo.querySelectorAll('[data-step]')];
  steps.forEach(el => el.classList.remove('is-shown'));
  demo.classList.remove('is-syncing');
  demo.dataset.state = 'playing';
  replay.hidden = true;
  demoTimers = STEP_DELAYS.map((delay, i) =>
    setTimeout(() => {
      const n = String(i + 1);
      if (n === '5') {
        demo.classList.add('is-syncing');
        demoTimers.push(setTimeout(() => showStep(steps, n), 900));
      } else {
        showStep(steps, n);
      }
    }, delay)
  );
};

const showStep = (steps, n) => {
  steps.filter(el => el.dataset.step === n).forEach(el => el.classList.add('is-shown'));
  if (n === '5') {
    demo.dataset.state = 'done';
    replay.hidden = false;
  }
};

if (demo && !reducedMotion.matches && 'IntersectionObserver' in window) {
  demo.dataset.state = 'idle';
  const demoObserver = new IntersectionObserver(
    entries => {
      if (!entries.some(e => e.isIntersecting)) return;
      demoObserver.disconnect();
      playDemo();
    },
    { threshold: 0.45 }
  );
  demoObserver.observe(demo);
  replay.addEventListener('click', playDemo);
}

const layerEls = [...document.querySelectorAll('#security [data-layer]')];
let layerTimer = null;
const activateLayer = n => layerEls.forEach(el => el.classList.toggle('is-active', el.dataset.layer === n));
activateLayer('1');
document.querySelectorAll('.layer-list li').forEach(li => {
  const pick = () => {
    clearInterval(layerTimer);
    activateLayer(li.dataset.layer);
  };
  li.addEventListener('mouseenter', pick);
  li.addEventListener('focus', pick);
});
const security = document.getElementById('security');
if (security && !reducedMotion.matches && 'IntersectionObserver' in window) {
  const securityObserver = new IntersectionObserver(entries => {
    if (!entries.some(e => e.isIntersecting)) return;
    securityObserver.disconnect();
    let n = 1;
    layerTimer = setInterval(() => {
      n = (n % 5) + 1;
      activateLayer(String(n));
    }, 2800);
  }, { threshold: 0.3 });
  securityObserver.observe(security);
}

document.querySelectorAll('pre > code[data-copy]').forEach(code => {
  const button = document.createElement('button');
  button.type = 'button';
  button.className = 'copy';
  button.textContent = 'Copy';
  let reset;
  button.addEventListener('click', async () => {
    const result = await window.CrystalCove.copyText(code.textContent, navigator.clipboard);
    button.dataset.state = result;
    button.textContent = result === 'copied' ? 'Copied' : 'Select and copy';
    if (result === 'manual') getSelection().selectAllChildren(code);
    clearTimeout(reset);
    reset = setTimeout(() => {
      button.textContent = 'Copy';
      delete button.dataset.state;
    }, 2000);
  });
  code.closest('.code').append(button);
});

const layers = [...document.querySelectorAll('.hero .parallax')];
let ticking = false;
const parallax = () => {
  ticking = false;
  const y = Math.min(window.scrollY, window.innerHeight);
  for (const layer of layers) layer.style.transform = `translateY(${y * Number(layer.dataset.depth)}px)`;
};
window.addEventListener(
  'scroll',
  () => {
    if (reducedMotion.matches || ticking) return;
    ticking = true;
    requestAnimationFrame(parallax);
  },
  { passive: true }
);

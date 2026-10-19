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

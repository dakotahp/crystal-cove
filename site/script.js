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

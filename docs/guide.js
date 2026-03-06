/* GX Documentation — Shared JS */

// ── Scroll progress bar ──────────────────────────────────────────────────────
const progressBar = document.getElementById('progress');
if (progressBar) {
  window.addEventListener('scroll', () => {
    const scrolled = document.documentElement.scrollTop;
    const total    = document.documentElement.scrollHeight - window.innerHeight;
    progressBar.style.width = total > 0 ? (scrolled / total * 100) + '%' : '0%';
  }, { passive: true });
}

// ── Active sidebar link (current page) ───────────────────────────────────────
// Mark the link matching the current page filename as active
(function () {
  const page = window.location.pathname.split('/').pop() || 'overview.html';
  document.querySelectorAll('.sidebar-link[href]').forEach(link => {
    if (link.getAttribute('href') === page) {
      link.classList.add('active');
      // Scroll the sidebar so this link is visible
      link.scrollIntoView({ block: 'nearest' });
    }
  });
})();

// ── Heading scroll-spy for right TOC ────────────────────────────────────────
(function () {
  const tocLinks = document.querySelectorAll('.toc-link[href^="#"]');
  if (!tocLinks.length) return;

  const headings = Array.from(tocLinks)
    .map(l => document.querySelector(l.getAttribute('href')))
    .filter(Boolean);

  const observer = new IntersectionObserver(entries => {
    entries.forEach(e => {
      if (e.isIntersecting) {
        tocLinks.forEach(l => l.classList.remove('active'));
        const active = document.querySelector(`.toc-link[href="#${e.target.id}"]`);
        if (active) active.classList.add('active');
      }
    });
  }, { rootMargin: '-10% 0px -75% 0px', threshold: 0 });

  headings.forEach(h => observer.observe(h));
})();

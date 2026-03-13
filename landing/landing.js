/* ============================================================
   Kula Landing Page — Interactive behaviors
   ============================================================ */

document.addEventListener('DOMContentLoaded', () => {

    // ---- Theme Toggle Logic ----
    const themeBtn = document.getElementById('btn-theme');
    const previewDark = document.getElementById('preview-dark');
    const previewLight = document.getElementById('preview-light');

    // Check for saved theme preference or default to dark
    const savedTheme = localStorage.getItem('kula-theme') || 'dark';
    if (savedTheme === 'light') {
        document.body.classList.add('light-mode');
        showLightPreview();
    }

    function showLightPreview() {
        if (previewDark && previewLight) {
            previewDark.classList.add('hidden');
            previewLight.classList.remove('hidden');
        }
    }

    function showDarkPreview() {
        if (previewDark && previewLight) {
            previewLight.classList.add('hidden');
            previewDark.classList.remove('hidden');
        }
    }

    themeBtn.addEventListener('click', () => {
        document.body.classList.toggle('light-mode');
        const isLight = document.body.classList.contains('light-mode');
        localStorage.setItem('kula-theme', isLight ? 'light' : 'dark');

        if (isLight) {
            showLightPreview();
        } else {
            showDarkPreview();
        }
    });

    // ---- Fetch GitHub Stars ----
    async function fetchStars() {
        const badges = document.querySelectorAll('.github-stars-count');
        try {
            const resp = await fetch('https://api.github.com/repos/c0m4r/kula');
            if (resp.status === 403) {
                console.warn('GitHub API rate limit exceeded. Stars counter hidden.');
                badges.forEach(badge => badge.classList.add('hidden'));
                return;
            }
            if (resp.ok) {
                const data = await resp.json();
                const stars = data.stargazers_count;
                if (stars !== undefined) {
                    const starsText = stars >= 1000 ? (stars / 1000).toFixed(1) + 'k' : stars;
                    badges.forEach(badge => {
                        badge.textContent = '⭐ ' + starsText;
                        badge.classList.remove('hidden');
                    });
                }
            }
        } catch (e) {
            console.error('Failed to fetch stars:', e);
            badges.forEach(badge => badge.classList.add('hidden'));
        }
    }
    fetchStars();

    // ---- Install tabs ----
    document.querySelectorAll('.install-tab').forEach(tab => {
        tab.addEventListener('click', () => {
            document.querySelectorAll('.install-tab').forEach(t => t.classList.remove('active'));
            document.querySelectorAll('.install-panel').forEach(p => p.classList.remove('active'));

            tab.classList.add('active');
            const panel = document.getElementById('panel-' + tab.dataset.tab);
            if (panel) panel.classList.add('active');
        });
    });

    // ---- Copy buttons ----
    document.querySelectorAll('.copy-btn').forEach(btn => {
        btn.addEventListener('click', () => {
            const code = btn.closest('pre').querySelector('code');
            if (!code) return;
            navigator.clipboard.writeText(code.textContent.trim()).then(() => {
                const originalText = btn.textContent;
                btn.textContent = '✓ copied';
                btn.style.color = 'var(--accent-green)';
                btn.style.borderColor = 'var(--accent-green)';

                setTimeout(() => {
                    btn.textContent = originalText;
                    btn.style.color = '';
                    btn.style.borderColor = '';
                }, 2000);
            });
        });
    });

    // ---- Scroll-reveal (fade-up) ----
    const observer = new IntersectionObserver((entries) => {
        entries.forEach(entry => {
            if (entry.isIntersecting) {
                entry.target.classList.add('visible');
                observer.unobserve(entry.target);
            }
        });
    }, { threshold: 0.1 });

    document.querySelectorAll('.fade-up').forEach(el => observer.observe(el));

    // ---- Navigation background transparent to blur on scroll ----
    window.addEventListener('scroll', () => {
        const nav = document.getElementById('nav');
        if (window.scrollY > 40) {
            nav.style.boxShadow = '0 4px 30px rgba(0, 0, 0, 0.1)';
        } else {
            nav.style.boxShadow = 'none';
        }
    }, { passive: true });

    // ---- i18n Logic ----
    const langSelect = document.getElementById('lang-select');
    let currentTranslations = {};

    async function loadLocale(lang) {
        try {
            const response = await fetch(`locales/${lang}.json`);
            if (!response.ok) throw new Error(`Could not load ${lang} locale`);
            currentTranslations = await response.json();
            applyTranslations();
            document.documentElement.lang = lang;
            localStorage.setItem('kula-lang', lang);
            langSelect.value = lang;
        } catch (error) {
            console.error('Error loading locale:', error);
            if (lang !== 'en') loadLocale('en');
        }
    }

    function applyTranslations() {
        // Text translations
        document.querySelectorAll('[data-i18n]').forEach(el => {
            const key = el.getAttribute('data-i18n');
            if (currentTranslations[key]) {
                el.textContent = currentTranslations[key];
            }
        });

        // HTML translations (for tags like <br> or <a>)
        document.querySelectorAll('[data-i18n-html]').forEach(el => {
            const key = el.getAttribute('data-i18n-html');
            if (currentTranslations[key]) {
                el.innerHTML = currentTranslations[key];
            }
        });
    }

    langSelect.addEventListener('change', (e) => {
        loadLocale(e.target.value);
    });

    // Detect language: URL (?lang=) -> Saved -> Browser -> Default (en)
    const urlParams = new URLSearchParams(window.location.search);
    const urlLang = urlParams.get('lang');
    const savedLang = localStorage.getItem('kula-lang');
    const browserLang = navigator.language.split('-')[0];
    const supportedLangs = ['en', 'ar', 'bn', 'cs', 'de', 'es', 'fr', 'he', 'hi', 'id', 'it', 'ja', 'ko', 'ms', 'nl', 'pl', 'pt', 'ro', 'ru', 'sv', 'th', 'tr', 'uk', 'ur', 'vi', 'zh'];
    
    let initialLang = 'en';

    if (urlLang && supportedLangs.includes(urlLang)) {
        initialLang = urlLang;
    } else if (savedLang && supportedLangs.includes(savedLang)) {
        initialLang = savedLang;
    } else if (supportedLangs.includes(browserLang)) {
        initialLang = browserLang;
    }

    loadLocale(initialLang);

    // ---- Smooth scroll for anchor links ----
    document.querySelectorAll('a[href^="#"]').forEach(a => {
        a.addEventListener('click', e => {
            const href = a.getAttribute('href');
            if (href === '#') return;
            const target = document.querySelector(href);
            if (target) {
                e.preventDefault();
                const navHeight = document.getElementById('nav').offsetHeight;
                window.scrollTo({
                    top: target.offsetTop - navHeight - 20,
                    behavior: 'smooth'
                });
            }
        });
    });
});

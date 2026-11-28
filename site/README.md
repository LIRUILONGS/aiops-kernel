# OpsAGI website (GitHub Pages)

Static, self-contained landing page — plain HTML, CSS and JS, no build step.

- `index.html` — the page
- `assets/css/style.css` — the theme
- `assets/js/main.js` — nav, product-tour tabs, copy buttons, live GitHub star/fork counts
- `assets/screenshots/` — the flow screenshots used by the tour and the hero

## Deploy

Pushed changes under `site/` on `main` are published by `.github/workflows/pages.yml`
(`actions/upload-pages-artifact` + `actions/deploy-pages`).

One-time setup in the repository: **Settings → Pages → Build and deployment → Source = GitHub Actions**.

## Preview locally

```bash
python -m http.server 4173 --directory site
# open http://127.0.0.1:4173/
```

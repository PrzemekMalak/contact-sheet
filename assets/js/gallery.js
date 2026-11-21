import PhotoSwipeLightbox from './lib/photoswipe/photoswipe-lightbox.esm.min.js';

// Browser-history integration. PhotoSwipe 5 has no built-in history handling,
// so opening a photo creates no history entry and the Back button navigates
// away from the page. Instead we push a history entry when the lightbox opens,
// close the lightbox on Back (popstate), and pop our entry on a manual close —
// so Back just closes the photo and keeps you on the current page.
const lightboxes = [];      // every lightbox, so popstate can find the open one
let statePushed = false;    // did we add a history entry for the current open?
let closingViaBack = false; // is this close driven by the Back button?

// Back button: close whichever lightbox is currently open instead of navigating.
window.addEventListener('popstate', () => {
  const open = lightboxes.find((lb) => lb.pswp);
  if (open) {
    closingViaBack = true;
    open.pswp.close();
  }
});

document.querySelectorAll('.grid').forEach((grid, i) => {
  const id = 'grid-' + i;
  grid.id = id;
  const lightbox = new PhotoSwipeLightbox({
    gallery: '#' + id,
    children: 'a.ph',
    pswpModule: () => import('./lib/photoswipe/photoswipe.esm.min.js'),
  });
  lightboxes.push(lightbox);

  // Push a history entry when the photo opens; drop it again on a manual close.
  lightbox.on('beforeOpen', () => {
    if (!statePushed) {
      history.pushState({ pswp: true }, '');
      statePushed = true;
    }
  });
  lightbox.on('destroy', () => {
    if (statePushed && !closingViaBack) history.back();
    statePushed = false;
    closingViaBack = false;
  });

  // Caption + EXIF panel + "original" button
  lightbox.on('uiRegister', () => {
    lightbox.pswp.ui.registerElement({
      name: 'caption', order: 9, isButton: false, appendTo: 'root',
      html: '', onInit: (el, pswp) => {
        const update = () => {
          const slide = pswp.currSlide;
          const a = slide && slide.data && slide.data.element;
          if (!a) return;
          const cap = a.dataset.caption || '';
          const wrap = document.createElement('div');
          wrap.className = 'pswp-caption';
          if (cap) wrap.appendChild(document.createTextNode(cap));
          if (a.dataset.exif) {
            try {
              const e = JSON.parse(a.dataset.exif);
              // camera/lens become links to their gear page when a slug is present.
              const gear = (text, slug) => {
                if (!text) return null;
                if (slug) {
                  const link = document.createElement('a');
                  link.href = '/gear/' + slug + '/';
                  link.textContent = text;
                  return link;
                }
                return document.createTextNode(text);
              };
              const txt = (s) => (s ? document.createTextNode(s) : null);
              const nodes = [
                gear(a.dataset.cameraName || e.camera, a.dataset.cameraGear),
                gear(a.dataset.lensName || e.lens, a.dataset.lensGear),
                txt(e.aperture), txt(e.shutter),
                txt(e.iso ? 'ISO ' + e.iso : null), txt(e.focal),
              ].filter(Boolean);
              if (nodes.length) {
                const span = document.createElement('span');
                span.className = 'pswp-exif';
                nodes.forEach((n, i) => {
                  if (i) span.appendChild(document.createTextNode(' · '));
                  span.appendChild(n);
                });
                wrap.appendChild(span);
              }
            } catch (_) {}
          }
          el.replaceChildren(wrap);
        };
        pswp.on('change', update);
      }
    });
    // "Link" copies the photo page URL (/p/<id>/); "Img" copies the stable
    // 1600px JPG URL (/p/<id>/<id>.jpg) for embedding on forums.
    const copyButton = (name, order, label, attr) => {
      let timer;
      lightbox.pswp.ui.registerElement({
        name, order, isButton: true, appendTo: 'bar', html: label,
        onClick: (e, el, pswp) => {
          const url = pswp.currSlide.data.element.dataset[attr];
          if (!url) return;
          const fallback = () => window.prompt('Copy link', url);
          if (!navigator.clipboard) return fallback();
          navigator.clipboard.writeText(url).then(() => {
            el.textContent = 'Copied';
            clearTimeout(timer);
            timer = setTimeout(() => { el.textContent = label; }, 1500);
          }, fallback);
        }
      });
    };
    copyButton('link', 6, 'Link', 'page');
    copyButton('img', 7, 'Img', 'image');
    lightbox.pswp.ui.registerElement({
      name: 'original', order: 8, isButton: true, appendTo: 'bar',
      html: 'Original',
      onClick: (e, el, pswp) => {
        const url = pswp.currSlide.data.element.dataset.original;
        if (url) window.open(url, '_blank', 'noopener');
      }
    });
  });

  lightbox.init();
});

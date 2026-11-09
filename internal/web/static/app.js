/* Wi-See map: pan, zoom, drag. HTMX handles rendering; this file owns the
 * viewport transform and pointer interaction. */
(function () {
  'use strict';

  var map = document.getElementById('map');
  var viewport = document.getElementById('viewport');
  if (!map || !viewport) return;

  var view = { x: 0, y: 0, k: 1 };
  var drag = null; // {el, id, cx, cy, x, y, moved}
  var pan = null;  // {cx, cy, x, y}
  var suppressClick = false;

  function apply() {
    viewport.style.transform = 'translate(' + view.x + 'px,' + view.y + 'px) scale(' + view.k + ')';
    map.classList.toggle('zoom-detail', view.k >= 1.6);
  }

  function center() {
    view.x = map.clientWidth / 2;
    view.y = map.clientHeight / 2;
    apply();
  }

  map.addEventListener('pointerdown', function (e) {
    var dev = e.target.closest('.device');
    map.setPointerCapture(e.pointerId);
    if (dev) {
      drag = {
        el: dev,
        id: dev.dataset.id,
        cx: e.clientX,
        cy: e.clientY,
        x: parseFloat(dev.style.left) || 0,
        y: parseFloat(dev.style.top) || 0,
        moved: false
      };
    } else {
      pan = { cx: e.clientX, cy: e.clientY, x: view.x, y: view.y };
    }
  });

  map.addEventListener('pointermove', function (e) {
    if (drag) {
      var dx = (e.clientX - drag.cx) / view.k;
      var dy = (e.clientY - drag.cy) / view.k;
      if (Math.abs(e.clientX - drag.cx) + Math.abs(e.clientY - drag.cy) > 4) {
        drag.moved = true;
      }
      drag.el.style.left = drag.x + dx + 'px';
      drag.el.style.top = drag.y + dy + 'px';
      updateEdges();
    } else if (pan) {
      view.x = pan.x + e.clientX - pan.cx;
      view.y = pan.y + e.clientY - pan.cy;
      apply();
    }
  });

  map.addEventListener('pointerup', function () {
    if (drag && drag.moved) {
      suppressClick = true;
      var id = drag.id;
      var x = parseFloat(drag.el.style.left);
      var y = parseFloat(drag.el.style.top);
      fetch('/devices/' + id + '/position', {
        method: 'POST',
        headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
        body: 'x=' + x + '&y=' + y
      }).then(function () {
        if (window.htmx) htmx.trigger(viewport, 'refresh');
      });
    }
    drag = null;
    pan = null;
  });

  map.addEventListener('pointercancel', function () {
    drag = null;
    pan = null;
  });

  /* Swallow the click that follows a drag so the detail panel stays shut. */
  document.addEventListener('click', function (e) {
    if (suppressClick) {
      suppressClick = false;
      e.stopPropagation();
      e.preventDefault();
    }
  }, true);

  /* Never replace the bubble layer mid-drag. */
  document.body.addEventListener('htmx:beforeSwap', function (e) {
    if (drag && e.detail.target && e.detail.target.id === 'viewport') {
      e.preventDefault();
      e.detail.shouldSwap = false;
    }
  });

  map.addEventListener('wheel', function (e) {
    e.preventDefault();
    var rect = map.getBoundingClientRect();
    var px = e.clientX - rect.left;
    var py = e.clientY - rect.top;
    var factor = e.deltaY < 0 ? 1.15 : 1 / 1.15;
    var k2 = Math.min(5, Math.max(0.25, view.k * factor));
    view.x = px - (px - view.x) * (k2 / view.k);
    view.y = py - (py - view.y) * (k2 / view.k);
    view.k = k2;
    apply();
  }, { passive: false });

  /* Keep hub edges glued to their bubbles while dragging. */
  function updateEdges() {
    var hub = document.querySelector('.device.hub');
    if (!hub) return;
    var hx = parseFloat(hub.style.left) || 0;
    var hy = parseFloat(hub.style.top) || 0;
    document.querySelectorAll('.edge').forEach(function (edge) {
      var dev = document.querySelector('.device[data-id="' + edge.id.slice(5) + '"]');
      if (!dev) return;
      var x = parseFloat(dev.style.left) || 0;
      var y = parseFloat(dev.style.top) || 0;
      edge.style.left = hx + 'px';
      edge.style.top = hy + 'px';
      edge.style.width = Math.hypot(x - hx, y - hy) + 'px';
      edge.style.transform = 'rotate(' + (Math.atan2(y - hy, x - hx) * 180 / Math.PI) + 'deg)';
    });
  }

  window.addEventListener('resize', apply);
  center();
})();

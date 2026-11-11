/* kwatch dashboard: fills the "Look at these first" card on the
   Overview. Beside Bad releases (wide screen) the two cards are as tall
   as the taller one; a short list would leave the left card half
   empty. So the card holds up to LIMIT issues, shows MIN of them, and
   adds rows while the row stays as tall as it already is. When every
   active issue is shown and there is still room, a short line at the
   bottom says so. Stacked (phone, narrow, or no bad releases) it just
   shows MIN. Refits on resize (one rAF-debounced listener) and after
   each live update (KW.lookFit(...).fit()). */
(function () {
  "use strict";
  var KW = window.KW;
  KW.LOOK_LIMIT = 8;
  var MIN = 5;
  var WIDE = "(min-width: 1100px)";

  function setShown(rows, n) {
    for (var i = 0; i < rows.length; i++) rows[i].hidden = i >= n;
  }

  /* fit: show as many rows as the neighbour's height allows. */
  function fit(pair, card, rel) {
    var ul = card.querySelector(".ov-look");
    if (!ul) return;
    var rows = ul.children, end = card.querySelector(".ov-end");
    var n = Math.min(rows.length, MIN);
    setShown(rows, n);
    if (end) end.hidden = true;
    if (rel.hidden || !window.matchMedia(WIDE).matches) return;
    var h = pair.offsetHeight;
    while (n < rows.length) {
      setShown(rows, n + 1);
      if (pair.offsetHeight > h) {
        setShown(rows, n);
        break;
      }
      n++;
    }
    if (end && n === rows.length && ul.dataset.all === "1") {
      end.hidden = false;
      if (pair.offsetHeight > h) end.hidden = true;
    }
  }

  KW.lookFit = function (pair, card, rel) {
    var frame = 0;
    function later() {
      if (frame) return;
      frame = requestAnimationFrame(function () {
        frame = 0;
        fit(pair, card, rel);
      });
    }
    window.addEventListener("resize", later);
    if (document.fonts && document.fonts.ready) document.fonts.ready.then(later);
    return {
      fit: function () { fit(pair, card, rel); },
      stop: function () { window.removeEventListener("resize", later); }
    };
  };
})();

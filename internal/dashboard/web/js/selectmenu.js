/* kwatch dashboard: every single-choice picker (start time, resolved
   within, sort, rows per page, the View options, the notifications
   level, the log container) drawn as the site's own dropdown instead
   of the browser's. The native <select> stays in the page, hidden, and
   keeps the value: code reads and sets select.value and listens for
   "change" as before, and the button follows. A select added to the
   page is picked up by itself. Styles: dropdown.css. */
(function () {
  "use strict";
  var KW = window.KW;
  var VALUE = Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype,
    "value");
  var seq = 0;

  /* nameOf: what the select is called (its label, or aria-label). */
  function nameOf(sel) {
    if (sel.getAttribute("aria-label")) return sel.getAttribute("aria-label");
    var label = sel.closest("label");
    if (!label) return "";
    var own = KW.$(".sr-only", label);
    if (own) return own.textContent.trim();
    var text = "";
    label.childNodes.forEach(function (c) {
      if (c.nodeType === 3) text += c.textContent;
    });
    return text.trim();
  }

  function enhance(sel) {
    if (sel.dataset.dd !== undefined) return;
    sel.dataset.dd = "";
    var wrap = KW.el("span", "dd dd-one");
    var trigger = KW.el("button", "dd-btn");
    trigger.type = "button";
    trigger.setAttribute("aria-haspopup", "listbox");
    var text = KW.el("span", "dd-text");
    trigger.appendChild(text);
    trigger.appendChild(KW.ddChevron());
    var list = KW.el("ul", "dd-pop dd-list dd-menu");
    list.setAttribute("role", "listbox");
    wrap.appendChild(trigger);
    wrap.appendChild(list);
    sel.parentNode.insertBefore(wrap, sel.nextSibling);
    sel.hidden = true;
    sel.tabIndex = -1;
    var id = "dd-one-" + (++seq);

    function paint() {
      var o = sel.options[sel.selectedIndex];
      var name = nameOf(sel);
      text.textContent = o ? o.textContent : "";
      trigger.setAttribute("aria-label", (name ? name + ": " : "") + text.textContent);
      list.setAttribute("aria-label", name);
      trigger.title = text.textContent;
      wrap.classList.toggle("is-set", sel.hasAttribute("data-dd-mark") &&
        sel.selectedIndex > 0);
    }

    function draw() {
      list.textContent = "";
      Array.prototype.forEach.call(sel.options, function (o, i) {
        var li = KW.el("li", "dd-opt");
        li.setAttribute("role", "option");
        li.id = id + "-" + i;
        li.tabIndex = -1;
        li.dataset.index = String(i);
        li.setAttribute("aria-selected", i === sel.selectedIndex ? "true" : "false");
        li.appendChild(KW.icon("check"));
        li.appendChild(KW.el("span", "dd-label", o.textContent));
        list.appendChild(li);
      });
    }

    var pv = KW.popover(wrap, trigger, list, {
      opened: function () {
        draw();
        var on = KW.$('[aria-selected="true"]', list) || KW.$("[role=option]", list);
        if (on) on.focus();
      }
    });

    function choose(li) {
      var i = parseInt(li.dataset.index, 10);
      pv.close(true);
      if (i === sel.selectedIndex) return;
      sel.selectedIndex = i;
      paint();
      sel.dispatchEvent(new Event("change", { bubbles: true }));
    }
    list.addEventListener("click", function (e) {
      var li = e.target.closest("[role=option]");
      if (li) choose(li);
    });
    list.addEventListener("keydown", function (e) {
      if (e.key === "Tab") {
        e.preventDefault();
        return pv.close(true);
      }
      if ((e.key === "Enter" || e.key === " ") && e.target.dataset.index) {
        e.preventDefault();
        choose(e.target);
      }
    });
    KW.listKeys(list, "[role=option]");
    Object.defineProperty(sel, "value", {
      configurable: true,
      get: function () { return VALUE.get.call(sel); },
      set: function (v) {
        VALUE.set.call(sel, v);
        paint();
      }
    });
    sel.addEventListener("change", paint);
    paint();
  }

  /* enhanceAll draws every select under root. */
  function enhanceAll(root) {
    if (root.tagName === "SELECT") return enhance(root);
    if (root.querySelectorAll) KW.$$("select", root).forEach(enhance);
  }

  new MutationObserver(function (records) {
    records.forEach(function (r) {
      r.addedNodes.forEach(function (node) {
        if (node.nodeType === 1) enhanceAll(node);
      });
    });
  }).observe(document.documentElement, { childList: true, subtree: true });
})();

/* kwatch dashboard: dropdowns. KW.popover opens a panel under a button
   (closes on Esc or a click outside, keeps Tab inside while open and
   gives focus back to the button). KW.listKeys moves
   through a list with the arrow keys, Home and End. KW.multiSelect is
   the multi-select picker of the Issues filters ("Namespace · 3"; the
   values picked show once, as chips under the bar): a search box for long lists, a checkbox per value with
   its count, "Select all" and "Clear". The single-value pickers are in
   selectmenu.js. Styles: dropdown.css. */
(function () {
  "use strict";
  var KW = window.KW;
  var seq = 0;
  /* SHOW caps the values drawn at once; the search finds the rest. */
  var SHOW = 100;

  var n = function (x) { return KW.num(x); };

  function focusables(root) {
    return KW.$$("button, input, [tabindex]", root).filter(function (e) {
      return !e.disabled && e.tabIndex >= 0 && e.offsetParent !== null;
    });
  }

  /* popover wires trigger to open pop (a sibling inside wrap). hooks:
     opened() after it opens, closed() after it closes. */
  KW.popover = function (wrap, trigger, pop, hooks) {
    hooks = hooks || {};
    pop.id = pop.id || "dd-pop-" + (++seq);
    trigger.setAttribute("aria-expanded", "false");
    trigger.setAttribute("aria-controls", pop.id);
    pop.hidden = true;
    var api = {
      isOpen: function () { return !pop.hidden; },
      open: function () {
        if (!pop.hidden) return;
        pop.hidden = false;
        trigger.setAttribute("aria-expanded", "true");
        wrap.classList.add("is-open");
        document.addEventListener("pointerdown", outside, true);
        if (hooks.opened) hooks.opened();
        place();
      },
      close: function (refocus) {
        if (pop.hidden) return;
        pop.hidden = true;
        trigger.setAttribute("aria-expanded", "false");
        wrap.classList.remove("is-open");
        document.removeEventListener("pointerdown", outside, true);
        if (refocus) trigger.focus();
        if (hooks.closed) hooks.closed();
      }
    };
    /* place keeps the panel on screen: right-aligned when it would
       pass the right edge. */
    function place() {
      pop.classList.remove("dd-right");
      var r = pop.getBoundingClientRect();
      if (r.right > document.documentElement.clientWidth - 8) {
        pop.classList.add("dd-right");
      }
    }
    function outside(e) {
      if (!wrap.contains(e.target)) api.close(false);
    }
    trigger.addEventListener("click", function () {
      if (api.isOpen()) api.close(true); else api.open();
    });
    trigger.addEventListener("keydown", function (e) {
      if (e.key !== "ArrowDown" || api.isOpen()) return;
      e.preventDefault();
      api.open();
    });
    pop.addEventListener("keydown", function (e) {
      if (e.key === "Escape") {
        e.preventDefault();
        e.stopPropagation();
        return api.close(true);
      }
      if (e.key !== "Tab") return;
      var all = focusables(pop);
      if (!all.length) return;
      var first = all[0], last = all[all.length - 1];
      if (e.shiftKey && document.activeElement === first) {
        e.preventDefault();
        last.focus();
      } else if (!e.shiftKey && document.activeElement === last) {
        e.preventDefault();
        first.focus();
      }
    });
    return api;
  };

  /* listKeys: arrow keys, Home and End move the focus through the
     items of list (selector item); escape(e) runs when Up leaves the
     first item. */
  KW.listKeys = function (list, item, escapeUp) {
    list.addEventListener("keydown", function (e) {
      var items = KW.$$(item, list).filter(function (x) { return !x.hidden; });
      var at = items.indexOf(document.activeElement);
      var to = { ArrowDown: at + 1, ArrowUp: at - 1, Home: 0,
        End: items.length - 1 }[e.key];
      if (to === undefined || at < 0) return;
      e.preventDefault();
      if (to < 0 && escapeUp) return escapeUp();
      if (items[Math.max(0, Math.min(items.length - 1, to))]) {
        items[Math.max(0, Math.min(items.length - 1, to))].focus();
      }
    });
  };

  function chevron() {
    var i = KW.icon("chevron");
    i.classList.add("dd-chev");
    return i;
  }
  KW.ddChevron = chevron;

  /* box: the drawn checkbox of an option. */
  function box() {
    var b = KW.el("span", "dd-box");
    b.appendChild(KW.icon("check"));
    return b;
  }
  KW.ddBox = box;

  /* multiSelect builds a picker. cfg: name ("Namespace"), noun
     ("namespace", for the search box), all (the button with nothing
     picked: "Any namespace"), mono (values in the code font), search
     (always show the search box), onChange(values). It returns el, and
     set(values) for what is picked and items(list) for the values
     offered: [{value, label, count, title}], most issues first. */
  KW.multiSelect = function (cfg) {
    var wrap = KW.el("div", "dd dd-multi");
    var trigger = KW.el("button", "dd-btn");
    trigger.type = "button";
    trigger.setAttribute("aria-haspopup", "listbox");
    var text = KW.el("span", "dd-text");
    trigger.appendChild(text);
    trigger.appendChild(chevron());
    var pop = KW.el("div", "dd-pop");
    var find = KW.el("input", "dd-find");
    find.type = "search";
    find.autocomplete = "off";
    find.placeholder = "Find a " + (cfg.noun || cfg.name.toLowerCase());
    find.setAttribute("aria-label", "Find a " + (cfg.noun || cfg.name));
    var list = KW.el("ul", "dd-list");
    list.setAttribute("role", "listbox");
    list.setAttribute("aria-multiselectable", "true");
    list.setAttribute("aria-label", cfg.name);
    var note = KW.el("p", "dd-note");
    note.setAttribute("role", "status");
    var foot = KW.el("div", "dd-foot");
    var allBtn = KW.el("button", "link", "Select all");
    var noneBtn = KW.el("button", "link", "Clear");
    allBtn.type = noneBtn.type = "button";
    foot.appendChild(allBtn);
    foot.appendChild(noneBtn);
    pop.appendChild(find);
    pop.appendChild(list);
    pop.appendChild(note);
    pop.appendChild(foot);
    wrap.appendChild(trigger);
    wrap.appendChild(pop);

    var picked = [], offered = [], order = null;
    var pv = KW.popover(wrap, trigger, pop, {
      opened: function () {
        order = {};
        all().forEach(function (x, i) { order[x.value] = i; });
        find.value = "";
        draw();
        var on = KW.$('[aria-selected="true"]', list) || KW.$("[role=option]", list);
        (find.hidden ? on : find).focus();
      },
      closed: function () { order = null; }
    });

    /* all: the values offered plus the picked ones not among them. */
    function all() {
      var out = offered.slice();
      picked.forEach(function (v) {
        if (!out.some(function (x) { return x.value === v; })) {
          out.push({ value: v, label: v, count: 0 });
        }
      });
      if (order) {
        out.sort(function (a, b) {
          var x = order[a.value], y = order[b.value];
          return (x === undefined ? 1e9 : x) - (y === undefined ? 1e9 : y);
        });
      }
      return out;
    }

    function label(v) {
      var x = all().filter(function (o) { return o.value === v; })[0];
      return x ? x.label : v;
    }

    function paint() {
      wrap.classList.toggle("is-set", picked.length > 0);
      text.textContent = "";
      if (!picked.length) {
        text.textContent = cfg.all;
      } else {
        text.appendChild(KW.el("span", "dd-name", cfg.name));
        text.appendChild(KW.el("b", "dd-count", n(picked.length)));
      }
      trigger.title = picked.length ? cfg.name + ": " +
        picked.map(label).join(", ") : cfg.all;
    }

    function option(x) {
      var li = KW.el("li", "dd-opt");
      li.setAttribute("role", "option");
      li.tabIndex = -1;
      li.dataset.value = x.value;
      var on = picked.indexOf(x.value) >= 0;
      li.setAttribute("aria-selected", on ? "true" : "false");
      li.appendChild(box());
      li.appendChild(KW.el("span", "dd-label" + (cfg.mono && x.value ? " mono" : ""),
        x.label));
      li.appendChild(KW.el("span", "dd-n", x.count === undefined ||
        x.count === null ? "" : n(x.count)));
      li.title = x.title || x.label;
      return li;
    }

    /* draw lists the values matching the search, at most SHOW. */
    function draw() {
      var had = document.activeElement && list.contains(document.activeElement) ?
        document.activeElement.dataset.value : null;
      var top = list.scrollTop, q = find.value.trim().toLowerCase();
      var every = all();
      find.hidden = !cfg.search && every.length <= 8;
      var match = every.filter(function (x) {
        return !q || x.label.toLowerCase().indexOf(q) >= 0;
      });
      list.textContent = "";
      match.slice(0, SHOW).forEach(function (x) { list.appendChild(option(x)); });
      // The list is one Tab stop; the arrow keys move inside it.
      if (list.firstChild) list.firstChild.tabIndex = 0;
      note.textContent = !match.length ? "Nothing matches." :
        match.length > SHOW ? "Showing " + SHOW + " of " + n(match.length) +
          ". Type to find the rest." : "";
      note.hidden = !note.textContent;
      list.scrollTop = top;
      var back = had !== null && KW.$$("[role=option]", list).filter(function (li) {
        return li.dataset.value === had;
      })[0];
      if (back) back.focus({ preventScroll: true });
    }

    function toggle(li) {
      var v = li.dataset.value, at = picked.indexOf(v);
      if (at >= 0) picked.splice(at, 1); else picked.push(v);
      li.setAttribute("aria-selected", at >= 0 ? "false" : "true");
      paint();
      cfg.onChange(picked.slice());
    }

    list.addEventListener("click", function (e) {
      var li = e.target.closest("[role=option]");
      if (li) toggle(li);
    });
    list.addEventListener("keydown", function (e) {
      if ((e.key === " " || e.key === "Enter") && e.target.dataset.value !== undefined) {
        e.preventDefault();
        toggle(e.target);
      }
    });
    KW.listKeys(list, "[role=option]", function () {
      if (!find.hidden) find.focus();
    });
    find.addEventListener("input", draw);
    find.addEventListener("keydown", function (e) {
      var first = KW.$("[role=option]", list);
      if (e.key === "ArrowDown" && first) {
        e.preventDefault();
        first.focus();
      } else if (e.key === "Enter" && first) {
        e.preventDefault();
        toggle(first);
      }
    });
    allBtn.addEventListener("click", function () {
      KW.$$("[role=option]", list).forEach(function (li) {
        if (picked.indexOf(li.dataset.value) < 0) picked.push(li.dataset.value);
      });
      draw();
      paint();
      cfg.onChange(picked.slice());
    });
    noneBtn.addEventListener("click", function () {
      picked = [];
      draw();
      paint();
      cfg.onChange([]);
    });

    paint();
    return {
      el: wrap,
      set: function (values) {
        picked = values.slice();
        paint();
        if (pv.isOpen()) draw();
      },
      items: function (list2) {
        offered = list2;
        paint();
        if (pv.isOpen()) draw();
      },
      close: function () { pv.close(false); }
    };
  };
})();

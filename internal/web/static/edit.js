// Card page editing without save buttons. Every field is still the ordinary
// form it was, posting to the same route, so post/redirect/get, the
// same-origin guard and REL-1 (no success claimed before the write is on disk)
// are untouched: this file only decides when a form submits. A form marked
// data-autosave submits when one of its fields is left with a changed value;
// Enter submits a one-line field as it always did; ⌘S or Ctrl+S submits the
// notes; Esc puts a field back. Outside the fields, the TUI's detail-screen
// keys work here too. Without this file every form keeps its button.
//
// It loads in <head>, unlike live.js and dnd.js, so the "js" class is on the
// page before its first paint. The buttons it hides would otherwise flash on
// every load, and every save is a load.
(function () {
  document.documentElement.classList.add("js");

  function isField(el) {
    return !!el && (el.tagName === "INPUT" || el.tagName === "TEXTAREA" || el.tagName === "SELECT");
  }

  // live.js reloads the page when the board file changes, and a save is such
  // a change. Marking the body busy keeps that reload from landing ahead of
  // this page's own navigation and dropping the edit: the flag dnd.js sets for
  // a drop. The navigation replaces the document, flag and all.
  var submitting = false;
  function markBusy() {
    submitting = true;
    document.body.setAttribute("data-busy", "");
  }

  function submit(form) {
    if (submitting) return;
    markBusy();
    form.submit();
  }

  // Enter, and a button where one still shows, submit through the browser; a
  // submit event fires only for a form that passed validation.
  document.addEventListener("submit", markBusy);

  document.addEventListener("change", function (e) {
    var el = e.target;
    var form = el.form;
    if (!form || !form.hasAttribute("data-autosave") || submitting) return;
    // The server refuses an empty title or checklist item with a 400; put the
    // field back instead of posting one.
    if (el.required && el.value.trim() === "") {
      el.value = el.defaultValue;
      return;
    }
    submit(form);
  });

  function focusEnd(el) {
    if (!el) return;
    el.focus();
    try {
      el.setSelectionRange(el.value.length, el.value.length);
    } catch (err) {}
  }

  function follow(selector) {
    var a = document.querySelector(selector);
    if (a) location.href = a.getAttribute("href");
  }

  // J and K open the next and previous card of the lane list, wrapping, as on
  // the TUI's detail screen. With no row current, J starts at the top and K at
  // the bottom.
  function step(delta) {
    var links = document.querySelectorAll(".siblings a[href]");
    if (!links.length) return;
    var cur = -1;
    for (var i = 0; i < links.length; i++) {
      if (links[i].getAttribute("aria-current") === "page") cur = i;
    }
    var next = cur < 0 ? (delta > 0 ? 0 : links.length - 1) : (cur + delta + links.length) % links.length;
    location.href = links[next].getAttribute("href");
  }

  var keys = {
    Escape: function () { follow("[data-key-back]"); },
    T: function () { focusEnd(document.querySelector("[data-key-title]")); },
    t: function () { focusEnd(document.querySelector("[data-key-tag]")); },
    e: function () { focusEnd(document.querySelector("[data-key-notes]")); },
    o: function () { focusEnd(document.querySelector("[data-key-new]")); },
    b: function () { focusEnd(document.querySelector("[data-key-block]")); },
    m: function () {
      var el = document.querySelector("[data-key-move]");
      if (!el) return;
      el.focus();
      try {
        el.showPicker();
      } catch (err) {}
    },
    d: function () {
      var form = document.querySelector("form[data-key-done]");
      if (form) submit(form);
    },
    J: function () { step(1); },
    K: function () { step(-1); }
  };

  document.addEventListener("keydown", function (e) {
    if (e.isComposing || !document.querySelector("[data-card-page]")) return;
    var el = e.target;
    if (isField(el)) {
      if (e.key === "Escape") {
        e.preventDefault();
        if (el.tagName !== "SELECT") el.value = el.defaultValue;
        el.blur();
      } else if (el.tagName === "TEXTAREA" && (e.metaKey || e.ctrlKey) && (e.key === "s" || e.key === "S")) {
        e.preventDefault();
        if (el.value !== el.defaultValue && el.form) submit(el.form);
      }
      return;
    }
    if (e.metaKey || e.ctrlKey || e.altKey || submitting) return;
    if (!Object.prototype.hasOwnProperty.call(keys, e.key)) return;
    e.preventDefault();
    keys[e.key]();
  });
})();

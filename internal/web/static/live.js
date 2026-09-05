// Live reload: the server says "board-changed", the page re-fetches itself.
// Served from a file rather than inlined so the Content-Security-Policy can
// stay at script-src 'self' instead of allowing inline script.
(function () {
  var board = document.body.dataset.board;
  if (!board || !window.EventSource) return;
  var source = new EventSource("/b/" + encodeURIComponent(board) + "/events");
  var pending = false;

  // Reload a page the user is reading, but never one mid-edit or mid-drag:
  // a focused field means unsaved text a reload would throw away, and a
  // reload during a gesture cancels it — dnd.js marks a drag in flight. The
  // reload is deferred, not dropped — the server sends one event per change,
  // so forgetting it would leave the tab stale until some later, unrelated
  // edit happened to arrive while nothing was busy.
  function busy() {
    if (document.body.hasAttribute("data-busy")) return true;
    var el = document.activeElement;
    return !!el && (el.tagName === "INPUT" || el.tagName === "TEXTAREA" || el.tagName === "SELECT");
  }

  function reload() {
    if (busy()) {
      pending = true;
      return;
    }
    pending = false;
    location.reload();
  }

  // After the browser has moved focus or finished the drag, so busy() sees
  // where things landed. The deferral is what makes the timeout load-bearing:
  // dragend bubbles to document, and if this ran before dnd.js cleared the
  // flag the reload would defer itself again, forever.
  function flush() {
    if (pending) setTimeout(reload, 0);
  }

  source.addEventListener("board-changed", reload);
  document.addEventListener("focusout", flush);
  document.addEventListener("dragend", flush);
})();

// Live reload: the server says "board-changed", the page re-fetches itself.
// Served from a file rather than inlined so the Content-Security-Policy can
// stay at script-src 'self' instead of allowing inline script.
(function () {
  var board = document.body.dataset.board;
  if (!board || !window.EventSource) return;
  var source = new EventSource("/b/" + encodeURIComponent(board) + "/events");
  var pending = false;

  // Reload a page the user is reading, but never one mid-edit: a focused
  // field means unsaved text a reload would throw away. The reload is
  // deferred, not dropped — the server sends one event per change, so
  // forgetting it would leave the tab stale until some later, unrelated
  // edit happened to arrive while nothing was focused.
  function editing() {
    var el = document.activeElement;
    return !!el && (el.tagName === "INPUT" || el.tagName === "TEXTAREA" || el.tagName === "SELECT");
  }

  function reload() {
    if (editing()) {
      pending = true;
      return;
    }
    pending = false;
    location.reload();
  }

  source.addEventListener("board-changed", reload);
  document.addEventListener("focusout", function () {
    // After the browser has moved focus, so editing() sees where it landed.
    if (pending) setTimeout(reload, 0);
  });
})();

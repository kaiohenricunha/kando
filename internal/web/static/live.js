// Live reload: the server says "board-changed", the page re-fetches itself.
// Served from a file rather than inlined so the Content-Security-Policy can
// stay at script-src 'self' instead of allowing inline script.
(function () {
  var board = document.body.dataset.board;
  if (!board || !window.EventSource) return;
  var source = new EventSource("/b/" + encodeURIComponent(board) + "/events");
  source.addEventListener("board-changed", function () {
    // Only reload a page the user is reading, not one mid-edit: a focused
    // input means unsaved text that a reload would throw away.
    var el = document.activeElement;
    if (el && (el.tagName === "INPUT" || el.tagName === "TEXTAREA" || el.tagName === "SELECT")) return;
    location.reload();
  });
})();

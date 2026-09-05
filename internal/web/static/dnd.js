// Drag-and-drop on the board page: pick a card up, drop it where the line
// shows. The drop is a real form POST to the same /move route the card
// detail page's lane picker uses, so post/redirect/get, the same-origin
// guard and the Content-Security-Policy all stay exactly as they are — this
// file moves the gesture, never the board. Served from a file rather than
// inlined so the CSP can stay at script-src 'self'.
(function () {
  var lanes = document.querySelectorAll(".lane[data-lane]");
  if (!lanes.length) return;

  var dragged = null; // the card being dragged
  var drop = null; // where it would land: {pos: "before"|"after"|"start", id}
  var lane = null; // the lane the pointer is over
  var submitting = false; // a move has been posted; this page is about to navigate away

  // Where a drop at y lands, named against a card the user can actually see.
  // Never an index: this page may be filtered, so the last card on screen is
  // not the last card in the lane, and "after the last one I can see" is the
  // only reading of that gesture the server can honour.
  //
  // The dragged card is deliberately not skipped. Hovering either half of
  // its own box then names a position it already holds, which the server
  // resolves to a no-op — no special case here or there.
  function landing(section, y) {
    var cards = section.querySelectorAll(".card[data-id]");
    for (var i = 0; i < cards.length; i++) {
      var r = cards[i].getBoundingClientRect();
      if (y < r.top + r.height / 2) return { pos: "before", id: cards[i].dataset.id };
    }
    if (cards.length) return { pos: "after", id: cards[cards.length - 1].dataset.id };
    return { pos: "start" };
  }

  // paint is the whole visual state: one drop line, one highlighted lane. It
  // clears everything first, so a pointer crossing lanes never leaves a line
  // behind in the one it left, and so dragleave never has to be tracked —
  // that event also fires when the pointer crosses into a child, which makes
  // any highlight it drives strobe.
  function paint() {
    var marked = document.querySelectorAll(".drop-here, .drop-end, .drag-over");
    for (var i = 0; i < marked.length; i++) {
      marked[i].classList.remove("drop-here", "drop-end", "drag-over");
    }
    if (!lane || !drop) return;
    lane.classList.add("drag-over");
    if (drop.pos === "before") {
      var card = lane.querySelector('.card[data-id="' + CSS.escape(drop.id) + '"]');
      if (card) card.classList.add("drop-here");
    } else {
      lane.classList.add("drop-end");
    }
  }

  function reset() {
    if (dragged) dragged.classList.remove("dragging");
    dragged = drop = lane = null;
    paint();
  }

  function hidden(form, name, value) {
    var input = document.createElement("input");
    input.type = "hidden";
    input.name = name;
    input.value = value;
    form.appendChild(input);
  }

  // submit posts the move. The action comes from the card's own href, which
  // the server already escaped with the same rule it will parse the path
  // with — building it here from a board name and a card id would mean
  // reimplementing Go's url.PathEscape in JavaScript, and
  // encodeURIComponent is not the same function.
  function submit(card, key, where) {
    var form = document.createElement("form");
    form.method = "post";
    form.action = card.getAttribute("href") + "/move";
    hidden(form, "lane", key);
    hidden(form, "pos", where.pos);
    if (where.id) hidden(form, "anchor", where.id);
    // The redirect goes back to this board; carry the filter so it comes
    // back to the view the card was dragged on.
    var q = new URLSearchParams(location.search).get("q");
    if (q) hidden(form, "q", q);
    document.body.appendChild(form);
    form.submit();
  }

  document.addEventListener("dragstart", function (e) {
    var card = e.target.closest && e.target.closest(".card[data-id]");
    if (!card) return;
    dragged = card;
    card.classList.add("dragging");
    // A reload mid-drag cancels the gesture, and a native drag cannot be
    // restarted programmatically. live.js waits while this is set; the
    // dragend listener below decides when it is actually safe to clear.
    document.body.setAttribute("data-busy", "");
    if (e.dataTransfer) {
      e.dataTransfer.effectAllowed = "move";
      // Firefox will not start a drag without this, and a card is an <a>,
      // which the browser would otherwise drag (and drop) as its link.
      e.dataTransfer.setData("text/plain", card.dataset.id);
    }
  });

  document.addEventListener("dragend", function () {
    reset();
    // A submit already under way is about to navigate this page away.
    // Clearing the flag here would let live.js's own dragend listener
    // (registered first, so it runs first) schedule a reload that could beat
    // the POST's own navigation and silently drop the move — dragend fires
    // right after drop, well before a same-origin POST's response returns.
    // Leave it set; the navigation replaces the whole document, flag
    // included. An abandoned drag (no submit) still clears it normally.
    if (!submitting) document.body.removeAttribute("data-busy");
  });

  for (var i = 0; i < lanes.length; i++) {
    lanes[i].addEventListener("dragover", function (e) {
      // A drag that did not start here — a file, a link from another tab —
      // is left to the browser: no preventDefault, so it is not a drop
      // target at all.
      if (!dragged) return;
      e.preventDefault(); // without this the lane never receives a drop
      if (e.dataTransfer) e.dataTransfer.dropEffect = "move";
      lane = e.currentTarget;
      drop = landing(lane, e.clientY);
      paint();
    });

    lanes[i].addEventListener("drop", function (e) {
      if (!dragged) return;
      // The source is a link, so without this the browser navigates to it.
      e.preventDefault();
      var card = dragged;
      var key = e.currentTarget.dataset.lane;
      var where = drop;
      reset();
      if (where) {
        submitting = true;
        submit(card, key, where);
      }
    });
  }
})();

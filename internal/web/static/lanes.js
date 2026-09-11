// One lane at a time on a narrow screen. The stylesheet lays the lanes out
// side by side in a strip that scrolls sideways and snaps to each lane; this
// file keeps the tab strip and the add button in step with the lane in view,
// scrolls the strip when a tab is tapped, and opens on the lane this tab last
// showed — so a live reload does not throw the user back to the start — or
// else on Todo, the TUI's starting lane. It never posts anything. On a wide
// screen every lane is already on screen: the tabs are hidden and it only
// tracks, never scrolls.
(function () {
  var strip = document.querySelector(".lanes");
  var tabs = document.querySelectorAll(".tabs a[data-tab]");
  if (!strip || !tabs.length) return;
  var add = document.querySelector("[data-dock-add]");
  var narrow = window.matchMedia("(max-width: 720px)");
  var memory = "kando-lane:" + (document.body.dataset.board || "");

  function lane(key) {
    return document.getElementById("lane-" + key);
  }

  // Where a lane's left edge sits in the strip's scroll range.
  function offset(el) {
    return el.getBoundingClientRect().left - strip.getBoundingClientRect().left + strip.scrollLeft;
  }

  // show marks key's tab current and points the add button at key's lane,
  // using that lane's own add link so no URL is ever built here.
  function show(key) {
    var el = lane(key);
    if (!el) return;
    for (var i = 0; i < tabs.length; i++) {
      tabs[i].setAttribute("aria-current", tabs[i].dataset.tab === key ? "true" : "false");
    }
    var link = el.querySelector(".add");
    if (add && link) {
      add.setAttribute("href", link.getAttribute("href"));
      add.textContent = "+ add to " + el.dataset.label;
    }
    try {
      sessionStorage.setItem(memory, key);
    } catch (err) {}
  }

  // The lane in view is the one whose left edge is nearest the strip's.
  function inView() {
    var lanes = strip.querySelectorAll(".lane[data-lane]");
    var left = strip.getBoundingClientRect().left;
    var best = null;
    var dist = Infinity;
    for (var i = 0; i < lanes.length; i++) {
      var d = Math.abs(lanes[i].getBoundingClientRect().left - left);
      if (d < dist) {
        dist = d;
        best = lanes[i];
      }
    }
    return best && best.dataset.lane;
  }

  var queued = false;
  strip.addEventListener(
    "scroll",
    function () {
      if (queued || !narrow.matches) return;
      queued = true;
      requestAnimationFrame(function () {
        queued = false;
        var key = inView();
        if (key) show(key);
      });
    },
    { passive: true }
  );

  for (var i = 0; i < tabs.length; i++) {
    tabs[i].addEventListener("click", function (e) {
      var el = lane(this.dataset.tab);
      if (!narrow.matches || !el) return;
      e.preventDefault();
      strip.scrollTo({ left: offset(el), behavior: "smooth" });
      show(this.dataset.tab);
    });
  }

  var start = null;
  var hash = /^#lane-([a-z]+)$/.exec(location.hash);
  if (hash && lane(hash[1])) start = hash[1];
  if (!start) {
    try {
      start = sessionStorage.getItem(memory);
    } catch (err) {}
  }
  if (!start || !lane(start)) start = "todo";
  if (narrow.matches && lane(start)) strip.scrollLeft = offset(lane(start));
  show(start);
})();

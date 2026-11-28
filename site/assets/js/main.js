(function () {
  "use strict";

  var REPO = "LIRUILONGS/aiops-kernel";

  /* current year */
  var yearEl = document.getElementById("year");
  if (yearEl) yearEl.textContent = String(new Date().getFullYear());

  /* header shadow on scroll */
  var header = document.querySelector(".site-header");
  var onScroll = function () {
    if (!header) return;
    header.classList.toggle("scrolled", window.scrollY > 8);
  };
  window.addEventListener("scroll", onScroll, { passive: true });
  onScroll();

  /* mobile nav */
  var toggle = document.getElementById("navToggle");
  if (toggle && header) {
    toggle.addEventListener("click", function () {
      var open = header.classList.toggle("open");
      toggle.setAttribute("aria-expanded", open ? "true" : "false");
      toggle.textContent = open ? "✕" : "☰";
    });
    document.querySelectorAll("#navLinks a").forEach(function (a) {
      a.addEventListener("click", function () {
        header.classList.remove("open");
        toggle.textContent = "☰";
      });
    });
  }

  /* GitHub repo stats */
  function formatCount(n) {
    if (typeof n !== "number" || !isFinite(n)) return "—";
    if (n >= 1000) {
      var k = n / 1000;
      return (k >= 10 ? Math.round(k) : Math.round(k * 10) / 10) + "k";
    }
    return String(n);
  }

  function paintStats(stars, forks) {
    document.querySelectorAll("[data-stars]").forEach(function (el) {
      el.textContent = stars == null ? (el.classList.contains("count") ? "GitHub" : "—") : formatCount(stars);
    });
    document.querySelectorAll("[data-forks]").forEach(function (el) {
      el.textContent = forks == null ? "—" : formatCount(forks);
    });
  }

  if (window.fetch) {
    fetch("https://api.github.com/repos/" + REPO, { headers: { Accept: "application/vnd.github+json" } })
      .then(function (r) { return r.ok ? r.json() : null; })
      .then(function (data) {
        if (!data) return paintStats(null, null);
        paintStats(data.stargazers_count, data.forks_count);
      })
      .catch(function () { paintStats(null, null); });
  } else {
    paintStats(null, null);
  }

  /* product tour tabs */
  var captions = {
    dashboard: "<b>概览。</b> 会话活动、工具调用与 Token 用量按天铺开——哪几天真在干活，一眼看得出。",
    flows: "<b>会话列表。</b> 标题就是告警原文，状态、耗时与终端镜像都在列表上。",
    conclusion: "<b>会话详情。</b> 一份真跑出来的收尾报告：证据缺口逐条列明，置信度 0.60，<code>needs_human=true</code>。",
    roles: "<b>角色视图。</b> 十二个角色按层摆放，节点带各自的 chain 数、日志条数与 token 用量。"
  };

  var tabs = Array.prototype.slice.call(document.querySelectorAll(".tour-tab"));
  var shots = Array.prototype.slice.call(document.querySelectorAll(".tour-stage img"));
  var captionEl = document.getElementById("tourCaption");

  function activateShot(key) {
    tabs.forEach(function (t) {
      t.classList.toggle("active", t.getAttribute("data-shot") === key);
    });
    shots.forEach(function (img) {
      img.classList.toggle("active", img.getAttribute("data-key") === key);
    });
    if (captionEl && captions[key]) captionEl.innerHTML = captions[key];
  }

  tabs.forEach(function (tab) {
    tab.addEventListener("click", function () {
      activateShot(tab.getAttribute("data-shot"));
    });
  });

  /* copy buttons */
  document.querySelectorAll(".copy-btn").forEach(function (btn) {
    btn.addEventListener("click", function () {
      var target = document.getElementById(btn.getAttribute("data-copy"));
      if (!target) return;
      var text = target.innerText;
      var done = function () {
        var old = btn.textContent;
        btn.textContent = "已复制";
        btn.classList.add("copied");
        setTimeout(function () {
          btn.textContent = old;
          btn.classList.remove("copied");
        }, 1600);
      };
      if (navigator.clipboard && navigator.clipboard.writeText) {
        navigator.clipboard.writeText(text).then(done).catch(done);
      } else {
        var ta = document.createElement("textarea");
        ta.value = text;
        ta.style.position = "fixed";
        ta.style.opacity = "0";
        document.body.appendChild(ta);
        ta.select();
        try { document.execCommand("copy"); } catch (e) {}
        document.body.removeChild(ta);
        done();
      }
    });
  });

  /* scroll reveal */
  if ("IntersectionObserver" in window) {
    var io = new IntersectionObserver(function (entries) {
      entries.forEach(function (entry) {
        if (entry.isIntersecting) {
          entry.target.classList.add("in");
          io.unobserve(entry.target);
        }
      });
    }, { threshold: 0.12, rootMargin: "0px 0px -40px 0px" });
    document.querySelectorAll(".reveal").forEach(function (el) { io.observe(el); });
  } else {
    document.querySelectorAll(".reveal").forEach(function (el) { el.classList.add("in"); });
  }
})();

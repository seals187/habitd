// Alpine components (CSP build: all logic lives here), push and the service worker.
const readConfig = (el) => JSON.parse(el.dataset.config || "{}");

document.addEventListener("alpine:init", () => {
  const multipliers = { easy: 10, medium: 15, hard: 25 };
  const measureDefaults = { count: { target: "8", step: "1" }, amount: { target: "10000", step: "1000" } };
  const stepSize = (n) => 10 ** Math.floor(Math.log10(n) + 1e-9);
  const tidy = (n) => Math.round(n * 1000) / 1000;

  Alpine.data("habitForm", () => ({
    kind: "build",
    difficulty: "easy",
    measure: "check",
    target: "8",
    step: "1",
    edited: false,
    goalPeriod: "day",
    init() {
      Object.assign(this, readConfig(this.$el));
      this.$watch("measure", (m) => {
        if (!this.edited && measureDefaults[m]) Object.assign(this, measureDefaults[m]);
      });
    },
    get goalLabel() {
      return this.goalPeriod === "week" ? "Wochenziel" : "Tagesziel";
    },
    get payLabel() {
      return this.goalPeriod === "week" && this.measure !== "check" ? "pro Woche" : "pro Tag";
    },
    get goalHint() {
      return "XP, Taler und Serie gibt es, sobald das " + this.goalLabel + " erreicht ist.";
    },
    setGoal(target, step) {
      this.edited = true;
      this.step = String(tidy(step));
      this.target = String(tidy(Math.max(target, step)));
    },
    incTarget() {
      this.setGoal(Number(this.target) + Number(this.step), Number(this.step));
    },
    decTarget() {
      this.setGoal(Number(this.target) - Number(this.step), Number(this.step));
    },
    incStep() {
      const step = Number(this.step);
      this.setGoal(Number(this.target), step + stepSize(step));
    },
    decStep() {
      const step = Number(this.step);
      const size = stepSize(step);
      this.setGoal(Number(this.target), Math.max(step - (step === size ? size / 10 : size), 0.001));
    },
    get xp() {
      return this.kind === "avoid" ? 20 : multipliers[this.difficulty];
    },
    get xpLabel() {
      return "+" + this.xp + " XP";
    },
    get coinLabel() {
      return "+" + Math.round(this.xp / 3) + " Taler";
    },
  }));

  // Counts ticked boxes for the button label in the onboarding.
  Alpine.data("picker", () => ({
    count: 0,
    init() {
      this.update();
    },
    update() {
      this.count = this.$root.querySelectorAll("input[type=checkbox]:checked").length;
    },
    get label() {
      const d = this.$root.dataset;
      if (this.count === 0) return d.empty;
      return d.prefix + " · " + this.count + " " + (this.count === 1 ? d.one : d.many);
    },
    get blocked() {
      return this.count === 0 && this.$root.dataset.required === "true";
    },
  }));

  // Live preview of the badge form.
  const units = { streak: "Tage", count: "Check-ins", amount: "Einheiten" };
  const scopes = { any_habit: "habit: any", habit_tag: "tag", area: "area", kind_build: "habit: build", kind_avoid: "habit: avoid" };

  Alpine.data("badgeEditor", () => ({
    name: "",
    icon: "flame",
    color: "",
    category: "",
    catColors: {},
    rule: "streak",
    scope: "any_habit",
    value: "",
    tag: "",
    area: "",
    target: "1",
    jokers: false,
    key: "",
    sprite: "",
    fixedHint: "",
    init() {
      Object.assign(this, readConfig(this.$el));
      if (this.scope === "habit_tag") this.tag = this.value;
      if (this.scope === "area") this.area = this.value;
    },
    get unit() {
      return units[this.rule] || "";
    },
    get accent() {
      return "--c:" + (this.color || this.catColors[this.category] || "#A3A69F");
    },
    get iconRef() {
      return this.sprite + "#" + this.icon;
    },
    get hint() {
      return this.rule === "special" ? this.fixedHint : this.target + " " + this.unit;
    },
    get formula() {
      if (this.rule === "special") return "special(" + this.key + ") >= " + this.target;
      let scope = scopes[this.scope] || "";
      if (this.scope === "habit_tag") scope += ": " + this.tag;
      if (this.scope === "area") scope += ": " + this.area;
      return this.rule + "(" + scope + ") >= " + this.target;
    },
  }));

  // Live preview of the quest form.
  Alpine.data("questEditor", () => ({
    name: "",
    text: "",
    icon: "sun",
    color: "#F2C14E",
    kind: "until",
    count: "3",
    until: "12:00",
    percent: "120",
    rewardKind: "coins",
    reward: "30",
    sprite: "",
    init() {
      Object.assign(this, readConfig(this.$el));
    },
    get accent() {
      return "--c:" + this.color;
    },
    get iconRef() {
      return this.sprite + "#" + this.icon;
    },
    get label() {
      const count = Number(this.count) > 0 ? this.count : "alle";
      return this.text.replaceAll("{{anzahl}}", count).replaceAll("{{uhrzeit}}", this.until).replaceAll("{{prozent}}", this.percent);
    },
    get rewardLabel() {
      const unit = { coins: "Taler", xp: "XP", joker: "Joker" }[this.rewardKind];
      return "+" + this.reward + " " + unit;
    },
    get formula() {
      const need = Number(this.count) > 0 ? this.count : "all";
      switch (this.kind) {
        case "until":
          return "count(checkins where time < " + this.until + ") >= " + need;
        case "area":
          return "count(checkins where area) >= " + need;
        case "counter":
          return "any(counter >= " + this.percent + "%)";
        case "catchup":
          return "any(checkin where missed yesterday)";
        case "avoid":
          return "count(relapses) = 0";
      }
      return "count(checkins) >= all";
    },
  }));

  Alpine.data("rewardForm", () => ({
    mode: "coins",
    price: "250",
    target: "30",
    habit: "0",
    rate: 0,
    days: {},
    init() {
      const c = readConfig(this.$el);
      this.mode = c.mode;
      this.price = String(c.price);
      this.target = String(c.target);
      this.rate = c.rate;
      this.days = c.days;
      const ids = Object.keys(c.days);
      this.habit = c.habit !== "0" ? c.habit : ids[0] || "0";
    },
    step() {
      return Number(this.price) >= 100 ? 50 : 10;
    },
    inc() {
      this.price = String(Number(this.price) + this.step());
    },
    dec() {
      this.price = String(Math.max(Number(this.price) - this.step(), 10));
    },
    get estimate() {
      if (this.mode === "streak") {
        const left = Number(this.target) - (this.days[this.habit] || 0);
        if (left <= 0) return "Schon erreicht, du kannst sie direkt einlösen.";
        return "Noch etwa " + left + (left === 1 ? " Tag" : " Tage") + " Serie bis dahin.";
      }
      if (this.rate <= 0) return "";
      const days = Math.max(Math.ceil(Number(this.price) / this.rate), 1);
      return "Bei ca. " + this.rate + " Talern pro Tag in etwa " + days + (days === 1 ? " Tag" : " Tagen") + " erreichbar.";
    },
  }));
});

function showToast(text) {
  const toast = document.getElementById("toast");
  if (!toast) return;
  toast.textContent = text;
  toast.hidden = false;
  setTimeout(() => (toast.hidden = true), 3000);
}

// Questions before a risky action open in our own dialog. The first sentence is
// the title, the button says data-yes on the asking element or "Löschen".
document.addEventListener("htmx:confirm", (e) => {
  if (!e.detail.question) return;
  e.preventDefault();
  const dialog = document.getElementById("confirm");
  const [title, ...rest] = e.detail.question.split(/(?<=\?) /);
  dialog.querySelector("h2").textContent = title;
  dialog.querySelector("p").textContent = rest.join(" ");
  dialog.querySelector("[value=yes]").textContent = e.detail.elt.closest("[hx-confirm]")?.dataset.yes || "Löschen";
  dialog.returnValue = "";
  dialog.addEventListener("close", () => {
    if (dialog.returnValue === "yes") e.detail.issueRequest(true);
  }, { once: true });
  dialog.showModal();
});
document.addEventListener("click", (e) => {
  if (e.target.id === "confirm") e.target.close();
});

// A rejected form comes back as the whole page with its message on it.
document.addEventListener("htmx:beforeSwap", (e) => {
  const type = e.detail.xhr.getResponseHeader("Content-Type") || "";
  if (e.detail.xhr.status >= 400 && type.startsWith("text/html")) {
    e.detail.shouldSwap = true;
    e.detail.isError = false;
  }
});

// Other server errors of HTMX requests arrive as short plain text.
document.addEventListener("htmx:responseError", (e) => {
  showToast(e.detail.xhr.responseText || "Das hat nicht geklappt.");
});

// A toast with an undo button stays longer than a plain message.
document.addEventListener("htmx:afterSettle", () => {
  const toast = document.querySelector("#toast[data-timeout]");
  if (!toast) return;
  toast.removeAttribute("data-timeout");
  setTimeout(() => (toast.hidden = true), 8000);
});

// Boosted links bypass the offline page of the service worker.
document.addEventListener("htmx:sendError", () => showToast("Keine Verbindung zum Server."));

// Every response replaces the buttons on the card, which would drop what htmx
// queues on them. This queue outlives the swap, so fast clicks all count.
let clicks = Promise.resolve();
document.addEventListener("click", (e) => {
  const button = e.target.closest("[data-post]");
  if (!button) return;
  const url = button.dataset.post;
  clicks = clicks.then(() => htmx.ajax("POST", url, { target: "#page" })).catch(() => {});
});

const saveCookie = (name, value) => (document.cookie = `${name}=${value}; path=/; max-age=31536000; samesite=lax`);

const readCookie = (name) => {
  const found = document.cookie.split("; ").find((c) => c.startsWith(`${name}=`));
  return found ? found.slice(name.length + 1) : "";
};

const isFolded = (button) => button.closest(".hcard").classList.contains("collapsed");

// Folds a card. Each device keeps its own state, the server reads the cookies.
// Reached cards are folded by default, so for them the list holds the opened ones.
function foldCard(button, folded) {
  const { fold: id, day } = button.dataset;
  const name = day ? "habit_opened" : "habit_folded";
  let saved = readCookie(name);
  if (day) saved = saved.startsWith(`${day}:`) ? saved.slice(day.length + 1) : "";
  const known = [...document.querySelectorAll("[data-fold]")].map((b) => b.dataset.fold);
  const ids = new Set(saved.split(".").filter((x) => known.includes(x)));
  if (day ? folded : !folded) ids.delete(id);
  else ids.add(id);
  saveCookie(name, (day ? `${day}:` : "") + [...ids].join("."));
  button.closest(".hcard").classList.toggle("collapsed", folded);
  button.setAttribute("aria-expanded", String(!folded));
  button.setAttribute("aria-label", button.getAttribute("aria-label").replace(/\w+klappen$/, folded ? "aufklappen" : "einklappen"));
}

// The button of a group offers what most helps: folding as long as a card is open.
// Reached habits open as a compact list, so there the list itself counts as open.
const anyOpen = (group) => {
  if (group.querySelector("[data-fold-reached]")) return !group.classList.contains("shut");
  return [...group.querySelectorAll("[data-fold]")].some((b) => !isFolded(b));
};

function labelGroup(group) {
  const all = group?.querySelector("[data-fold-all]");
  if (all) all.textContent = anyOpen(group) ? "Alle einklappen" : "Alle aufklappen";
}

// The line of the reached habits hides their list. Like an opened reached card,
// an opened list only stays open for this day on this device.
function shutReached(button, shut) {
  button.closest(".habit-group").classList.toggle("shut", shut);
  saveCookie("reached_open", shut ? "" : button.dataset.day);
  button.setAttribute("aria-expanded", String(!shut));
  button.setAttribute("aria-label", button.getAttribute("aria-label").replace(/\w+klappen$/, shut ? "aufklappen" : "einklappen"));
}

document.addEventListener("click", (e) => {
  const button = e.target.closest("[data-fold]");
  if (button) {
    foldCard(button, !isFolded(button));
    labelGroup(button.closest(".habit-group"));
  }
  const line = e.target.closest("[data-fold-reached]");
  if (line) {
    const group = line.closest(".habit-group");
    shutReached(line, !group.classList.contains("shut"));
    labelGroup(group);
  }
  const all = e.target.closest("[data-fold-all]");
  if (all) {
    const group = all.closest(".habit-group");
    const fold = anyOpen(group);
    const reached = group.querySelector("[data-fold-reached]");
    group.querySelectorAll("[data-fold]").forEach((b) => foldCard(b, fold || Boolean(reached)));
    if (reached) shutReached(reached, fold);
    labelGroup(group);
  }
});

// Today follows what happens on other devices. It asks while it is visible and at
// once on coming back, and leaves the page alone while someone is working on it.
const LIVE_MS = 15000;
let liveStamp = 0;
const touchLive = () => liveStamp++;
document.addEventListener("click", touchLive);
document.addEventListener("htmx:beforeRequest", touchLive);
document.addEventListener("htmx:afterRequest", touchLive);

const liveBusy = (page) => document.hidden || document.querySelector("dialog[open], .htmx-request") || page.querySelector("details[open]");

async function refreshToday() {
  const page = document.getElementById("page");
  if (!page?.querySelector(".today-head") || liveBusy(page)) return;
  const stamp = liveStamp;
  const res = await fetch("/today/live").catch(() => null);
  if (!res?.ok || res.redirected) return;
  const html = await res.text();
  if (stamp !== liveStamp || liveBusy(page)) return; // this answer may be older than the page
  const fresh = document.createElement("template");
  fresh.innerHTML = html;
  if (fresh.innerHTML !== page.innerHTML) htmx.swap(page, html, { swapStyle: "innerHTML" });
}

setInterval(refreshToday, LIVE_MS);
document.addEventListener("visibilitychange", refreshToday);

// Folds the level card. Each device keeps its own state, the server reads the cookie.
document.addEventListener("click", (e) => {
  const button = e.target.closest("[data-fold-level]");
  if (!button) return;
  const folded = button.closest(".level-card").classList.toggle("folded");
  saveCookie("level_folded", folded ? 1 : 0);
  button.setAttribute("aria-expanded", String(!folded));
  button.setAttribute("aria-label", `Level-Anzeige ${folded ? "aufklappen" : "einklappen"}`);
});

// Size of the habit cards and the saving goals. Each device keeps its own, the server reads the cookie.
document.addEventListener("click", (e) => {
  const button = e.target.closest("[data-zoom]");
  if (!button || button.disabled) return;
  const box = button.closest(".zoom");
  const zoom = Number(box.dataset.value) + Number(button.dataset.zoom);
  box.dataset.value = zoom;
  box.querySelector("b").textContent = `${zoom}%`;
  box.querySelectorAll("button").forEach((b) => (b.disabled = zoom === Number(b.dataset.stop)));
  document.getElementById("page").style.setProperty(box.dataset.var, zoom / 100);
  saveCookie(box.dataset.cookie, zoom);
});

// Role select of the user list.
document.addEventListener("change", (e) => {
  if (e.target.matches("[data-submit]")) e.target.form.requestSubmit();
});

// Taking a redemption back also dismisses its toast.
document.addEventListener("click", (e) => {
  const toast = e.target.closest(".toast.undo");
  if (toast && e.target.closest("button")) toast.hidden = true;
});

// Copies a one-time secret; plain HTTP has no clipboard, the text stays selected.
document.addEventListener("click", (e) => {
  const button = e.target.closest("[data-copy]");
  if (!button) return;
  const field = document.querySelector(button.dataset.copy);
  field.select();
  navigator.clipboard?.writeText(field.value);
});

// History pages come fresh from the server, htmx keeps no scroll position for them.
const scrolls = new Map();
history.scrollRestoration = "manual";
const scroller = () => {
  const app = document.querySelector(".app.has-tabs");
  return app && getComputedStyle(app).overflowY === "auto" ? app : document.documentElement;
};
document.addEventListener("htmx:beforeHistorySave", (e) => scrolls.set(e.detail.path, scroller().scrollTop));
document.addEventListener("htmx:historyRestore", (e) => { scroller().scrollTop = scrolls.get(e.detail.path) || 0; });

if ("serviceWorker" in navigator) {
  navigator.serviceWorker.register("/sw.js").catch(() => {});
}

const csrf = () => JSON.parse(document.body.getAttribute("hx-headers") || "{}")["X-CSRF-Token"];

const postJSON = (url, body) =>
  fetch(url, {
    method: "POST",
    headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf() },
    body: JSON.stringify(body),
  });

function pushKey(base64) {
  const text = atob(base64.replace(/-/g, "+").replace(/_/g, "/"));
  return Uint8Array.from(text, (c) => c.charCodeAt(0));
}

function setupPush() {
  const on = document.getElementById("push-on");
  const off = document.getElementById("push-off");
  const state = document.getElementById("push-state");
  if (!on || on.dataset.ready) return;
  on.dataset.ready = "true";
  if (!("PushManager" in window) || !navigator.serviceWorker) {
    state.textContent = "Dieser Browser oder diese Verbindung unterstützt kein Web-Push.";
    on.disabled = off.disabled = true;
    return;
  }
  on.addEventListener("click", async () => {
    try {
      if ((await Notification.requestPermission()) !== "granted") {
        state.textContent = "Benachrichtigungen sind blockiert.";
        return;
      }
      const reg = await navigator.serviceWorker.ready;
      const sub = await reg.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: pushKey(on.dataset.key) });
      await postJSON("/push/subscribe", sub.toJSON());
      state.textContent = "Aktiv auf diesem Gerät.";
    } catch {
      state.textContent = "Aktivieren fehlgeschlagen.";
    }
  });
  off.addEventListener("click", async () => {
    const reg = await navigator.serviceWorker.ready;
    const sub = await reg.pushManager.getSubscription();
    if (sub) {
      await postJSON("/push/unsubscribe", { endpoint: sub.endpoint });
      await sub.unsubscribe();
    }
    state.textContent = "Auf diesem Gerät deaktiviert.";
  });
}

setupPush();
document.addEventListener("htmx:afterSettle", setupPush);

// Sheets open from a button and close on their handle or beside them.
document.addEventListener("click", (e) => {
  const opener = e.target.closest("[data-open-sheet]");
  if (opener) {
    document.getElementById(opener.dataset.openSheet)?.showModal();
    return;
  }
  const sheet = e.target.closest("dialog.sheet");
  if (sheet && (e.target === sheet || e.target.closest("[data-close-sheet]"))) sheet.close();
});

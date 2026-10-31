// Alpine components (CSP build: all logic lives here), push and the service worker.
const readConfig = (el) => JSON.parse(el.dataset.config || "{}");

document.addEventListener("alpine:init", () => {
  const multipliers = { easy: 10, medium: 15, hard: 25 };
  const measureDefaults = { count: { target: "8", step: "1" }, amount: { target: "10000", step: "1000" } };
  const stepSize = (n) => 10 ** Math.floor(Math.log10(Math.max(n, 1)));

  Alpine.data("habitForm", () => ({
    kind: "build",
    difficulty: "easy",
    measure: "check",
    target: "8",
    step: "1",
    edited: false,
    init() {
      Object.assign(this, readConfig(this.$el));
      this.$watch("measure", (m) => {
        if (!this.edited && measureDefaults[m]) Object.assign(this, measureDefaults[m]);
      });
    },
    setGoal(target, step) {
      this.edited = true;
      this.step = String(step);
      this.target = String(Math.max(target, step));
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
      this.setGoal(Number(this.target), Math.max(step - stepSize(step - 1), 1));
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

// Server errors of HTMX requests arrive as short plain text.
document.addEventListener("htmx:responseError", (e) => {
  showToast(e.detail.xhr.responseText || "Das hat nicht geklappt.");
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

// History pages come fresh from the server, htmx keeps no scroll position for them.
const scrolls = new Map();
history.scrollRestoration = "manual";
document.addEventListener("htmx:beforeHistorySave", (e) => scrolls.set(e.detail.path, scrollY));
document.addEventListener("htmx:historyRestore", (e) => scrollTo(0, scrolls.get(e.detail.path) || 0));

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

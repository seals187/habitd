// Alpine components (CSP build: all logic lives here), push and the service worker.
const readConfig = (el) => JSON.parse(el.dataset.config || "{}");

document.addEventListener("alpine:init", () => {
  const multipliers = { easy: 10, medium: 15, hard: 25 };

  Alpine.data("habitForm", () => ({
    kind: "build",
    difficulty: "easy",
    init() {
      Object.assign(this, readConfig(this.$el));
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

// Server errors of HTMX requests arrive as short plain text.
document.addEventListener("htmx:responseError", (e) => {
  const toast = document.getElementById("toast");
  if (!toast) return;
  toast.textContent = e.detail.xhr.responseText || "Das hat nicht geklappt.";
  toast.hidden = false;
  setTimeout(() => (toast.hidden = true), 3000);
});

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
  if (!on) return;
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

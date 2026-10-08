// The pieces every screen builds from: icons, banners, and the small amount of
// formatting that would otherwise be written out five times.

export const icon = {
  logo: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M4 18v-7a3 3 0 0 1 3-3h10a3 3 0 0 1 3 3v7"/><path d="M4 18h16"/><circle cx="12" cy="12" r="2.2" fill="currentColor" stroke="none"/></svg>`,
  calendar: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"><rect x="3" y="5" width="18" height="16" rx="3"/><path d="M8 3v4M16 3v4M3 10h18"/></svg>`,
  ticket: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linejoin="round"><path d="M3 8a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2v2a2 2 0 0 0 0 4v2a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-2a2 2 0 0 0 0-4Z"/><path d="M14 6v12" stroke-dasharray="2 3"/></svg>`,
  person: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"><circle cx="12" cy="8" r="3.5"/><path d="M5 20c0-3.3 3.1-6 7-6s7 2.7 7 6"/></svg>`,
  sliders: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"><path d="M4 8h10M18 8h2M4 16h3M11 16h9"/><circle cx="16" cy="8" r="2"/><circle cx="9" cy="16" r="2"/></svg>`,
  clock: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="8"/><path d="M12 8v4.5l3 1.6" stroke-linecap="round"/></svg>`,
  check: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"><path d="M5 12.5 10 17.5 19 7"/></svg>`,
  alert: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="9"/><path d="M12 7v6" stroke-linecap="round"/><circle cx="12" cy="16.5" r="1" fill="currentColor"/></svg>`,
  warn: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linejoin="round"><path d="M12 3.5 22 20H2Z"/><path d="M12 10v4" stroke-linecap="round"/><circle cx="12" cy="17" r="1" fill="currentColor"/></svg>`,
  good: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="9"/><path d="M8 12.5 11 15.5 16 9" stroke-linecap="round" stroke-linejoin="round"/></svg>`,
  offline: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M2 4l20 16"/><path d="M5 12.5a10 10 0 0 1 4-2.4"/><path d="M8.5 16a5 5 0 0 1 2.5-1.3"/><circle cx="12" cy="19.5" r="1" fill="currentColor"/></svg>`,
  back: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M15 5 8 12l7 7"/></svg>`,
  chevron: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" width="18" height="18"><path d="M9 5l7 7-7 7"/></svg>`,
  bell: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"><path d="M6 9a6 6 0 1 1 12 0c0 4 1.5 5.5 1.5 5.5h-15S6 13 6 9Z"/><path d="M10 18a2 2 0 0 0 4 0"/></svg>`,
  signout: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M10 5H6a2 2 0 0 0-2 2v10a2 2 0 0 0 2 2h4"/><path d="M15 8l4 4-4 4M19 12H9"/></svg>`,
  lock: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6"><rect x="5" y="10" width="14" height="11" rx="3"/><path d="M8.5 10V7a3.5 3.5 0 0 1 7 0v3"/><circle cx="12" cy="15" r="1.6" fill="currentColor"/></svg>`,
  queue: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="4" cy="12" r="1.6" fill="currentColor" stroke="none"/><circle cx="9" cy="12" r="1.6" fill="currentColor" stroke="none"/><path d="M13 12h3" stroke-linecap="round"/><circle cx="20" cy="12" r="3" stroke-dasharray="3 2"/></svg>`,
  stageEmpty: `<svg viewBox="0 0 120 90" fill="none"><rect x="30" y="6" width="14" height="8" rx="2" fill="currentColor" opacity=".5"/><rect x="76" y="6" width="14" height="8" rx="2" fill="currentColor" opacity=".5"/><path d="M37 14 24 52h26Z" fill="currentColor" opacity=".12"/><path d="M83 14 70 52h26Z" fill="currentColor" opacity=".12"/><path d="M16 54q44-9 88 0" stroke="var(--primary)" stroke-width="3" stroke-linecap="round"/><g fill="currentColor" opacity=".5"><rect x="24" y="62" width="11" height="8" rx="2"/><rect x="39" y="62" width="11" height="8" rx="2"/><rect x="54" y="62" width="11" height="8" rx="2"/><rect x="70" y="62" width="11" height="8" rx="2"/><rect x="85" y="62" width="11" height="8" rx="2"/><rect x="31" y="74" width="11" height="8" rx="2"/><rect x="46" y="74" width="11" height="8" rx="2"/><rect x="63" y="74" width="11" height="8" rx="2"/><rect x="78" y="74" width="11" height="8" rx="2"/></g></svg>`,
  ticketEmpty: `<svg viewBox="0 0 120 90" fill="none"><g transform="rotate(-8 60 45)"><rect x="18" y="28" width="84" height="36" rx="8" stroke="currentColor" stroke-width="2.4"/><path d="M78 28v36" stroke="currentColor" stroke-width="2" stroke-dasharray="3 4"/><rect x="28" y="38" width="34" height="5" rx="2.5" fill="currentColor" opacity=".55"/><rect x="28" y="49" width="24" height="5" rx="2.5" fill="currentColor" opacity=".55"/><rect x="84" y="39" width="13" height="13" rx="4" fill="var(--primary)"/></g></svg>`,
  lockEmpty: `<svg viewBox="0 0 120 90" fill="none"><rect x="40" y="38" width="40" height="34" rx="9" stroke="currentColor" stroke-width="2.6"/><path d="M48 38v-7a12 12 0 0 1 24 0v7" stroke="currentColor" stroke-width="2.6"/><circle cx="60" cy="52" r="4" fill="var(--primary)"/><rect x="58" y="54" width="4" height="9" rx="2" fill="var(--primary)"/></svg>`,
};

// The one place a server code becomes a sentence. Codes are stable, prose is
// not, so nothing branches on a message.
const sentences = {
  seat_not_available: (c) => [
    "Someone beat you to it",
    `${c.seat ?? "That seat"} was taken a moment ago. Pick another seat.`,
  ],
  hold_expired: (c) => [
    "Your time ran out",
    `${c.seat ?? "The seat"} is open again. Tap any seat to start a new 5-minute hold.`,
  ],
  seat_not_held: () => ["Nothing to confirm", "That seat is not being held."],
  hold_not_found: () => ["That hold is gone", "It ran out or was given up. Pick a seat again."],
  not_hold_owner: () => ["That is not your hold", "Somebody else is holding that seat."],
  unauthenticated: () => [
    "Sign in to hold a seat",
    "Browsing is open to everyone. Holding needs an account.",
  ],
  invalid_token: () => ["Your session has ended", "Sign in again to carry on."],
  email_not_verified: () => [
    "Confirm your email first",
    "We sent a link when you signed up. Holding a seat needs a confirmed address.",
  ],
  not_permitted: () => ["You don't have access", "This area is for SeatHold admins only."],
  event_cancelled: () => ["This event was cancelled", "It is no longer going ahead."],
  invalid_credentials: () => ["Email or password is incorrect", ""],
  email_taken: () => ["This email is already registered", "Sign in instead."],
  weak_password: () => ["Password must be at least 8 characters", ""],
  invalid_email: () => ["Enter a valid email address", ""],
  verification_not_usable: () => [
    "That link no longer works",
    "It was used already or has expired. Ask for a new one from your account.",
  ],
  already_verified: () => ["Already confirmed", "Nothing more to do."],
  event_in_use: () => [
    "This event has tickets",
    "Seats are held or sold, so it cannot be removed. Cancel it instead.",
  ],
  invalid_seat_map: () => ["That seat map does not work", "Check the rows and the count."],
};

export function explain(err, context = {}) {
  if (err.code === "too_many_requests") {
    return ["Too many attempts", `Try again in ${err.retryAfter || 30} seconds.`];
  }

  const say = sentences[err.code];
  if (say) return say(context);

  return ["Something went wrong", err.message];
}

// ---- banners ----

const banners = () => document.getElementById("banners");

export function clearBanners() {
  banners().replaceChildren();
}

export function banner(kind, title, detail, extra = {}) {
  const glyph = { error: icon.alert, warning: icon.warn, success: icon.good, info: icon.alert };

  const node = el("div", `banner banner-${kind}`);
  node.innerHTML = `
    ${extra.icon ?? glyph[kind]}
    <div class="body"><strong></strong><p></p></div>
  `;

  node.querySelector("strong").textContent = title;

  const p = node.querySelector("p");
  if (detail) p.textContent = detail;
  else p.remove();

  if (extra.action) {
    const action = el("button", "link", extra.action.label);
    action.addEventListener("click", extra.action.onClick);
    node.querySelector(".body").append(action);
  }

  if (extra.seconds) {
    // A rate limit swaps the close button for a live countdown: the useful thing
    // to know is when it ends, not how to dismiss it.
    const ring = el("div", "countdown-ring", String(extra.seconds));
    node.append(ring);

    let left = extra.seconds;
    const tick = setInterval(() => {
      left -= 1;
      ring.textContent = String(Math.max(0, left));
      if (left <= 0) {
        clearInterval(tick);
        node.remove();
      }
    }, 1000);
  } else if (extra.dismissible !== false) {
    const close = el("button", "close", "×");
    close.setAttribute("aria-label", "Dismiss");
    close.addEventListener("click", () => node.remove());
    node.append(close);
  }

  if (extra.id) {
    banners().querySelector(`[data-banner="${extra.id}"]`)?.remove();
    node.dataset.banner = extra.id;
  }

  banners().append(node);

  return node;
}

export function dismissBanner(id) {
  banners().querySelector(`[data-banner="${id}"]`)?.remove();
}

export function showError(err, context) {
  const [title, detail] = explain(err, context);

  banner(err.code === "too_many_requests" ? "warning" : "error", title, detail, {
    seconds: err.code === "too_many_requests" ? err.retryAfter || 30 : 0,
  });
}

// ---- small helpers ----

export function el(tag, className, text) {
  const node = document.createElement(tag);
  if (className) node.className = className;
  // textContent, never innerHTML: an event name is data, and data does not get
  // to write markup.
  if (text !== undefined) node.textContent = text;

  return node;
}

export function clock(seconds) {
  const safe = Math.max(0, Math.floor(seconds));

  return `${String(Math.floor(safe / 60)).padStart(2, "0")}:${String(safe % 60).padStart(2, "0")}`;
}

const MONTHS = ["JAN", "FEB", "MAR", "APR", "MAY", "JUN", "JUL", "AUG", "SEP", "OCT", "NOV", "DEC"];
const DAYS = ["SUN", "MON", "TUE", "WED", "THU", "FRI", "SAT"];

export function dateParts(iso) {
  const at = new Date(iso);

  return { mon: MONTHS[at.getMonth()], day: String(at.getDate()), dow: DAYS[at.getDay()] };
}

export function longDate(iso) {
  return new Date(iso).toLocaleString(undefined, {
    weekday: "short",
    day: "numeric",
    month: "short",
    hour: "2-digit",
    minute: "2-digit",
  });
}

export function shortDate(iso) {
  return new Date(iso)
    .toLocaleString(undefined, {
      weekday: "short",
      day: "numeric",
      month: "short",
      hour: "2-digit",
      minute: "2-digit",
    })
    .toUpperCase();
}

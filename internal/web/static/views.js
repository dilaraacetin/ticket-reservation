// The screens. Each one builds a node and nothing else: what is on the page is
// the router's decision, not theirs.

import { api, ApiError, setToken } from "./api.js";
import {
  banner,
  clearBanners,
  clock,
  dateParts,
  dismissBanner,
  el,
  icon,
  longDate,
  shortDate,
  showError,
} from "./ui.js";

// ---- sign in and create account ----

export function authView(ctx) {
  const node = el("div", "auth");
  let mode = ctx.state.authMode ?? "signin";
  let locked = 0;

  const render = () => {
    node.replaceChildren();

    const signup = mode === "signup";

    const brand = el("div", "brandmark");
    brand.innerHTML = `<span class="mark">${icon.logo}</span>`;
    brand.append(el("span", "name", "SeatHold"));

    const title = el("h1", "title", signup ? "Create your account" : "Welcome back");
    const blurb = el(
      "p",
      "muted",
      signup
        ? "Your tickets and the seats you are holding stay in one place."
        : "Sign in to hold seats and see your tickets.",
    );

    const form = el("form");
    form.noValidate = true;

    const email = field("Email", "email", "you@example.com");
    email.input.autocomplete = "email";
    email.input.inputMode = "email";
    email.input.autocapitalize = "none";
    email.input.spellcheck = false;

    const password = field("Password", "password", "");
    password.input.autocomplete = signup ? "new-password" : "current-password";
    password.input.minLength = 8;

    // Shown rather than counted. A rule the person can read once is enough; a
    // meter that moves on every keystroke is noise dressed up as feedback.
    if (signup) {
      const hint = el("div", "field-hint", "At least 8 characters.");
      password.wrap.append(hint);
    }

    // Typing a password on a phone keyboard is where most of them go wrong, so
    // there is a way to look at what was typed.
    const reveal = el("button", "reveal", "Show");
    reveal.type = "button";
    reveal.addEventListener("click", () => {
      const hidden = password.input.type === "password";
      password.input.type = hidden ? "text" : "password";
      reveal.textContent = hidden ? "Hide" : "Show";
    });
    password.row.append(reveal);

    const submit = el("button", "btn btn-primary btn-block");
    submit.type = "submit";
    submit.textContent = signup ? "Create account" : "Sign in";

    form.append(email.wrap, password.wrap, submit);

    const busy = (on, text) => {
      submit.disabled = on || locked > 0;
      submit.replaceChildren();
      if (on) submit.append(el("span", "spinner"), document.createTextNode(text));
      else submit.textContent = locked > 0 ? `Try again in 0:${String(locked).padStart(2, "0")}` : text;
    };

    const label = () => (signup ? "Create account" : "Sign in");

    form.addEventListener("submit", async (event) => {
      event.preventDefault();
      clearBanners();
      email.clear();
      password.clear();

      const credentials = { email: email.input.value, password: password.input.value };

      busy(true, signup ? "Creating account…" : "Signing in…");

      try {
        if (signup) {
          await api("POST", "/auth/register", { body: credentials });
        }

        const session = await api("POST", "/auth/login", { body: credentials });
        setToken(session.token);

        await ctx.signedIn();
      } catch (err) {
        busy(false, label());

        if (!(err instanceof ApiError)) throw err;

        // Where the message goes matters: a field problem belongs under the
        // field, and the password is cleared so a mistyped one is not resent.
        if (err.code === "email_taken") {
          email.fail("This email is already registered.", {
            label: "Sign in instead",
            onClick: () => {
              mode = "signin";
              ctx.state.authMode = mode;
              render();
            },
          });
        } else if (err.code === "invalid_email") {
          email.fail("Enter a valid email address.");
        } else if (err.code === "weak_password") {
          password.fail("Password must be at least 8 characters.");
        } else if (err.code === "invalid_credentials") {
          password.input.value = "";
          form.insertBefore(inlineError("Email or password is incorrect."), submit);
        } else if (err.code === "too_many_requests") {
          locked = err.retryAfter || 30;
          busy(false, label());

          const tick = setInterval(() => {
            locked -= 1;
            busy(false, label());
            if (locked <= 0) clearInterval(tick);
          }, 1000);

          showError(err);
        } else {
          showError(err);
        }
      }
    });

    // One way across, at the bottom, which is where somebody looks once they
    // find they are on the wrong screen.
    const swap = el("p", "auth-swap");
    swap.append(document.createTextNode(signup ? "Already have an account? " : "New to SeatHold? "));

    const other = el("button", "link", signup ? "Sign in" : "Create an account");
    other.addEventListener("click", () => {
      mode = signup ? "signin" : "signup";
      ctx.state.authMode = mode;
      render();
    });
    swap.append(other);

    const browse = el("button", "link auth-browse", "Browse events without an account");
    browse.addEventListener("click", () => ctx.go("events"));

    node.append(brand, title, blurb, form, swap, browse);
  };

  render();

  return node;
}

// Ids have to be unique on the page, and two text fields on one form is the
// ordinary case. Numbered, or the second label focuses the first box.
let fieldSeq = 0;

function choice(label, options, selected) {
  const wrap = el("div", "field");
  const name = el("label", null, label);
  const input = el("select");

  for (const [value, text] of options) {
    const option = el("option", null, text);
    option.value = value;
    option.selected = value === selected;
    input.append(option);
  }

  fieldSeq += 1;
  input.id = `field-select-${fieldSeq}`;
  name.htmlFor = input.id;

  wrap.append(name, input);

  return { wrap, input };
}

function field(label, type, placeholder) {
  const wrap = el("div", "field");
  const row = el("div", "row");
  const name = el("label", null, label);
  const input = el(type === "textarea" ? "textarea" : "input");

  if (type !== "textarea") input.type = type;
  input.placeholder = placeholder;

  if (type === "email") input.autocomplete = "email";
  else if (type === "password") input.autocomplete = "current-password";

  fieldSeq += 1;
  input.id = `field-${type}-${fieldSeq}`;
  name.htmlFor = input.id;

  row.append(name);
  wrap.append(row, input);

  let message = null;

  return {
    wrap,
    row,
    input,
    clear() {
      wrap.classList.remove("invalid");
      message?.remove();
      message = null;
    },
    fail(text, action) {
      wrap.classList.add("invalid");
      message?.remove();
      message = el("div", "field-error");
      message.innerHTML = icon.alert;
      message.append(el("span", null, text));

      if (action) {
        const link = el("button", "link", action.label);
        link.addEventListener("click", action.onClick);
        message.append(link);
      }

      wrap.append(message);
    },
  };
}

function inlineError(text) {
  const node = el("div", "banner banner-error");
  node.style.marginBottom = "16px";
  node.innerHTML = icon.alert;
  node.append(el("div", "body", text));

  return node;
}

// ---- events ----

// The display names and the order they are offered in. Presentation, so it lives
// here rather than being sent by the server; it mirrors domain.Categories.
const CATEGORIES = [
  ["concert", "Concert"],
  ["theatre", "Theatre"],
  ["comedy", "Comedy"],
  ["festival", "Festival"],
  ["sport", "Sport"],
  ["family", "Family"],
  ["other", "Other"],
];

function categoryName(value) {
  return CATEGORIES.find(([key]) => key === value)?.[1] ?? "Other";
}

export function eventsView(ctx, events) {
  const node = el("div");
  const head = el("div", "screen-head");
  head.append(el("h1", "title", "Events"), el("p", "muted", "Pick a show, then pick your seat."));
  node.append(head);

  if (events.length === 0) {
    node.append(
      empty(icon.stageEmpty, "Nothing on sale yet.", "New events show up here the moment they open. Check back soon."),
    );

    return node;
  }

  // Both filters are built from what is on sale, so neither can offer something
  // that matches nothing.
  const kinds = CATEGORIES.filter(([key]) => events.some((event) => event.category === key));
  const cities = [...new Set(events.map((event) => event.city).filter(Boolean))].sort();

  if (kinds.length > 1) node.append(categoryFilter(ctx, kinds));
  if (cities.length > 1) node.append(cityFilter(ctx, cities));

  const shown = events.filter(
    (event) =>
      (!ctx.state.category || event.category === ctx.state.category) &&
      (!ctx.state.city || event.city === ctx.state.city),
  );

  if (shown.length === 0) {
    node.append(
      empty(icon.stageEmpty, "Nothing matches that.", "Try another city or another kind of night out.", {
        label: "Clear the filters",
        onClick: () => ctx.setFilters({ category: "", city: "" }),
      }),
    );

    return node;
  }

  const list = el("div", "event-list");
  for (const event of shown) list.append(eventCard(ctx, event));

  node.append(list);

  return node;
}

function categoryFilter(ctx, kinds) {
  const bar = el("div", "filter");
  bar.setAttribute("role", "group");
  bar.setAttribute("aria-label", "Filter by kind");

  for (const [value, text] of [["", "All"], ...kinds]) {
    const pill = el("button", "pill", text);
    const on = ctx.state.category === value;

    pill.setAttribute("aria-pressed", String(on));
    if (on) pill.classList.add("on");
    pill.addEventListener("click", () => ctx.setFilters({ category: value }));

    bar.append(pill);
  }

  return bar;
}

// A select rather than the pills above it. The kinds are a closed set of seven;
// the cities are however many the catalogue grows to, and a scrolling row of
// eighty is not something a thumb can use.
function cityFilter(ctx, cities) {
  const wrap = el("label", "city-filter");

  const mark = el("span", "mark");
  mark.innerHTML = icon.ticket;

  const select = el("select");
  for (const [value, text] of [["", "All cities"], ...cities.map((city) => [city, city])]) {
    const option = el("option", null, text);
    option.value = value;
    option.selected = ctx.state.city === value;
    select.append(option);
  }

  select.addEventListener("change", () => ctx.setFilters({ city: select.value }));

  wrap.append(mark, select);

  return wrap;
}

// A hue from the id, so an event with no poster still gets a face of its own and
// gets the same one every time.
function hue(id) {
  let total = 0;
  for (const ch of id) total = (total * 31 + ch.codePointAt(0)) % 360;

  return total;
}

function poster(event, { tall = false } = {}) {
  const box = el("div", tall ? "poster poster-hero" : "poster");
  box.style.setProperty("--hue", hue(event.id));

  if (event.imageUrl) {
    const img = el("img");
    img.src = event.imageUrl;
    img.alt = "";
    img.loading = "lazy";

    // A poster that will not load leaves the colour behind it rather than the
    // browser's broken-image mark.
    img.addEventListener("error", () => img.remove());
    box.append(img);
  } else {
    box.append(el("span", "initial", event.name.slice(0, 1).toUpperCase()));
  }

  return box;
}

function statusTag(event) {
  if (event.cancelled) return el("span", "tag tag-cancelled", "Cancelled");
  if (event.hasStarted) return el("span", "tag tag-started", "Started");

  return null;
}

function eventCard(ctx, event) {
  const card = el("button", "event");
  const when = dateParts(event.startsAt);

  const art = poster(event);

  const date = el("div", "date-chip");
  date.append(el("span", "mon", when.mon), el("span", "day", when.day), el("span", "dow", when.dow));
  art.append(date);

  const tag = statusTag(event);
  if (tag) {
    tag.classList.add("on-poster");
    art.append(tag);
  }

  const body = el("div", "body");
  body.append(
    el("div", "kind", categoryName(event.category)),
    el("div", "name", event.name),
    el("div", "where", [event.venue, event.city].filter(Boolean).join(" · ")),
    el("div", "caption", timeOf(event.startsAt)),
  );

  card.append(art, body);

  if (event.cancelled) {
    card.disabled = true;
  } else {
    // A started event still opens: the page explains what happened, which an
    // unresponsive row does not.
    const chev = el("span", "chev");
    chev.innerHTML = icon.chevron;
    body.append(chev);
    card.addEventListener("click", () => ctx.go("event", { eventID: event.id }));
  }

  return card;
}

// The detail page. Everything somebody wants before they commit to a seat, and
// one button to go and take one.
export function eventDetailView(ctx, event) {
  const node = el("div", "detail");

  const back = el("button", "back");
  back.innerHTML = icon.back;
  back.setAttribute("aria-label", "Back to events");
  back.addEventListener("click", () => ctx.go("events"));

  const hero = poster(event, { tall: true });
  hero.append(el("div", "scrim"));

  const over = el("div", "over");

  const tags = el("div", "tags");
  tags.append(el("span", "tag tag-kind", categoryName(event.category)));

  const tag = statusTag(event);
  if (tag) tags.append(tag);

  over.append(tags, el("h1", null, event.name));
  hero.append(over);

  node.append(back, hero);

  const facts = el("div", "facts");
  facts.append(
    fact(icon.calendar, "When", longDate(event.startsAt)),
    fact(icon.ticket, "Where", [event.venue, event.city].filter(Boolean).join(", ")),
  );
  node.append(facts);

  if (event.total > 0) node.append(availability(event));

  if (event.description) node.append(section("About this event", event.description));
  if (event.rules) node.append(section("Good to know", event.rules));

  node.append(detailAction(ctx, event));

  return node;
}

function fact(art, label, text) {
  const row = el("div", "fact");
  const mark = el("span", "mark");
  mark.innerHTML = art;

  const body = el("div");
  body.append(el("div", "label", label), el("div", "value", text));

  row.append(mark, body);

  return row;
}

function availability(event) {
  const box = el("div", "avail");
  const sold = event.total - event.available;

  const top = el("div", "row");
  top.append(
    el("div", "n", String(event.available)),
    el("div", "muted", `of ${event.total} seats open`),
  );

  const meter = el("div", "meter");
  const fill = el("i");
  fill.style.width = `${Math.round((sold / event.total) * 100)}%`;
  meter.append(fill);

  box.append(top, meter);

  if (event.available === 0) {
    box.classList.add("none");
    box.append(el("p", "caption", "Every seat is taken. Join the line and you get the next one that frees up."));
  }

  return box;
}

// Paragraphs rather than one block: a description written with blank lines in it
// should read the way it was written.
function section(title, text) {
  const box = el("section", "prose");
  box.append(el("h2", null, title));

  for (const para of text.split(/\n{2,}/)) {
    if (para.trim()) box.append(el("p", null, para.trim()));
  }

  return box;
}

function detailAction(ctx, event) {
  const foot = el("div", "detail-action");

  if (event.cancelled) {
    foot.append(el("p", "muted", "This event has been cancelled. Tickets already bought stay valid as a record."));

    return foot;
  }

  const go = el("button", "btn btn-primary btn-block", event.available === 0 ? "Join the waiting list" : "Pick your seat");
  go.addEventListener("click", () => ctx.go("seats", { eventID: event.id }));
  foot.append(go);

  if (event.hasStarted) {
    foot.append(el("p", "caption", "This event has already started. Seats may still be open."));
  }

  return foot;
}

function timeOf(iso) {
  return new Date(iso).toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" });
}

export function empty(art, title, text, action) {
  const node = el("div", "empty");
  const picture = el("div");
  picture.innerHTML = art;

  node.append(picture, el("h2", null, title), el("p", null, text));

  if (action) {
    const button = el("button", "btn btn-primary", action.label);
    button.addEventListener("click", action.onClick);
    node.append(button);
  }

  return node;
}

export { banner, clearBanners, dismissBanner, clock, longDate, shortDate, showError, el, icon };

// ---- the seat map ----
//
// The screen the whole product is about. Four seat states, a countdown that owns
// the one loud colour, and a queue that appears only when there is nothing left
// to tap.

export function seatMapView(ctx, event, seats) {
  const node = el("div");

  const head = el("div", "seatmap-head");
  const back = el("button", "back");
  back.innerHTML = icon.back;
  back.setAttribute("aria-label", "Back to the event");
  back.addEventListener("click", () => ctx.go("event", { eventID: event.id }));

  const titles = el("div");
  titles.style.flex = "1";
  titles.style.minWidth = "0";
  titles.append(el("h1", null, event.name), el("div", "caption", `${event.venue} · ${longDate(event.startsAt)}`));

  const live = el("span", "tag tag-live", "Live");
  live.id = "live-tag";

  head.append(back, titles, live);

  const counts = el("div", "counts");
  counts.id = "counts";

  const stage = el("div", "stage", "STAGE");

  const grid = el("div", "seat-grid");
  grid.id = "seat-grid";

  const legend = el("div", "legend");
  legend.innerHTML = `
    <span><i class="sw-open"></i> Open</span>
    <span><i class="sw-held"></i> Held</span>
    <span><i class="sw-sold"></i> Sold</span>
    <span><i class="sw-mine"></i> Yours</span>
  `;

  node.append(head, counts, stage, grid, legend);

  renderSeats(ctx, grid, counts, seats);

  return node;
}

export function renderSeats(ctx, grid, counts, seats) {
  grid.replaceChildren();

  const rows = new Map();
  for (const seat of seats) {
    if (!rows.has(seat.row)) rows.set(seat.row, []);
    rows.get(seat.row).push(seat);
  }

  const widest = Math.max(0, ...[...rows.values()].map((row) => row.length));

  // Column numbers, with the same aisle the rows have so the two line up.
  const header = el("div", "col-head");
  header.append(el("span", "col-label"));
  for (let i = 1; i <= widest; i += 1) {
    if (i === 6) header.append(el("span", "aisle"));
    header.append(el("span", "col-label", String(i)));
  }

  grid.append(header);

  const tally = { available: 0, held: 0, reserved: 0, mine: 0 };

  for (const [label, rowSeats] of rows) {
    const row = el("div", "seat-row");
    row.append(el("span", "row-label", label));

    rowSeats.forEach((seat, index) => {
      if (index === 5) row.append(el("span", "aisle"));

      const mine = ctx.state.hold?.seatId === seat.id;
      const state = mine ? "mine" : seat.status;

      tally[mine ? "mine" : seat.status] += 1;

      const button = el("button", `seat ${state}`);
      button.setAttribute("aria-label", `Seat ${seat.id}, ${mine ? "yours" : seat.status}`);

      if (state === "held") button.innerHTML = icon.clock;
      if (state === "mine") button.innerHTML = icon.check;

      // Held and sold are not focusable either: tabbing onto something that
      // cannot be used is a dead end.
      if (state !== "available" || ctx.state.hold) {
        button.disabled = true;
      } else {
        button.addEventListener("click", () => ctx.holdSeat(seat));
      }

      row.append(button);
    });

    grid.append(row);
  }

  counts.replaceChildren();

  const open = tally.available;
  const openPart = el("span", open === 0 ? "none" : null);
  openPart.append(el("b", null, String(open)), document.createTextNode(" open"));

  counts.append(openPart, el("span", null, "·"));
  counts.append(withCount(tally.held + tally.mine, "held"), el("span", null, "·"));
  counts.append(withCount(tally.reserved, "sold"));

  if (ctx.state.stale) {
    counts.append(el("span", null, "·"), el("span", "stale", "may be stale"));
  }

  return tally;
}

function withCount(n, word) {
  const span = el("span");
  span.append(el("b", null, String(n)), document.createTextNode(` ${word}`));

  return span;
}

// ---- the hold bar ----

export function holdBar(ctx) {
  const node = el("div", "holdbar");
  node.id = "holdbar";
  node.style.setProperty("--ring-color", "var(--primary)");

  const top = el("div", "top");

  const ring = el("div", "ring");
  const ringInner = el("span", null, ctx.state.hold.seatId);
  ring.append(ringInner);

  const what = el("div", "what");
  const label = el("div", "label", "Held for you");
  what.append(label, el("div", "name", ctx.state.hold.eventName ?? ""));

  const clockBox = el("div", "clock");
  const time = el("div", "time numeral", "05:00");
  clockBox.append(time, el("div", "left", "left to confirm"));

  top.append(ring, what, clockBox);

  const actions = el("div", "actions");
  const release = el("button", "btn btn-danger", "Release");
  const confirm = el("button", "btn btn-primary", "Confirm seat");

  release.addEventListener("click", () => ctx.releaseHold());
  confirm.addEventListener("click", () => ctx.confirmHold());

  actions.append(release, confirm);
  node.append(top, actions);

  if (ctx.state.hold.fromWaitlist) {
    label.textContent = "Your turn · held for you";
  }

  // The countdown runs off the expiry the server sent, not a local count, so a
  // slow request does not leave the two disagreeing.
  const total = Math.max(1, (ctx.state.hold.expiresAt - ctx.state.hold.startedAt) / 1000);

  const tick = () => {
    const left = Math.max(0, (ctx.state.hold.expiresAt - Date.now()) / 1000);

    time.textContent = clock(left);
    ring.style.setProperty("--ring-fraction", String(left / total));

    // Under a minute everything that was amber turns red together, and the last
    // ten seconds pulse.
    node.classList.toggle("urgent", left < 60);
    node.classList.toggle("ending", left <= 10);
    node.style.setProperty("--ring-color", left < 60 ? "var(--danger)" : "var(--primary)");

    if (left < 60 && !ctx.state.hold.fromWaitlist) label.textContent = "Almost out of time";

    if (left <= 0) {
      clearInterval(ctx.state.countdown);
      ctx.holdRanOut();
    }
  };

  clearInterval(ctx.state.countdown);
  tick();
  ctx.state.countdown = setInterval(tick, 1000);

  return node;
}

export function holdPrompt() {
  const node = el("div", "prompt");
  const ring = el("div", "ring", "5:00");
  const body = el("div");
  body.append(
    el("strong", null, "Tap an open seat to hold it"),
    el("p", null, "It's yours for 5 minutes while you decide."),
  );

  node.append(ring, body);

  return node;
}

// ---- the waiting list ----

export function waitlistPanel(ctx, place) {
  const node = el("div", "waitlist");

  if (place === null) {
    node.append(
      el("div", "label", "Sold out right now"),
      el("h2", null, "Get the next free seat"),
      el(
        "p",
        null,
        "Held seats often come back. Join the line, and when one frees up we'll hold it for you for 5 minutes — automatically.",
      ),
    );

    const nobody = el("div", "nobody");
    nobody.innerHTML = icon.queue;
    nobody.append(el("span", null, "Join the line and we'll hold the next seat for you."));

    const join = el("button", "btn btn-primary btn-block", "Join the line");
    join.addEventListener("click", () => ctx.joinQueue());

    node.append(nobody, join);

    return node;
  }

  node.append(el("div", "label", "You're in line"));

  const row = el("div", "place");
  row.append(el("span", "n", ordinal(place)), el("span", null, "in line"));

  // Three dots and a dashed one: your place, and the seat that is coming.
  const dots = el("div", "dots");
  for (let i = 1; i <= Math.min(place, 3); i += 1) {
    dots.append(el("i", i === place ? "you" : null));
  }
  dots.append(el("i", "next"));
  row.append(dots);

  node.append(
    row,
    el(
      "p",
      null,
      "When it's your turn, we'll hold the freed seat for you for 5 minutes — you don't need to do anything. Keep this page open or allow notifications.",
    ),
  );

  if (ctx.state.pushOn) {
    const on = el("div", "notify-on");
    on.innerHTML = icon.bell;
    on.append(el("span", null, "Notifications on for this event"));
    node.append(on);
  }

  const leave = el("button", "btn btn-secondary btn-block", "Leave the line");
  leave.addEventListener("click", () => ctx.leaveQueue());
  node.append(leave);

  return node;
}

function ordinal(n) {
  const tens = n % 100;
  if (tens >= 11 && tens <= 13) return `${n}th`;

  return `${n}${["th", "st", "nd", "rd"][n % 10] ?? "th"}`;
}

// ---- tickets ----

export function ticketsView(ctx, tickets) {
  const node = el("div");
  const head = el("div", "screen-head");
  head.append(
    el("h1", "title", "My tickets"),
    el("p", "muted", "Show the code at the door. No printing needed."),
  );
  node.append(head);

  const now = Date.now();
  const upcoming = tickets.filter((t) => new Date(t.startsAt).getTime() >= now);
  const past = tickets.filter((t) => new Date(t.startsAt).getTime() < now);

  if (tickets.length === 0) {
    node.append(
      empty(icon.ticketEmpty, "No tickets yet", "Seats you confirm land here, ready to scan at the door.", {
        label: "Browse events",
        onClick: () => ctx.go("events"),
      }),
    );

    return node;
  }

  if (upcoming.length > 0) {
    node.append(el("div", "label", `Upcoming · ${upcoming.length}`));
    for (const ticket of upcoming) node.append(ticketCard(ctx, ticket));
  }

  if (past.length > 0) {
    const label = el("div", "label", "Past");
    label.style.marginTop = "22px";
    node.append(label);

    for (const ticket of past) {
      const row = el("div", "ticket-past");
      const body = el("div");
      body.append(
        el("div", null, ticket.eventName),
        el("div", "caption", `${ticket.venue} · ${longDate(ticket.startsAt)}`),
      );
      row.append(body, el("span", "n", ticket.seatId));
      node.append(row);
    }
  }

  return node;
}

function ticketCard(ctx, ticket) {
  const card = el("div", "ticket");

  const head = el("div", "head");
  const left = el("div");
  left.append(
    el("div", "when", shortDate(ticket.startsAt)),
    el("div", "name", ticket.eventName),
    el("div", "where", ticket.venue),
  );

  const right = el("div", "seat-no");
  right.append(el("span", "label", "Seat"), el("span", "n", ticket.seatId));

  head.append(left, right);

  const stub = el("div", "stub");

  // The image is rendered by the server: encoding a QR is error correction and
  // bit placement, and a page that gets it subtly wrong makes a code that scans
  // everywhere except at the door.
  const qr = el("img", "qr");
  qr.src = `/tickets/${encodeURIComponent(ticket.ticketCode)}/qr.png`;
  qr.alt = `Ticket ${ticket.ticketCode}`;
  qr.loading = "lazy";

  const detail = el("div");
  detail.append(
    el("div", "label", "Ticket code"),
    el("div", "code", ticket.ticketCode),
    el("div", "caption", `Row ${ticket.row} · Seat ${ticket.number}`),
  );

  const open = el("button", "link", "Open full screen");
  open.addEventListener("click", () => ctx.go("ticket", { code: ticket.ticketCode }));
  detail.append(open);

  stub.append(qr, detail);
  card.append(head, el("div", "tear"), stub);

  return card;
}

export function ticketView(ctx, ticket) {
  const node = el("div");

  const head = el("div", "seatmap-head");
  const back = el("button", "back");
  back.innerHTML = icon.back;
  back.setAttribute("aria-label", "Back to tickets");
  back.addEventListener("click", () => ctx.go("tickets"));
  head.append(back, el("h1", null, "Your ticket"));

  const hint = el("div", "banner banner-info");
  hint.innerHTML = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M2 12h2M20 12h2M5 5l1.5 1.5M17.5 17.5 19 19M19 5l-1.5 1.5M6.5 17.5 5 19" stroke-linecap="round"/></svg>`;
  hint.append(el("div", "body", "Turn your screen brightness up for a faster scan."));
  hint.style.margin = "0 0 18px";

  const full = el("div", "ticket-full");

  const top = el("div");
  top.style.width = "100%";
  top.style.display = "flex";
  top.style.justifyContent = "space-between";
  top.style.gap = "14px";

  const when = el("div");
  const whenLabel = el("div", "when", shortDate(ticket.startsAt));
  whenLabel.style.color = "#b05400";
  when.append(whenLabel, el("div", "name", ticket.eventName), el("div", "where", ticket.venue));
  when.style.textAlign = "left";

  const seatNo = el("div", "n", ticket.seatId);
  seatNo.style.font = "700 34px/1 var(--font-display)";
  seatNo.style.color = "#b05400";

  top.append(when, seatNo);

  const qr = el("img", "qr");
  qr.src = `/tickets/${encodeURIComponent(ticket.ticketCode)}/qr.png`;
  qr.alt = `Ticket ${ticket.ticketCode}`;

  const rowseat = el("div", "rowseat");
  for (const [label, value] of [["Row", ticket.row], ["Seat", String(ticket.number)]]) {
    const cell = el("div");
    cell.append(el("div", "label", label), el("div", null, value));
    cell.querySelector("div:last-child").style.font = "700 24px/1 var(--font-display)";
    rowseat.append(cell);
  }

  full.append(top, qr, el("div", "code", ticket.ticketCode), rowseat);
  node.append(head, hint, full);

  return node;
}

// ---- account ----

export function accountView(ctx, account) {
  const node = el("div");
  node.append(el("h1", "title", "Account"));

  const card = el("div", "card");
  card.style.display = "flex";
  card.style.alignItems = "center";
  card.style.gap = "16px";
  card.style.marginTop = "18px";

  const avatar = el("div", null, (account.email[0] ?? "?").toUpperCase());
  avatar.style.cssText =
    "width:64px;height:64px;border-radius:50%;background:var(--surface-raised);display:grid;place-items:center;font:700 26px/1 var(--font-display);flex:none";

  const who = el("div");
  who.append(el("div", "label", "Email"), el("div", "heading", account.email));

  card.append(avatar, who);
  node.append(card);

  if (!account.emailVerified) {
    const warn = el("div", "card");
    warn.style.marginTop = "14px";
    warn.style.borderColor = "color-mix(in srgb, var(--warning) 40%, transparent)";
    warn.style.background = "var(--warning-bg)";
    warn.append(
      el("div", "heading", "Confirm your email"),
      el(
        "p",
        "muted",
        `We sent a link to ${account.email}. Holding a seat needs a confirmed address.`,
      ),
    );

    const resend = el("button", "btn btn-secondary", "Send the link again");
    resend.addEventListener("click", async () => {
      resend.disabled = true;
      try {
        await api("POST", "/auth/verify/resend");
        banner("success", "On its way", "Check your inbox, and the spam folder.");
      } catch (err) {
        showError(err);
      } finally {
        resend.disabled = false;
      }
    });

    warn.append(resend);
    node.append(warn);
  }

  if (ctx.state.notifications?.length) {
    const label = el("div", "label", "Notifications");
    label.style.margin = "24px 0 10px";
    node.append(label);

    for (const notice of ctx.state.notifications.slice(0, 8)) {
      const row = el("div", "card");
      row.style.marginBottom = "10px";
      if (!notice.read) row.style.borderColor = "color-mix(in srgb, var(--primary) 45%, transparent)";

      row.append(
        el("div", "heading", noticeTitle(notice)),
        el("div", "caption", longDate(notice.createdAt)),
      );
      node.append(row);
    }
  }

  const out = el("button", "btn btn-danger btn-block");
  out.style.marginTop = "28px";
  out.innerHTML = icon.signout;
  out.append(el("span", null, "Sign out"));
  out.addEventListener("click", () => ctx.confirmSignOut());

  const note = el("p", "caption");
  note.style.textAlign = "center";
  note.textContent = "Signs out on this device only. Your other devices stay signed in.";

  node.append(out, note);

  return node;
}

function noticeTitle(notice) {
  if (notice.kind === "event_cancelled") {
    return notice.seatId
      ? `${notice.eventName} was cancelled — seat ${notice.seatId}`
      : `${notice.eventName} was cancelled — you were in the line`;
  }

  return notice.eventName;
}

export function notPermittedView(ctx) {
  return empty(
    icon.lockEmpty,
    "You don't have access to this page",
    "This area is for SeatHold admins only.",
    { label: "Back to events", onClick: () => ctx.go("events") },
  );
}

// ---- manage ----
//
// There is no "make admin" control anywhere here. The first administrator is
// made with the admin command, which needs database access; an interface that
// could create one is an interface that could be talked into creating one.

export function newEventView(ctx) {
  const node = el("div");
  node.append(el("div", "label", "Manage"), el("h1", "title", "New event"));

  const form = el("form");
  form.noValidate = true;
  form.style.marginTop = "20px";

  const name = field("Event name", "text", "Midnight Strings Quartet");
  const venue = field("Venue", "text", "Harbor Hall");
  const city = field("City", "text", "Istanbul");
  const kind = choice("Kind", CATEGORIES, "concert");
  const image = field("Poster address", "url", "https://…");
  const about = field("About this event", "textarea", "What people are coming to see.");
  const rules = field("Good to know", "textarea", "Doors, age limits, anything they should know before they buy.");

  const whenWrap = el("div", "field");
  whenWrap.append(el("label", null, "Starts"));

  const whenRow = el("div");
  whenRow.style.display = "flex";
  whenRow.style.gap = "10px";

  const date = el("input");
  date.type = "date";
  date.style.flex = "1";

  const time = el("input");
  time.type = "time";
  time.value = "20:00";
  time.style.flex = "0 0 40%";

  whenRow.append(date, time);
  whenWrap.append(whenRow);

  const submit = el("button", "btn btn-primary btn-block", "Create event");
  submit.type = "submit";
  submit.style.marginTop = "8px";

  form.append(name.wrap, venue.wrap, city.wrap, kind.wrap, whenWrap, image.wrap, about.wrap, rules.wrap, submit);

  form.addEventListener("submit", async (event) => {
    event.preventDefault();
    clearBanners();

    if (!date.value) {
      banner("error", "Pick a date", "An event needs a day and a time.");

      return;
    }

    submit.disabled = true;

    try {
      // Sent as an instant, so the server is never left guessing which clock a
      // wall time belongs to.
      const startsAt = new Date(`${date.value}T${time.value || "00:00"}`).toISOString();

      const created = await api("POST", "/admin/events", {
        body: {
          name: name.input.value,
          venue: venue.input.value,
          startsAt,
          city: city.input.value,
          category: kind.input.value,
          imageUrl: image.input.value,
          description: about.input.value,
          rules: rules.input.value,
        },
      });

      ctx.go("manage-created", { event: created });
    } catch (err) {
      submit.disabled = false;

      if (err.code === "invalid_event") banner("error", "Check the details", err.message);
      else showError(err);
    }
  });

  node.append(form);

  return node;
}

export function eventCreatedView(ctx, event) {
  const node = el("div");
  node.append(el("div", "label", "Manage"), el("h1", "title", "New event"));

  const tick = el("div", "tick");
  tick.innerHTML = icon.check;

  const centre = el("div");
  centre.style.textAlign = "center";
  centre.style.margin = "40px 0 28px";
  centre.append(
    tick,
    el("h2", "title", "Event created"),
    el("p", "muted", "It's in the events list now. Add seats so people can start holding them."),
  );

  const card = el("div", "event");
  card.disabled = true;
  const when = dateParts(event.startsAt);
  const dateBox = el("div", "date");
  dateBox.append(el("div", "mon", when.mon), el("div", "day", when.day), el("div", "dow", when.dow));

  const body = el("div");
  body.append(el("div", "name", event.name), el("div", "where", `${event.venue} · ${timeOf(event.startsAt)}`));

  const noSeats = el("span", "tag");
  noSeats.style.cssText = "border:1px dashed var(--line-strong);color:var(--text-muted);margin-top:6px";
  noSeats.textContent = "No seats yet";
  body.append(noSeats);

  card.append(dateBox, body);

  const add = el("button", "btn btn-primary btn-block", "+ Add seats");
  add.style.marginTop = "28px";
  add.addEventListener("click", () => ctx.go("manage-seats", { event }));

  const another = el("button", "btn btn-secondary btn-block", "Create another event");
  another.style.marginTop = "10px";
  another.addEventListener("click", () => ctx.go("manage-new"));

  node.append(centre, card, add, another);

  return node;
}

export function addSeatsView(ctx, event) {
  const node = el("div");
  node.append(el("div", "label", "Manage"), el("h1", "title", "Add seats"));
  node.append(el("p", "muted", `${event.name} · ${event.venue}`));

  const LIMIT = 2000;
  let rows = ["A", "B", "C", "D"];
  let perRow = 10;

  const rowsWrap = el("div", "field");
  const rowsHead = el("div", "row");
  rowsHead.append(el("label", null, "Row labels"));
  const rowCount = el("span", "field-hint", "");
  rowsHead.append(rowCount);

  const chips = el("div", "chips");
  const chipInput = el("input");
  chipInput.placeholder = "Add row…";
  chipInput.setAttribute("aria-label", "Add a row label");

  rowsWrap.append(rowsHead, chips, el("div", "field-hint", "Press Enter or comma after each label."));

  const perWrap = el("div", "field");
  const perRowHead = el("div", "row");
  perRowHead.append(el("label", null, "Seats per row"));

  const stepper = el("div", "stepper");
  const minus = el("button", null, "−");
  const amount = el("input");
  const plus = el("button", null, "+");
  minus.type = plus.type = "button";
  amount.type = "number";
  amount.min = "1";
  amount.value = String(perRow);
  stepper.append(minus, amount, plus);
  perRowHead.append(stepper);
  perWrap.append(perRowHead);

  const total = el("div", "total");
  const preview = el("div", "preview");
  const submit = el("button", "btn btn-primary btn-block");
  submit.type = "button";

  const draw = () => {
    rowCount.textContent = rows.length > 6 ? `${rows.length} rows` : "";

    chips.replaceChildren();
    for (const label of rows) {
      const chip = el("span", "chip", label);
      const remove = el("button", null, "×");
      remove.type = "button";
      remove.setAttribute("aria-label", `Remove row ${label}`);
      remove.addEventListener("click", () => {
        rows = rows.filter((r) => r !== label);
        draw();
      });
      chip.append(remove);
      chips.append(chip);
    }
    chips.append(chipInput);

    const count = rows.length * perRow;
    const over = count > LIMIT;

    total.className = `total${over ? " over" : ""}`;
    total.replaceChildren();

    const row = el("div", "row");
    row.append(el("span", "muted", `${rows.length} rows × ${perRow} seats`));
    const n = el("span");
    n.append(el("span", "n", count.toLocaleString()), document.createTextNode(" seats"));
    row.append(n);

    const meter = el("div", "meter");
    const fill = el("i");
    fill.style.width = `${Math.min(100, (count / LIMIT) * 100)}%`;
    meter.append(fill);

    total.append(row, meter);

    if (over) {
      const message = el("div", "field-error");
      message.innerHTML = icon.alert;
      message.append(
        el(
          "span",
          null,
          `One request can add up to ${LIMIT.toLocaleString()} seats. Remove a row or lower seats per row, then send the rest in a second request.`,
        ),
      );
      total.append(message);
    } else {
      total.append(
        el(
          "div",
          "field-hint",
          `Up to ${LIMIT.toLocaleString()} per request · ${(LIMIT - count).toLocaleString()} to spare`,
        ),
      );
    }

    preview.replaceChildren();
    const head = el("div", "head");
    head.append(el("span", "label", "Preview"), el("span", "label", "Stage ↑"));
    preview.append(head);

    for (const label of rows.slice(0, 6)) {
      const prow = el("div", "prow");
      prow.append(el("span", "row-label", label));
      for (let i = 0; i < Math.min(perRow, 40); i += 1) prow.append(el("i"));
      preview.append(prow);
    }

    if (rows.length > 6) preview.append(el("div", "caption", `+ ${rows.length - 6} more rows`));

    submit.disabled = over || rows.length === 0;
    submit.textContent = over
      ? `Over the ${LIMIT.toLocaleString()} limit`
      : `Add ${count.toLocaleString()} seats`;
  };

  const addLabel = () => {
    const value = chipInput.value.trim().toUpperCase();
    chipInput.value = "";
    // A label used twice would build the same seat twice, which the server
    // refuses — so it is refused here instead of being sent to be refused.
    if (value && !rows.includes(value)) rows.push(value);
    draw();
  };

  chipInput.addEventListener("keydown", (event) => {
    if (event.key === "Enter" || event.key === ",") {
      event.preventDefault();
      addLabel();
    } else if (event.key === "Backspace" && chipInput.value === "" && rows.length > 0) {
      rows.pop();
      draw();
    }
  });
  chipInput.addEventListener("blur", addLabel);

  minus.addEventListener("click", () => {
    perRow = Math.max(1, perRow - 1);
    amount.value = String(perRow);
    draw();
  });
  plus.addEventListener("click", () => {
    perRow += 1;
    amount.value = String(perRow);
    draw();
  });
  amount.addEventListener("input", () => {
    perRow = Math.max(1, Number(amount.value) || 1);
    draw();
  });

  submit.addEventListener("click", async () => {
    clearBanners();
    submit.disabled = true;

    try {
      const result = await api("POST", `/admin/events/${encodeURIComponent(event.id)}/seats`, {
        body: { rows, seatsPerRow: perRow },
      });

      ctx.go("manage-added", { event, rows: [...rows], perRow, created: result.created });
    } catch (err) {
      submit.disabled = false;
      showError(err);
    }
  });

  node.append(
    rowsWrap,
    perWrap,
    total,
    preview,
    el(
      "p",
      "caption",
      "Seats that already exist are left as they are, so adding the same row twice is safe.",
    ),
    submit,
  );

  draw();

  return node;
}

export function seatsAddedView(ctx, result) {
  const node = el("div");
  node.append(el("div", "label", "Manage"), el("h1", "title", "Add seats"));

  const tick = el("div", "tick");
  tick.innerHTML = icon.check;

  const asked = result.rows.length * result.perRow;
  const existing = asked - result.created;

  const centre = el("div");
  centre.style.textAlign = "center";
  centre.style.margin = "36px 0 24px";
  centre.append(tick, el("h2", "title", `${result.created.toLocaleString()} seats added`));

  if (existing > 0) {
    centre.append(
      el(
        "p",
        "muted",
        `${existing.toLocaleString()} were already there, so we left them as they were. Adding the same rows again never creates duplicates.`,
      ),
    );
  }

  const card = el("div", "card");
  card.style.padding = "0";

  // Which rows were new is not something the server reports per row, so the
  // honest thing is to say how many landed rather than to guess row by row.
  const summary = el("div", "result-row");
  summary.append(
    el("span", null, `${result.rows.join(", ")} · ${result.perRow} per row`),
    el("span", "badge badge-added", `${result.created} added`),
  );
  card.append(summary);

  if (existing > 0) {
    const kept = el("div", "result-row");
    kept.append(el("span", null, "Already there"), el("span", "badge badge-existing", String(existing)));
    card.append(kept);
  }

  const view = el("button", "btn btn-primary btn-block", "View seat map");
  view.style.marginTop = "26px";
  view.addEventListener("click", () => ctx.go("seats", { eventID: result.event.id }));

  const more = el("button", "btn btn-secondary btn-block", "Add more rows");
  more.style.marginTop = "10px";
  more.addEventListener("click", () => ctx.go("manage-seats", { event: result.event }));

  node.append(centre, card, view, more);

  return node;
}

// ---- manage: the events that exist ----
//
// Withdrawing and removing are different things and the screen says so.
// Cancelling leaves the event where people holding tickets can still find it;
// removing is for a mistake, and the server refuses it the moment a seat is
// spoken for.

export function manageView(ctx, events) {
  const node = el("div");

  const head = el("div", "screen-head");
  head.append(el("div", "label", "Manage"), el("h1", "title", "Events"));
  node.append(head);

  const create = el("button", "btn btn-primary btn-block", "+ New event");
  create.style.marginBottom = "22px";
  create.addEventListener("click", () => ctx.go("manage-new"));
  node.append(create);

  if (events.length === 0) {
    node.append(
      empty(icon.stageEmpty, "No events yet", "Create one, then add the rows people will sit in."),
    );

    return node;
  }

  for (const event of events) {
    const card = el("div", "card");
    card.style.marginBottom = "12px";

    const top = el("div");
    top.style.display = "flex";
    top.style.gap = "14px";

    const when = dateParts(event.startsAt);
    const date = el("div", "date");
    date.style.cssText =
      "width:64px;flex:none;text-align:center;background:var(--surface-raised);border-radius:12px;padding:8px 0";
    date.append(el("div", "mon", when.mon), el("div", "day", when.day), el("div", "dow", when.dow));

    const body = el("div");
    body.style.flex = "1";
    body.style.minWidth = "0";

    if (event.cancelled) body.append(el("span", "tag tag-cancelled", "Cancelled"));
    else if (event.hasStarted) body.append(el("span", "tag tag-started", "Started"));

    body.append(
      el("div", "name", event.name),
      el("div", "where", `${event.venue} · ${longDate(event.startsAt)}`),
    );

    top.append(date, body);

    const actions = el("div");
    actions.style.cssText = "display:flex;gap:8px;flex-wrap:wrap;margin-top:14px";

    const seats = el("button", "btn btn-secondary", "Add seats");
    seats.style.flex = "1";
    seats.addEventListener("click", () => ctx.go("manage-seats", { event }));
    actions.append(seats);

    // A cancelled event is a record of something that is not happening. Editing
    // it would quietly turn it into a different one, so the server refuses and
    // the button is not offered.
    if (!event.cancelled) {
      const edit = el("button", "btn btn-secondary", "Edit");
      edit.style.flex = "1";
      edit.addEventListener("click", () => ctx.go("manage-edit", { event }));
      actions.append(edit);

      const cancel = el("button", "btn btn-danger", "Withdraw");
      cancel.style.flex = "1";
      cancel.addEventListener("click", () => ctx.confirmCancelEvent(event));
      actions.append(cancel);
    }

    const remove = el("button", "btn btn-danger", "Delete");
    remove.style.flex = "1";
    remove.addEventListener("click", () => ctx.confirmDeleteEvent(event));
    actions.append(remove);

    card.append(top, actions);
    node.append(card);
  }

  return node;
}

export function editEventView(ctx, event) {
  const node = el("div");
  node.append(el("div", "label", "Manage"), el("h1", "title", "Edit event"));
  node.append(el("p", "muted", "Leave a field as it is to keep it."));

  const form = el("form");
  form.noValidate = true;
  form.style.marginTop = "20px";

  const name = field("Event name", "text", "");
  const venue = field("Venue", "text", "");
  const city = field("City", "text", "Istanbul");
  const kind = choice("Kind", CATEGORIES, event.category ?? "other");
  const image = field("Poster address", "url", "https://…");
  const about = field("About this event", "textarea", "");
  const rules = field("Good to know", "textarea", "");

  name.input.value = event.name;
  venue.input.value = event.venue;
  city.input.value = event.city ?? "";
  image.input.value = event.imageUrl ?? "";
  about.input.value = event.description ?? "";
  rules.input.value = event.rules ?? "";

  const whenWrap = el("div", "field");
  whenWrap.append(el("label", null, "Starts"));

  const whenRow = el("div");
  whenRow.style.display = "flex";
  whenRow.style.gap = "10px";

  const at = new Date(event.startsAt);
  const pad = (n) => String(n).padStart(2, "0");

  const date = el("input");
  date.type = "date";
  date.style.flex = "1";
  date.value = `${at.getFullYear()}-${pad(at.getMonth() + 1)}-${pad(at.getDate())}`;

  const time = el("input");
  time.type = "time";
  time.style.flex = "0 0 40%";
  time.value = `${pad(at.getHours())}:${pad(at.getMinutes())}`;

  whenRow.append(date, time);
  whenWrap.append(whenRow);

  const save = el("button", "btn btn-primary btn-block", "Save changes");
  save.type = "submit";
  save.style.marginTop = "8px";

  const back = el("button", "btn btn-secondary btn-block", "Cancel");
  back.type = "button";
  back.style.marginTop = "10px";
  back.addEventListener("click", () => ctx.go("manage"));

  form.append(name.wrap, venue.wrap, city.wrap, kind.wrap, whenWrap, image.wrap, about.wrap, rules.wrap, save, back);

  form.addEventListener("submit", async (submitted) => {
    submitted.preventDefault();
    clearBanners();
    save.disabled = true;

    try {
      // Only what changed. Sending a field back unchanged is how a stale value
      // overwrites one somebody else just fixed.
      const body = {};
      if (name.input.value !== event.name) body.name = name.input.value;
      if (venue.input.value !== event.venue) body.venue = venue.input.value;

      const startsAt = new Date(`${date.value}T${time.value || "00:00"}`).toISOString();
      if (startsAt !== new Date(event.startsAt).toISOString()) body.startsAt = startsAt;

      // Sent only when changed, which is what lets the server tell "leave it"
      // from "clear it".
      for (const [key, box, was] of [
        ["city", city, event.city],
        ["category", kind, event.category],
        ["imageUrl", image, event.imageUrl],
        ["description", about, event.description],
        ["rules", rules, event.rules],
      ]) {
        if (box.input.value !== (was ?? "")) body[key] = box.input.value;
      }

      if (Object.keys(body).length === 0) {
        ctx.go("manage");

        return;
      }

      await api("PATCH", `/admin/events/${encodeURIComponent(event.id)}`, { body });
      ctx.go("manage", { flash: ["success", "Saved", `${name.input.value} is updated.`] });
    } catch (err) {
      save.disabled = false;
      showError(err);
    }
  });

  node.append(form);

  return node;
}

// sheet is the shape every irreversible answer is asked in: one question, the
// consequence spelled out, and the destructive choice named rather than called
// "OK".
export function sheet({ title, text, confirmLabel, danger = true, onConfirm }) {
  const scrim = el("div", "scrim");
  const panel = el("div", "sheet");

  panel.append(el("div", "grab"), el("h2", null, title), el("p", null, text));

  const actions = el("div", "actions");
  const go = el("button", `btn ${danger ? "btn-danger-solid" : "btn-primary"}`, confirmLabel);
  const cancel = el("button", "btn btn-secondary", "Cancel");

  go.addEventListener("click", async () => {
    go.disabled = true;
    go.replaceChildren(el("span", "spinner"), document.createTextNode("Working…"));

    try {
      await onConfirm();
      scrim.remove();
    } catch (err) {
      scrim.remove();
      showError(err);
    }
  });

  cancel.addEventListener("click", () => scrim.remove());
  scrim.addEventListener("click", (event) => {
    if (event.target === scrim) scrim.remove();
  });

  actions.append(go, cancel);
  panel.append(actions);
  scrim.append(panel);

  return scrim;
}

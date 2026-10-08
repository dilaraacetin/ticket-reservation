// SeatHold. The router decides what is on the page; everything else hands it a
// node and gets out of the way.

import { api, ApiError, hasToken, openStream, setToken } from "./api.js";
import { banner, clearBanners, dismissBanner, el, icon, showError } from "./ui.js";
import {
  accountView,
  addSeatsView,
  authView,
  editEventView,
  eventCreatedView,
  eventDetailView,
  eventsView,
  holdBar,
  holdPrompt,
  manageView,
  newEventView,
  notPermittedView,
  renderSeats,
  seatMapView,
  seatsAddedView,
  sheet,
  ticketView,
  ticketsView,
  waitlistPanel,
} from "./views.js";

const state = {
  account: null,
  route: "auth",
  params: {},

  city: "",
  category: "",

  event: null,
  seats: [],
  hold: null,
  place: null,
  stale: false,
  pushOn: false,
  notifications: [],

  stream: null,
  countdown: null,
  poll: null,
  authMode: "signin",
};

const screen = () => document.getElementById("screen");
const layer = () => document.getElementById("layer");

// ---- the actions the views call back into ----

const ctx = {
  state,
  go,
  setFilters,
  signedIn,
  holdSeat,
  confirmHold,
  releaseHold,
  holdRanOut,
  joinQueue,
  leaveQueue,
  confirmSignOut,
  confirmCancelEvent,
  confirmDeleteEvent,
};

// ---- routing ----

async function go(route, params = {}) {
  // Leaving a seat map tears down everything it was running, so nothing keeps
  // ticking behind a screen nobody is looking at.
  if (state.route === "seats" && route !== "seats") {
    state.stream?.close();
    state.stream = null;
    clearInterval(state.poll);
    state.stale = false;
  }

  state.route = route;
  state.params = params;

  clearBanners();
  await render();

  screen().focus({ preventScroll: true });
  window.scrollTo(0, 0);
}

// setFilters narrows the catalogue. It redraws rather than routing, because the
// filters are a view of the same screen and going back from an event should not
// land on an unfiltered list.
async function setFilters({ category = state.category, city = state.city }) {
  state.category = category;
  state.city = city;
  await render();
}

async function render() {
  const view = screen();
  view.replaceChildren();
  layer().replaceChildren();

  renderTabs();

  try {
    switch (state.route) {
      case "auth":
        view.append(authView(ctx));
        break;

      case "events": {
        const events = await api("GET", "/events");
        view.append(eventsView(ctx, events));
        break;
      }

      case "event": {
        const event = await api("GET", `/events/${encodeURIComponent(state.params.eventID)}`);
        view.append(eventDetailView(ctx, event));
        break;
      }

      case "seats":
        await renderSeatMap(view);
        break;

      case "tickets": {
        const body = await api("GET", "/tickets");
        // The hold bar covers holds; this screen is about tickets.
        view.append(ticketsView(ctx, body.reservations));
        break;
      }

      case "ticket": {
        const body = await api("GET", "/tickets");
        const ticket = body.reservations.find((t) => t.ticketCode === state.params.code);

        if (!ticket) {
          await go("tickets");

          return;
        }

        view.append(ticketView(ctx, ticket));
        break;
      }

      case "account":
        await loadNotifications();
        view.append(accountView(ctx, state.account));
        break;

      case "manage": {
        if (!isAdmin()) {
          view.append(notPermittedView(ctx));
          break;
        }

        const events = await api("GET", "/events");
        view.append(manageView(ctx, events));

        // Carried across a redirect rather than shown before it, so the message
        // is not wiped by the render it is about.
        if (state.params.flash) banner(...state.params.flash);
        break;
      }

      case "manage-new":
        if (!isAdmin()) {
          view.append(notPermittedView(ctx));
          break;
        }
        view.append(newEventView(ctx));
        break;

      case "manage-edit":
        if (!isAdmin()) {
          view.append(notPermittedView(ctx));
          break;
        }
        view.append(editEventView(ctx, state.params.event));
        break;

      case "manage-created":
        view.append(eventCreatedView(ctx, state.params.event));
        break;

      case "manage-seats":
        view.append(addSeatsView(ctx, state.params.event));
        break;

      case "manage-added":
        view.append(seatsAddedView(ctx, state.params));
        break;

      default:
        await go("events");
    }
  } catch (err) {
    if (err instanceof ApiError && (err.code === "unauthenticated" || err.code === "invalid_token")) {
      setToken(null);
      state.account = null;
      await go("auth");

      return;
    }

    showError(err);
  }
}

function isAdmin() {
  return state.account?.role === "admin";
}

function renderTabs() {
  const nav = document.getElementById("tabs");

  // No account, no sections: the only thing on screen is the way in.
  if (!state.account && state.route === "auth") {
    nav.classList.add("hidden");

    return;
  }

  nav.classList.remove("hidden");
  nav.replaceChildren();

  const tabs = [
    ["events", "Events", icon.calendar],
    ["tickets", "My tickets", icon.ticket],
    ["account", "Account", icon.person],
  ];

  // The Manage tab exists only for administrators. Somebody who reaches the
  // route another way still meets the refusal, so this is tidiness rather than
  // the control itself.
  if (isAdmin()) tabs.push(["manage", "Manage", icon.sliders]);

  for (const [route, text, art] of tabs) {
    const tab = el("button", "tab");
    const pill = el("span", "pill");
    pill.innerHTML = art;

    tab.append(pill, el("span", null, text));

    const here = state.route === route || state.route.startsWith(`${route}-`) ||
      (route === "tickets" && state.route === "ticket") ||
      (route === "events" && (state.route === "event" || state.route === "seats"));

    if (here) tab.setAttribute("aria-current", "page");

    tab.addEventListener("click", () => {
      if (!state.account && route !== "events") {
        go("auth");

        return;
      }

      go(route);
    });

    nav.append(tab);
  }
}

// ---- the seat map ----

async function renderSeatMap(view) {
  const id = encodeURIComponent(state.params.eventID);

  let event, map;
  try {
    [event, map] = await Promise.all([
      api("GET", `/events/${id}`),
      api("GET", `/events/${id}/seats`),
    ]);
  } catch (err) {
    // Gone means back to the catalogue. Anything else is a real failure and
    // belongs to the caller, which puts it on screen.
    if (err.status !== 404) throw err;

    await go("events");

    return;
  }

  state.event = event;
  state.seats = map.seats;

  view.append(seatMapView(ctx, state.event, state.seats));

  if (state.event.cancelled) {
    banner("error", "This event was cancelled", "It is no longer going ahead.", { dismissible: false });
  }

  await recoverHold();
  await refreshPlace();

  paintBottom();
  openSeatStream();

  // A slow poll behind the stream, not instead of it. It covers the gap while a
  // dropped stream reconnects.
  clearInterval(state.poll);
  state.poll = setInterval(reloadSeats, 15000);
}

// recoverHold puts the page back where it was. A reload during a hold would
// otherwise leave the seat held with nothing on screen able to confirm it.
async function recoverHold() {
  if (!state.account) return;

  const body = await api("GET", "/tickets");
  const mine = body.holds.find((h) => h.eventId === state.event.id);

  if (!mine) {
    state.hold = null;

    return;
  }

  state.hold = {
    holdId: mine.holdId,
    seatId: mine.seatId,
    eventName: mine.eventName,
    expiresAt: new Date(mine.expiresAt).getTime(),
    startedAt: Date.now() - (300 - mine.expiresInSeconds) * 1000,
  };
}

async function refreshPlace() {
  state.place = null;

  if (!state.account) return;

  try {
    const place = await api("GET", `/events/${encodeURIComponent(state.event.id)}/waiting-list`);
    state.place = place.position;
  } catch (err) {
    // Not being in the queue is the ordinary case, not a failure.
    if (!(err instanceof ApiError) || err.code !== "not_waiting") throw err;
  }
}

// paintBottom decides what sits at the bottom of the seat map: a running hold, a
// queue, or the invitation to tap a seat.
function paintBottom() {
  layer().replaceChildren();

  if (state.hold) {
    layer().append(holdBar(ctx));

    return;
  }

  clearInterval(state.countdown);

  const open = state.seats.filter((s) => s.status === "available").length;

  if (open === 0 && state.account && !state.event.cancelled) {
    layer().append(waitlistPanel(ctx, state.place));

    return;
  }

  if (state.account && !state.event.cancelled) layer().append(holdPrompt());
}

async function reloadSeats() {
  if (state.route !== "seats") return;

  const map = await api("GET", `/events/${encodeURIComponent(state.event.id)}/seats`);
  state.seats = map.seats;

  repaintSeats();
}

function repaintSeats() {
  const grid = document.getElementById("seat-grid");
  const counts = document.getElementById("counts");
  if (!grid || !counts) return;

  renderSeats(ctx, grid, counts, state.seats);
  paintBottom();
}

function openSeatStream() {
  state.stream?.close();

  state.stream = openStream(state.event.id, {
    onOpen() {
      if (state.stale) {
        banner("success", "Back online", "The seat map is up to date.", { id: "stream" });
        setTimeout(() => dismissBanner("stream"), 4000);
      }

      state.stale = false;
      document.getElementById("live-tag")?.replaceChildren(document.createTextNode("Live"));
      reloadSeats().catch(() => {});
    },

    onLost() {
      state.stale = true;

      const tag = document.getElementById("live-tag");
      if (tag) {
        tag.className = "tag tag-started";
        tag.textContent = "Reconnecting…";
      }

      banner("warning", "Reconnecting…", "Seat changes may be out of date for a moment.", {
        id: "stream",
        icon: icon.offline,
        action: { label: "Retry now", onClick: () => state.stream?.retry() },
      });

      repaintSeats();
    },

    seat_changed() {
      reloadSeats().catch(() => {});
    },

    event_cancelled() {
      state.hold = null;
      state.place = null;
      banner("error", "This event was cancelled", "It is no longer going ahead.", { dismissible: false });
      go("seats", state.params);
    },

    // A seat came free and the server has already held it. The page steps into
    // that hold as if the person had tapped the seat themselves.
    turn_came(notice) {
      state.place = null;
      state.hold = {
        holdId: notice.holdId,
        seatId: notice.seatId,
        eventName: state.event?.name ?? "",
        expiresAt: new Date(notice.expiresAt).getTime(),
        startedAt: Date.now(),
        fromWaitlist: true,
      };

      banner("info", "It's your turn!", `${notice.seatId} is held for you for 5 minutes.`, {
        icon: icon.check,
      });

      reloadSeats().catch(() => {});
    },
  });
}

// ---- seat actions ----

async function holdSeat(seat) {
  if (!state.account) {
    await go("auth");

    return;
  }

  clearBanners();

  try {
    const hold = await api(
      "POST",
      `/events/${encodeURIComponent(state.event.id)}/seats/${encodeURIComponent(seat.id)}/hold`,
      { idempotencyKey: crypto.randomUUID() },
    );

    state.hold = {
      holdId: hold.holdId,
      seatId: hold.seatId,
      eventName: state.event.name,
      expiresAt: new Date(hold.expiresAt).getTime(),
      startedAt: Date.now(),
    };

    await reloadSeats();

    document.querySelector(".seat.mine")?.classList.add("just-held");
  } catch (err) {
    if (err instanceof ApiError) showError(err, { seat: seat.id });
    else throw err;

    await reloadSeats();
  }
}

async function confirmHold() {
  clearBanners();

  const seatId = state.hold?.seatId;

  try {
    const reservation = await api("POST", `/holds/${encodeURIComponent(state.hold.holdId)}/confirm`, {
      // A key, so a retry after a lost answer replays the first one rather than
      // reporting that the hold has gone.
      idempotencyKey: crypto.randomUUID(),
    });

    dropHold();
    banner("success", `Seat ${reservation.seatId} is yours`, "It's in My tickets, ready to scan at the door.");
    await reloadSeats();
  } catch (err) {
    dropHold();
    if (err instanceof ApiError) showError(err, { seat: seatId });
    else throw err;

    await reloadSeats();
  }
}

async function releaseHold() {
  clearBanners();

  try {
    await api("DELETE", `/holds/${encodeURIComponent(state.hold.holdId)}`);
  } catch (err) {
    if (err instanceof ApiError) showError(err);
  } finally {
    dropHold();
    await reloadSeats();
  }
}

function holdRanOut() {
  const seatId = state.hold?.seatId;

  dropHold();
  banner("error", "Your time ran out", `${seatId} is open again. Tap any seat to start a new 5-minute hold.`);
  reloadSeats().catch(() => {});
}

function dropHold() {
  state.hold = null;
  clearInterval(state.countdown);
  paintBottom();
}

// ---- the queue ----

async function joinQueue() {
  clearBanners();

  try {
    const place = await api("POST", `/events/${encodeURIComponent(state.event.id)}/waiting-list`);
    state.place = place.position;

    // Asked for only now: somebody who has joined a queue has a reason to want
    // telling, which is the moment to ask rather than on the way in.
    await enablePush();
    paintBottom();
  } catch (err) {
    if (err instanceof ApiError) showError(err);
    else throw err;
  }
}

async function leaveQueue() {
  clearBanners();

  try {
    await api("DELETE", `/events/${encodeURIComponent(state.event.id)}/waiting-list`);
    state.place = null;
    paintBottom();
  } catch (err) {
    if (err instanceof ApiError) showError(err);
  }
}

// ---- account ----

async function signedIn() {
  state.account = await api("GET", "/account");

  if (!state.account.emailVerified) {
    banner(
      "warning",
      "Confirm your email",
      `We sent a link to ${state.account.email}. Holding a seat needs a confirmed address.`,
      { id: "verify" },
    );
  }

  await go("events");
}

async function loadNotifications() {
  try {
    const body = await api("GET", "/notifications");
    state.notifications = body.notifications;

    if (body.unread > 0) await api("POST", "/notifications/read");
  } catch {
    // A deployment with nowhere to keep notices has no routes for them, which
    // is not an error worth showing anybody.
    state.notifications = [];
  }
}

// Withdrawing leaves the event where the people holding tickets can still find
// it. The sheet says what will happen rather than asking whether the person is
// sure.
function confirmCancelEvent(event) {
  layer().append(
    sheet({
      title: `Withdraw ${event.name}?`,
      text:
        "It stays in the list, marked cancelled, so anyone holding a ticket can see what happened. " +
        "No new seats can be taken, and everybody affected is told. This cannot be undone.",
      confirmLabel: "Withdraw it",
      async onConfirm() {
        await api("POST", `/admin/events/${encodeURIComponent(event.id)}/cancel`);
        await go("manage", {
          flash: ["success", "Withdrawn", `${event.name} is no longer on sale. Everyone affected has been told.`],
        });
      },
    }),
  );
}

// Removing is for a mistake. The server refuses the moment a seat is spoken for,
// and that refusal is the useful answer rather than an error.
function confirmDeleteEvent(event) {
  layer().append(
    sheet({
      title: `Delete ${event.name}?`,
      text:
        "This removes the event and its seats for good. It is refused if any seat is held or sold — " +
        "withdraw it instead when people already have tickets.",
      confirmLabel: "Delete it",
      async onConfirm() {
        try {
          await api("DELETE", `/admin/events/${encodeURIComponent(event.id)}`);
        } catch (err) {
          if (err instanceof ApiError && err.code === "event_in_use") {
            await go("manage");
            banner(
              "error",
              "This event has tickets",
              "Seats are held or sold, so it cannot be removed. Withdraw it instead.",
            );

            return;
          }

          throw err;
        }

        await go("manage", { flash: ["success", "Deleted", `${event.name} is gone.`] });
      },
    }),
  );
}

function confirmSignOut() {
  const scrim = el("div", "scrim");
  const sheet = el("div", "sheet");

  sheet.append(
    el("div", "grab"),
    el("h2", null, "Sign out of SeatHold?"),
    el("p", null, "You'll be signed out on this device. Your other devices stay signed in."),
  );

  const actions = el("div", "actions");
  const out = el("button", "btn btn-danger-solid", "Sign out");
  const cancel = el("button", "btn btn-secondary", "Cancel");

  out.addEventListener("click", signOut);
  cancel.addEventListener("click", () => scrim.remove());
  scrim.addEventListener("click", (event) => {
    if (event.target === scrim) scrim.remove();
  });

  actions.append(out, cancel);
  sheet.append(actions);
  scrim.append(sheet);
  document.body.append(scrim);
}

async function signOut() {
  // The server is told first, while the token is still in hand. Dropping it here
  // only stops this page from using it; the token stays good until somebody
  // records that it should not be.
  try {
    await api("POST", "/auth/logout");
  } catch (err) {
    if (err instanceof ApiError && err.code !== "logout_unavailable") showError(err);
  }

  setToken(null);
  state.account = null;
  state.hold = null;
  state.place = null;
  state.notifications = [];
  clearInterval(state.countdown);
  state.stream?.close();
  state.stream = null;

  document.querySelector(".scrim")?.remove();
  await go("auth");
}

// ---- push ----

async function enablePush() {
  if (!("serviceWorker" in navigator) || !("PushManager" in window)) return;

  let key;
  try {
    key = (await api("GET", "/push/key")).publicKey;
  } catch {
    // A deployment with no VAPID pair has no push routes at all.
    return;
  }

  if (!key) return;

  try {
    const registration = await navigator.serviceWorker.register("/sw.js");

    // The browser's own prompt. Refusing is a perfectly good answer that this
    // must not nag about.
    if ((await Notification.requestPermission()) !== "granted") return;

    const subscription = await registration.pushManager.subscribe({
      // Every push must be visible to the person. A silent one is how a push
      // subscription becomes a tracking channel, and browsers refuse it.
      userVisibleOnly: true,
      applicationServerKey: key,
    });

    await api("POST", "/push/subscriptions", { body: subscription.toJSON() });
    state.pushOn = true;
  } catch {
    // Permission refused, no service worker, an insecure origin: all the same
    // to this page, which is that there will be no push.
    state.pushOn = false;
  }
}

// ---- boot ----

async function start() {
  // A verification link lands on /verify?token=… and the page spends it. A GET
  // on the link itself would let a mailbox scanner spend it first.
  const params = new URLSearchParams(location.search);
  const token = params.get("token");

  if (location.pathname === "/verify" && token) {
    history.replaceState(null, "", "/");

    try {
      await api("POST", "/auth/verify", { body: { token } });
      banner("success", "Email confirmed", "You can hold seats now.");
    } catch (err) {
      if (err instanceof ApiError) showError(err);
    }
  }

  await go(hasToken() ? "events" : "auth");
}

start();

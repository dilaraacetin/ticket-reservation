// The browser side of the reservation service.
//
// Deliberately dependency free: the page is served from the same origin as the
// API, so there is nothing to configure and no build step between writing this
// and running it.

const $ = (id) => document.getElementById(id);

// The token lives in memory only. sessionStorage would survive a reload, but it
// is also readable by any script that ends up on the page, and a reload asking
// for a sign in is a small price.
const state = {
  token: null,
  userId: null,
  event: null,
  hold: null,        // { holdId, seatId, expiresAt }
  countdownTimer: null,
  refreshTimer: null,
};

// --- talking to the API -------------------------------------------------

// The one place a request is made, so headers and error handling are not spread
// across every call site.
async function api(method, path, { body, idempotencyKey } = {}) {
  const headers = {};

  if (state.token) headers["Authorization"] = `Bearer ${state.token}`;
  if (body) headers["Content-Type"] = "application/json";
  if (idempotencyKey) headers["Idempotency-Key"] = idempotencyKey;

  const response = await fetch(path, {
    method,
    headers,
    body: body ? JSON.stringify(body) : undefined,
  });

  // 204 has no body to read.
  const payload = response.status === 204 ? null : await response.json().catch(() => null);

  if (!response.ok) {
    throw new ApiError(response.status, payload, response.headers.get("Retry-After"));
  }

  return payload;
}

class ApiError extends Error {
  constructor(status, payload, retryAfter) {
    const code = payload?.error?.code ?? "unknown";
    super(payload?.error?.message ?? `request failed with ${status}`);

    this.status = status;
    this.code = code;
    this.retryAfter = retryAfter;
  }
}

// --- messages -----------------------------------------------------------

function showError(err) {
  const alert = $("alert");
  alert.classList.remove("hidden", "ok");

  // The server's codes are stable; its prose is not. Explaining the ones worth
  // explaining here keeps the page useful without inventing new vocabulary.
  const explanations = {
    seat_not_available: "Somebody else got there first.",
    seat_not_held: "That seat is not held, so there is nothing to confirm.",
    hold_expired: "Your hold ran out. Pick a seat again.",
    hold_not_found: "That hold no longer exists.",
    not_hold_owner: "That hold belongs to somebody else.",
    invalid_credentials: "That email address or password is wrong.",
    email_taken: "That address already has an account.",
    weak_password: "Passwords are at least 8 characters.",
    invalid_email: "That does not look like an email address.",
    unauthenticated: "Sign in first.",
    invalid_token: "Your session is no longer valid. Sign in again.",
    too_many_requests: err.retryAfter
      ? `Too many requests. Try again in ${err.retryAfter} seconds.`
      : "Too many requests, slow down.",
  };

  const explanation = explanations[err.code] ?? err.message;
  alert.innerHTML = `${explanation} <code>${err.code} · ${err.status}</code>`;
}

function showNote(message) {
  const alert = $("alert");
  alert.classList.remove("hidden");
  alert.classList.add("ok");
  alert.textContent = message;
}

function clearMessage() {
  $("alert").classList.add("hidden");
}

// --- screens ------------------------------------------------------------

function show(screen) {
  for (const id of ["screen-auth", "screen-events", "screen-seats"]) {
    $(id).classList.toggle("hidden", id !== screen);
  }
}

// --- signing in ---------------------------------------------------------

async function signIn(event) {
  event.preventDefault();
  clearMessage();

  const credentials = { email: $("email").value, password: $("password").value };

  try {
    const session = await api("POST", "/auth/login", { body: credentials });

    state.token = session.token;
    state.userId = session.userId;

    $("whoami-name").textContent = credentials.email;
    $("whoami").classList.remove("hidden");

    await loadEvents();
  } catch (err) {
    showError(err);
  }
}

async function signUp() {
  clearMessage();

  const credentials = { email: $("email").value, password: $("password").value };

  try {
    await api("POST", "/auth/register", { body: credentials });
    showNote("Account created. Signing you in.");

    // Registration deliberately returns no token, so signing in is its own call.
    await signIn(new Event("submit"));
  } catch (err) {
    showError(err);
  }
}

function signOut() {
  state.token = null;
  state.userId = null;
  releaseLocalHold();

  $("whoami").classList.add("hidden");
  clearInterval(state.refreshTimer);
  show("screen-auth");
}

// --- events -------------------------------------------------------------

async function loadEvents() {
  try {
    const events = await api("GET", "/events");
    const list = $("event-list");
    list.innerHTML = "";

    if (events.length === 0) {
      list.innerHTML = `<p class="muted">Nothing on sale yet.</p>`;
    }

    for (const event of events) {
      const card = document.createElement("button");
      card.className = "event";
      card.innerHTML = `
        <div class="event-name"></div>
        <div class="event-meta"></div>
        ${event.hasStarted ? `<div class="event-started">Already started</div>` : ""}
      `;

      // textContent rather than interpolation: an event name is data, and data
      // does not get to write markup.
      card.querySelector(".event-name").textContent = event.name;
      card.querySelector(".event-meta").textContent =
        `${event.venue} · ${new Date(event.startsAt).toLocaleString()}`;

      card.addEventListener("click", () => openEvent(event));
      list.appendChild(card);
    }

    show("screen-events");
  } catch (err) {
    showError(err);
  }
}

async function openEvent(event) {
  state.event = event;

  $("event-name").textContent = event.name;
  $("event-meta").textContent =
    `${event.venue} · ${new Date(event.startsAt).toLocaleString()}`;

  await loadSeats();
  show("screen-seats");

  // Polling, so a seat taken in another tab shows up here. Three seconds sits
  // well inside the rate limit and is fast enough to feel live.
  clearInterval(state.refreshTimer);
  state.refreshTimer = setInterval(loadSeats, 3000);
}

async function loadSeats() {
  try {
    const map = await api("GET", `/events/${encodeURIComponent(state.event.id)}/seats`);
    renderSeats(map.seats);
  } catch (err) {
    showError(err);
  }
}

function renderSeats(seats) {
  const container = $("seat-map");
  container.innerHTML = "";

  // Grouped by row, in the order the server sent them, which is already sorted.
  const rows = new Map();
  for (const seat of seats) {
    if (!rows.has(seat.row)) rows.set(seat.row, []);
    rows.get(seat.row).push(seat);
  }

  for (const [label, rowSeats] of rows) {
    const row = document.createElement("div");
    row.className = "seat-row";
    row.innerHTML = `<div class="seat-row-label"></div><div class="seat-row-seats"></div>`;
    row.querySelector(".seat-row-label").textContent = label;

    const cells = row.querySelector(".seat-row-seats");

    for (const seat of rowSeats) {
      const cell = document.createElement("button");

      // The seat map never says who holds a seat, so "yours" is something this
      // page knows locally rather than something the server tells it.
      const mine = state.hold?.seatId === seat.id;
      cell.className = `seat ${mine ? "mine" : seat.status}`;
      cell.textContent = seat.number;
      cell.disabled = seat.status !== "available" || state.hold !== null;

      if (seat.status === "available" && !state.hold) {
        cell.addEventListener("click", () => holdSeat(seat));
      }

      cells.appendChild(cell);
    }

    container.appendChild(row);
  }
}

// --- holding, confirming, releasing -------------------------------------

async function holdSeat(seat) {
  clearMessage();

  try {
    const hold = await api(
      "POST",
      `/events/${encodeURIComponent(state.event.id)}/seats/${encodeURIComponent(seat.id)}/hold`,
      { idempotencyKey: crypto.randomUUID() },
    );

    state.hold = { holdId: hold.holdId, seatId: hold.seatId, expiresAt: new Date(hold.expiresAt) };

    $("hold-seat").textContent = hold.seatId;
    $("hold-bar").classList.remove("hidden");

    startCountdown();
    await loadSeats();
  } catch (err) {
    showError(err);
    await loadSeats();
  }
}

async function confirmHold() {
  clearMessage();

  try {
    const reservation = await api("POST", `/holds/${encodeURIComponent(state.hold.holdId)}/confirm`, {
      // A key, so that a retry after a lost response replays the first answer
      // rather than reporting that the hold has gone.
      idempotencyKey: crypto.randomUUID(),
    });

    showNote(`Seat ${reservation.seatId} is yours.`);
    releaseLocalHold();
    await loadSeats();
  } catch (err) {
    showError(err);
    releaseLocalHold();
    await loadSeats();
  }
}

async function releaseHold() {
  clearMessage();

  try {
    await api("DELETE", `/holds/${encodeURIComponent(state.hold.holdId)}`);
  } catch (err) {
    showError(err);
  } finally {
    releaseLocalHold();
    await loadSeats();
  }
}

function releaseLocalHold() {
  state.hold = null;
  clearInterval(state.countdownTimer);
  $("hold-bar").classList.add("hidden");
}

// The countdown is the clearest sign that a hold is a temporary thing. It runs
// off the expiry the server sent, not off a local count, so a slow request does
// not leave the two disagreeing.
function startCountdown() {
  clearInterval(state.countdownTimer);

  const tick = async () => {
    if (!state.hold) return;

    const remaining = Math.max(0, Math.floor((state.hold.expiresAt - Date.now()) / 1000));

    const minutes = String(Math.floor(remaining / 60)).padStart(2, "0");
    const seconds = String(remaining % 60).padStart(2, "0");
    $("hold-countdown").textContent = `${minutes}:${seconds}`;

    if (remaining === 0) {
      showNote("Your hold ran out and the seat is free again.");
      releaseLocalHold();
      await loadSeats();
    }
  };

  tick();
  state.countdownTimer = setInterval(tick, 1000);
}

// --- wiring -------------------------------------------------------------

$("auth-form").addEventListener("submit", signIn);
$("sign-up").addEventListener("click", signUp);
$("sign-out").addEventListener("click", signOut);
$("back").addEventListener("click", () => {
  clearInterval(state.refreshTimer);
  loadEvents();
});
$("confirm").addEventListener("click", confirmHold);
$("release").addEventListener("click", releaseHold);

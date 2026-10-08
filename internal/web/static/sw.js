// The service worker. A push arrives here, not in the page: the whole point is
// that it reaches somebody whose tab is closed, and only a worker runs then.

self.addEventListener("push", (message) => {
  // A push with no payload is still worth showing: something happened, even if
  // this browser could not read what.
  let notice = { title: "Tickets", body: "Something about your tickets has changed.", link: "/" };

  try {
    notice = { ...notice, ...message.data.json() };
  } catch {
    // Leave the fallback.
  }

  message.waitUntil(
    self.registration.showNotification(notice.title, {
      body: notice.body,
      data: { link: notice.link },
      // Collapses repeats: three cancellations in a row should not stack three
      // identical banners.
      tag: "tickets",
    }),
  );
});

self.addEventListener("notificationclick", (message) => {
  message.notification.close();

  const link = message.notification.data?.link ?? "/";

  message.waitUntil(
    // A tab that is already open is focused rather than a second one opened.
    self.clients.matchAll({ type: "window", includeUncontrolled: true }).then((windows) => {
      for (const window of windows) {
        if (window.url.endsWith(link) && "focus" in window) return window.focus();
      }

      return self.clients.openWindow(link);
    }),
  );
});

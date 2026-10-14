/* Push only: this worker deliberately has no fetch handler or offline cache. */
self.addEventListener("push", (event) => {
  let payload;
  try {
    payload = event.data.json();
  } catch {
    return;
  }
  if (!payload || typeof payload.title !== "string" || typeof payload.body !== "string") return;
  event.waitUntil(
    self.registration.showNotification(payload.title.slice(0, 100), {
      body: payload.body.slice(0, 300),
      data: { url: "/app/notifications" },
    }),
  );
});
self.addEventListener("notificationclick", (event) => {
  event.notification.close();
  event.waitUntil(
    (async () => {
      const windows = await self.clients.matchAll({ type: "window", includeUncontrolled: true });
      for (const window of windows) {
        if (new URL(window.url).origin === self.location.origin) {
          await window.navigate("/app/notifications");
          return window.focus();
        }
      }
      return self.clients.openWindow("/app/notifications");
    })(),
  );
});

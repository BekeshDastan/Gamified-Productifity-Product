import { authClient, authParams, withAuthLock } from "@/lib/auth/session";
const deviceKey = (owner: string) => `ascend.push.${owner}`;
export async function connectPush(owner: string, publicKey: string) {
  if (!("Notification" in window) || !("serviceWorker" in navigator) || !("PushManager" in window))
    return "Браузер не поддерживает push. Центр уведомлений доступен.";
  // This is the first await: permission remains directly inside the user's click.
  const permission = await Notification.requestPermission();
  if (permission !== "granted")
    return "Разрешение не выдано. Центр уведомлений продолжает работать.";
  if (!publicKey) return "Push пока не настроен на сервере.";
  const registration = await navigator.serviceWorker.register("/push-sw.js", { scope: "/" });
  await navigator.serviceWorker.ready;
  const bytes = Uint8Array.from(atob(publicKey.replace(/-/g, "+").replace(/_/g, "/")), (c) =>
    c.charCodeAt(0),
  );
  const sub =
    (await registration.pushManager.getSubscription()) ??
    (await registration.pushManager.subscribe({
      userVisibleOnly: true,
      applicationServerKey: bytes,
    }));
  const json = sub.toJSON();
  if (!json.keys?.auth || !json.keys.p256dh) throw Error("Браузер вернул неполную подписку.");
  const { auth, p256dh } = json.keys;
  await withAuthLock(async () => {
    const client = await authClient();
    const current = await client.GET("/api/v1/notifications/push");
    const saved = localStorage.getItem(deviceKey(owner));
    const existing = current.data?.data.items.find((d) => d.id === saved && d.revoked_at === null);
    const r = await client.POST("/api/v1/notifications/push", {
      params: {
        ...authParams(),
        header: {
          ...authParams().header,
          "If-Match": `"${existing?.version ?? 0}"`,
          "Idempotency-Key": crypto.randomUUID(),
        },
      },
      body: {
        endpoint: sub.endpoint,
        p256dh,
        auth,
        confirm: true,
      },
    });
    if (r.data) localStorage.setItem(deviceKey(owner), r.data.data.id);
  });
  return "Устройство подключено. Выберите типы push в настройках каналов.";
}
// Called inside the existing auth lock; never acquire that lock again here.
export async function revokeCurrentPush(owner: string) {
  const id = localStorage.getItem(deviceKey(owner));
  if (!id) return;
  const client = await authClient();
  const r = await client.GET("/api/v1/notifications/push");
  const d = r.data?.data.items.find((d) => d.id === id);
  if (d && d.revoked_at === null)
    await client.POST("/api/v1/notifications/push/{id}/revoke", {
      params: {
        ...authParams(),
        path: { id },
        header: {
          ...authParams().header,
          "If-Match": `"${d.version}"`,
          "Idempotency-Key": crypto.randomUUID(),
        },
      },
      body: { confirm: true },
    });
  localStorage.removeItem(deviceKey(owner));
  const registration = await navigator.serviceWorker?.getRegistration("/");
  const sub = await registration?.pushManager.getSubscription();
  await sub?.unsubscribe();
}

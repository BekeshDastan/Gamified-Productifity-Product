"use client";
// The root provider/styles may be unavailable here. Keep this boundary self-contained.
export default function GlobalError({
  retry,
}: {
  error: Error & { digest?: string };
  retry: () => void;
}) {
  return (
    <html lang="ru">
      <body>
        <main style={{ padding: "32px", fontFamily: "system-ui, sans-serif" }}>
          <h1>Не удалось открыть ASCEND</h1>
          <p>Попробуйте загрузить страницу ещё раз.</p>
          <button type="button" onClick={retry} style={{ padding: "12px 20px" }}>
            Повторить
          </button>
        </main>
      </body>
    </html>
  );
}

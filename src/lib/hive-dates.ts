import type { Lang } from "./hive-i18n";

export function formatDate(day: string, lang: Lang) {
  const d = new Date(`${day}T00:00:00`);
  return d.toLocaleDateString(lang === "ar" ? "ar-AE" : "en-GB", {
    weekday: "long",
    day: "numeric",
    month: "long",
    year: "numeric",
  });
}

export function formatTime(time: string, lang: Lang) {
  const [h, m] = time.split(":").map(Number);
  const d = new Date(2000, 0, 1, h, m);
  return d.toLocaleTimeString(lang === "ar" ? "ar-AE" : "en-GB", { hour: "numeric", minute: "2-digit" });
}

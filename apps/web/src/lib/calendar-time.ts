import { DateTime } from "luxon";
export function toWall(value: string, zone: string) {
  return DateTime.fromISO(value, { zone }).toFormat("yyyy-MM-dd'T'HH:mm");
}
export function fromWall(value: string, zone: string) {
  const date = DateTime.fromISO(value, { zone });
  if (!date.isValid || date.toFormat("yyyy-MM-dd'T'HH:mm") !== value)
    throw new Error("Это местное время не существует. Выберите другой час.");
  const earlier = date.getPossibleOffsets().sort((a, b) => a.toMillis() - b.toMillis())[0] ?? date;
  return earlier.toUTC().toISO({ suppressMilliseconds: true })!;
}

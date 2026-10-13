// Private drafts belong to this browser tab and are erased on sign-out or an
// account change. Only validated domain form data is stored; never credentials.
const prefix = "ascend-private-draft:";
export function clearPrivateDrafts() {
  if (typeof window === "undefined") return;
  try {
    for (const key of Object.keys(sessionStorage))
      if (key.startsWith(prefix)) sessionStorage.removeItem(key);
  } catch {
    /* Storage can be disabled by the browser. */
  }
}
export function draftKey(account: string, name: string) {
  return prefix + account + ":" + name;
}

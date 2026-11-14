export function singleFlightRefresh(refresh) {
  let pending = null;
  return current => {
    if (pending) return pending;
    pending = refresh(current).finally(() => { pending = null; });
    return pending;
  };
}

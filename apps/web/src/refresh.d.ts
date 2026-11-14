export function singleFlightRefresh<T>(refresh: (current: T) => Promise<T>): (current: T) => Promise<T>;

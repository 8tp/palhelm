type UnauthorizedHandler = () => void;

const unauthorizedHandlers = new Set<UnauthorizedHandler>();

/** Subscribe to session expiry detected by any API request. */
export function onUnauthorized(handler: UnauthorizedHandler): () => void {
  unauthorizedHandlers.add(handler);
  return () => {
    unauthorizedHandlers.delete(handler);
  };
}

/** Notify session listeners only for an authentication failure. */
export function notifyUnauthorized(status: number): void {
  if (status === 401) unauthorizedHandlers.forEach((handler) => handler());
}

/** TanStack retry policy: transient failures retry once; auth and throttling failures do not. */
export function shouldRetryRequest(failureCount: number, error: unknown): boolean {
  const status = typeof error === "object" && error !== null && "status" in error ? error.status : undefined;
  if (status === 401 || status === 403 || status === 429) return false;
  return failureCount < 1;
}

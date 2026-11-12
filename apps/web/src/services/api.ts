/** Never hard-code any API host, credentials, or database URL in the browser. */
export async function checkApiHealth(signal?: AbortSignal): Promise<boolean> {
  try {
    const response = await fetch('/api/v1/healthz', { method: 'GET', cache: 'no-store', credentials: 'same-origin', signal })
    if (!response.ok) return false
    const data: unknown = await response.json()
    return typeof data === 'object' && data !== null && 'status' in data && data.status === 'ok'
  } catch {
    return false
  }
}

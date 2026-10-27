// The back office's only way to the API: staff token on every call, an idempotency key on every write, problems
// surfaced as the server wrote them, and a lost answer to a write never reported as a failure.

export class ProblemError extends Error {
  constructor(
    readonly status: number,
    readonly slug: string,
    readonly title: string,
    readonly detail?: string,
  ) {
    super(title)
  }
}

/** A write whose answer was lost: it may or may not have happened. Retrying with the same key is safe. */
export class UncertainError extends Error {
  constructor(readonly retry: () => Promise<unknown>) {
    super('The server did not answer; the action may or may not have been done.')
  }
}

/** A read that never reached the server (offline, DNS, or it took longer than the timeout). */
export class NetworkError extends Error {
  constructor(message = 'The server could not be reached') {
    super(message)
  }
}

export function newIdempotencyKey(): string {
  const b = new Uint8Array(16)
  crypto.getRandomValues(b)
  return 'bo-' + Array.from(b, (x) => x.toString(16).padStart(2, '0')).join('')
}

type Fetch = typeof globalThis.fetch

/** How long a read or a write may take before it counts as lost. Writes get longer: the server may be working. */
export const READ_TIMEOUT_MS = 20_000
export const WRITE_TIMEOUT_MS = 30_000

export interface ReadOptions {
  /** aborting it cancels the request (a page that unmounted, a filter that changed) */
  signal?: AbortSignal | undefined
}

export class ApiClient {
  constructor(
    private readonly baseUrl: string,
    private readonly token: () => Promise<string | null>,
    private readonly fetchFn: Fetch = (...a) => globalThis.fetch(...a),
    /** called when the server no longer accepts the session (it ended, or the member was disabled) */
    private readonly onUnauthenticated?: () => void,
  ) {}

  async get<T>(path: string, query?: Record<string, string>, opts: ReadOptions = {}): Promise<T> {
    const url = new URL(path, this.baseUrl)
    for (const [k, v] of Object.entries(query ?? {})) url.searchParams.set(k, v)
    const res = await this.read(url.toString(), { headers: await this.headers() }, opts.signal)
    return this.decode<T>(res)
  }

  /** The raw body of a download (e.g. a KRA batch file). */
  async download(path: string, opts: ReadOptions = {}): Promise<{ blob: Blob; sha256: string | null }> {
    const res = await this.read(new URL(path, this.baseUrl).toString(), { headers: await this.headers() }, opts.signal)
    if (!res.ok) await this.decode(res)
    return { blob: await res.blob(), sha256: res.headers.get('X-File-SHA256') }
  }

  post<T = unknown>(path: string, body: unknown = {}, key = newIdempotencyKey()): Promise<T> {
    return this.write<T>('POST', path, body, key)
  }

  put<T = unknown>(path: string, body: unknown = {}, key = newIdempotencyKey()): Promise<T> {
    return this.write<T>('PUT', path, body, key)
  }

  async delete(path: string, key = newIdempotencyKey()): Promise<void> {
    await this.write('DELETE', path, undefined, key)
  }

  /**
   * Every write, one way: the same idempotency key on a retry, so a request that did arrive is replayed and never
   * repeated. A network failure is retried once; a second failure, a timeout, a 5xx or a request still in progress
   * is reported as uncertain, with a retry that keeps the key.
   */
  private async write<T>(method: 'POST' | 'PUT' | 'DELETE', path: string, body: unknown, key: string): Promise<T> {
    const retry = () => this.write<T>(method, path, body, key)
    const send = async (): Promise<Response> =>
      this.withTimeout(WRITE_TIMEOUT_MS, async (signal) =>
        this.fetchFn(new URL(path, this.baseUrl).toString(), {
          method,
          headers: {
            ...(await this.headers()),
            ...(body === undefined ? {} : { 'Content-Type': 'application/json' }),
            'Idempotency-Key': key,
          },
          ...(body === undefined ? {} : { body: JSON.stringify(body) }),
          signal,
        }),
      )
    let res: Response
    try {
      res = await send()
    } catch {
      try {
        res = await send()
      } catch {
        throw new UncertainError(retry)
      }
    }
    if (res.status >= 500 || (res.status === 409 && (await isInProgress(res.clone())))) {
      throw new UncertainError(retry)
    }
    return this.decode<T>(res)
  }

  private async read(url: string, init: RequestInit, signal?: AbortSignal): Promise<Response> {
    try {
      return await this.withTimeout(READ_TIMEOUT_MS, (s) => this.fetchFn(url, { ...init, signal: s }), signal)
    } catch (e) {
      if (signal?.aborted) throw e // the caller cancelled it: not an error to show
      throw new NetworkError()
    }
  }

  /** Runs fn with a signal that aborts after ms, or when the caller's own signal aborts. */
  private async withTimeout<R>(ms: number, fn: (signal: AbortSignal) => Promise<R>, outer?: AbortSignal): Promise<R> {
    const ac = new AbortController()
    const timer = setTimeout(() => { ac.abort() }, ms)
    const relay = () => { ac.abort() }
    if (outer?.aborted) ac.abort()
    outer?.addEventListener('abort', relay)
    try {
      return await fn(ac.signal)
    } finally {
      clearTimeout(timer)
      outer?.removeEventListener('abort', relay)
    }
  }

  private async headers(): Promise<Record<string, string>> {
    const t = await this.token()
    return { Accept: 'application/json, application/problem+json', ...(t ? { Authorization: `Bearer ${t}` } : {}) }
  }

  private async decode<T>(res: Response): Promise<T> {
    if (res.status === 204) return undefined as T
    const text = await res.text()
    let body: unknown
    try {
      body = text ? JSON.parse(text) : undefined
    } catch {
      body = undefined // a proxy's HTML page, or a truncated answer: still a problem with a status
    }
    if (res.ok) return body as T
    if (res.status === 401) this.onUnauthenticated?.()
    const p = (body ?? {}) as { type?: string; title?: string; detail?: string }
    const slug = p.type?.split('/').pop() ?? `http-${res.status}`
    throw new ProblemError(res.status, slug, p.title ?? `Request failed (${res.status})`, p.detail)
  }
}

async function isInProgress(res: Response): Promise<boolean> {
  try {
    const p = (await res.json()) as { type?: string }
    return p.type?.endsWith('/request-in-progress') ?? false
  } catch {
    return false
  }
}

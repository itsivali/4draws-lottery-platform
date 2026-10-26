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

export function newIdempotencyKey(): string {
  const b = new Uint8Array(16)
  crypto.getRandomValues(b)
  return 'bo-' + Array.from(b, (x) => x.toString(16).padStart(2, '0')).join('')
}

type Fetch = typeof globalThis.fetch

export class ApiClient {
  constructor(
    private readonly baseUrl: string,
    private readonly token: () => Promise<string | null>,
    private readonly fetchFn: Fetch = (...a) => globalThis.fetch(...a),
  ) {}

  async get<T>(path: string, query?: Record<string, string>): Promise<T> {
    const url = new URL(path, this.baseUrl)
    for (const [k, v] of Object.entries(query ?? {})) url.searchParams.set(k, v)
    const res = await this.fetchFn(url.toString(), { headers: await this.headers() })
    return this.decode<T>(res)
  }

  /** The raw body of a download (e.g. a KRA batch file). */
  async download(path: string): Promise<{ blob: Blob; sha256: string | null }> {
    const res = await this.fetchFn(new URL(path, this.baseUrl).toString(), { headers: await this.headers() })
    if (!res.ok) await this.decode(res)
    return { blob: await res.blob(), sha256: res.headers.get('X-File-SHA256') }
  }

  async post<T = unknown>(path: string, body: unknown = {}, key = newIdempotencyKey()): Promise<T> {
    const send = async (): Promise<Response> =>
      this.fetchFn(new URL(path, this.baseUrl).toString(), {
        method: 'POST',
        headers: { ...(await this.headers()), 'Content-Type': 'application/json', 'Idempotency-Key': key },
        body: JSON.stringify(body),
      })
    let res: Response
    try {
      res = await send()
    } catch {
      try {
        res = await send() // the same key: the API replays a request that did arrive, never repeats it
      } catch {
        throw new UncertainError(() => this.post(path, body, key))
      }
    }
    if (res.status >= 500 || res.status === 409 && (await isInProgress(res.clone()))) {
      throw new UncertainError(() => this.post(path, body, key))
    }
    return this.decode<T>(res)
  }

  private async headers(): Promise<Record<string, string>> {
    const t = await this.token()
    return { Accept: 'application/json, application/problem+json', ...(t ? { Authorization: `Bearer ${t}` } : {}) }
  }

  private async decode<T>(res: Response): Promise<T> {
    if (res.status === 204) return undefined as T
    const text = await res.text()
    const body: unknown = text ? JSON.parse(text) : undefined
    if (res.ok) return body as T
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

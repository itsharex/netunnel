export interface ApiClientOptions {
  baseUrl: string
  accessToken?: string
}

async function parseJson(response: Response) {
  return response.json().catch(() => ({}))
}

export async function apiRequest<T>(path: string, options: ApiClientOptions, init?: RequestInit): Promise<T> {
  const normalizedBaseUrl = options.baseUrl.trim().replace(/\/+$/, '')
  const { headers: initHeaders, ...restInit } = init ?? {}
  const response = await fetch(`${normalizedBaseUrl}${path}`, {
    headers: {
      'Content-Type': 'application/json',
      ...(options.accessToken ? { Authorization: `Bearer ${options.accessToken}` } : {}),
      ...(initHeaders ?? {}),
    },
    ...restInit,
  })

  const payload = await parseJson(response)
  if (!response.ok) {
    throw new Error(payload.error ?? `HTTP ${response.status}`)
  }

  return payload as T
}

export function createApiClient(options: ApiClientOptions) {
  return {
    request<T>(path: string, init?: RequestInit) {
      return apiRequest<T>(path, options, init)
    },
  }
}

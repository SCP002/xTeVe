import type { Command, Commands, ServerResponse, Session } from './types'

// Every command opens a WebSocket to /data/, sends one request and gets one answer with the whole state of xTeVe.
// The answer carries a new login token and the old one is no longer valid: commands are sent one after the other.

const tokenCookie = 'Token'

/** Thrown when xTeVe does not answer, or answers that the login is no longer valid. */
export class ConnectionError extends Error {}
export class LoginRequired extends Error {}

function token(): string {
  for (const part of document.cookie.split(';')) {
    const [name, ...value] = part.trim().split('=')
    if (name === tokenCookie) return decodeURIComponent(value.join('='))
  }
  return '-'
}

function setToken(value: string) {
  // The path of the cookie xTeVe sets at the login
  document.cookie = `${tokenCookie}=${encodeURIComponent(value)}; path=/web/; max-age=3600; SameSite=Strict`
}

/** Forgets the login token. */
export function logout() {
  document.cookie = `${tokenCookie}=; path=/web/; max-age=0; SameSite=Strict`
}

let queue: Promise<unknown> = Promise.resolve()

/** Sends a command to xTeVe and returns its answer; commands wait for the ones sent before. */
export function send<C extends Command>(cmd: C, data: Commands[C]): Promise<ServerResponse> {
  const result = queue.then(() => request(cmd, data))
  queue = result.catch(() => undefined)
  return result
}

function request(cmd: string, data: object): Promise<ServerResponse> {
  return new Promise((resolve, reject) => {
    const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:'
    const ws = new WebSocket(`${protocol}//${location.host}/data/?Token=${encodeURIComponent(token())}`)
    let answered = false

    ws.onopen = () => ws.send(JSON.stringify({ ...data, cmd }))
    ws.onmessage = (event) => {
      answered = true
      ws.close()
      const response = JSON.parse(event.data) as ServerResponse
      if (response.token) setToken(response.token)
      // An invalid token is answered with an error and reload; other errors do not reload
      if (!response.status && response.reload) {
        reject(new LoginRequired(response.err ?? 'Login required'))
        return
      }
      resolve(response)
    }
    ws.onerror = () => {
      if (!answered) reject(new ConnectionError('No connection to xTeVe'))
    }
    ws.onclose = () => {
      if (!answered) reject(new ConnectionError('The connection to xTeVe was closed'))
    }
  })
}

async function api<T>(path: string, body?: object): Promise<T> {
  const response = await fetch(`/web/api/${path}`, {
    method: body ? 'POST' : 'GET',
    headers: body ? { 'Content-Type': 'application/json' } : undefined,
    body: body ? JSON.stringify(body) : undefined,
  })
  const content = await response.json()
  if (!response.ok) throw new Error(content.error ?? response.statusText)
  return content as T
}

export const session = () => api<Session>('session')
export const login = (username: string, password: string) => api<{ token: string }>('login', { username, password })
export const createFirstUser = (username: string, password: string, confirm: string) =>
  api<{ token: string }>('first-user', { username, password, confirm })

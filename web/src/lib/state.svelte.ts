import { toast } from 'svelte-sonner'
import { ConnectionError, LoginRequired, logout, send, session } from './api/server'
import type { Command, Commands, ServerResponse } from './api/types'

type Screen = 'loading' | 'login' | 'first-user' | 'wizard' | 'app' | 'offline'

/** The state of the web UI: what is shown and the last answer of xTeVe. */
export const app = $state({
  screen: 'loading' as Screen,
  /** The last full answer of xTeVe, null before the first one. */
  server: null as ServerResponse | null,
  /** A command is running. */
  busy: false,
  /** Why xTeVe can not be reached. */
  offline: '',
})

/** The state of xTeVe; only valid while app.screen is "app". */
export function server(): ServerResponse {
  if (!app.server) throw new Error('no answer of xTeVe yet')
  return app.server
}

/** Decides what to show: login, first user or xTeVe. */
export async function start() {
  app.screen = 'loading'
  try {
    const s = await session()
    if (s.authentication && !s.wizard && !s.loggedIn) {
      app.screen = s.firstUser ? 'first-user' : 'login'
      return
    }
    if (await command('getServerConfig', {})) app.screen = app.server?.configurationWizard ? 'wizard' : 'app'
  } catch (error) {
    app.offline = error instanceof Error ? error.message : String(error)
    app.screen = 'offline'
  }
}

export function signOut() {
  logout()
  app.server = null
  void start()
}

/**
 * Sends a command and keeps its answer. Errors and alerts of xTeVe are shown as toasts. Returns whether xTeVe
 * accepted the command.
 */
export async function command<C extends Command>(cmd: C, data: Commands[C]): Promise<boolean> {
  const quiet = cmd === 'updateLog'
  if (!quiet) app.busy = true
  try {
    const response = await send(cmd, data)

    // updateLog answers only with the log and the status
    if (quiet && app.server) {
      app.server.log = response.log
      app.server.clientInfo = response.clientInfo
      app.server.notification = response.notification
    } else {
      app.server = response
    }

    if (!response.status) {
      toast.error(response.err ?? 'xTeVe did not accept the request')
      return false
    }
    if (response.alert) toast.info(response.alert)
    if (response.openLink) {
      // A download (backup) or the new address after a change of the host or TLS
      location.href = response.openLink
    } else if (response.reload) {
      void start()
    }
    return true
  } catch (error) {
    if (error instanceof LoginRequired) {
      app.server = null
      void start()
    } else if (error instanceof ConnectionError) {
      toast.error(error.message)
    } else {
      throw error
    }
    return false
  } finally {
    if (!quiet) app.busy = false
  }
}

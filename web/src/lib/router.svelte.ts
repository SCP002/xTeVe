// The page is kept in the hash of the URL (#/playlists), so that a reload stays on it and xTeVe needs no routes.

export const pages = ['overview', 'playlists', 'filters', 'xmltv', 'mapping', 'users', 'settings', 'log'] as const

export type Page = (typeof pages)[number]

function current(): Page {
  const name = location.hash.replace(/^#\/?/, '')
  return (pages as readonly string[]).includes(name) ? (name as Page) : 'overview'
}

export const route = $state({ page: current() })

addEventListener('hashchange', () => {
  route.page = current()
})

export function href(page: Page): string {
  return `#/${page}`
}

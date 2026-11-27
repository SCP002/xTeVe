<script lang="ts">
  import type { Component } from 'svelte'
  import FileText from '@lucide/svelte/icons/file-text'
  import Filter from '@lucide/svelte/icons/filter'
  import LayoutDashboard from '@lucide/svelte/icons/layout-dashboard'
  import ListVideo from '@lucide/svelte/icons/list-video'
  import LoaderCircle from '@lucide/svelte/icons/loader-circle'
  import LogOut from '@lucide/svelte/icons/log-out'
  import Settings from '@lucide/svelte/icons/settings'
  import TvMinimal from '@lucide/svelte/icons/tv-minimal'
  import Users from '@lucide/svelte/icons/users'
  import Workflow from '@lucide/svelte/icons/workflow'
  import { Badge } from '$lib/components/ui/badge'
  import { Separator } from '$lib/components/ui/separator'
  import * as Sidebar from '$lib/components/ui/sidebar'
  import { href, route, type Page } from '$lib/router.svelte'
  import { app, server, signOut } from '$lib/state.svelte'
  import Filters from '../pages/Filters.svelte'
  import Log from '../pages/Log.svelte'
  import Mapping from '../pages/Mapping.svelte'
  import NotYet from '../pages/NotYet.svelte'
  import Overview from '../pages/Overview.svelte'
  import Playlists from '../pages/Playlists.svelte'
  import Xmltv from '../pages/Xmltv.svelte'

  const menu: { page: Page; title: string; icon: Component }[] = [
    { page: 'overview', title: 'Overview', icon: LayoutDashboard },
    { page: 'playlists', title: 'Playlists', icon: ListVideo },
    { page: 'filters', title: 'Filters', icon: Filter },
    { page: 'xmltv', title: 'XMLTV', icon: TvMinimal },
    { page: 'mapping', title: 'Mapping', icon: Workflow },
    { page: 'users', title: 'Users', icon: Users },
    { page: 'settings', title: 'Settings', icon: Settings },
    { page: 'log', title: 'Log', icon: FileText },
  ]

  const pages: Partial<Record<Page, Component>> = {
    overview: Overview,
    playlists: Playlists,
    filters: Filters,
    xmltv: Xmltv,
    mapping: Mapping,
    log: Log,
  }

  const info = $derived(server().clientInfo)
  const current = $derived(menu.find((m) => m.page === route.page) ?? menu[0])
  const Content = $derived(pages[route.page] ?? NotYet)
  // Mapping is only used with the XEPG source
  const visible = $derived(menu.filter((m) => m.page !== 'mapping' || info.epgSource === 'XEPG'))
</script>

<Sidebar.Provider>
  <Sidebar.Root collapsible="icon">
    <Sidebar.Header>
      <div class="flex items-center gap-2 px-2 py-1.5 font-semibold group-data-[collapsible=icon]:hidden">
        xTeVe
        <span class="text-xs font-normal text-muted-foreground">{info.version}</span>
      </div>
    </Sidebar.Header>
    <Sidebar.Content>
      <Sidebar.Group>
        <Sidebar.Menu>
          {#each visible as item (item.page)}
            <Sidebar.MenuItem>
              <Sidebar.MenuButton isActive={route.page === item.page} tooltipContent={item.title}>
                {#snippet child({ props })}
                  <a href={href(item.page)} {...props}>
                    <item.icon />
                    <span>{item.title}</span>
                  </a>
                {/snippet}
              </Sidebar.MenuButton>
              {#if item.page === 'log' && info.errors + info.warnings > 0}
                <Sidebar.MenuBadge class={info.errors > 0 ? 'text-destructive' : ''}>
                  {info.errors + info.warnings}
                </Sidebar.MenuBadge>
              {/if}
            </Sidebar.MenuItem>
          {/each}
        </Sidebar.Menu>
      </Sidebar.Group>
    </Sidebar.Content>
    {#if server().settings['authentication.web']}
      <Sidebar.Footer>
        <Sidebar.Menu>
          <Sidebar.MenuItem>
            <Sidebar.MenuButton onclick={signOut} tooltipContent="Log out">
              <LogOut />
              <span>Log out</span>
            </Sidebar.MenuButton>
          </Sidebar.MenuItem>
        </Sidebar.Menu>
      </Sidebar.Footer>
    {/if}
    <Sidebar.Rail />
  </Sidebar.Root>

  <Sidebar.Inset>
    <header class="flex h-14 shrink-0 items-center gap-2 border-b px-4">
      <Sidebar.Trigger class="-ml-1" />
      <Separator orientation="vertical" class="mr-2 data-vertical:h-4 data-vertical:self-center" />
      <h1 class="font-medium">{current.title}</h1>
      <div class="ml-auto flex items-center gap-2 text-sm">
        {#if app.busy}
          <LoaderCircle class="size-4 animate-spin text-muted-foreground" aria-label="Working" />
        {/if}
        <Badge variant="outline">{info.epgSource}</Badge>
        <Badge variant="outline">Streams {info.streams}</Badge>
      </div>
    </header>
    <div class="flex-1 p-4 md:p-6">
      <Content />
    </div>
  </Sidebar.Inset>
</Sidebar.Provider>

<script lang="ts">
  import ProviderTable from '../components/ProviderTable.svelte'
</script>

<div class="grid gap-4">
  <ProviderTable kind="m3u" title="M3U playlists" description="Local or remote playlists of streams." />
  <ProviderTable
    kind="hdhr"
    title="HDHomeRun tuners"
    description="Tuners in the network, whose channels xTeVe passes on."
  />
</div>

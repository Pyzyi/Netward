<script lang="ts">
  import Icon from './Icon.svelte';
  import { typeIcons } from '../devices';
  import type { DeviceType } from '../types';

  // A device glyph on a square paper tile. The state mark sits on the corner:
  // filled signal green when online, hollow when offline, amber when new.
  let {
    type,
    online = undefined,
    fresh = false,
    size = 40,
  }: { type: DeviceType; online?: boolean; fresh?: boolean; size?: number } = $props();
</script>

<span class="tile" style:width="{size}px" style:height="{size}px" aria-hidden="true">
  <Icon name={typeIcons[type] ?? typeIcons.unknown} size={Math.round(size * 0.56)} />
  {#if online !== undefined}
    <span class="mark" class:on={online} class:fresh></span>
  {/if}
</span>

<style>
  .tile {
    position: relative;
    display: inline-grid;
    place-items: center;
    flex: none;
    border-radius: 9px;
    color: var(--text);
    background: var(--tile);
  }
  .mark {
    position: absolute;
    right: -3px;
    bottom: -3px;
    width: 11px;
    height: 11px;
    border-radius: 50%;
    background: var(--surface);
    border: 2px solid var(--off);
    box-shadow: 0 0 0 2px var(--surface);
  }
  .mark.on {
    background: var(--signal);
    border-color: var(--signal);
  }
  .mark.fresh {
    background: var(--amber);
    border-color: var(--amber);
  }
  :global(.sheet-bg) .mark,
  :global(.detail) .mark {
    background: var(--surface);
    box-shadow: 0 0 0 2px var(--surface);
  }
  :global(.detail) .mark.on {
    background: var(--signal);
  }
</style>

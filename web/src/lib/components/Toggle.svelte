<script lang="ts">
  let {
    checked = $bindable(false),
    label,
    hint = '',
    disabled = false,
    onchange,
  }: { checked?: boolean; label: string; hint?: string; disabled?: boolean; onchange?: (v: boolean) => void } = $props();
  const id = `t-${Math.random().toString(36).slice(2, 9)}`;
</script>

<div class="toggle-row">
  <div class="text">
    <label for={id}>{label}</label>
    {#if hint}<p class="hint">{hint}</p>{/if}
  </div>
  <button
    {id}
    type="button"
    role="switch"
    aria-checked={checked}
    aria-label={label}
    {disabled}
    class="switch"
    onclick={() => {
      checked = !checked;
      onchange?.(checked);
    }}
  >
    <span class="knob"></span>
  </button>
</div>

<style>
  .toggle-row {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 16px;
  }
  .text {
    display: grid;
    gap: 3px;
  }
  label {
    font-weight: 520;
    cursor: pointer;
  }
  .switch {
    flex: none;
    position: relative;
    width: 40px;
    height: 24px;
    margin-top: 1px;
    border: 0;
    border-radius: 999px;
    background: var(--border-strong);
    cursor: pointer;
    transition: background 150ms var(--ease);
  }
  .switch[aria-checked='true'] {
    background: var(--accent);
  }
  .switch:disabled {
    opacity: 0.5;
    cursor: default;
  }
  .knob {
    position: absolute;
    top: 3px;
    left: 3px;
    width: 18px;
    height: 18px;
    border-radius: 50%;
    background: #fff;
    box-shadow: 0 1px 2px rgb(0 0 0 / 0.25);
    transition: transform 150ms var(--ease);
  }
  .switch[aria-checked='true'] .knob {
    transform: translateX(16px);
  }
</style>

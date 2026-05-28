<script setup lang="ts">
import { onMounted } from 'vue'

// Rendered only inside the DefaultTheme `not-found` slot, so onMounted fires
// only on unmatched routes — never on the homepage or any real page.
//
// Removed pages (the v0.4.5 landing-page refresh stripped the site to a single
// page) are served by GitHub Pages as 404.html. GH Pages cannot emit a real
// 301, so we bounce unmatched routes to the homepage client-side. The first
// response is still HTTP 404; this is a convenience redirect for humans.
onMounted(() => {
  if (typeof window !== 'undefined' && window.location.pathname !== '/') {
    window.location.replace('/')
  }
})
</script>

<template>
  <div style="text-align: center; padding: 4rem 1rem; color: var(--vp-c-text-2)">
    Redirecting…
  </div>
</template>

<script setup lang="ts">
import DefaultTheme from 'vitepress/theme'
import NotFoundRedirect from './NotFoundRedirect.vue'

const { Layout } = DefaultTheme
</script>

<template>
  <Layout>
    <template #not-found>
      <NotFoundRedirect />
    </template>
  </Layout>
</template>

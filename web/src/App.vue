<script setup lang="ts">
import '@/style.css'
import Sidebar from '@/components/Sidebar.vue'
</script>

<template>
  <Sidebar />
  <main class="main">
    <RouterView />
  </main>
</template>

<style scoped>
.main {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  height: 100vh;
  overflow: hidden;
}
</style>

<template>
  <div id="app">
    <!-- 如果当前路由需要隐藏布局（例如登录页），直接渲染 router-view -->
    <component :is="hideLayout ? 'router-view' : BasicLayout" />
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { useRoute } from "vue-router";
import BasicLayout from "@/layouts/BasicLayout.vue";

const route = useRoute();
const hideLayout = computed(() => Boolean(route.meta.hideLayout));
</script>

<style>
#app {
}
</style>

<template>
  <div id="basicLayout">
    <a-layout style="min-height: 100vh">
      <!-- 固定在左侧的 Sider -->
      <a-layout-sider
        v-model:collapsed="collapsed"
        collapsible
        :style="{
          position: 'fixed',
          left: 0,
          top: 0,
          bottom: 0,
          height: '100vh',
          overflow: 'auto',
        }"
      >
        <GlobalSider />
      </a-layout-sider>

      <a-layout class="right-layout" :style="{ marginLeft: siderWidth + 'px' }">
        <a-layout-header class="header">
          <GlobalHeader />
        </a-layout-header>

        <a-layout-content class="content">
          <router-view />
        </a-layout-content>

        <a-layout-footer class="footer">
          DevSecOps-platform ©2025 Created by Yuy0ung
        </a-layout-footer>
      </a-layout>
    </a-layout>
  </div>
</template>

<script lang="ts" setup>
import { ref, computed } from "vue";
import GlobalSider from "@/components/GlobalSider.vue";
import GlobalHeader from "@/components/GlobalHeader.vue";

const collapsed = ref<boolean>(false);

const siderWidth = computed(() => (collapsed.value ? 80 : 200));
</script>

<style scoped>
.right-layout {
  transition: margin-left 0.2s ease;
  height: 100vh;
  display: flex;
  flex-direction: column;
}

#basicLayout .header {
  background: #ffffff;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.08);
  position: relative;
  z-index: 1;
}

#basicLayout .content {
  background: linear-gradient(to right, #fefefe, #fff);
  padding: 20px;
  flex: 1;
  overflow-y: auto;
}

#basicLayout .footer {
  background: #ffffff;
  text-align: center;
  font-size: 12px;
}
</style>

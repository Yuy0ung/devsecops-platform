<template>
  <div id="sider">
    <img class="logo" src="../assets/logo.jpg" />
    <a-menu
      :selected-keys="[$route.name as string]"
      theme="dark"
      mode="inline"
      @click="onMenuClick"
    >
      <a-sub-menu key="sub1">
        <template #title>
          <span>
            <BugOutlined />
            <span>DAST</span>
          </span>
        </template>
        <a-menu-item key="dastTask">
          <template #icon>
            <BarsOutlined />
          </template>
          任务列表
        </a-menu-item>
        <a-menu-item key="dastPoc">
          <template #icon>
            <AppstoreAddOutlined />
          </template>
          POC管理
        </a-menu-item>
      </a-sub-menu>
      <a-sub-menu key="sub2">
        <template #title>
          <span>
            <CodeOutlined />
            <span>SAST</span>
          </span>
        </template>
        <a-menu-item key="sastCodeql">
          <template #icon>
            <DeploymentUnitOutlined />
          </template>
          CodeQL
        </a-menu-item>
        <a-menu-item key="sastVuln">
          <template #icon>
            <BarsOutlined />
          </template>
          漏洞列表
        </a-menu-item>
      </a-sub-menu>
      <a-sub-menu key="sub3">
        <template #title>
          <span>
            <BranchesOutlined />
            <span>SCA</span>
          </span>
        </template>
        <a-menu-item key="scaList">
          <template #icon>
            <BarsOutlined />
          </template>
          项目列表
        </a-menu-item>
        <a-menu-item key="scaVuln">
          <template #icon>
            <ApartmentOutlined />
          </template>
          漏洞分析
        </a-menu-item>
      </a-sub-menu>
    </a-menu>
  </div>
</template>

<script lang="ts" setup>
import {
  BugOutlined,
  CodeOutlined,
  BranchesOutlined,
  BarsOutlined,
  AppstoreAddOutlined,
  DeploymentUnitOutlined,
  ApartmentOutlined,
} from "@ant-design/icons-vue";
import { useRouter, useRoute } from "vue-router";

const router = useRouter();
const $route = useRoute();

const onMenuClick = ({ key }: { key: string }) => {
  if (router.hasRoute(key)) {
    router.push({ name: key });
  }
};
</script>
<style scoped>
#sider .logo {
  height: 45px;
  margin: 16px;
  background: rgba(255, 255, 255, 0.3);
}
</style>

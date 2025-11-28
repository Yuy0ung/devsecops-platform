<template>
  <div class="global-header">
    <div class="header-left">
      <a-breadcrumb>
        <a-breadcrumb-item v-for="(item, index) in breadcrumb" :key="index">
          {{ item }}
        </a-breadcrumb-item>
      </a-breadcrumb>
    </div>

    <div class="header-right" v-if="username">
      <span class="username">您好，{{ username }}</span>
      <a-button type="link" size="small" @click="handleLogout">
        退出登录
      </a-button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, onMounted, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { message } from "ant-design-vue";
import request from "@/utils/request";

const route = useRoute();
const router = useRouter();

// 路由路径到“位置数组”的映射
const routeMap: Record<string, string[]> = {
  // DAST
  "/dast": ["DAST"],
  "/dast/task": ["DAST", "任务列表"],
  "/dast/task/targets": ["DAST", "任务列表", "扫描目标"],
  "/dast/poc": ["DAST", "POC 列表"],

  // SAST
  "/sast": ["SAST"],
  "/sast/codeql": ["SAST", "CodeQL"],
  "/sast/vuln": ["SAST", "漏洞列表"],

  // SCA
  "/sca": ["SCA"],
  "/sca/list": ["SCA", "项目列表"],
  "/sca/vuln": ["SCA", "漏洞分析"],
};

// 根据当前路径匹配“最长前缀”
const breadcrumb = computed(() => {
  const path = route.path;
  const matchKey = Object.keys(routeMap)
    .sort((a, b) => b.length - a.length)
    .find((key) => path.startsWith(key));

  return matchKey ? routeMap[matchKey] : [];
});

// ref 保存用户名
const username = ref("");

// 从 localStorage 里同步用户名
const syncUsername = () => {
  username.value = localStorage.getItem("username") || "";
};

// 首次挂载时同步一次
onMounted(() => {
  syncUsername();
});

// 每次路由变化时同步一次（从 /login 跳到 /dast/task 时会触发）
watch(
  () => route.fullPath,
  () => {
    syncUsername();
  }
);

// 退出登录
const handleLogout = async () => {
  try {
    await request.post("/api/logout");
  } catch (e) {
    // ignore
  } finally {
    localStorage.removeItem("token");
    localStorage.removeItem("username");
    syncUsername(); // 立刻清掉 header 上的名字

    message.success("已退出登录");
    router.push("/login");
  }
};
</script>

<style scoped>
.global-header {
  background: #ffffff;
  height: 64px;
  display: flex;
  align-items: center;
  justify-content: space-between; /* 左右两边 */
  padding: 0 24px;
  box-sizing: border-box;
  border-bottom: 1px solid #f0f0f0;
}

.header-left {
  display: flex;
  align-items: center;
}

.header-right {
  display: flex;
  align-items: center;
  gap: 8px;
}

.username {
  font-size: 13px;
  color: #4b5563;
}
</style>

<template>
  <div class="vuln-detail-container">
    <div class="custom-header">
      <div class="header-left">
        <a-button type="link" @click="$router.back()">
          <template #icon><arrow-left-outlined /></template>
        </a-button>
        <span class="header-title">扫描结果 (任务 ID: {{ taskId }})</span>
      </div>
      <div class="header-right">
        <a-button size="small" type="primary" @click="fetchVulns"
          >刷新</a-button
        >
      </div>
    </div>

    <div class="vuln-content">
      <!-- Left side: Vulnerability List -->
      <div
        class="vuln-list-panel"
        :class="{ 'vuln-list-collapsed': selectedVuln }"
      >
        <a-table
          :columns="computedColumns"
          :data-source="vulns"
          :loading="loading"
          row-key="id"
          :custom-row="customRow"
          size="small"
          :pagination="false"
          :scroll="{ y: 'calc(100vh - 250px)' }"
          class="vuln-table"
        >
          <template #bodyCell="{ column, record }">
            <template v-if="column.key === 'severity'">
              <a-tag :color="getSeverityColor(record.severity)">
                {{ record.severity.toUpperCase() }}
              </a-tag>
            </template>
          </template>
        </a-table>
      </div>

      <!-- Right side: Detail View -->
      <div v-if="selectedVuln" class="vuln-detail-wrapper">
        <!-- Code Preview Header -->
        <div class="code-header">
          <span class="file-path">
            <file-text-outlined />
            {{ selectedVuln.file }}
          </span>
          <span class="line-info">Line: {{ selectedVuln.line }}</span>
        </div>

        <!-- Code Preview Area (Middle) -->
        <div class="code-preview-area">
          <div v-if="codeLoading" class="code-loading">
            <a-spin tip="正在加载代码..." />
          </div>
          <div v-else-if="codeContent" class="code-container">
            <div
              v-for="(line, index) in codeContent.split('\n')"
              :key="index"
              class="code-line"
              :class="{ 'highlight-line': index + 1 === selectedVuln.line }"
              :id="'line-' + (index + 1)"
            >
              <span class="line-number">{{ index + 1 }}</span>
              <!-- Use v-html for highlighted code -->
              <span class="code-text" v-html="highlightLine(line)"></span>
            </div>
          </div>
          <div v-else class="code-error">无法加载代码预览。</div>
        </div>

        <!-- AI Analysis Panel -->
        <div class="taint-flow-panel" v-if="selectedVuln.aiAnalysis">
          <div
            class="taint-header"
            style="background-color: #f0f5ff; color: #2f54eb"
          >
            <robot-outlined /> AI Analysis Logic
          </div>
          <div
            class="code-container"
            style="
              padding: 12px;
              background: #fafafa;
              border-bottom: 1px solid #f0f0f0;
              max-height: 300px;
              overflow-y: auto;
            "
          >
            <pre
              style="
                white-space: pre-wrap;
                font-family: monospace;
                font-size: 12px;
              "
              >{{ selectedVuln.aiAnalysis }}</pre
            >
          </div>
        </div>

        <!-- Taint Flow Analysis (Bottom) -->
        <div class="taint-flow-panel" v-if="selectedVuln.codeFlow">
          <div class="taint-header">污点追踪分析</div>
          <div class="taint-list">
            <div
              v-for="(flow, index) in parseCodeFlow(selectedVuln.codeFlow)"
              :key="index"
              class="taint-step"
              @click="handleFlowClick(flow)"
            >
              <span class="step-index">{{ index + 1 }}</span>
              <span class="step-file">{{ flow.file }}:{{ flow.line }}</span>
              <span class="step-msg">{{ flow.message }}</span>
            </div>
          </div>
        </div>
      </div>

      <div v-else class="empty-state">
        <a-empty description="Select a vulnerability to view details" />
      </div>
    </div>
  </div>
</template>

<script lang="ts">
import { defineComponent, ref, onMounted, watch, computed } from "vue";
import { useRoute } from "vue-router";
import request from "@/utils/request";
import { message } from "ant-design-vue";
import {
  FileTextOutlined,
  ArrowLeftOutlined,
  RobotOutlined,
} from "@ant-design/icons-vue";
import hljs from "highlight.js/lib/core";
import java from "highlight.js/lib/languages/java";
import "highlight.js/styles/github.css"; // Or any style you prefer

hljs.registerLanguage("java", java);

interface Vulnerability {
  id: string;
  ruleId: string;
  description: string;
  severity: "high" | "medium" | "low";
  file: string;
  line: number;
  codeFlow?: string;
  aiAnalysis?: string;
}

interface CodeFlowLocation {
  file: string;
  line: number;
  message: string;
}

export default defineComponent({
  name: "VulnDetail",
  components: {
    FileTextOutlined,
    ArrowLeftOutlined,
  },
  setup() {
    const route = useRoute();
    const taskId = route.params.id;
    const loading = ref(false);
    const vulns = ref<Vulnerability[]>([]);
    const selectedVuln = ref<Vulnerability | null>(null);
    const codeContent = ref("");
    const codeLoading = ref(false);
    const currentFile = ref("");

    // Columns change based on selection
    const computedColumns = computed(() => {
      if (selectedVuln.value) {
        return [
          {
            title: "Rule ID",
            dataIndex: "ruleId",
            key: "ruleId",
            ellipsis: true,
          },
          {
            title: "Severity",
            dataIndex: "severity",
            key: "severity",
            width: 80,
          },
        ];
      }
      return [
        {
          title: "Rule ID",
          dataIndex: "ruleId",
          key: "ruleId",
          width: 150,
        },
        {
          title: "Description",
          dataIndex: "description",
          key: "description",
          ellipsis: true,
        },
        {
          title: "Severity",
          dataIndex: "severity",
          key: "severity",
          width: 100,
        },
      ];
    });

    const getSeverityColor = (severity: string) => {
      switch (severity?.toLowerCase()) {
        case "high":
          return "red";
        case "medium":
          return "orange";
        case "low":
          return "blue";
        case "误报":
          return "green";
        default:
          return "default";
      }
    };

    const fetchVulns = async () => {
      loading.value = true;
      try {
        const res = await request.get(`/api/sast/vuln/result/${taskId}`);
        vulns.value = res.data;
      } catch (error) {
        message.error("获取扫描结果失败");
        console.error(error);
      } finally {
        loading.value = false;
      }
    };

    const customRow = (record: Vulnerability) => {
      return {
        onClick: () => {
          selectedVuln.value = record;
        },
        style: {
          cursor: "pointer",
          backgroundColor:
            selectedVuln.value?.id === record.id ? "#e6f7ff" : "",
        },
      };
    };

    const parseCodeFlow = (jsonStr?: string): CodeFlowLocation[] => {
      if (!jsonStr) return [];
      try {
        return JSON.parse(jsonStr);
      } catch (e) {
        return [];
      }
    };

    const scrollToLine = (line: number) => {
      setTimeout(() => {
        const el = document.getElementById("line-" + line);
        if (el) {
          el.scrollIntoView({ behavior: "smooth", block: "center" });
          // Optional: Add a temporary flash effect class
          el.classList.add("flash-highlight");
          setTimeout(() => el.classList.remove("flash-highlight"), 1500);
        }
      }, 200);
    };

    const handleFlowClick = (flow: CodeFlowLocation) => {
      if (flow.file !== currentFile.value) {
        fetchCode(flow.file).then(() => {
          scrollToLine(flow.line);
        });
      } else {
        scrollToLine(flow.line);
      }
    };

    const fetchCode = async (file: string) => {
      codeLoading.value = true;
      // codeContent.value = ""; // Don't clear immediately to avoid flicker if it's fast? Or keep it?
      // Better to clear if switching files to avoid confusion
      if (file !== currentFile.value) {
        codeContent.value = "";
      }

      try {
        const res = await request.get(`/api/sast/file/${taskId}`, {
          params: { path: file },
        });
        codeContent.value = res.data.content;
        currentFile.value = file;

        // Initial scroll if triggered by vulnerability selection
        if (
          selectedVuln.value &&
          selectedVuln.value.file === file &&
          !codeLoading.value
        ) {
          // This logic is a bit tricky.
          // If we just loaded the file for the main vuln, we want to scroll to vuln line.
          // But if we loaded it for a flow step, handleFlowClick handles scrolling.
          // We can leave the auto-scroll here only if it matches selectedVuln
        }
      } catch (error) {
        console.error("Failed to load code:", error);
      } finally {
        codeLoading.value = false;
      }
    };

    const highlightLine = (code: string) => {
      try {
        return hljs.highlight(code, { language: "java" }).value;
      } catch (e) {
        return code;
      }
    };

    watch(selectedVuln, (newVal) => {
      if (newVal) {
        if (newVal.file !== currentFile.value) {
          fetchCode(newVal.file).then(() => {
            scrollToLine(newVal.line);
          });
        } else {
          scrollToLine(newVal.line);
        }
      } else {
        codeContent.value = "";
        currentFile.value = "";
      }
    });

    onMounted(() => {
      fetchVulns();
    });

    return {
      taskId,
      vulns,
      loading,
      computedColumns,
      selectedVuln,
      codeContent,
      codeLoading,
      fetchVulns,
      getSeverityColor,
      customRow,
      parseCodeFlow,
      handleFlowClick,
      highlightLine,
    };
  },
});
</script>

<style scoped>
.vuln-detail-container {
  padding: 0;
  height: calc(100vh - 99px);
  display: flex;
  flex-direction: column;
  background: #f0f2f5;
  overflow: hidden; /* Prevent body scroll */
}

.custom-header {
  height: 48px;
  background: #fff;
  border-bottom: 1px solid #e8e8e8;
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0 16px;
  flex-shrink: 0; /* Prevent shrinking */
}

.header-left {
  display: flex;
  align-items: center;
  gap: 8px;
}

.header-title {
  font-weight: 500;
  font-size: 16px;
}

.vuln-content {
  display: flex;
  flex: 1;
  overflow: hidden; /* Ensure only children scroll */
  height: calc(100vh - 48px);
}

.vuln-list-panel {
  width: 100%;
  max-width: 100%;
  transition: all 0.3s ease;
  border-right: 1px solid #e8e8e8;
  height: 100%;
  overflow: hidden;
  display: flex;
  flex-direction: column;
  background: #fff;
  padding-left: 10px; /* Added indentation */
}

.vuln-list-collapsed {
  width: 300px;
  min-width: 300px;
  max-width: 300px;
}

/* Ensure table header stays fixed */
:deep(.ant-table-header) {
  background: #fafafa;
}

.vuln-detail-wrapper {
  flex: 1;
  display: flex;
  flex-direction: column;
  background: #fff;
  height: 100%;
  overflow: hidden;
  position: relative;
}

.code-header {
  padding: 8px 16px;
  background: #fafafa;
  border-bottom: 1px solid #e8e8e8;
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-weight: 500;
  height: 40px;
  flex-shrink: 0;
}

.file-path {
  display: flex;
  align-items: center;
  gap: 8px;
  color: #1890ff;
}

.code-preview-area {
  flex: 1;
  overflow: auto; /* Allow both X and Y scrolling */
  padding: 0;
  position: relative;
  background: #fff;
  /* Adjust height calculation to account for header (40px) and bottom panel (200px) */
  /* But flex:1 handles it automatically if container is flex-column */
}

.code-container {
  font-family: "Consolas", "Monaco", "Courier New", monospace;
  font-size: 13px;
  line-height: 1.5;
  min-width: fit-content; /* Ensure container expands for long lines */
}

.code-line {
  display: flex;
  padding: 0;
  min-width: 100%; /* Ensure background covers full width */
}

.code-line:hover {
  background-color: #f5f5f5;
}

.highlight-line {
  background-color: #fff1f0 !important; /* Light red */
}

.flash-highlight {
  animation: flash 1.5s ease-out;
}

@keyframes flash {
  0% {
    background-color: #ffe58f;
  }
  100% {
    background-color: transparent;
  }
}

.line-number {
  width: 50px;
  min-width: 50px; /* Prevent shrinking */
  text-align: right;
  padding-right: 16px;
  color: #999;
  user-select: none;
  border-right: 1px solid #e8e8e8;
  background: #fafafa;
  margin-right: 12px;
  position: sticky; /* Stick to left */
  left: 0;
  z-index: 10; /* Ensure it stays above scrolled content if needed */
}

.code-text {
  white-space: pre;
  tab-size: 4;
}

.taint-flow-panel {
  max-height: 120px;
  border-top: 1px solid #e8e8e8;
  display: flex;
  flex-direction: column;
  background: #fff;
  flex-shrink: 0; /* Prevent shrinking */
  z-index: 20;
  box-shadow: 0 -2px 8px rgba(0, 0, 0, 0.05); /* Optional shadow for separation */
}

.taint-header {
  padding: 8px 16px;
  background: #fafafa;
  border-bottom: 1px solid #e8e8e8;
  font-weight: bold;
  font-size: 12px;
  flex-shrink: 0;
}

.taint-list {
  flex: 1;
  overflow-y: auto;
  padding: 0;
}

.taint-step {
  padding: 8px 16px;
  border-bottom: 1px solid #f0f0f0;
  cursor: pointer;
  display: flex;
  align-items: center;
  gap: 12px;
  font-size: 12px;
}

.taint-step:hover {
  background-color: #e6f7ff;
}

.step-index {
  background: #1890ff;
  color: #fff;
  width: 20px;
  height: 20px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 10px;
  flex-shrink: 0;
}

.step-file {
  font-weight: 500;
  color: #333;
  white-space: nowrap;
}

.step-msg {
  color: #666;
  flex: 1;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.empty-state {
  flex: 1;
  display: flex;
  justify-content: center;
  align-items: center;
  background: #f0f2f5;
}

.code-loading {
  padding: 40px;
  text-align: center;
}

/* Scrollbar styling */
::-webkit-scrollbar {
  width: 8px;
  height: 8px;
}
::-webkit-scrollbar-track {
  background: #f1f1f1;
}
::-webkit-scrollbar-thumb {
  background: #ccc;
  border-radius: 4px;
}
::-webkit-scrollbar-thumb:hover {
  background: #999;
}
</style>

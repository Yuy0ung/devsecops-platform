<template>
  <div class="sast-vuln-list">
    <a-card title="任务列表" :bordered="false">
      <a-row :gutter="16">
        <!-- Left: Findings List -->
        <a-col :span="8">
          <div style="margin-bottom: 16px" v-if="taskIdFilter">
            <a-alert
              :message="`当前展示任务 #${taskIdFilter} 的扫描结果`"
              type="info"
              show-icon
              closable
              @close="clearFilter"
            />
          </div>
          <!-- Scanning Status Alert -->
          <div
            style="margin-bottom: 16px"
            v-if="
              currentTaskId &&
              ['pending', 'running', 'cancelling'].includes(taskStatus)
            "
          >
            <a-alert
              :message="`任务 #${currentTaskId} 正在进行中: ${taskStatus}`"
              type="info"
              show-icon
            >
              <template #action>
                <a-button size="small" type="danger" @click="cancelTask"
                  >取消任务</a-button
                >
                <a-button
                  size="small"
                  @click="showLogModal"
                  style="margin-left: 8px"
                  >查看日志</a-button
                >
              </template>
            </a-alert>
          </div>
          <div
            style="margin-bottom: 16px"
            v-if="!currentTaskId && taskIdFilter"
          >
            <a-button
              type="default"
              block
              @click="showLogModalWithId(taskIdFilter)"
              >查看任务日志</a-button
            >
          </div>
          <div style="margin-bottom: 16px; text-align: right">
            <a-button
              type="primary"
              @click="showUploadModal"
              style="margin-right: 8px"
              >上传扫描</a-button
            >
            <a-button type="primary" @click="loadFindings" :loading="loading"
              >刷新列表</a-button
            >
          </div>
          <a-list
            item-layout="horizontal"
            :data-source="findings"
            :loading="loading"
            :pagination="pagination"
          >
            <template #renderItem="{ item }">
              <a-list-item
                @click="selectFinding(item)"
                :class="{ 'selected-item': selectedFinding?.id === item.id }"
                style="cursor: pointer"
              >
                <a-list-item-meta
                  :description="`Task #${item.task_id} - ${item.severity}`"
                >
                  <template #title>
                    <a href="javascript:;">{{ item.title }}</a>
                  </template>
                  <template #avatar>
                    <a-tag :color="getSeverityColor(item.severity)">{{
                      item.templateId
                    }}</a-tag>
                  </template>
                </a-list-item-meta>
              </a-list-item>
            </template>
          </a-list>
        </a-col>

        <!-- Right: Detail & Code View -->
        <a-col :span="16">
          <div v-if="selectedFinding">
            <a-descriptions title="漏洞详情" bordered size="small">
              <a-descriptions-item label="规则 ID">{{
                selectedFinding.templateId
              }}</a-descriptions-item>
              <a-descriptions-item label="严重程度">{{
                selectedFinding.severity
              }}</a-descriptions-item>
              <a-descriptions-item label="任务 ID">{{
                selectedFinding.task_id
              }}</a-descriptions-item>
              <a-descriptions-item label="位置">{{
                selectedFinding.target
              }}</a-descriptions-item>
            </a-descriptions>

            <a-divider orientation="left">污点追踪路径</a-divider>
            <div v-if="parsedCodeFlows.length > 0">
              <a-steps
                direction="vertical"
                :current="currentStepIndex"
                size="small"
              >
                <a-step
                  v-for="(step, index) in parsedCodeFlows"
                  :key="index"
                  :title="step.message"
                  :description="`${step.file}:${step.line}`"
                  @click="loadStepCode(step, index)"
                  style="cursor: pointer"
                />
              </a-steps>
            </div>
            <div v-else>
              <a-empty description="无污点追踪信息" />
            </div>

            <a-divider orientation="left">代码预览</a-divider>
            <a-spin :spinning="codeLoading">
              <div class="code-viewer">
                <div class="code-header">{{ currentFile }}</div>
                <pre><code v-html="highlightedCode"></code></pre>
              </div>
            </a-spin>
          </div>
          <a-empty v-else description="请选择一个漏洞查看详情" />
        </a-col>
      </a-row>
    </a-card>

    <a-modal
      v-model:visible="uploadVisible"
      title="上传 CodeQL 数据库扫描"
      @ok="handleUpload"
      :confirmLoading="uploadLoading"
    >
      <a-form layout="vertical">
        <a-form-item label="选择语言">
          <a-select v-model:value="uploadForm.language">
            <a-select-option value="java">Java</a-select-option>
            <a-select-option value="go">Go</a-select-option>
            <a-select-option value="python">Python</a-select-option>
            <a-select-option value="javascript">JavaScript</a-select-option>
            <a-select-option value="cpp">C/C++</a-select-option>
            <a-select-option value="csharp">C#</a-select-option>
          </a-select>
        </a-form-item>
        <a-form-item label="数据库压缩包 (.zip)">
          <a-upload
            :before-upload="beforeUpload"
            :file-list="fileList"
            :max-count="1"
            @remove="handleRemove"
          >
            <a-button> <upload-outlined /> 选择文件 </a-button>
          </a-upload>
        </a-form-item>
      </a-form>
    </a-modal>
  </div>
</template>

<script lang="ts">
import {
  defineComponent,
  onMounted,
  ref,
  computed,
  watch,
  reactive,
} from "vue";
import { useRoute, useRouter } from "vue-router";
import request from "@/utils/request";
import { message } from "ant-design-vue";
import { UploadOutlined } from "@ant-design/icons-vue";

interface Finding {
  id: number;
  task_id: string;
  templateId: string;
  severity: string;
  title: string;
  target: string;
  details: string; // JSON string
}

interface CodeFlowStep {
  file: string;
  line: number;
  message: string;
}

export default defineComponent({
  components: {
    UploadOutlined,
  },
  setup() {
    const route = useRoute();
    const router = useRouter();
    const findings = ref<Finding[]>([]);
    const loading = ref(false);
    const selectedFinding = ref<Finding | null>(null);
    const parsedCodeFlows = ref<CodeFlowStep[]>([]);
    const currentStepIndex = ref(0);
    const currentFile = ref("");
    const codeContent = ref("");
    const codeLoading = ref(false);

    // Upload related
    const uploadVisible = ref(false);
    const uploadLoading = ref(false);
    const uploadForm = reactive({
      language: "java",
    });
    const fileList = ref<any[]>([]);

    // Task polling & cancellation
    const currentTaskId = ref("");
    const taskStatus = ref("");
    let pollTimer: any = null;

    const taskIdFilter = computed(() => route.query.taskId as string);

    const pagination = {
      pageSize: 10,
    };

    const showUploadModal = () => {
      uploadVisible.value = true;
    };

    const beforeUpload = (file: any) => {
      fileList.value = [file];
      return false;
    };

    const handleRemove = () => {
      fileList.value = [];
    };

    const handleUpload = async () => {
      if (fileList.value.length === 0) {
        message.error("请选择文件");
        return;
      }

      uploadLoading.value = true;
      const formData = new FormData();
      formData.append("file", fileList.value[0]);
      formData.append("language", uploadForm.language);

      try {
        const res = await request.post("/api/sast/task/upload", formData, {
          headers: {
            "Content-Type": "multipart/form-data",
          },
        });
        message.success("上传成功，扫描任务已开始");
        uploadVisible.value = false;
        fileList.value = [];

        // Start polling
        currentTaskId.value = res.data.taskId;
        taskStatus.value = "pending";
        startPolling();
      } catch (e) {
        message.error("上传失败");
      } finally {
        uploadLoading.value = false;
      }
    };

    const startPolling = () => {
      if (pollTimer) clearInterval(pollTimer);
      pollTimer = setInterval(async () => {
        if (!currentTaskId.value) return;
        try {
          const res = await request.get(
            `/api/sast/task/${currentTaskId.value}/status`
          );
          taskStatus.value = res.data.status;
          if (["completed", "failed", "cancelled"].includes(taskStatus.value)) {
            clearInterval(pollTimer);
            pollTimer = null;
            message.info(`任务已结束: ${taskStatus.value}`);
            loadFindings(); // Refresh findings
            // If completed, maybe filter by this task ID?
            if (taskStatus.value === "completed") {
              router.push({ query: { taskId: currentTaskId.value } });
            }
          }
        } catch (e) {
          console.error(e);
        }
      }, 2000);
    };

    const cancelTask = async () => {
      if (!currentTaskId.value) return;
      try {
        await request.post(`/api/sast/task/${currentTaskId.value}/cancel`);
        message.info("已发送取消请求");
      } catch (e) {
        message.error("取消失败");
      }
    };

    const loadFindings = async () => {
      loading.value = true;
      try {
        const params: any = {};
        if (taskIdFilter.value) {
          params.taskId = taskIdFilter.value;
        }
        const res = await request.get("/api/sast/findings", { params });
        findings.value = res.data.findings;
      } catch (error) {
        message.error("加载任务列表失败");
      } finally {
        loading.value = false;
      }
    };

    const clearFilter = () => {
      router.push({ path: "/sast/vuln" });
    };

    // Watch for route query changes to reload
    watch(
      () => route.query.taskId,
      () => {
        loadFindings();
        // Clear selection when filter changes
        selectedFinding.value = null;
      }
    );

    const getSeverityColor = (severity: string) => {
      switch (severity.toLowerCase()) {
        case "error":
          return "red";
        case "warning":
          return "orange";
        default:
          return "blue";
      }
    };

    const selectFinding = async (item: Finding) => {
      selectedFinding.value = item;
      parsedCodeFlows.value = [];
      currentStepIndex.value = -1;
      codeContent.value = "";
      currentFile.value = "";
      codeLoading.value = false;

      // If details are missing, fetch them
      if (!item.details) {
        try {
          codeLoading.value = true;
          const res = await request.get(`/api/sast/finding/${item.id}`);
          // Update the item in the list and selectedFinding
          item.details = res.data.details;
          selectedFinding.value = { ...item, details: res.data.details };
        } catch (e) {
          message.error("获取漏洞详情失败");
          return;
        } finally {
          codeLoading.value = false;
        }
      }

      try {
        const details = JSON.parse(selectedFinding.value!.details);
        if (details.codeFlows && details.codeFlows.length > 0) {
          // Parse first thread flow
          const threadFlow = details.codeFlows[0].threadFlows[0];
          parsedCodeFlows.value = threadFlow.locations.map((loc: any) => {
            const physical = loc.location.physicalLocation;
            return {
              file: physical.artifactLocation.uri,
              line: physical.region.startLine,
              message: loc.location.message?.text || "Step",
            };
          });

          // Load first step automatically
          if (parsedCodeFlows.value.length > 0) {
            loadStepCode(parsedCodeFlows.value[0], 0);
          }
        } else if (item.target) {
          // Fallback if no code flow, parse target "file:line"
          const parts = item.target.split(":");
          if (parts.length >= 2) {
            const file = parts[0];
            const line = parseInt(parts[1]);
            loadStepCode({ file, line, message: "Location" }, 0);
          }
        }
      } catch (e) {
        console.error("Failed to parse details", e);
      }
    };

    const loadStepCode = async (step: CodeFlowStep, index: number) => {
      if (!selectedFinding.value) return;
      currentStepIndex.value = index;

      // Check if file is already loaded
      if (currentFile.value === step.file && codeContent.value) {
        // Just rely on computed highlighting
      } else {
        codeLoading.value = true;
        try {
          // Use task_id from the selected finding
          const res = await request.get(
            `/api/sast/task/${selectedFinding.value.task_id}/file`,
            {
              params: { path: step.file },
            }
          );
          codeContent.value = res.data.content;
          currentFile.value = step.file;
        } catch (e) {
          message.error("无法加载代码文件");
          codeContent.value = "File not found or inaccessible.";
        } finally {
          codeLoading.value = false;
        }
      }
    };

    const highlightedCode = computed(() => {
      if (!codeContent.value) return "";
      const lines = codeContent.value.split("\n");
      let currentLine = 0;
      if (parsedCodeFlows.value.length > 0 && currentStepIndex.value >= 0) {
        currentLine = parsedCodeFlows.value[currentStepIndex.value].line;
      } else if (selectedFinding.value && selectedFinding.value.target) {
        const parts = selectedFinding.value.target.split(":");
        if (parts.length >= 2) currentLine = parseInt(parts[1]);
      }

      return lines
        .map((line, idx) => {
          const lineNum = idx + 1;
          const isHighlight = lineNum === currentLine;
          const style = isHighlight
            ? "background-color: #fffb8f; display: block; width: 100%;"
            : "";
          // Escape HTML
          const safeLine = line
            .replace(/&/g, "&amp;")
            .replace(/</g, "&lt;")
            .replace(/>/g, "&gt;");
          return `<span style="color: #999; margin-right: 10px; user-select: none;">${lineNum}</span><span style="${style}">${safeLine}</span>`;
        })
        .join("\n");
    });

    onMounted(() => {
      loadFindings();
    });

    return {
      findings,
      loading,
      selectedFinding,
      selectFinding,
      getSeverityColor,
      parsedCodeFlows,
      currentStepIndex,
      loadStepCode,
      currentFile,
      codeLoading,
      highlightedCode,
      taskIdFilter,
      clearFilter,
      pagination,
      loadFindings,
      // Upload & Task Control
      showUploadModal,
      handleUpload,
      uploadVisible,
      uploadLoading,
      uploadForm,
      fileList,
      beforeUpload,
      handleRemove,
      currentTaskId,
      taskStatus,
      cancelTask,
    };
  },
});
</script>

<style scoped>
.selected-item {
  background-color: #e6f7ff;
}
.code-viewer {
  background: #f5f5f5;
  padding: 10px;
  border: 1px solid #ddd;
  border-radius: 4px;
  max-height: 500px;
  overflow: auto;
}
.code-header {
  font-weight: bold;
  margin-bottom: 5px;
  color: #666;
}
pre {
  margin: 0;
  font-family: monospace;
}
</style>

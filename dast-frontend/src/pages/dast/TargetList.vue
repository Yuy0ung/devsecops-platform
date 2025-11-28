<template>
  <div class="dast-target-page">
    <div class="page-header">
      <div class="page-header-left">
        <h2 class="page-title">DAST 扫描目标 / 结果</h2>
        <div class="page-sub">
          <span>任务 ID：</span>
          <a-tooltip v-if="taskId" :title="taskId">
            <span class="task-id-text">{{ shortTaskId(taskId) }}</span>
          </a-tooltip>
          <a-tag
            v-if="status"
            size="small"
            :color="statusTagColor(status)"
            style="margin-left: 8px"
          >
            {{ renderStatusText(status) }}
          </a-tag>
        </div>
      </div>
      <a-space>
        <a-button @click="handleBack">返回任务列表</a-button>
        <a-button @click="handleRefresh" :loading="loading">刷新</a-button>

        <!-- pending 状态：可以直接在列表中添加 / 删除目标 -->
        <template v-if="status === 'pending'">
          <a-button type="primary" @click="openAddModal">添加目标</a-button>
          <a-button
            danger
            :disabled="selectedTargetKeys.length === 0"
            @click="handleDeleteTargets"
          >
            删除所选
          </a-button>
        </template>

        <!-- stopped / finished 状态：结果页面有“更改目标”按钮 -->
        <template v-else-if="isEditableStatus">
          <a-button type="primary" @click="openEditModal">更改目标</a-button>
        </template>
      </a-space>
    </div>

    <!-- pending：目标列表 -->
    <a-table
      v-if="status === 'pending'"
      :columns="targetColumns"
      :data-source="targets"
      :loading="loading"
      row-key="key"
      :row-selection="rowSelectionTargets"
      :pagination="false"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.dataIndex === 'index'">
          <span>{{ record.index }}</span>
        </template>
        <template v-else>
          {{ record[column.dataIndex] }}
        </template>
      </template>
    </a-table>

    <!-- 非 pending：扫描结果列表 -->
    <a-table
      v-else
      :columns="resultColumns"
      :data-source="results"
      :loading="loading"
      row-key="key"
      :pagination="resultPagination"
      @change="handleResultTableChange"
    >
      <template #bodyCell="{ column, record }">
        <!-- 漏洞等级 -->
        <template v-if="column.dataIndex === 'severity'">
          <a-tag :color="severityColor(record.severity)">
            {{ (record.severity || "-").toUpperCase() }}
          </a-tag>
        </template>
        <!-- 默认展示 -->
        <template v-else>
          {{ record[column.dataIndex] }}
        </template>
      </template>
    </a-table>

    <!-- 添加目标（pending 用） -->
    <a-modal
      v-model:open="addModalVisible"
      title="添加扫描目标"
      :confirm-loading="addLoading"
      @ok="handleAddTargets"
      @cancel="handleCancelAdd"
      ok-text="添加"
      cancel-text="取消"
      destroy-on-close
    >
      <a-form layout="vertical">
        <a-form-item
          label="扫描目标 IP:Port"
          extra="每行一个，例如：1.2.3.4:80"
        >
          <a-textarea
            v-model:value="addForm.targetsText"
            :rows="6"
            placeholder="例如：
1.2.3.4:80
5.6.7.8:443"
          />
        </a-form-item>
      </a-form>
    </a-modal>

    <!-- 更改目标（stopped / finished 用） -->
    <a-modal
      v-model:open="editModalVisible"
      title="更改扫描目标"
      :confirm-loading="editLoading"
      @ok="handleSaveEditTargets"
      @cancel="handleCancelEdit"
      ok-text="保存"
      cancel-text="取消"
      destroy-on-close
      width="720px"
    >
      <a-form layout="vertical">
        <a-form-item label="当前目标（可多选删除）">
          <div
            style="display: flex; justify-content: flex-end; margin-bottom: 8px"
          >
            <a-button
              danger
              size="small"
              :disabled="editSelectedTargetKeys.length === 0"
              @click="handleEditDeleteTargets"
            >
              删除所选
            </a-button>
          </div>

          <a-table
            :columns="targetColumns"
            :data-source="targets"
            row-key="key"
            :row-selection="editRowSelectionTargets"
            :pagination="false"
            size="small"
          >
            <template #bodyCell="{ column, record }">
              <template v-if="column.dataIndex === 'index'">
                <span>{{ record.index }}</span>
              </template>
              <template v-else>
                {{ record[column.dataIndex] }}
              </template>
            </template>
          </a-table>
        </a-form-item>

        <a-form-item
          label="新增扫描目标 IP:Port"
          extra="每行一个，例如：1.2.3.4:80，留空则不添加"
        >
          <a-textarea
            v-model:value="editForm.targetsText"
            :rows="4"
            placeholder="例如：
1.2.3.4:80
5.6.7.8:443"
          />
        </a-form-item>
      </a-form>
    </a-modal>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from "vue";
import { useRoute, useRouter } from "vue-router";
import request from "@/utils/request";
import { message, Modal } from "ant-design-vue";

const route = useRoute();
const router = useRouter();

// 从路由获取 taskId，例如 /dast/task/targets?taskId=xxxx
const taskId = computed(() => (route.query.taskId as string) || "");

// ----------------- 状态 & 数据 -----------------
const status = ref<string>(""); // pending / running / stopped / finished
const loading = ref(false);

// 目标列表
interface TargetItem {
  key: number; // 唯一 key（行索引）
  index: number; // 行号（从 1 开始）
  value: string; // IP:Port
}
const targets = ref<TargetItem[]>([]);
const selectedTargetKeys = ref<number[]>([]); // pending 页面选中要删的目标

// 扫描结果列表
interface ResultItem {
  key: number;
  templateId: string;
  name: string;
  severity: string;
  url: string;
  timestamp: string;
}
const results = ref<ResultItem[]>([]);

// 结果分页状态
const resultPage = ref(1);
const resultPageSize = ref(20);
const resultTotal = ref(0);

// pending 添加目标弹窗
const addModalVisible = ref(false);
const addLoading = ref(false);
const addForm = ref({
  targetsText: "",
});

// stopped/finished 更改目标弹窗
const editModalVisible = ref(false);
const editLoading = ref(false);
const editForm = ref({
  targetsText: "",
});
const editSelectedTargetKeys = ref<number[]>([]);

// ----------------- 计算属性 / 小工具 -----------------
const isEditableStatus = computed(() => {
  const s = (status.value || "").toLowerCase();
  return s === "stopped" || s === "stoped" || s === "finished" || s === "error";
});

const shortTaskId = (id: string) => {
  if (!id) return "";
  if (id.length <= 8) return id;
  return id.slice(0, 8) + "...";
};

const renderStatusText = (s: string) => {
  const v = (s || "").toLowerCase();
  if (v === "pending") return "待开始";
  if (v === "running") return "运行中";
  if (v === "finished") return "已完成";
  if (s === "error") return "错误";
  if (v === "stopped" || v === "stoped") return "已停止";
  return s;
};

const statusTagColor = (s: string) => {
  const v = (s || "").toLowerCase();
  if (v === "pending") return "default";
  if (v === "running") return "processing";
  if (v === "finished") return "success";
  if (v === "error") return "error";
  if (v === "stopped" || v === "stoped") return "warning";
  return "default";
};

const severityColor = (severity: string) => {
  const s = (severity || "").toLowerCase();
  if (s === "info") return "default";
  if (s === "low") return "success";
  if (s === "medium") return "warning";
  if (s === "high" || s === "critical") return "error";
  return "default";
};

// ----------------- 表格列定义 -----------------
const targetColumns = [
  { title: "ID", dataIndex: "index", width: 80 },
  { title: "目标 IP:Port", dataIndex: "value" },
];

const resultColumns = [
  { title: "漏洞名称", dataIndex: "name" },
  { title: "漏洞等级", dataIndex: "severity", width: 120 },
  { title: "漏洞地址", dataIndex: "url" },
  { title: "发现时间", dataIndex: "timestamp", width: 220 },
  { title: "模板 ID", dataIndex: "templateId" },
];

// pending 页面目标行选择
const rowSelectionTargets = computed(() => ({
  selectedRowKeys: selectedTargetKeys.value,
  onChange: (keys: (string | number)[]) => {
    selectedTargetKeys.value = keys as number[];
  },
}));

// 编辑弹窗内目标行选择
const editRowSelectionTargets = computed(() => ({
  selectedRowKeys: editSelectedTargetKeys.value,
  onChange: (keys: (string | number)[]) => {
    editSelectedTargetKeys.value = keys as number[];
  },
}));

// ----------------- API 调用 -----------------

// 获取任务状态（从任务列表中查）
const fetchTaskStatus = async () => {
  const res = await request.get("/api/task/list");
  const list = res.data?.tasks || [];
  const item = list.find((t: any) => t.taskId === taskId.value);
  status.value = item?.status || "pending";
};

// 获取目标列表
const fetchTargets = async () => {
  const res = await request.get("/api/target/list", {
    params: { taskId: taskId.value },
  });
  const arr: string[] = res.data?.targets || [];
  targets.value = arr.map((t, index) => ({
    key: index,
    index: index + 1,
    value: t,
  }));
  selectedTargetKeys.value = [];
  editSelectedTargetKeys.value = [];
};

// 获取扫描结果
const fetchResults = async () => {
  const res = await request.get("/api/target/result", {
    params: {
      taskId: taskId.value,
      page: resultPage.value,
      pageSize: resultPageSize.value,
    },
  });

  const data = res.data || {};
  const arr: any[] = data.results || [];

  // 更新总数 & 后端回传的分页信息（防止后端有校正）
  resultTotal.value = data.total || 0;
  if (data.page) resultPage.value = data.page;
  if (data.pageSize) resultPageSize.value = data.pageSize;

  results.value = arr.map((item, index) => ({
    key: index,
    templateId: item["template-id"] || "",
    name: item.info?.name || item["template-id"] || "-",
    severity: item.info?.severity || "-",
    url: item.url || item.host || "",
    timestamp: item.timestamp || "",
  }));
};

// 结果表格分页、排序、过滤变化时触发
const handleResultTableChange = (pagination: any) => {
  resultPage.value = pagination.current;
  resultPageSize.value = pagination.pageSize;
  fetchResults(); // 重新拉当前页数据
};

// 根据状态加载对应数据
const loadDataByStatus = async () => {
  if (!taskId.value) return;
  await fetchTaskStatus();
  const s = (status.value || "").toLowerCase();
  if (s === "pending") {
    await fetchTargets();
  } else {
    await fetchResults();
  }
};

// ----------------- 页面动作 -----------------
const handleBack = () => {
  router.push("/dast/task"); // 根据你的路由实际调整
};

const resultPagination = computed(() => ({
  current: resultPage.value,
  pageSize: resultPageSize.value,
  total: resultTotal.value,
  showSizeChanger: true,
  pageSizeOptions: ["10", "20", "50", "100"],
  showTotal: (total: number) => `共 ${total} 条`,
}));

const handleRefresh = async () => {
  try {
    loading.value = true;
    // 刷新时回到第一页
    resultPage.value = 1;
    await loadDataByStatus();
  } catch (e) {
    message.error("刷新失败");
  } finally {
    loading.value = false;
  }
};

onMounted(async () => {
  if (!taskId.value) {
    message.error("缺少 taskId 参数");
    return;
  }
  await handleRefresh();
});

// ---------- pending：添加 / 删除目标 ----------
const openAddModal = () => {
  addModalVisible.value = true;
};

const handleCancelAdd = () => {
  addModalVisible.value = false;
  addForm.value.targetsText = "";
};

const handleAddTargets = async () => {
  const targetsToAdd = addForm.value.targetsText
    .split(/\r?\n/)
    .map((t) => t.trim())
    .filter(Boolean);

  if (!targetsToAdd.length) {
    message.warning("请至少输入一个目标");
    return;
  }

  addLoading.value = true;
  try {
    await request.post("/api/target/add", {
      taskId: taskId.value,
      targets: targetsToAdd,
    });
    message.success("添加成功");
    addModalVisible.value = false;
    addForm.value.targetsText = "";
    await fetchTargets();
  } catch (e) {
    message.error("添加目标失败");
  } finally {
    addLoading.value = false;
  }
};

const handleDeleteTargets = () => {
  const selected = targets.value.filter((t) =>
    selectedTargetKeys.value.includes(t.key)
  );
  const deleteList = selected.map((t) => t.value);

  if (!deleteList.length) {
    message.warning("请选择要删除的目标");
    return;
  }

  Modal.confirm({
    title: "确认删除选中的目标吗？",
    content: "删除后不可恢复。",
    okText: "删除",
    okType: "danger",
    cancelText: "取消",
    async onOk() {
      try {
        await request.post("/api/target/delete", {
          taskId: taskId.value,
          targets: deleteList,
        });
        message.success("删除成功");
        await fetchTargets();
      } catch (e) {
        message.error("删除目标失败");
      }
    },
  });
};

const handleEditDeleteTargets = () => {
  const deleteList = targets.value
    .filter((t) => editSelectedTargetKeys.value.includes(t.key))
    .map((t) => t.value);

  if (!deleteList.length) {
    message.warning("请选择要删除的目标");
    return;
  }

  Modal.confirm({
    title: "确认删除选中的目标吗？",
    content: "删除后不可恢复。",
    okText: "删除",
    okType: "danger",
    cancelText: "取消",
    async onOk() {
      try {
        await request.post("/api/target/delete", {
          taskId: taskId.value,
          targets: deleteList,
        });
        message.success("删除成功");
        editSelectedTargetKeys.value = [];
        await fetchTargets();
      } catch (e) {
        message.error("删除目标失败");
      }
    },
  });
};

// ---------- stopped / finished：更改目标 ----------
const openEditModal = async () => {
  try {
    editLoading.value = true;
    await fetchTargets(); // 打开前先拉取最新目标列表
    editModalVisible.value = true;
  } catch (e) {
    message.error("获取目标列表失败");
  } finally {
    editLoading.value = false;
  }
};

const handleCancelEdit = () => {
  editModalVisible.value = false;
  editForm.value.targetsText = "";
  editSelectedTargetKeys.value = [];
};

const handleSaveEditTargets = async () => {
  const deleteTargets = targets.value
    .filter((t) => editSelectedTargetKeys.value.includes(t.key))
    .map((t) => t.value);

  const addTargets = editForm.value.targetsText
    .split(/\r?\n/)
    .map((t) => t.trim())
    .filter(Boolean);

  if (!deleteTargets.length && !addTargets.length) {
    message.warning("没有任何变更");
    return;
  }

  editLoading.value = true;
  try {
    const reqs: Promise<any>[] = [];
    if (addTargets.length) {
      reqs.push(
        request.post("/api/target/add", {
          taskId: taskId.value,
          targets: addTargets,
        })
      );
    }
    if (deleteTargets.length) {
      reqs.push(
        request.post("/api/target/delete", {
          taskId: taskId.value,
          targets: deleteTargets,
        })
      );
    }
    await Promise.all(reqs);
    message.success("目标已更新");
    editModalVisible.value = false;
    editForm.value.targetsText = "";
    editSelectedTargetKeys.value = [];
    await fetchTargets();
  } catch (e) {
    message.error("更新目标失败");
  } finally {
    editLoading.value = false;
  }
};
</script>

<style scoped>
.dast-target-page {
  min-height: 100vh;
  padding: 16px 24px;
  background-color: #ffffff;
  box-sizing: border-box;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 16px;
}

.page-header-left {
  display: flex;
  flex-direction: column;
}

.page-title {
  margin: 0;
  font-size: 20px;
  font-weight: 600;
  color: #111827;
}

.page-sub {
  margin-top: 4px;
  font-size: 12px;
  color: #6b7280;
  display: flex;
  align-items: center;
}

.task-id-text {
  color: #1677ff;
  text-decoration: underline;
  cursor: pointer;
}
</style>

<template>
  <div class="sca-vuln-container">
    <a-page-header
      style="border: 1px solid rgb(235, 237, 240)"
      title="SCA Vulnerability Results"
      @back="() => $router.go(-1)"
    />
    <a-card :bordered="false" style="margin-top: 16px">
      <a-table
        :columns="columns"
        :data-source="findings"
        :loading="loading"
        row-key="id"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'severity'">
            <a-tag :color="getSeverityColor(record.severity)">
              {{ record.severity }}
            </a-tag>
          </template>
          <template v-else-if="column.key === 'vulnId'">
            <a
              :href="record.reference"
              target="_blank"
              v-if="record.reference"
              >{{ record.vulnId }}</a
            >
            <span v-else>{{ record.vulnId }}</span>
          </template>
        </template>
        <template #expandedRowRender="{ record }">
          <p style="margin: 0">
            <strong>Description:</strong> {{ record.description }}
          </p>
          <p style="margin: 0" v-if="record.fixedVersion">
            <strong>Fixed Version:</strong> {{ record.fixedVersion }}
          </p>
          <p style="margin: 0" v-if="record.filePath">
            <strong>File Path:</strong> {{ record.filePath }}
          </p>
        </template>
      </a-table>
    </a-card>
  </div>
</template>

<script lang="ts">
import { defineComponent, ref, onMounted } from "vue";
import { useRoute } from "vue-router";
import request from "@/utils/request";
import { message } from "ant-design-vue";

export default defineComponent({
  name: "ScaVuln",
  setup() {
    const route = useRoute();
    const loading = ref(false);
    const findings = ref([]);

    const columns = [
      {
        title: "Package",
        dataIndex: "packageName",
        key: "packageName",
      },
      {
        title: "Version",
        dataIndex: "version",
        key: "version",
      },
      {
        title: "Language",
        dataIndex: "language",
        key: "language",
      },
      {
        title: "Vuln ID",
        dataIndex: "vulnId",
        key: "vulnId",
      },
      {
        title: "Severity",
        dataIndex: "severity",
        key: "severity",
      },
      {
        title: "Fixed Version",
        dataIndex: "fixedVersion",
        key: "fixedVersion",
      },
    ];

    const getSeverityColor = (severity: string) => {
      if (!severity) return "default";
      const s = severity.toLowerCase();
      if (s === "critical") return "purple";
      if (s === "high") return "red";
      if (s === "medium") return "orange";
      if (s === "low") return "blue";
      return "default";
    };

    const fetchFindings = async () => {
      loading.value = true;
      const id = route.params.id;
      try {
        const res = await request.get(`/api/sca/vuln/result/${id}`);
        findings.value = res.data;
      } catch (error) {
        message.error("Failed to fetch findings");
      } finally {
        loading.value = false;
      }
    };

    onMounted(() => {
      fetchFindings();
    });

    return {
      findings,
      loading,
      columns,
      getSeverityColor,
    };
  },
});
</script>

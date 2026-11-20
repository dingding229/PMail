<script setup>
import { i18n } from '@/i18n/i18n'
import { ref, onMounted, h } from 'vue'
import {
  NCard,
  NDataTable,
  NButton,
  NModal,
  NForm,
  NFormItem,
  NInput,
  NSelect,
  NSpace,
  useMessage,
  useDialog
} from 'naive-ui'
import { ruleApi } from '@/api'
const rows = ref([]),
  show = ref(false),
  form = ref({ name: '', action: 2, params: '', sort: 1, conditions: [] }),
  message = useMessage(),
  dialog = useDialog()
const fields = [
  { label: i18n.global.t('legacy_388b425983'), value: 'From' },
  { label: i18n.global.t('legacy_e848ddd482'), value: 'Subject' },
  { label: i18n.global.t('legacy_529414cfe5'), value: 'To' },
  { label: i18n.global.t('legacy_a0d95bc304'), value: 'Text' },
  { label: i18n.global.t('fieldHtml'), value: 'Html' }
]
const types = [
  { label: i18n.global.t('legacy_f896fb3776'), value: 'contains' },
  { label: i18n.global.t('legacy_bd0f309115'), value: 'eq' },
  { label: i18n.global.t('legacy_fd769f85e3'), value: 'regex' }
]
async function load() {
  const res = await ruleApi.list()
  if (!res.errorNo) rows.value = res.data || []
}
function addCondition() {
  form.value.conditions.push({ field: 'Subject', type: 'contains', rule: '' })
}
function open(row) {
  form.value = row
    ? { ...row, conditions: row.rules || [] }
    : { name: '', action: 2, params: '', sort: 1, conditions: [] }
  show.value = true
}
async function save() {
  const res = await ruleApi.save({
    id: form.value.id,
    name: form.value.name,
    action: Number(form.value.action),
    params: form.value.params,
    sort: Number(form.value.sort || 1),
    rules: form.value.conditions
  })
  if (res.errorNo) message.error(res.errorMsg)
  else {
    show.value = false
    message.success(i18n.global.t('legacy_d86abd2844'))
    load()
  }
}
async function remove(row) {
  dialog.warning({
    title: i18n.global.t('legacy_f9ad34b946'),
    content: i18n.global.t('legacy_574e45d30f'),
    positiveText: i18n.global.t('legacy_3755f56f2f'),
    negativeText: i18n.global.t('legacy_4d0b4688c7'),
    onPositiveClick: async () => {
      const res = await ruleApi.remove(row.id)
      if (res.errorNo) message.error(res.errorMsg)
      else load()
    }
  })
}
onMounted(load)
</script>
<template>
  <n-card :title="$t('legacy_ed904c685a')" class="settings-card"
    ><template #header-extra
      ><n-button type="primary" @click="open()">{{ $t('legacy_945c08e42d') }}</n-button></template
    ><n-data-table
      :data="rows"
      :columns="[
        { title: $t('legacy_1be7ae4fc2'), key: 'name' },
        {
          title: i18n.global.t('legacy_7b5e31a7d7'),
          key: 'rules',
          render: (r) => (r.rules || []).map((x) => `${x.field} ${x.type} ${x.rule}`).join('；')
        },
        {
          title: $t('legacy_d9d9827827'),
          key: 'action',
          render: (r) =>
            r.action === 2
              ? $t('legacy_0d5a8ac36f')
              : r.action === 3
                ? $t('legacy_3755f56f2f')
                : r.action === 1
                  ? $t('legacy_504ecf732d')
                  : $t('legacy_5c16b7e950')
        },
        { title: i18n.global.t('legacy_1d3b8c1b2a'), key: 'params' },
        {
          title: $t('legacy_f3ea6d345e'),
          key: 'id',
          render: (r) =>
            h(NSpace, null, {
              default: () => [
                h(NButton, { size: 'small', onClick: () => open(r) }, () =>
                  i18n.global.t('legacy_08d7bb4a6a')
                ),
                h(NButton, { size: 'small', type: 'error', onClick: () => remove(r) }, () =>
                  $t('legacy_3755f56f2f')
                )
              ]
            })
        }
      ]"
    /><n-modal
      v-model:show="show"
      preset="card"
      :title="$t('legacy_05cc0a7da4')"
      style="width: 680px"
      ><n-form label-placement="top"
        ><n-form-item :label="$t('legacy_1be7ae4fc2')"
          ><n-input v-model:value="form.name" /></n-form-item
        ><n-form-item :label="$t('legacy_d9d9827827')"
          ><n-select
            v-model:value="form.action"
            :options="[
              { label: $t('legacy_0d5a8ac36f'), value: 2 },
              { label: $t('legacy_504ecf732d'), value: 1 },
              { label: $t('legacy_3755f56f2f'), value: 3 },
              { label: i18n.global.t('legacy_5c16b7e950'), value: 4 }
            ]" /></n-form-item
        ><n-form-item :label="$t('legacy_5b69181a60')" v-if="Number(form.action) === 2"
          ><n-input
            v-model:value="form.params"
            :placeholder="$t('legacy_58e5add57d')" /></n-form-item
        ><n-form-item :label="$t('legacy_4ce78e8185')" v-if="Number(form.action) === 4"
          ><n-input v-model:value="form.params"
        /></n-form-item>
        <div class="condition-header">
          <strong>{{ $t('legacy_1726e33902') }}</strong><n-button
            size="small"
            @click="addCondition"
            >{{ $t('legacy_cf97988a59') }}</n-button
          >
        </div>
        <div v-for="(condition, index) in form.conditions" :key="index" class="condition-row">
          <n-select v-model:value="condition.field" :options="fields" /><n-select
            v-model:value="condition.type"
            :options="types"
          /><n-input
            v-model:value="condition.rule"
            :placeholder="$t('legacy_4cd5b3ba3f')"
          /><n-button text type="error" @click="form.conditions.splice(index, 1)">{{
            $t('legacy_2f752c005e')
          }}</n-button>
        </div>
        <n-button type="primary" @click="save">{{ $t('legacy_f5d4126103') }}</n-button></n-form
      ></n-modal
    ></n-card
  >
</template>

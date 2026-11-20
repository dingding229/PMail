<script setup>
import { i18n } from '@/i18n/i18n'
import { ref, onMounted } from 'vue'
import { NCard, NList, NListItem, NButton, NInput, NForm, NFormItem, useMessage } from 'naive-ui'
import { pluginApi } from '@/api'
const plugins = ref([]),
  selected = ref(null),
  form = ref({}),
  message = useMessage()
async function load() {
  const res = await pluginApi.list()
  if (!res.errorNo) plugins.value = res.data || []
}
async function loadSettings(item) {
  selected.value = item
  const res = await pluginApi.settings(item.name, { action: 'get' })
  form.value = res.data || {}
}
async function save() {
  const res = await pluginApi.settings(selected.value.name, { action: 'set', ...form.value })
  if (res.errorNo) message.error(res.errorMsg)
  else message.success(i18n.global.t('legacy_84d5c3ac79'))
}
onMounted(load)
</script>
<template>
  <n-card :title="$t('legacy_70a6a18cbf')" class="settings-card"
    ><n-list
      ><n-list-item v-for="item in plugins" :key="item.name"
        ><div class="plugin-row">
          <div>
            <strong>{{ item.name }}</strong><small>{{
              item.description || $t('legacy_70a6a18cbf')
            }}</small>
          </div>
          <n-button size="small" @click="loadSettings(item)">{{
            $t('legacy_d7d7ce790b')
          }}</n-button>
        </div></n-list-item
      ><n-empty v-if="!plugins.length" :description="$t('legacy_6f3f073942')" /></n-list
    ><n-form label-placement="top" v-if="selected" class="plugin-form"
      ><n-form-item :label="$t('legacy_c23577a30a')"
        ><n-input v-model:value="form.json" type="textarea" :rows="5" /></n-form-item
      ><n-button type="primary" @click="save">{{ $t('legacy_2ecc8be151') }}</n-button></n-form
    ></n-card
  >
</template>

<script setup>
// 阅读设置.
//
// Everything here is instant: the settings only change what is rendered, and
// both languages of every translation and gloss are already in the payload.
// Nothing in here re-fetches.
import { Settings2, RotateCcw } from 'lucide-vue-next'
import { computed } from 'vue'
import { useSettings, SCALE_STEPS } from '../store/settings'

const S = useSettings()

const options = {

  dictLang: [
    { v: 'all', l: '全部' },
    { v: 'dpd', l: 'DPD' },
    { v: 'zh', l: '仅中' },
    { v: 'en', l: '仅英' },
  ],
  refMode: [
    { v: 'off', l: '隐藏' },
    { v: 'all', l: '全部展开' },
  ],
  nameLang: [
    { v: 'zh', l: '中文' },
    { v: 'pi', l: '巴利语' },
    { v: 'both', l: '对照' },
  ],
  bodyFont: [
    { v: 'serif', l: '衬线' },
    { v: 'sans', l: '无衬线' },
  ],
}

const specimen = computed(() => (S.scale >= 1.15 ? 'evaṃ me sutaṃ – ekaṃ samayaṃ' : 'evaṃ me sutaṃ'))
</script>

<template>
  <div class="settings-pop card" role="dialog" aria-label="阅读设置">
    <div class="set-head">
      <span class="sectitle"><Settings2 :size="14" /> 阅读设置</span>
      <button class="ghostlink" style="margin: 0" @click="S.reset()">
        <RotateCcw :size="13" />恢复默认
      </button>
    </div>

    <div class="set-row">
      <span class="set-label">字号</span>
      <div class="seg">
        <button
          v-for="o in SCALE_STEPS"
          :key="o.value"
          :aria-pressed="S.scale === o.value"
          @click="S.set('scale', o.value)"
        >
          {{ o.label }}
        </button>
      </div>
    </div>
    <p class="set-specimen">{{ specimen }}</p>

    <div class="set-row">
      <span class="set-label">参考译文</span>
      <div class="seg">
        <button
          v-for="o in options.refMode"
          :key="o.v"
          :aria-pressed="S.refMode === o.v"
          @click="S.set('refMode', o.v)"
        >
          {{ o.l }}
        </button>
      </div>
    </div>
    <p class="set-note">
      只决定一开始显不显示。句子末尾的 <span class="pi">中</span>／<span class="pi">英</span>
      开<b>这一句</b>，段落左上角那一对开<b>整段</b>；两种设置下都能点，
      也可以把已经显示的关掉。
    </p>

    <div class="set-row">
      <span class="set-label">词典</span>
      <div class="seg">
        <button
          v-for="o in options.dictLang"
          :key="o.v"
          :aria-pressed="S.dictLang === o.v"
          @click="S.set('dictLang', o.v)"
        >
          {{ o.l }}
        </button>
      </div>
    </div>

    <div class="set-row">
      <span class="set-label">目录与书名</span>
      <div class="seg">
        <button
          v-for="o in options.nameLang"
          :key="o.v"
          :aria-pressed="S.nameLang === o.v"
          @click="S.set('nameLang', o.v)"
        >
          {{ o.l }}
        </button>
      </div>
    </div>

    <div class="set-row">
      <span class="set-label">正文字体</span>
      <div class="seg">
        <button
          v-for="o in options.bodyFont"
          :key="o.v"
          :aria-pressed="S.bodyFont === o.v"
          @click="S.set('bodyFont', o.v)"
        >
          {{ o.l }}
        </button>
      </div>
    </div>

    <div class="set-row">
      <span class="set-label">显示异读</span>
      <div class="seg">
        <button :aria-pressed="!S.variants" @click="S.set('variants', false)">关</button>
        <button :aria-pressed="S.variants" @click="S.set('variants', true)">开</button>
      </div>
    </div>
    <p class="set-note">
      「词典」决定查词面板给出哪几部：DPD 最全，也可以只看它；「全部」会同时列出
      巴漢詞典、漢譯パーリ語辭典、PTS、Concise、DPPN 等，点条目里的标签即可换用哪一部。
    </p>

    <p class="set-note">
      异读是第六次结集本行间的校勘记，如 <span class="pi">[suriyaggāho (sī. syā. kaṃ. pī.)]</span>，
      已从正文中抽出，默认不显示。
    </p>

    <p class="set-note">
      参考译文只在该段有已出版的译本时出现；没有就留空，不会用相邻段落凑。
      中文与英文来自 ePitaka 发布的两种译本，覆盖 137 卷。
    </p>
    <p class="set-note">
      「目录与书名」只影响典籍名。章节标题（如 <span class="pi">paribbājakakathā</span>）
      没有通行的中文译名，一律保留巴利语。
    </p>
  </div>
</template>

<style scoped>
.settings-pop {
  width: 348px;
  padding: 14px 16px 16px;
  box-shadow: var(--whisper);
}
.set-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 12px;
}
.sectitle {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  font-weight: 500;
  letter-spacing: 0.5px;
  color: var(--fg-2);
}
.set-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 7px 0;
}
.set-row + .set-row {
  border-top: 1px solid var(--border-soft);
}
.set-label {
  flex: none;
  font-size: 13px;
  color: var(--fg-2);
}
.seg {
  display: flex;
  gap: 1px;
  padding: 2px;
  background: var(--surface-warm);
  border-radius: var(--radius-md);
}
.seg button {
  padding: 4px 8px;
  border-radius: 5px;
  font-size: 11.5px;
  letter-spacing: 0.3px;
  color: var(--fg-2);
  transition: background var(--fast), color var(--fast);
}
.seg button:hover {
  background: var(--surface);
  color: var(--fg);
}
.seg button[aria-pressed='true'] {
  background: var(--accent);
  color: var(--surface);
}
.set-specimen {
  padding: 8px 10px;
  margin: 2px 0 6px;
  border-radius: var(--radius-sm);
  background: var(--surface-warm);
  font-family: var(--sans);
  font-size: calc(var(--fs-pali) * 0.72);
  line-height: 1.6;
  color: var(--fg-2);
}
.set-note {
  margin-top: 8px;
  font-size: 11px;
  line-height: 1.7;
  color: var(--meta);
}
</style>

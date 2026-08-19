const form = document.querySelector('#analyze-form');
const input = document.querySelector('#deck-url');
const decklistInput = document.querySelector('#decklist-input');
const submitButton = document.querySelector('#submit-button');
const message = document.querySelector('#form-message');
const loading = document.querySelector('#loading');
const results = document.querySelector('#results');
const warning = document.querySelector('#warning');
const retryButton = document.querySelector('#retry-button');
const decklistToggle = document.querySelector('#decklist-toggle');
const copyDecklistButton = document.querySelector('#copy-decklist');
let currentDeckText = '';
let currentDeckCards = [];
let pendingSwapAdd = '';
let selectedSwapRemove = '';
const swapModal = document.querySelector('#swap-modal');
const swapSearch = document.querySelector('#swap-search');
const swapSubmit = document.querySelector('#swap-submit');
const swapMessage = document.querySelector('#swap-message');
const swapResult = document.querySelector('#swap-result');

// --- light deck editor state (pure front-end, direction A) ---------------------
// The editor works on a mutable copy of the analyzed deck. Every mutating action
// pushes the previous snapshot onto an undo stack (and clears redo); saved versions
// persist to localStorage keyed by the deck's source id.
const EDITOR_VERSIONS_KEY = 'powerlevel.deck.versions.v1';
let editorCards = [];           // mutable deck_cards array under edit
let editorUndo = [];            // snapshots before each applied action
let editorRedo = [];            // snapshots after each undone action
let editorDirty = false;        // true when a local edit has not been re-analyzed
const editorToolbar = document.querySelector('#editor-toolbar');
const editorAddInput = document.querySelector('#editor-add-card');
const editorAddButton = document.querySelector('#editor-add-submit');
const editorUndoButton = document.querySelector('#editor-undo');
const editorRedoButton = document.querySelector('#editor-redo');
const editorSaveVersionButton = document.querySelector('#editor-save-version');
const editorExportButton = document.querySelector('#editor-export');
const editorVersionsButton = document.querySelector('#editor-versions-toggle');
const editorVersionsPanel = document.querySelector('#editor-versions-panel');
const editorVersionsList = document.querySelector('#editor-versions-list');
const editorDirtyNote = document.querySelector('#editor-dirty-note');
let activeSourceId = '';

// --- guided deck builder state ---------------------------------------------
const buildEntryButton = document.querySelector('#build-entry-button');
const builder = document.querySelector('#builder');
const builderClose = document.querySelector('#builder-close');
const builderCommanderInput = document.querySelector('#builder-commander');
const builderCommanderSuggestions = document.querySelector('#builder-commander-suggestions');
const builderStartButton = document.querySelector('#builder-start');
const builderMessage = document.querySelector('#builder-message');
const builderWorkflow = document.querySelector('#builder-workflow');
const builderCandidates = document.querySelector('#builder-candidates');
const builderSkip = document.querySelector('#builder-skip');
const builderLandsButton = document.querySelector('#builder-lands');
const builderLandsPanel = document.querySelector('#builder-lands-panel');
const builderLandsClose = document.querySelector('#builder-lands-close');
const builderLandsGrid = document.querySelector('#builder-lands-grid');
const builderSidebar = document.querySelector('#builder-sidebar');
const builderComplete = document.querySelector('#builder-complete');
const builderExport = document.querySelector('#builder-export');
const builderAnalyze = document.querySelector('#builder-analyze');

// The draft being built: commander name + already-chosen mainboard card names.
let buildCommander = '';
let buildChosen = [];           // card names (lowercase) already added to the draft
let buildCards = [];            // { name, card? } resolved rows for export/analysis
let buildColors = [];           // commander color identity (for basic-land gating)
let buildCandidates = [];       // currently displayed 3 candidates

const BASIC_LANDS = ['Plains', 'Island', 'Swamp', 'Mountain', 'Forest'];
const BUILD_TARGET = 100;

// 一键出地的地牌分类。ID 与后端 service.LandCategories 对齐；点单类后按主将色组
// 过滤，再渲染可加入草稿的地牌小图。
const LAND_CATEGORIES = [
  { id: 'shock', label: '电震' },
  { id: 'surveil', label: '刺探' },
  { id: 'original_dual', label: '老圈' },
  { id: 'verge', label: '边陲' },
  { id: 'scry', label: '占卜地' },
  { id: 'multiplayer', label: '多人地' },
  { id: 'fetch', label: '找地' },
  { id: 'triome', label: '三色圈' }
];

// construction metric targets (labels mirror the server's construction.Report).
const BUILD_METRICS = [
  { id: 'lands', label: '正向法力', target: 38 },
  { id: 'plan', label: '计划相关', target: 30 },
  { id: 'mass_interaction', label: '群体干扰', target: 6 },
  { id: 'single_interaction', label: '单体干扰', target: 12 },
  { id: 'draw_discard', label: '牌差件', target: 12 },
  { id: 'ramp', label: '加速', target: 10 }
];

function openBuilder() {
  builder.hidden = false;
  builderMessage.textContent = '';
  builderWorkflow.hidden = true;
  builderComplete.hidden = true;
  builderCommanderInput.value = '';
  buildCommander = '';
  buildChosen = [];
  buildCards = [];
  buildColors = [];
  buildCandidates = [];
  builderCommanderInput.focus();
  builder.scrollIntoView({ behavior: 'smooth', block: 'start' });
}

function closeBuilder() {
  builder.hidden = true;
  hideCommanderSuggestions();
}

// --- commander autocomplete -------------------------------------------------
// Typeahead on the builder's commander field. Each keystroke debounces a query to
// the server, which filters Scryfall autocomplete down to cards legal as a
// Commander. Out-of-order responses are discarded via AbortController, and selected
// suggestions are simply written back into the input for the user to start with.
let commanderAutocompleteController = null;
let commanderAutocompleteTimer = null;
let commanderAutocompleteIndex = -1;

function commanderSuggestionItems() {
  return Array.from(builderCommanderSuggestions.querySelectorAll('[data-suggestion]'));
}

function renderCommanderSuggestions(names) {
  commanderAutocompleteIndex = -1;
  if (!names || names.length === 0) {
    hideCommanderSuggestions();
    return;
  }
  builderCommanderSuggestions.innerHTML = names.map((name) =>
    `<li role="option" data-suggestion="${escapeHTML(name)}">${escapeHTML(name)}</li>`).join('');
  builderCommanderSuggestions.hidden = false;
}

function hideCommanderSuggestions() {
  commanderAutocompleteController?.abort();
  builderCommanderSuggestions.hidden = true;
  builderCommanderSuggestions.innerHTML = '';
  commanderAutocompleteIndex = -1;
}

function highlightCommanderSuggestion(index) {
  const items = commanderSuggestionItems();
  items.forEach((item, i) => {
    item.classList.toggle('active', i === index);
    if (i === index) item.scrollIntoView({ block: 'nearest' });
  });
}

async function queryCommanderSuggestions(query) {
  const value = String(query ?? '').trim();
  if (value.length < 2) {
    hideCommanderSuggestions();
    return;
  }
  commanderAutocompleteController?.abort();
  const controller = new AbortController();
  commanderAutocompleteController = controller;
  try {
    const response = await fetch(`/api/v1/commander-autocomplete?q=${encodeURIComponent(value)}`, { signal: controller.signal });
    const payload = await response.json();
    if (response.ok && Array.isArray(payload.suggestions)) {
      renderCommanderSuggestions(payload.suggestions);
    } else {
      hideCommanderSuggestions();
    }
  } catch (error) {
    // Aborted requests land here intentionally; anything else is a transient miss.
    if (error.name !== 'AbortError') hideCommanderSuggestions();
  }
}

function chooseCommanderSuggestion(name) {
  builderCommanderInput.value = name;
  hideCommanderSuggestions();
  builderCommanderInput.focus();
}

builderCommanderInput.addEventListener('input', () => {
  clearTimeout(commanderAutocompleteTimer);
  commanderAutocompleteTimer = setTimeout(() => queryCommanderSuggestions(builderCommanderInput.value), 220);
});

builderCommanderInput.addEventListener('keydown', (event) => {
  const items = commanderSuggestionItems();
  if (event.key === 'ArrowDown') {
    event.preventDefault();
    commanderAutocompleteIndex = Math.min(commanderAutocompleteIndex + 1, items.length - 1);
    highlightCommanderSuggestion(commanderAutocompleteIndex);
  } else if (event.key === 'ArrowUp') {
    event.preventDefault();
    commanderAutocompleteIndex = Math.max(commanderAutocompleteIndex - 1, 0);
    highlightCommanderSuggestion(commanderAutocompleteIndex);
  } else if (event.key === 'Enter') {
    const active = items[commanderAutocompleteIndex];
    if (active) {
      event.preventDefault();
      chooseCommanderSuggestion(active.dataset.suggestion);
    }
  } else if (event.key === 'Escape') {
    hideCommanderSuggestions();
  }
});

builderCommanderInput.addEventListener('blur', () => {
  // Delay so a click on a suggestion lands before we tear the list down.
  setTimeout(hideCommanderSuggestions, 120);
});

builderCommanderSuggestions.addEventListener('mousedown', (event) => {
  const item = event.target.closest('[data-suggestion]');
  if (item) {
    event.preventDefault();
    chooseCommanderSuggestion(item.dataset.suggestion);
  }
});


// 一键出地：展开/收起八个地牌分类按钮。点击单个分类按主将色组请求可用地牌，
// 渲染成可点选的小图，点某张地把它加入草稿。
function toggleLandsPanel() {
  const visible = !builderLandsPanel.hidden;
  if (visible) {
    builderLandsPanel.hidden = true;
    return;
  }
  builderLandsGrid.innerHTML = LAND_CATEGORIES.map((category) => `
    <button type="button" class="builder-land-category" data-land-category="${category.id}">
      <strong>${category.label}</strong>
    </button>`).join('');
  builderLandsPanel.hidden = false;
  builderLandsGrid.innerHTML += '<div id="builder-lands-result" class="builder-lands-result"></div>';
}

function closeLandsPanel() {
  builderLandsPanel.hidden = true;
}

async function loadLandCategory(categoryID) {
  const resultBox = document.querySelector('#builder-lands-result');
  if (!resultBox) return;
  const category = LAND_CATEGORIES.find((item) => item.id === categoryID);
  resultBox.innerHTML = '<p class="editor-empty">正在加载地牌…</p>';
  try {
    const response = await fetch('/api/v1/build-lands', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ category: categoryID, color_identity: buildColors })
    });
    const payload = await response.json();
    if (!response.ok) throw new Error(payload.error?.message || '无法加载地牌。');
    const lands = Array.isArray(payload.lands) ? payload.lands : [];
    if (!lands.length) {
      resultBox.innerHTML = '<p class="editor-empty">该类别在你的主将色组内没有可用地牌。</p>';
      return;
    }
    resultBox.innerHTML = `<div class="builder-lands-head"><strong>${escapeHTML(payload.category_label || category?.label || '')}</strong><small>点击地牌加入草稿</small></div>
      <div class="builder-lands-cards">${lands.map((land) => {
        const image = cardImage(land.card);
        return `<button type="button" class="builder-land-card" data-land-name="${escapeHTML(land.name)}">${image ? `<img loading="lazy" src="${escapeHTML(image)}" alt="${escapeHTML(land.name)}">` : '<div class="builder-candidate-placeholder"></div>'}<span>${escapeHTML(land.name)}</span></button>`;
      }).join('')}</div>`;
  } catch (error) {
    resultBox.innerHTML = `<p class="form-message">${escapeHTML(error.message || '加载失败')}</p>`;
  }
}

function addLandCard(name) {
  if (!name) return;
  if (buildCards.length + (buildCommander ? 1 : 0) >= BUILD_TARGET) return;
  buildChosen.push(normalizeBuildName(name));
  buildCards.push({ name, card: { name, type_line: 'Land' } });
  renderBuilderSidebar();
  if (isBuilderComplete()) {
    builderWorkflow.hidden = true;
    builderComplete.hidden = false;
  }
}

function isBuilderComplete() {
  return buildCards.length + (buildCommander ? 1 : 0) >= BUILD_TARGET;
}

async function startBuild() {
  const name = builderCommanderInput.value.trim();
  if (!name) {
    builderMessage.textContent = '请输入主将名称。';
    return;
  }
  builderMessage.textContent = '';
  builderStartButton.disabled = true;
  builderStartButton.textContent = '加载中…';
  try {
    const response = await fetch('/api/v1/build-suggest', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ commander: name, chosen: [], seen: [], count: 3 })
    });
    const payload = await response.json();
    if (!response.ok) throw new Error(payload.error?.message || '无法加载主将建议。');
    buildCommander = payload.commander_name || name;
    buildColors = payload.color_identity || [];
    buildChosen = [];
    buildCards = [];
    builderWorkflow.hidden = false;
    builderComplete.hidden = true;
    applyBuildCandidates(Array.isArray(payload.candidates) ? payload.candidates : []);
    renderBuilderSidebar();
  } catch (error) {
    builderMessage.textContent = error.message || '加载失败，请重试。';
  } finally {
    builderStartButton.disabled = false;
    builderStartButton.textContent = '开始';
  }
}

function applyBuildCandidates(candidates) {
  buildCandidates = Array.isArray(candidates) ? candidates : [];
  builderCandidates.innerHTML = buildCandidates.map((card, index) => {
    const image = cardImage(card.card);
    const preview = cardPreviewImage(card.card);
    const fills = (card.fills || []).map((id) => buildMetricLabel(id)).filter(Boolean).join(' · ');
    return `
      <button type="button" class="builder-candidate" data-candidate="${index}" data-preview-src="${escapeHTML(preview)}" data-preview-name="${escapeHTML(card.name)}">
        ${image ? `<img loading="lazy" src="${escapeHTML(image)}" alt="${escapeHTML(card.name)}">` : '<div class="builder-candidate-placeholder"></div>'}
        <div class="builder-candidate-body">
          <strong>${escapeHTML(card.name)}</strong>
          <span class="builder-synergy">Synergy ${(Number(card.synergy) || 0).toFixed(0)}%</span>
          ${fills ? `<small>补足：${escapeHTML(fills)}</small>` : ''}
        </div>
      </button>`;
  }).join('') || '<p class="editor-empty">暂时没有更多建议，可快速加基本地或直接完成。</p>';
}

function cardImage(card) {
  const cardObj = card || {};
  return cardObj.image_small || cardObj.image_normal || (cardObj.faces || []).find((face) => face.image_small || face.image_normal)?.image_small || (cardObj.faces || []).find((face) => face.image_small || face.image_normal)?.image_normal || '';
}

// Prefer the larger face image for hover previews; falls back to the small grid
// image when the card has no normal-size art.
function cardPreviewImage(card) {
  const cardObj = card || {};
  return cardObj.image_normal || cardObj.image_small || (cardObj.faces || []).find((face) => face.image_normal || face.image_small)?.image_normal || (cardObj.faces || []).find((face) => face.image_normal || face.image_small)?.image_small || '';
}

function buildMetricLabel(id) {
  const metric = BUILD_METRICS.find((item) => item.id === id);
  return metric ? metric.label : id;
}

async function nextBuildBatch() {
  if (!buildCommander) return;
  // Every refresh draws a fresh random hand straight from the server. There is no
  // local "seen" tracking or role ordering anymore: the only exclusion criterion is
  // "already chosen", so skipped cards are immediately eligible to reappear.
  builderCandidates.innerHTML = '<p class="editor-empty">正在换一批…</p>';
  try {
    const response = await fetch('/api/v1/build-suggest', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ commander: buildCommander, chosen: buildChosen, seen: [], count: 3 })
    });
    const payload = await response.json();
    if (!response.ok) throw new Error(payload.error?.message || '无法加载建议。');
    const candidates = Array.isArray(payload.candidates) ? payload.candidates : [];
    applyBuildCandidates(candidates);
  } catch (error) {
    builderCandidates.innerHTML = `<p class="form-message">${escapeHTML(error.message || '加载失败')}</p>`;
  }
}

// Normalize a card name to its front-face, lowercased form so split cards
// ("X // Y") dedupe against the same card offered by its front face ("X").
function normalizeBuildName(name) {
  const trimmed = String(name ?? '').trim().toLowerCase();
  const idx = trimmed.indexOf(' // ');
  return idx > 0 ? trimmed.slice(0, idx).trim() : trimmed;
}

function addBuildCard(candidate) {
  if (!candidate?.name) return;
  const key = normalizeBuildName(candidate.name);
  if (buildChosen.includes(key)) return;
  buildChosen.push(key);
  buildCards.push({ name: candidate.name, card: candidate.card || {} });
  renderBuilderSidebar();
  if (isBuilderComplete()) {
    builderWorkflow.hidden = true;
    builderComplete.hidden = false;
  } else {
    // Fetch a fresh random hand immediately; the un-picked cards vanish with it.
    nextBuildBatch();
  }
}

function addBasicLand(type) {
  const key = type.toLowerCase();
  const count = buildCards.filter((card) => card.name.toLowerCase() === key).length;
  // Basic lands may appear in multiples; still avoid exceeding the 100-card ceiling.
  if (buildCards.length + (buildCommander ? 1 : 0) >= BUILD_TARGET) return;
  buildCards.push({ name: type, card: { name: type, type_line: `Basic Land — ${type}` } });
  renderBuilderSidebar();
  if (isBuilderComplete()) {
    builderWorkflow.hidden = true;
    builderComplete.hidden = false;
  }
}

function renderBuilderSidebar() {
  const current = {};
  for (const item of buildCards) {
    const card = item.card || {};
    for (const metric of BUILD_METRICS) {
      if (builderCardMatches(metric.id, card)) {
        current[metric.id] = (current[metric.id] || 0) + 1;
      }
    }
  }
  const total = buildCards.length + (buildCommander ? 1 : 0);

  // Aggregate the drafted mainboard by name so the list shows "2× Sol Ring" style
  // rows. Card objects are kept so a later hover preview can reuse their art.
  const byName = new Map();
  for (const item of buildCards) {
    const key = normalizeBuildName(item.name);
    const entry = byName.get(key) || { name: item.name, count: 0, card: item.card };
    entry.count += 1;
    if (!entry.card?.name) entry.card = item.card;
    byName.set(key, entry);
  }
  const chosenList = Array.from(byName.values())
    .sort((a, b) => String(a.name).localeCompare(String(b.name)))
    .map((entry) => `
      <li class="builder-chosen-item">
        <span class="builder-chosen-name">${escapeHTML(entry.name)}</span>
        <strong class="builder-chosen-count">${entry.count}×</strong>
      </li>`).join('');

  builderSidebar.innerHTML = `
    <div class="builder-progress"><strong>${total}</strong><span>/ ${BUILD_TARGET} 张</span></div>
    ${BUILD_METRICS.map((metric) => {
      const actual = current[metric.id] || 0;
      const pct = Math.min(100, Math.round((actual / Math.max(1, metric.target)) * 100));
      return `<div class="builder-metric"><div class="builder-metric-head"><span>${metric.label}</span><strong>${actual} / ${metric.target}</strong></div><div class="builder-metric-bar"><i style="width:${pct}%"></i></div></div>`;
    }).join('')}
    <div class="builder-color-identity"><span>主将色组</span><strong>${(buildColors || []).map((color) => escapeHTML(color)).join(' ') || '无色'}</strong></div>
    <div class="builder-chosen">
      <div class="builder-chosen-head"><strong>已选牌</strong><span>${buildCards.length}</span></div>
      ${chosenList ? `<ul class="builder-chosen-list">${chosenList}</ul>` : '<p class="builder-chosen-empty">还没有选择任何牌。</p>'}
    </div>`;
}

// A client-side mirror of the server's construction.Classify for live "正向法力"
// and category counts. It intentionally treats "lands" as "net-positive mana": a
// land OR a 0-cost artifact that produces mana (Sol Ring, Mox, Lotus Petal).
function builderCardMatches(id, card) {
  const text = String((card.oracle_text || '') + ' ' + (card.faces || []).map((face) => face.oracle_text || '').join(' ')).toLowerCase();
  const typeLine = String((card.type_line || '') + ' ' + (card.faces || []).map((face) => face.type_line || '').join(' ')).toLowerCase();
  const cmc = Number(card.cmc);
  switch (id) {
    case 'lands': {
      const isLand = typeLine.includes('land');
      const isFastMana = cmc === 0 && typeLine.includes('artifact') && /\badd\b/.test(text);
      return isLand || isFastMana;
    }
    case 'mass_interaction':
      return text.includes('each player') || text.includes('all creatures') || text.includes('destroy all') || text.includes('exile all');
    case 'single_interaction':
      return text.includes('target') && (text.includes('destroy') || text.includes('exile') || text.includes('counter target') || text.includes('return target'));
    case 'draw_discard':
      return text.includes('draw a card') || text.includes('draw cards') || text.includes('draw that many') || text.includes('discard');
    case 'ramp':
      return !typeLine.includes('land') && (text.includes('add {') || text.includes('additional land') || text.includes('search your library for a basic land') || text.includes('costs {'));
    case 'plan':
      return text.includes('token') || text.includes('proliferate') || text.includes('infect') || text.includes('poison') || text.includes('whenever');
    default:
      return false;
  }
}

function builderToDeckText() {
  const commanderLine = buildCommander ? `1 ${buildCommander}` : '';
  const mainboardLines = buildCards.map((card) => `1 ${card.name}`);
  return `Commander\n${commanderLine}\n\nDeck\n${mainboardLines.join('\n')}`;
}

buildEntryButton.addEventListener('click', openBuilder);
builderClose.addEventListener('click', closeBuilder);
builderStartButton.addEventListener('click', startBuild);
builderSkip.addEventListener('click', nextBuildBatch);
builderLandsButton.addEventListener('click', toggleLandsPanel);
builderLandsClose.addEventListener('click', closeLandsPanel);
builderExport.addEventListener('click', () => downloadText('decklist.txt', builderToDeckText()));
builderAnalyze.addEventListener('click', () => {
  decklistInput.value = builderToDeckText();
  closeBuilder();
  analyze();
});

// Delegate builder candidate clicks through the global click listener. Basic lands
// are wired here, self-contained and gated to the commander's color identity is
// optional; all five are offered for simplicity.
document.addEventListener('click', (event) => {
  const candidateButton = event.target.closest('[data-candidate]');
  if (candidateButton) {
    const index = Number(candidateButton.dataset.candidate);
    const candidate = buildCandidates[index];
    if (candidate) addBuildCard(candidate);
    return;
  }
  const landCategory = event.target.closest('[data-land-category]');
  if (landCategory) {
    loadLandCategory(landCategory.dataset.landCategory || '');
    return;
  }
  const landCard = event.target.closest('[data-land-name]');
  if (landCard) {
    addLandCard(landCard.dataset.landName || '');
    return;
  }
  const basicButton = event.target.closest('[data-basic]');
  if (basicButton) {
    addBasicLand(basicButton.dataset.basic || '');
    return;
  }
});

copyDecklistButton.addEventListener('click', async () => {
  if (!currentDeckText) return;
  try {
    await navigator.clipboard.writeText(currentDeckText);
    copyDecklistButton.textContent = '已复制';
    setTimeout(() => { copyDecklistButton.textContent = '复制牌表'; }, 1600);
  } catch {
    copyDecklistButton.textContent = '复制失败';
    setTimeout(() => { copyDecklistButton.textContent = '复制牌表'; }, 1600);
  }
});

decklistToggle.addEventListener('click', () => {
  const list = document.querySelector('#deck-card-list');
  const collapsed = list.classList.toggle('collapsed');
  decklistToggle.textContent = collapsed ? '展开牌表' : '收起牌表';
});

form.addEventListener('submit', async (event) => {
  event.preventDefault();
  await analyze();
});

retryButton.addEventListener('click', () => {
  results.hidden = true;
  input.focus();
  window.scrollTo({ top: 0, behavior: 'smooth' });
});

async function analyze() {
  const url = input.value.trim();
  const decklist = decklistInput.value.trim();
  message.textContent = '';
  if (!url && !decklist) {
    message.textContent = '请填写 Moxfield URL 或粘贴牌表文本。';
    input.focus();
    return;
  }
  if (url && !isMoxfieldDeckURL(url)) {
    message.textContent = '请输入有效的公开 Moxfield 牌组地址。';
    input.focus();
    return;
  }

  setLoading(true);
  results.hidden = true;
  try {
    const response = await fetch('/api/v1/analyze', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ url, decklist })
    });
    const payload = await response.json();
    if (!response.ok) {
      throw new Error(payload.error?.message || '分析失败，请稍后重试。');
    }
    render(payload);
  } catch (error) {
    message.textContent = error.message || '网络请求失败，请稍后重试。';
  } finally {
    setLoading(false);
  }
}

function isMoxfieldDeckURL(value) {
  try {
    const parsed = new URL(value);
    return parsed.protocol === 'https:' &&
      ['moxfield.com', 'www.moxfield.com'].includes(parsed.hostname.toLowerCase()) &&
      /^\/decks\/[A-Za-z0-9_-]{6,64}\/?$/.test(parsed.pathname);
  } catch {
    return false;
  }
}

function setLoading(active) {
  loading.hidden = !active;
  submitButton.disabled = active;
  submitButton.querySelector('.button-label').textContent = active ? '分析中…' : '开始分析';
}

function render(payload) {
  document.querySelector('#deck-name').textContent = payload.deck.name || payload.deck.commanders.join(' / ');
  document.querySelector('#deck-meta').textContent = `${payload.deck.commanders.join(' / ')} · ${payload.deck.card_count} 张牌`;
  renderProvider('salt', payload.results.commandersalt, [
    ['salt', 'Salt']
  ]);
  renderProvider('edh', payload.results.edhpowerlevel, [
    ['efficiency', 'Efficiency'], ['impact', 'Impact'],
    ['score', 'Score'], ['average_playability', 'Playability']
  ]);
  warning.hidden = !payload.warnings?.length;
  warning.textContent = payload.warnings?.join(' ') || '';
  renderManabase(payload.manabase);
  renderConstructionReport(payload.construction_report);
  renderCombos(payload.combos || []);
  renderRecommendations(payload.recommendations || [], payload.recommendation_keywords || []);
  renderDeckCards(payload.deck_cards || []);
  currentDeckCards = payload.deck_cards || [];
  currentDeckText = payload.canonical_decklist || buildDeckText(currentDeckCards);
  beginEditing(payload.deck?.id || '', currentDeckCards);
  results.hidden = false;
  results.scrollIntoView({ behavior: 'smooth', block: 'start' });
}

function renderManabase(manabase) {
  const section = document.querySelector('#manabase-section');
  const container = document.querySelector('#manabase-content');
  if (!manabase) {
    section.hidden = true;
    return;
  }
  section.hidden = false;

  const target = Number(manabase.target_lands);
  const actual = Number(manabase.actual_lands);
  const delta = Number(manabase.land_delta);
  const deltaText = delta > 0 ? `+${formatNumber(delta, 1)}` : formatNumber(delta, 1);
  const deltaClass = delta >= -1 ? 'healthy' : delta >= -2 ? 'warn' : 'short';
  const avgMv = formatNumber(manabase.average_mana_value, 2);
  const ramp = Number(manabase.ramp_and_draw_under_three || 0);
  const fastMana = Number(manabase.fast_mana || 0);
  const findings = Array.isArray(manabase.color_findings) ? manabase.color_findings : [];

  const landDeltaLabel = {
    healthy: '地数充足',
    warn: '接近目标',
    short: '地数不足'
  }[deltaClass];

  container.innerHTML = `
    <div class="manabase-hero">
      <div class="manabase-land-stat"><span>实际地数</span><strong>${Number.isFinite(actual) ? actual : '—'}</strong></div>
      <div class="manabase-land-stat"><span>推荐地数</span><strong>${Number.isFinite(target) ? formatNumber(target, 1) : '—'}</strong></div>
      <div class="manabase-land-stat ${deltaClass}"><span>偏差</span><strong>${deltaText}</strong><small>${landDeltaLabel}</small></div>
    </div>
    <p class="manabase-formula">平均法术力值 <strong>${avgMv}</strong> · 地牌/抽牌信用 <strong>${ramp}</strong> · 快法力 <strong>${fastMana}</strong></p>
    ${findings.length ? `
      <div class="manabase-colors-heading"><span>颜色来源需求</span><small>需要 vs 当前（加权）</small></div>
      <div class="manabase-color-grid">${findings.map(renderColorFinding).join('')}</div>` : ''}`;
}

function renderColorFinding(finding) {
  const color = escapeHTML(String(finding.color ?? ''));
  const actual = Number(finding.actual_sources);
  const required = Number(finding.required_sources);
  const deficit = required - actual;
  const adequate = !Number.isFinite(deficit) || deficit <= 0;
  const pct = required > 0 ? Math.min(100, Math.round((actual / required) * 100)) : 100;
  const driving = String(finding.driving_spell ?? '').trim();
  return `
    <div class="manabase-color ${adequate ? 'adequate' : 'short'}">
      <div class="manabase-color-head"><span class="mana-symbol">${color}</span><div><strong>${required}</strong> 需求来源 / <strong>${formatNumber(actual, 1)}</strong> 当前</div></div>
      <div class="manabase-color-bar"><i style="width:${pct}%"></i></div>
      <div class="manabase-color-meta">${adequate ? '来源充足' : `缺少约 ${formatNumber(deficit, 1)} 个来源`}${driving ? ` · 需求由 ${escapeHTML(driving)} 的多色费用驱动` : ''}</div>
    </div>`;
}

function renderConstructionReport(report) {
  const section = document.querySelector('#construction-section');
  const container = document.querySelector('#construction-metrics');
  const metrics = Array.isArray(report?.metrics) ? report.metrics : [];
  section.hidden = !metrics.length;
  container.innerHTML = metrics.map((metric) => {
    const percent = Math.min(100, Math.round((Number(metric.actual) / Math.max(1, Number(metric.target))) * 100));
    const cards = (metric.cards || []).map((card) => `<li><strong>${card.quantity}× ${escapeHTML(card.name)}</strong><span>${escapeHTML(card.reason)}</span></li>`).join('');
    return `<details class="construction-metric ${metric.status}">
      <summary><div><span>${escapeHTML(metric.label)}</span><strong>${metric.actual} / ${metric.target}</strong></div><div class="construction-bar"><i style="width:${percent}%"></i></div><small>${metric.gap > 0 ? `缺少 ${metric.gap}` : '已充分'}</small></summary>
      ${cards ? `<ul>${cards}</ul>` : '<p>没有识别到相关卡牌。</p>'}
    </details>`;
  }).join('');
}

function buildDeckText(cards) {
  const commanders = cards.filter((item) => item.commander).map((item) => `${item.quantity || 1} ${item.card?.name || ''}`);
  const mainboard = cards.filter((item) => !item.commander).map((item) => `${item.quantity || 1} ${item.card?.name || ''}`);
  return `Commander\n${commanders.join('\n')}\n\nDeck\n${mainboard.join('\n')}`;
}

function openSwap(addName) {
  pendingSwapAdd = addName;
  selectedSwapRemove = '';
  document.querySelector('#swap-add-name').textContent = addName;
  swapSearch.value = '';
  swapMessage.textContent = '';
  swapResult.hidden = true;
  swapSubmit.disabled = true;
  renderSwapRemoveList('');
  swapModal.hidden = false;
  swapSearch.focus();
}

function closeSwap() {
  swapModal.hidden = true;
  pendingSwapAdd = '';
  selectedSwapRemove = '';
}

function renderSwapRemoveList(query) {
  const normalized = query.trim().toLowerCase();
  const cards = currentDeckCards.filter((item) => !item.commander && (!normalized || String(item.card?.name || '').toLowerCase().includes(normalized)));
  const groups = [
    ['Nonlands', cards.filter((item) => !item.land)],
    ['Lands', cards.filter((item) => item.land)]
  ];
  document.querySelector('#swap-remove-list').innerHTML = groups.filter(([, items]) => items.length).map(([label, items]) => `<section><h3>${label}</h3>${items.map((item) => `<button type="button" class="swap-remove-option${selectedSwapRemove === item.card.name ? ' selected' : ''}" data-remove-name="${escapeHTML(item.card.name)}"><span>${item.quantity}×</span><strong>${escapeHTML(item.card.name)}</strong></button>`).join('')}</section>`).join('') || '<p>没有匹配的 Mainboard 卡牌。</p>';
}

async function compareSwap() {
  if (!currentDeckText || !pendingSwapAdd || !selectedSwapRemove) return;
  swapSubmit.disabled = true;
  swapSubmit.textContent = '比较中…';
  swapMessage.textContent = '';
  try {
    const response = await fetch('/api/v1/compare-swap', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ decklist: currentDeckText, remove_name: selectedSwapRemove, add_name: pendingSwapAdd })
    });
    const payload = await response.json();
    if (!response.ok) throw new Error(payload.error?.message || '替换比较失败。');
    renderSwapResult(payload);
  } catch (error) {
    swapMessage.textContent = error.message || '替换比较失败。';
  } finally {
    swapSubmit.disabled = !selectedSwapRemove;
    swapSubmit.textContent = '比较替换';
  }
}

function renderSwapResult(payload) {
  const deltas = (payload.deltas || []).map((metric) => {
    const delta = Number(metric.delta || 0);
    const deltaText = delta > 0 ? `+${delta}` : String(delta);
    return `<tr><th>${escapeHTML(metric.label)}</th><td>${metric.before}</td><td>${metric.after}</td><td class="swap-delta ${delta > 0 ? 'positive' : delta < 0 ? 'negative' : 'neutral'}">${deltaText}</td></tr>`;
  }).join('');
  const issues = (payload.legality?.issues || []).map((issue) => `<li>${escapeHTML(issue)}</li>`).join('');
  swapResult.innerHTML = `<div class="swap-result-title"><strong>${escapeHTML(payload.removed?.name || '')}</strong><span>→</span><strong>${escapeHTML(payload.added?.name || '')}</strong></div>
    <table><thead><tr><th>指标</th><th>替换前</th><th>替换后</th><th>变化</th></tr></thead><tbody>${deltas}</tbody></table>
    <div class="swap-legality ${payload.legality?.valid ? 'valid' : 'warning'}"><strong>基础合法性：${payload.legality?.valid ? '通过' : '需要注意'}</strong><span>牌张数 ${payload.before?.card_count} → ${payload.after?.card_count} · Commander 色组 ${(payload.legality?.color_identity || []).join('') || '无色'}</span>${issues ? `<ul>${issues}</ul>` : ''}</div>
    <textarea readonly aria-label="更新后的牌表">${escapeHTML(payload.updated_decklist || '')}</textarea>
    <button id="copy-swap-decklist" class="ghost-button" type="button">复制更新后牌表</button>`;
  swapResult.hidden = false;
  document.querySelector('#copy-swap-decklist').addEventListener('click', async (event) => {
    try {
      await navigator.clipboard.writeText(payload.updated_decklist || '');
      event.currentTarget.textContent = '已复制';
    } catch {
      event.currentTarget.textContent = '复制失败';
    }
  });
}

function renderCombos(combos) {
  const section = document.querySelector('#combo-section');
  const container = document.querySelector('#combo-list');
  section.hidden = !combos.length;
  container.innerHTML = combos.map((combo) => `
    <article class="combo-card">
      <div class="combo-header"><div><span class="source-badge">COMMANDER SPELLBOOK</span><h3>${escapeHTML(combo.name)}</h3></div>${combo.source_url ? `<a href="${escapeHTML(combo.source_url)}" target="_blank" rel="noopener noreferrer">查看来源 ↗</a>` : ''}</div>
      <div class="combo-components">${(combo.components || []).map(renderCard).join('')}</div>
      ${combo.result ? `<p class="combo-result"><strong>结果</strong>${escapeHTML(combo.result)}</p>` : ''}
      ${combo.steps?.length ? `<details><summary>执行步骤</summary><ol>${combo.steps.map((step) => `<li>${escapeHTML(step)}</li>`).join('')}</ol></details>` : ''}
    </article>`).join('');
}

function renderRecommendations(recommendations, keywords) {
  const section = document.querySelector('#recommendation-section');
  const container = document.querySelector('#recommendation-list');
  section.hidden = !recommendations.length;
  document.querySelector('#recommendation-keywords').textContent = keywords.length ? `EDHREC 主题：${keywords.join(' · ')}` : '';
  container.innerHTML = recommendations.map((group) => `
    <section class="recommendation-group" data-tag="${escapeHTML(group.tag || '')}">
      <div class="recommendation-group-heading"><h3>${escapeHTML(group.header || 'Recommendations')}</h3><span>${group.cards?.length || 0}</span></div>
      <div class="recommendation-row">${(group.cards || []).map((item) => {
        const fills = (item.fills || []).map((fill) => `<li><strong>${escapeHTML(fill.label)} · 还缺 ${Number(fill.gap) || 0}</strong><span>${escapeHTML(fill.reason || '')}</span></li>`).join('');
        return `<article class="recommendation-card">
          ${renderCard({ card: item.card, quantity: 1 })}
          <div class="recommendation-meta">
            <div><span>Synergy</span><strong>${(Number(item.synergy || 0) * 100).toFixed(1)}%</strong></div>
            <div><span>Inclusion</span><strong>${(Number(item.inclusion_rate || 0) * 100).toFixed(1)}%</strong></div>
          </div>
          ${fills ? `<ul class="recommendation-fills">${fills}</ul>` : ''}
          <button class="swap-start ghost-button" type="button" data-add-name="${escapeHTML(item.card?.name || '')}">加入构筑</button>
          <p>${escapeHTML(item.reason || '')}</p>
          <a href="${escapeHTML(item.source_url || 'https://edhrec.com/')}" target="_blank" rel="noopener noreferrer">在 EDHREC 查看 ↗</a>
        </article>`;
      }).join('')}</div>
    </section>`).join('');
}

function renderDeckCards(cards) {
  const section = document.querySelector('#decklist-section');
  const container = document.querySelector('#deck-card-list');
  section.hidden = !cards.length;
  const groups = [
    ['Commander', cards.filter((item) => item.commander)],
    ['Nonlands', cards.filter((item) => !item.commander && !item.land)],
    ['Lands', cards.filter((item) => !item.commander && item.land)]
  ];
  container.innerHTML = groups.filter(([, items]) => items.length).map(([title, items]) => `
    <section class="deck-group"><h3>${title} <span>${items.reduce((sum, item) => sum + item.quantity, 0)}</span></h3><div class="card-grid">${items.map(renderCard).join('')}</div></section>`).join('');
}

// --- light deck editor --------------------------------------------------------

// Begin editing the given deck. The cards array becomes the mutable editor state;
// invoke at the start of each edit session (after an analysis or a version load).
function beginEditing(sourceId, cards) {
  activeSourceId = String(sourceId || '');
  editorCards = cards.map((item) => ({ ...item }));
  editorUndo = [];
  editorRedo = [];
  editorDirty = false;
  syncEditorUI();
}

// A cheap structural clone sufficient for our flat card objects (card payloads are
// already plain JSON from the API). Keeps a real snapshot, not a shared ref.
function cloneCards(cards) {
  return cards.map((item) => ({ ...item }));
}

function pushUndo() {
  editorUndo.push(cloneCards(editorCards));
  if (editorUndo.length > 100) editorUndo.shift();
  editorRedo = [];
}

function markDirty() {
  editorDirty = true;
  editorDirtyNote.hidden = false;
}

function syncEditorUI() {
  editorUndoButton.disabled = editorUndo.length === 0;
  editorRedoButton.disabled = editorRedo.length === 0;
  editorDirtyNote.hidden = !editorDirty;
}

function indexPath(name) {
  return editorCards.findIndex((item) => String(item.card?.name || '').toLowerCase() === name.toLowerCase());
}

function isCommanderByName(name) {
  return editorCards.some((item) => item.commander && String(item.card?.name || '').toLowerCase() === name.toLowerCase());
}

function addCardToEditor(name) {
  const existing = indexPath(name);
  if (existing >= 0) {
    editorCards[existing].quantity += 1;
  } else {
    editorCards.push({ card: { name }, quantity: 1, commander: false, land: false });
  }
}

function removeCardFromEditor(name) {
  const index = indexPath(name);
  if (index < 0) return;
  const item = editorCards[index];
  if (item.quantity > 1) {
    item.quantity -= 1;
  } else {
    editorCards.splice(index, 1);
  }
}

// Apply one mutating operation with undo support, then re-render the deck list.
function applyEdit(mutate) {
  pushUndo();
  mutate(editorCards);
  editorRedo = [];
  markDirty();
  renderDeckCards(editorCards);
  syncEditorUI();
}

function undoEdit() {
  if (!editorUndo.length) return;
  editorRedo.push(cloneCards(editorCards));
  editorCards = editorUndo.pop();
  markDirty();
  renderDeckCards(editorCards);
  syncEditorUI();
}

function redoEdit() {
  if (!editorRedo.length) return;
  editorUndo.push(cloneCards(editorCards));
  editorCards = editorRedo.pop();
  markDirty();
  renderDeckCards(editorCards);
  syncEditorUI();
}

// Add a card by name. Resolves the Scryfall payload via the single-card endpoint so
// the new row gets its art/type/color identity; falls back to a bare name row and a
// message if the lookup fails, rather than blocking the local edit.
async function addCardByName(rawName) {
  const name = (rawName || '').trim();
  if (!name) return;
  let card = { name };
  try {
    const response = await fetch(`/api/v1/card?name=${encodeURIComponent(name)}`);
    if (response.ok) {
      const payload = await response.json();
      if (payload?.card?.name) {
        card = payload.card;
        card.land = String(payload.card.type_line || '').toLowerCase().includes('land');
      }
    }
  } catch {
    // Offline / transient failure: keep the bare name row.
  }
  applyEdit((cards) => {
    const existing = cards.findIndex((item) => String(item.card?.name || '').toLowerCase() === name.toLowerCase());
    if (existing >= 0) {
      cards[existing].quantity += 1;
    } else {
      cards.push({ card, quantity: 1, commander: false, land: Boolean(card.land) });
    }
  });
}

// Serialize the current editor state to the canonical decklist format. Uses the same
// "quantity name" shape PrintPlainText consumes; sorting is skipped to preserve the
// user's working order, which is fine for an editable local draft.
function editorToDeckText() {
  const commanders = editorCards.filter((item) => item.commander).map((item) => `${item.quantity || 1} ${item.card?.name || ''}`);
  const mainboard = editorCards.filter((item) => !item.commander).map((item) => `${item.quantity || 1} ${item.card?.name || ''}`);
  return `Commander\n${commanders.join('\n')}\n\nDeck\n${mainboard.join('\n')}`;
}

function downloadText(filename, text) {
  const blob = new Blob([text], { type: 'text/plain;charset=utf-8' });
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = filename;
  document.body.appendChild(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(url);
}

function exportEditedDeck() {
  downloadText('decklist.txt', editorToDeckText());
}

function versionsKey() {
  return activeSourceId ? `${EDITOR_VERSIONS_KEY}.${activeSourceId}` : EDITOR_VERSIONS_KEY;
}

function loadVersions() {
  try {
    return JSON.parse(localStorage.getItem(versionsKey()) || '[]');
  } catch {
    return [];
  }
}

function saveVersions(versions) {
  try {
    localStorage.setItem(versionsKey(), JSON.stringify(versions));
  } catch {
    // Quota / disabled storage: versions are best-effort.
  }
}

function saveVersion() {
  const versions = loadVersions();
  versions.unshift({
    name: document.querySelector('#deck-name').textContent || 'Untitled deck',
    savedAt: new Date().toISOString(),
    cards: cloneCards(editorCards)
  });
  if (versions.length > 20) versions.length = 20;
  saveVersions(versions);
  renderVersionsList();
}

function renderVersionsList() {
  const versions = loadVersions();
  if (!versions.length) {
    editorVersionsList.innerHTML = '<p class="editor-empty">还没有保存的版本。</p>';
    return;
  }
  editorVersionsList.innerHTML = versions.map((version, index) => `
    <div class="editor-version">
      <div><strong>${escapeHTML(version.name || '未命名')}</strong><small>${new Date(version.savedAt).toLocaleString('zh-CN')} · ${(version.cards || []).reduce((sum, item) => sum + (item.quantity || 0), 0)} 张</small></div>
      <div class="editor-version-actions">
        <button type="button" data-load-version="${index}">载入</button>
        <button type="button" data-delete-version="${index}">删除</button>
      </div>
    </div>`).join('');
}

function loadVersion(index) {
  const versions = loadVersions();
  const version = versions[index];
  if (!version?.cards) return;
  beginEditing(activeSourceId, version.cards);
  markDirty();
  renderDeckCards(editorCards);
  editorVersionsPanel.hidden = true;
}

function deleteVersion(index) {
  const versions = loadVersions();
  versions.splice(index, 1);
  saveVersions(versions);
  renderVersionsList();
}

// Wire the editor toolbar and hotkeys. Deck-list card action buttons are delegated
// through the global click listener above; add/remove/undo/redo live here.
editorAddButton.addEventListener('click', () => addCardByName(editorAddInput.value));
editorAddInput.addEventListener('keydown', (event) => {
  if (event.key === 'Enter') {
    event.preventDefault();
    addCardByName(editorAddInput.value);
  }
});
editorUndoButton.addEventListener('click', undoEdit);
editorRedoButton.addEventListener('click', redoEdit);
editorSaveVersionButton.addEventListener('click', saveVersion);
editorExportButton.addEventListener('click', exportEditedDeck);
editorVersionsButton.addEventListener('click', () => {
  editorVersionsPanel.hidden = !editorVersionsPanel.hidden;
  renderVersionsList();
});

function renderCard(item) {
  const card = item.card || {};
  const picturedFaces = Array.isArray(card.faces) ? card.faces.filter((face) => face.image_small || face.image_normal) : [];
  const image = card.image_small || card.image_normal || picturedFaces[0]?.image_small || picturedFaces[0]?.image_normal;
  const previewImage = card.image_normal || card.image_small || picturedFaces[0]?.image_normal || picturedFaces[0]?.image_small;
  const faceSwitch = picturedFaces.length > 1 ? `<div class="card-faces">${picturedFaces.map((face, index) => `<button type="button" data-face="${index}" class="${index === 0 ? 'active' : ''}" aria-pressed="${index === 0}">${index === 0 ? '正面' : '反面'}</button>`).join('')}</div>` : '';
  const faceData = picturedFaces.length > 1 ? ` data-faces="${escapeHTML(JSON.stringify(picturedFaces))}"` : '';
  const editorControls = `<div class="card-edit-controls">
    <button type="button" data-card-subtract="${escapeHTML(card.name || '')}" aria-label="减少 ${escapeHTML(card.name || '')}"><span aria-hidden="true">−</span></button>
    <button type="button" data-card-add="${escapeHTML(card.name || '')}" aria-label="增加 ${escapeHTML(card.name || '')}"><span aria-hidden="true">+</span></button>
  </div>`;
  return `<details class="mtg-card"${faceData} data-preview-src="${escapeHTML(previewImage || '')}" data-preview-name="${escapeHTML(card.name || '')}">
    <summary aria-describedby="card-preview">${image ? `<img loading="lazy" src="${escapeHTML(image)}" alt="${escapeHTML(card.name)}">` : '<div class="card-placeholder"></div>'}<span class="card-quantity">${item.quantity || 1}×</span><strong>${escapeHTML(card.name || 'Unknown card')}</strong></summary>
    ${faceSwitch}
    <div class="card-details">${card.printed_name ? `<small>${escapeHTML(card.printed_name)}</small>` : ''}<span>${escapeHTML(card.mana_cost || '')}</span><em>${escapeHTML(card.type_line || '')}</em><p>${escapeHTML(card.oracle_text || 'No Oracle text available.')}</p></div>
    ${editorControls}
  </details>`;
}

swapSearch.addEventListener('input', () => renderSwapRemoveList(swapSearch.value));
swapSubmit.addEventListener('click', compareSwap);
document.addEventListener('keydown', (event) => {
  if (event.key === 'Escape' && !swapModal.hidden) closeSwap();
  if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'z') {
    if (event.shiftKey) {
      event.preventDefault();
      redoEdit();
    } else {
      event.preventDefault();
      undoEdit();
    }
  }
});

document.addEventListener('click', (event) => {
  const subtractButton = event.target.closest('[data-card-subtract]');
  if (subtractButton) {
    const name = subtractButton.dataset.cardSubtract || '';
    const item = editorCards.find((entry) => String(entry.card?.name || '').toLowerCase() === name.toLowerCase());
    if (item?.commander) return;
    applyEdit((cards) => removeCardFromEditor(name));
    return;
  }
  const addButton = event.target.closest('[data-card-add]');
  if (addButton) {
    const name = addButton.dataset.cardAdd || '';
    if (addButton.closest('.deck-group') && isCommanderByName(name)) return;
    applyEdit((cards) => addCardToEditor(name));
    return;
  }
  const loadVersionButton = event.target.closest('[data-load-version]');
  if (loadVersionButton) {
    loadVersion(Number(loadVersionButton.dataset.loadVersion));
    return;
  }
  const deleteVersionButton = event.target.closest('[data-delete-version]');
  if (deleteVersionButton) {
    deleteVersion(Number(deleteVersionButton.dataset.deleteVersion));
    return;
  }
  const swapStart = event.target.closest('.swap-start');
  if (swapStart) {
    openSwap(swapStart.dataset.addName || '');
    return;
  }
  const removeOption = event.target.closest('.swap-remove-option');
  if (removeOption) {
    selectedSwapRemove = removeOption.dataset.removeName || '';
    swapSubmit.disabled = !selectedSwapRemove;
    renderSwapRemoveList(swapSearch.value);
    return;
  }
  if (event.target.closest('[data-swap-close]') || event.target.closest('#swap-close')) {
    closeSwap();
    return;
  }
  const button = event.target.closest('.card-faces button');
  if (!button) return;
  event.preventDefault();
  const cardElement = button.closest('.mtg-card');
  let faces;
  try { faces = JSON.parse(cardElement.dataset.faces || '[]'); } catch { return; }
  const face = faces[Number(button.dataset.face)];
  if (!face) return;
  const imageElement = cardElement.querySelector('img');
  imageElement?.setAttribute('src', face.image_small || face.image_normal || '');
  imageElement?.setAttribute('alt', face.name || '');
  cardElement.dataset.previewSrc = face.image_normal || face.image_small || '';
  cardElement.dataset.previewName = face.name || '';
  cardElement.querySelector('summary strong').textContent = face.name || '';
  const details = cardElement.querySelector('.card-details');
  details.querySelector('span').textContent = face.mana_cost || '';
  details.querySelector('em').textContent = face.type_line || '';
  details.querySelector('p').textContent = face.oracle_text || 'No Oracle text available.';
  cardElement.querySelectorAll('.card-faces button').forEach((item) => {
    const active = item === button;
    item.classList.toggle('active', active);
    item.setAttribute('aria-pressed', String(active));
  });
  if (activePreviewCard === cardElement) showCardPreview(cardElement.querySelector('summary'));
});

const preview = document.querySelector('#card-preview');
const previewImage = document.querySelector('#card-preview-image');
const previewName = document.querySelector('#card-preview-name');
const canHover = window.matchMedia('(hover: hover) and (pointer: fine)');
let activePreviewCard = null;

function showCardPreview(trigger) {
  if (!canHover.matches || !trigger) return;
  const card = trigger.closest('[data-preview-src]');
  const source = card?.dataset.previewSrc;
  if (!source) return;
  activePreviewCard = card;
  previewImage.src = source;
  previewImage.alt = card.dataset.previewName || '';
  previewName.textContent = card.dataset.previewName || '';
  preview.hidden = false;
  requestAnimationFrame(() => positionCardPreview(trigger));
}

function positionCardPreview(trigger) {
  if (preview.hidden) return;
  const gap = 12;
  const anchor = trigger.getBoundingClientRect();
  const box = preview.getBoundingClientRect();
  let left = anchor.right + gap;
  if (left + box.width > window.innerWidth - gap) left = anchor.left - box.width - gap;
  left = Math.max(gap, Math.min(left, window.innerWidth - box.width - gap));
  const top = Math.max(gap, Math.min(anchor.top, window.innerHeight - box.height - gap));
  preview.style.left = `${left}px`;
  preview.style.top = `${top}px`;
}

function hideCardPreview() {
  activePreviewCard = null;
  preview.hidden = true;
  previewImage.removeAttribute('src');
}

document.addEventListener('pointerover', (event) => {
  const trigger = event.target.closest('.mtg-card summary, .builder-candidate');
  if (trigger && !trigger.contains(event.relatedTarget)) showCardPreview(trigger);
});
document.addEventListener('pointerout', (event) => {
  const trigger = event.target.closest('.mtg-card summary, .builder-candidate');
  if (trigger && !trigger.contains(event.relatedTarget)) hideCardPreview();
});
document.addEventListener('focusin', (event) => { if (event.target.matches('.mtg-card summary, .builder-candidate')) showCardPreview(event.target); });
document.addEventListener('focusout', (event) => { if (event.target.matches('.mtg-card summary, .builder-candidate')) hideCardPreview(); });
document.addEventListener('keydown', (event) => { if (event.key === 'Escape') hideCardPreview(); });
window.addEventListener('scroll', hideCardPreview, { passive: true });
window.addEventListener('resize', hideCardPreview);

function renderProvider(prefix, provider, secondaryMetrics) {
  const status = document.querySelector(`#${prefix}-status`);
  const content = document.querySelector(`#${prefix}-content`);
  if (!provider || provider.status !== 'success') {
    status.textContent = '失败';
    status.className = 'status error';
    content.innerHTML = `<p class="provider-error">${escapeHTML(provider?.error?.message || '该评分网站暂时无法返回结果。')}</p>`;
    return;
  }

  status.textContent = '完成';
  status.className = 'status success';
  const metrics = provider.metrics || {};
  const power = formatNumber(metrics.power_level, 2);
  const list = secondaryMetrics
    .filter(([key]) => metrics[key] !== undefined)
    .map(([key, label]) => `<div class="metric"><small>${label}</small><strong>${formatMetric(key, metrics[key])}</strong></div>`)
    .join('');
  const brackets = renderBrackets(metrics);
  const suggestions = prefix === 'salt'
    ? renderSuggestions(metrics.suggestions)
    : renderEDHBracketDetails(metrics.bracket_details);
  content.innerHTML = `
    <div class="hero-metric"><strong>${power}</strong><span>/ 10 Power Level</span></div>
    ${brackets}
    <div class="metric-list">${list}</div>
    ${suggestions}`;
}

function renderEDHBracketDetails(details) {
  if (!details || typeof details !== 'object') return '';
  const reasons = Array.isArray(details.rules_bracket_reasons)
    ? details.rules_bracket_reasons.map((reason) => String(reason ?? '').trim()).filter(Boolean)
    : [];
  const evaluatedReason = String(details.evaluated_bracket_reason ?? '')
    .replace(/(Recommended Bracket:\s*\d+)/gi, '$1\n')
    .replace(/(Minimum Bracket:\s*\d+)/gi, '$1\n')
    .replace(/\.\s+(?=[A-Z])/g, '.\n')
    .trim();
  const counters = [
    ['Game Changers', Number(details.game_changers) || 0, details.game_changer_names || []],
    ['Early 2-Card Combos', Number(details.early_2_card_combos) || 0, details.early_2_card_combo_names || []],
    ['Extra Turns', Number(details.extra_turns) || 0, details.extra_turn_names || []],
    ['Mass Land Denial', Number(details.mass_land_denial) || 0, details.mass_land_denial_names || []]
  ];
  const counterChips = counters.map(([label, value]) => `<span class="bracket-counter"><strong>${value}</strong><small>${escapeHTML(label)}</small></span>`).join('');
  const enumerations = counters
    .filter(([, , names]) => Array.isArray(names) && names.length)
    .map(([label, , names]) => {
      const list = names.map((name) => `<li>${escapeHTML(name)}</li>`).join('');
      return `<div class="bracket-enum"><span>${escapeHTML(label)}</span><ul>${list}</ul></div>`;
    }).join('');
  if (!reasons.length && !evaluatedReason && !counterChips && !enumerations) return '';
  const rows = reasons.map((reason) => {
    const [title, description] = splitEDHReason(reason);
    return `
      <article class="suggestion-item">
        <div class="suggestion-item-title"><strong>${escapeHTML(title)}</strong></div>
        ${description ? `<p>${escapeHTML(description)}</p>` : ''}
      </article>`;
  }).join('');
  return `
    <section class="suggestions edh-bracket-details">
      <div class="suggestions-title"><span>BRACKET DETAILS</span><strong>基础 Bracket 判定依据</strong></div>
      ${counterChips ? `<div class="bracket-counters">${counterChips}</div>` : ''}
      ${enumerations ? `<div class="bracket-enumerations">${enumerations}</div>` : ''}
      ${rows ? `<div class="suggestion-list">${rows}</div>` : ''}
      ${evaluatedReason ? `<p class="suggestions-summary">${escapeHTML(evaluatedReason)}</p>` : ''}
    </section>`;
}

function splitEDHReason(reason) {
  const normalized = String(reason ?? '').replace(/\s+/g, ' ').trim();
  const separator = normalized.indexOf(' - ');
  if (separator < 0) return [normalized, ''];
  return [normalized.slice(0, separator), normalized.slice(separator + 3)];
}

function renderSuggestions(suggestions) {
  if (!suggestions || typeof suggestions !== 'object') return '';
  const groups = [
    ['rule_zero', '对局前说明', '在开始游戏前值得与牌桌沟通'],
    ['rationale', '当前档位原因', 'CommanderSalt 对当前强度判断的依据'],
    ['soften', '降低强度建议', '让牌组更适合较低强度环境'],
    ['harden', '提高强度建议', '进一步提升速度、稳定性或威胁']
  ];
  const renderedGroups = groups.map(([key, title, description]) => {
    const items = Array.isArray(suggestions[key]) ? suggestions[key] : [];
    if (!items.length) return '';
    const rows = items.map(renderSuggestionItem).filter(Boolean).join('');
    if (!rows) return '';
    return `
      <section class="suggestion-group suggestion-${key.replace('_', '-')}">
        <div class="suggestion-heading"><strong>${title}</strong><small>${description}</small></div>
        <div class="suggestion-list">${rows}</div>
      </section>`;
  }).filter(Boolean).join('');
  const summary = String(suggestions.summary ?? '').trim();
  if (!renderedGroups && !summary) return '';
  return `
    <section class="suggestions">
      <div class="suggestions-title"><span>DECK SUGGESTIONS</span><strong>CommanderSalt 建议</strong></div>
      ${summary ? `<p class="suggestions-summary">${escapeHTML(summary)}</p>` : ''}
      ${renderedGroups}
    </section>`;
}

function renderSuggestionItem(item) {
  if (!item || typeof item !== 'object') return '';
  const title = String(item.label || humanizeID(item.id) || '').trim();
  const why = String(item.why ?? '').trim();
  if (!title && !why) return '';
  const sentiment = ['caution', 'warning'].includes(String(item.sentiment).toLowerCase()) ? ' caution' : '';
  const direction = item.direction === 'up' ? '↑' : item.direction === 'down' ? '↓' : '';
  return `
    <article class="suggestion-item${sentiment}">
      <div class="suggestion-item-title"><strong>${escapeHTML(title || '建议')}</strong>${direction ? `<span>${direction}</span>` : ''}</div>
      ${why ? `<p>${escapeHTML(why)}</p>` : ''}
    </article>`;
}

function humanizeID(value) {
  return String(value ?? '')
    .replace(/[_-]+/g, ' ')
    .replace(/([a-z0-9])([A-Z])/g, '$1 $2')
    .replace(/^./, (char) => char.toUpperCase());
}

function renderBrackets(metrics) {
  const rules = metrics.rules_bracket;
  const evaluated = metrics.evaluated_bracket;
  if (rules === undefined && evaluated === undefined) return '';
  return `
    <div class="bracket-pair">
      <div class="bracket-value">
        <span class="bracket-number">${formatNumber(rules, 0)}</span>
        <div><strong>规则 Bracket</strong><small>按官方卡牌限制得到的最低档位</small></div>
      </div>
      <div class="bracket-value evaluated">
        <span class="bracket-number">${formatNumber(evaluated, 0)}</span>
        <div><strong>评估 Bracket</strong><small>结合整副牌强度后的建议档位</small></div>
      </div>
    </div>`;
}

function formatMetric(key, value) {
  if (key === 'average_playability') return `${formatNumber(value, 1)}%`;
  return formatNumber(value, key === 'salt' || key === 'impact' ? 1 : 2);
}

function formatNumber(value, digits) {
  const number = Number(value);
  if (!Number.isFinite(number)) return escapeHTML(String(value ?? '—'));
  return number.toLocaleString('zh-CN', { maximumFractionDigits: digits });
}

function escapeHTML(value) {
  return String(value ?? '').replace(/[&<>'"]/g, (char) => ({
    '&': '&amp;', '<': '&lt;', '>': '&gt;', "'": '&#39;', '"': '&quot;'
  })[char]);
}

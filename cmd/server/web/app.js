const form = document.querySelector('#analyze-form');
const input = document.querySelector('#deck-url');
const submitButton = document.querySelector('#submit-button');
const message = document.querySelector('#form-message');
const loading = document.querySelector('#loading');
const results = document.querySelector('#results');
const warning = document.querySelector('#warning');
const retryButton = document.querySelector('#retry-button');
const decklistToggle = document.querySelector('#decklist-toggle');

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
  message.textContent = '';
  if (!isMoxfieldDeckURL(url)) {
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
      body: JSON.stringify({ url })
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
  renderCombos(payload.combos || []);
  renderRecommendations(payload.recommendations || [], payload.recommendation_keywords || []);
  renderDeckCards(payload.deck_cards || []);
  results.hidden = false;
  results.scrollIntoView({ behavior: 'smooth', block: 'start' });
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
      <div class="recommendation-row">${(group.cards || []).map((item) => `
        <article class="recommendation-card">
          ${renderCard({ card: item.card, quantity: 1 })}
          <div class="recommendation-meta">
            <div><span>Synergy</span><strong>${(Number(item.synergy || 0) * 100).toFixed(1)}%</strong></div>
            <div><span>Inclusion</span><strong>${(Number(item.inclusion_rate || 0) * 100).toFixed(1)}%</strong></div>
          </div>
          <p>${escapeHTML(item.reason || '')}</p>
          <a href="${escapeHTML(item.source_url || 'https://edhrec.com/')}" target="_blank" rel="noopener noreferrer">在 EDHREC 查看 ↗</a>
        </article>`).join('')}</div>
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

function renderCard(item) {
  const card = item.card || {};
  const picturedFaces = Array.isArray(card.faces) ? card.faces.filter((face) => face.image_small || face.image_normal) : [];
  const image = card.image_small || card.image_normal || picturedFaces[0]?.image_small || picturedFaces[0]?.image_normal;
  const previewImage = card.image_normal || card.image_small || picturedFaces[0]?.image_normal || picturedFaces[0]?.image_small;
  const faceSwitch = picturedFaces.length > 1 ? `<div class="card-faces">${picturedFaces.map((face, index) => `<button type="button" data-face="${index}" class="${index === 0 ? 'active' : ''}" aria-pressed="${index === 0}">${index === 0 ? '正面' : '反面'}</button>`).join('')}</div>` : '';
  const faceData = picturedFaces.length > 1 ? ` data-faces="${escapeHTML(JSON.stringify(picturedFaces))}"` : '';
  return `<details class="mtg-card"${faceData} data-preview-src="${escapeHTML(previewImage || '')}" data-preview-name="${escapeHTML(card.name || '')}">
    <summary aria-describedby="card-preview">${image ? `<img loading="lazy" src="${escapeHTML(image)}" alt="${escapeHTML(card.name)}">` : '<div class="card-placeholder"></div>'}<span class="card-quantity">${item.quantity || 1}×</span><strong>${escapeHTML(card.name || 'Unknown card')}</strong></summary>
    ${faceSwitch}
    <div class="card-details">${card.printed_name ? `<small>${escapeHTML(card.printed_name)}</small>` : ''}<span>${escapeHTML(card.mana_cost || '')}</span><em>${escapeHTML(card.type_line || '')}</em><p>${escapeHTML(card.oracle_text || 'No Oracle text available.')}</p></div>
  </details>`;
}

document.addEventListener('click', (event) => {
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

function showCardPreview(summary) {
  if (!canHover.matches || !summary) return;
  const card = summary.closest('.mtg-card');
  const source = card?.dataset.previewSrc;
  if (!source) return;
  activePreviewCard = card;
  previewImage.src = source;
  previewImage.alt = card.dataset.previewName || '';
  previewName.textContent = card.dataset.previewName || '';
  preview.hidden = false;
  requestAnimationFrame(() => positionCardPreview(summary));
}

function positionCardPreview(summary) {
  if (preview.hidden) return;
  const gap = 12;
  const anchor = summary.getBoundingClientRect();
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
  const summary = event.target.closest('.mtg-card summary');
  if (summary && !summary.contains(event.relatedTarget)) showCardPreview(summary);
});
document.addEventListener('pointerout', (event) => {
  const summary = event.target.closest('.mtg-card summary');
  if (summary && !summary.contains(event.relatedTarget)) hideCardPreview();
});
document.addEventListener('focusin', (event) => { if (event.target.matches('.mtg-card summary')) showCardPreview(event.target); });
document.addEventListener('focusout', (event) => { if (event.target.matches('.mtg-card summary')) hideCardPreview(); });
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
  if (!reasons.length && !evaluatedReason) return '';
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

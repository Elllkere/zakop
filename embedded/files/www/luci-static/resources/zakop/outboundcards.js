'use strict';
'require baseclass';
'require zakop.i18n as zakopI18n';

var _ = zakopI18n.translate;

// Keep LuCI's own form nodes and event handlers. Only the outbound section is
// presented as cards; pools, subscriptions and edit modals retain their layout.
var cardStyle = `
.zakop-outbound-cards { --zakop-card-border:rgba(127,127,127,.25); }
.zakop-outbound-cards .zakop-outbound-toolbar { display:flex; flex-wrap:wrap; align-items:center; gap:10px; margin:0 0 12px; }
.zakop-outbound-cards .zakop-outbound-search { flex:1; width:100%; max-width:420px; min-width:0; margin:0; }
.zakop-outbound-cards .zakop-outbound-sort { width:auto; min-width:0; max-width:100%; margin:0; }
.zakop-outbound-cards .zakop-outbound-count { margin-left:auto; font-size:12px; opacity:.65; white-space:nowrap; font-variant-numeric:tabular-nums; }
.zakop-outbound-cards .cbi-section-table { display:block; width:100%; border:0; margin:0; background:none; overflow:visible; }
.zakop-outbound-cards .cbi-section-table > thead,
.zakop-outbound-cards .cbi-section-table > .thead { display:none; }
.zakop-outbound-cards .zakop-outbound-grid { display:grid; grid-template-columns:repeat(auto-fill,minmax(min(100%,235px),1fr)); gap:10px; width:100%; }
.zakop-outbound-cards .zakop-outbound-card {
 display:grid!important; grid-template-columns:minmax(0,1fr) auto;
 grid-template-areas:'name name' 'type latency' 'server server' 'actions actions';
 align-content:start; align-items:center; gap:5px 10px; min-width:0; width:auto!important;
 margin:0; padding:10px!important; border:1px solid var(--zakop-card-border)!important;
 border-radius:10px; background:rgba(127,127,127,.035)!important; color:inherit!important;
 box-shadow:0 1px 2px rgba(0,0,0,.035); transition:border-color .12s,box-shadow .12s;
}
.zakop-outbound-cards .zakop-outbound-card::before,
.zakop-outbound-cards .zakop-outbound-card::after { display:none!important; }
.zakop-outbound-cards .zakop-outbound-grid > .cbi-section-table-titles,
.zakop-outbound-cards .zakop-outbound-grid > .cbi-section-table-descr { display:none!important; }
.zakop-outbound-cards .zakop-outbound-card:hover { border-color:rgba(70,125,215,.6)!important; box-shadow:0 2px 8px rgba(0,0,0,.07); }
.zakop-outbound-cards .zakop-outbound-card:focus-within { outline:2px solid rgba(70,125,215,.7); outline-offset:2px; }
.zakop-outbound-cards .zakop-outbound-card[hidden] { display:none!important; }
.zakop-outbound-cards .zakop-outbound-card > .td { display:block!important; width:auto!important; min-width:0!important; max-width:none; margin:0; padding:0!important; border:0!important; background:none!important; color:inherit; text-align:left; }
.zakop-outbound-cards .zakop-outbound-card > .td::before { display:none!important; }
.zakop-outbound-cards .zakop-outbound-card > .zakop-outbound-name { grid-area:name; font-size:14px; font-weight:600; line-height:1.45; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
.zakop-outbound-cards .zakop-outbound-card > [data-name='type'] { grid-area:type; justify-self:start; padding:2px 7px!important; border-radius:5px; font-size:10px; font-weight:600; line-height:1.5; letter-spacing:.035em; text-transform:uppercase; background:rgba(70,125,215,.1)!important; color:#487ccc; }
.zakop-outbound-cards .zakop-outbound-card[data-protocol='hysteria2'] > [data-name='type'] { background:rgba(144,100,205,.12)!important; color:#9b77cf; }
.zakop-outbound-cards .zakop-outbound-card[data-protocol='trojan'] > [data-name='type'] { background:rgba(26,155,133,.12)!important; color:#299d8b; }
.zakop-outbound-cards .zakop-outbound-card[data-protocol='shadowsocks'] > [data-name='type'] { background:rgba(199,135,44,.12)!important; color:#b58435; }
.zakop-outbound-cards .zakop-outbound-card > [data-name='_latency'] { grid-area:latency; text-align:right; font-size:11px; font-variant-numeric:tabular-nums; }
.zakop-outbound-cards [data-zakop-latency-tag] { font-size:11px!important; font-weight:600; white-space:nowrap; }
.zakop-outbound-cards .zakop-outbound-card > [data-name='server'] { grid-area:server; font-size:12px; line-height:1.5; opacity:.7; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
.zakop-outbound-cards .zakop-outbound-card > [data-name='port'] { display:none!important; }
.zakop-outbound-cards .zakop-outbound-card > .cbi-section-actions { grid-area:actions; padding-top:8px!important; border-top:1px solid var(--zakop-card-border)!important; white-space:normal; }
.zakop-outbound-cards .cbi-section-actions > div { display:flex; align-items:center; gap:5px; width:100%!important; min-width:0!important; margin:0; }
.zakop-outbound-cards .cbi-section-actions .cbi-button { flex:1; min-width:0; width:auto; height:auto; margin:0!important; padding:4px 6px; font-size:11px; line-height:1.5; white-space:nowrap; border-radius:5px; }
.zakop-outbound-cards .zakop-outbound-grid > .placeholder { display:block; grid-column:1/-1; padding:20px; text-align:center; border:1px dashed var(--zakop-card-border); border-radius:10px; }
.zakop-outbound-cards .zakop-outbound-grid > .placeholder > .td { display:block; border:0; }
.zakop-outbound-cards .zakop-outbound-no-matches { margin:12px 0; padding:20px; text-align:center; opacity:.7; }
.zakop-outbound-cards .zakop-outbound-no-matches[hidden] { display:none; }
.zakop-outbound-cards .cbi-section-create { display:flex; flex-wrap:wrap; align-items:center; gap:8px; margin-top:14px; }
.zakop-outbound-cards .cbi-section-create .cbi-button { margin:0!important; }
@media (prefers-reduced-motion:reduce) { .zakop-outbound-cards .zakop-outbound-card { transition:none; } }
`;

function render(section, state) {
	var table = section.querySelector('.cbi-section-table');
	if (!table)
		return section;

	section.classList.add('zakop-outbound-cards');
	section.appendChild(E('style', {}, cardStyle));
	table.setAttribute('role', 'presentation');
	var grid = table.querySelector('tbody') || table;
	grid.classList.add('zakop-outbound-grid');
	grid.setAttribute('role', 'list');
	grid.setAttribute('aria-label', _('Outbounds'));
	var cards = Array.prototype.slice.call(grid.querySelectorAll('.cbi-section-table-row[data-sid]'));
	cards.forEach(function(card, index) {
		card.setAttribute('data-sort-index', String(index));
		card.classList.add('zakop-outbound-card');
		card.setAttribute('role', 'listitem');
		var name = card.firstElementChild;
		// LuCI 23.05 stores the section title on the row instead of in a cell.
		if (!name || !name.classList.contains('cbi-section-table-titles')) {
			name = E('td', { 'class': 'td cbi-section-table-titles' },
				card.getAttribute('data-title') || card.getAttribute('data-sid'));
			card.insertBefore(name, card.firstChild);
		}
		name.classList.add('zakop-outbound-name');
		name.title = name.textContent.trim();
		// Decorative flag/emoji prefixes should not determine alphabetic order.
		card.setAttribute('data-sort-name', name.title.replace(/^[^\p{L}\p{N}]+/u, '').trim());
		card.setAttribute('aria-label', name.title);
		var server = card.querySelector('[data-name="server"]');
		var port = card.querySelector('[data-name="port"]');
		if (server && port) {
			// Both are read-only previews; their UCI options remain in the form.
			server.textContent = server.textContent.trim() + ':' + port.textContent.trim();
			server.title = server.textContent;
		}
		var protocol = card.querySelector('[data-name="type"]');
		card.setAttribute('data-protocol', protocol ? protocol.textContent.trim().toLowerCase() : '');
		card.setAttribute('data-search', (name.title + ' ' + (server ? server.textContent : '') + ' ' +
			(protocol ? protocol.textContent : '') + ' ' + card.getAttribute('data-sid')).toLowerCase());
		Array.prototype.forEach.call(card.children, function(cell) { cell.setAttribute('role', 'presentation'); });
	});
	var collator = new Intl.Collator(undefined, { sensitivity: 'base', numeric: true, ignorePunctuation: true });
	function byName(a, b) {
		return collator.compare(a.getAttribute('data-sort-name'), b.getAttribute('data-sort-name')) ||
			Number(a.getAttribute('data-sort-index')) - Number(b.getAttribute('data-sort-index'));
	}
	function delay(card) {
		var indicator = card.querySelector('[data-zakop-latency-tag]');
		var result = indicator && state.outboundLatencyResults && state.outboundLatencyResults[indicator.getAttribute('data-zakop-latency-tag')];
		return result && result.ok && Number(result.latency_ms) > 0 ? Number(result.latency_ms) : Infinity;
	}
	function sort() {
		if (state.outboundSort == 'latency') {
			// Selecting delay before any successful measurement leaves order intact.
			if (!cards.some(function(card) { return delay(card) < Infinity; }))
				return;
			cards.sort(function(a, b) {
				var left = delay(a), right = delay(b);
				return left == right ? byName(a, b) : (left < right ? -1 : 1);
			});
		} else if (state.outboundSort == 'name') {
			cards.sort(byName);
		} else {
			cards.sort(function(a, b) { return Number(a.getAttribute('data-sort-index')) - Number(b.getAttribute('data-sort-index')); });
		}
		cards.forEach(function(card) { grid.appendChild(card); });
	}
	if (state.outboundSort != 'name' && state.outboundSort != 'latency')
		state.outboundSort = 'uci';
	state.sortOutboundCards = sort;
	// Preserve the last non-delay order across redraws if no delay is known.
	if (state.outboundSort == 'latency' && state.outboundSortFallback == 'name')
		cards.sort(byName).forEach(function(card) { grid.appendChild(card); });
	var sorting = E('select', {
		'class': 'cbi-input-select zakop-outbound-sort', 'aria-label': _('Sort outbounds'),
		'change': function(ev) {
			if (ev.target.value == 'latency' && state.outboundSort != 'latency')
				state.outboundSortFallback = state.outboundSort;
			state.outboundSort = ev.target.value;
			sort();
		}
	}, [ E('option', { 'value': 'uci' }, _('Default order')), E('option', { 'value': 'name' }, _('By name')), E('option', { 'value': 'latency' }, _('By latency')) ]);
	sorting.value = state.outboundSort;
	sort();
	var count = E('span', { 'class': 'zakop-outbound-count', 'aria-live': 'polite' });
	var empty = E('p', { 'class': 'zakop-outbound-no-matches', 'hidden': true }, _('No matching outbounds'));
	function filter(value) {
		state.outboundSearch = value;
		var words = value.trim().toLowerCase().split(/\s+/).filter(Boolean);
		var visible = 0;
		cards.forEach(function(card) {
			var haystack = card.getAttribute('data-search');
			card.hidden = !words.every(function(word) { return haystack.indexOf(word) >= 0; });
			if (!card.hidden) visible++;
		});
		count.textContent = String(visible) + ' / ' + String(cards.length);
		empty.hidden = visible > 0 || cards.length == 0;
	}
	var search = E('input', {
		'type': 'search', 'class': 'cbi-input-text zakop-outbound-search',
		'placeholder': _('Search by name, address or protocol…'),
		'aria-label': _('Search outbounds'), 'value': state.outboundSearch || '',
		'input': function(ev) { filter(ev.target.value); }
	});
	section.insertBefore(E('div', { 'class': 'zakop-outbound-toolbar' }, [ search, sorting, count ]), table);
	section.insertBefore(empty, table.nextSibling);
	filter(state.outboundSearch || '');
	return section;
}

return baseclass.extend({ render: render });

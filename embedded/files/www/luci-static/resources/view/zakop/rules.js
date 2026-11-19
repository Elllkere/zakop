'use strict';
'require form';
'require ui';
'require uci';
'require view';
'require zakop.i18n as zakopI18n';
'require zakop.ui as zakopUI';
'require zakop.ruleform as ruleForm';
'require zakop.rulebackup as ruleBackup';

var _ = zakopI18n.translate;
function ruleSectionIDs() {
	var ids = [];

	uci.sections('zakop', 'rule', function(section, sid) {
		ids.push(sid);
	});

	return ids;
}

var rulePriority = ruleBackup.rulePriority;
var sortRuleSectionIDs = ruleBackup.sortRuleSectionIDs;

function nextRulePriority() {
	var ids = ruleSectionIDs();
	var max = 0;

	for (var i = 0; i < ids.length; i++) {
		var priority = rulePriority(ids[i], 1000 + i);

		if (priority > max)
			max = priority;
	}

	return max + 100;
}

function initializeNewRuleSection(section_id, priority, outbound) {
	uci.set('zakop', section_id, 'enabled', '1');
	uci.set('zakop', section_id, 'action', 'proxy');
	uci.set('zakop', section_id, 'outbound', outbound);
	uci.set('zakop', section_id, 'dns_mode', 'auto');
	uci.set('zakop', section_id, 'priority', String(priority));
	uci.set('zakop', section_id, 'domain_input', 'fields');
	uci.set('zakop', section_id, 'ip_input', 'list');
}

function renderedRuleSectionIDs(ids) {
	var wanted = {};
	var seen = {};
	var out = [];
	var rows, sid;

	if (typeof document == 'undefined')
		return null;

	for (var i = 0; i < ids.length; i++)
		wanted[ids[i]] = true;

	rows = document.querySelectorAll('#cbi-zakop-rule tr.cbi-section-table-row[data-sid]');
	for (var j = 0; j < rows.length; j++) {
		sid = rows[j].getAttribute('data-sid');

		if (!wanted[sid] || seen[sid])
			continue;

		seen[sid] = true;
		out.push(sid);
	}

	return out.length == ids.length ? out : null;
}

function orderedRuleSectionIDs() {
	var ids = ruleSectionIDs();
	var rendered = renderedRuleSectionIDs(ids);

	return rendered || sortRuleSectionIDs(ids);
}

function rewriteRuleState() {
	var n = 0;
	var firstOutbound = ruleForm.firstOutboundTag();

	if (String(uci.get('zakop', 'main', 'routing_mode') || 'custom').trim() != 'custom')
		return;

	var ids = orderedRuleSectionIDs();
	for (var i = 0; i < ids.length; i++) {
		var sid = ids[i];
		n++;
		uci.set('zakop', sid, 'priority', String(n * 100));
		ruleForm.rewriteRule(sid, firstOutbound);
	}
}

return view.extend({
	load: function() {
		return uci.load('zakop').then(function() {
			zakopUI.syncRulesTab();
		});
	},

	showExportRules: function() {
		return ruleBackup.showExportRules(orderedRuleSectionIDs());
	},

	showImportRules: function() {
		return ruleBackup.showImportRules(this.map, rewriteRuleState);
	},

	handleSave: function() {
		return this.map.save(rewriteRuleState).then(function() {
			return ui.changes.init();
		});
	},

	handleSaveApply: function(ev) {
		return this.handleSave(ev)
			.then(function() {
				return zakopUI.applyAndRestart();
			});
	},

	render: function() {
		var m, s, routingMode, self, firstOutbound;

		zakopUI.syncRulesTab();

		m = new form.Map('zakop', _('zakop'));
		this.map = m;
		self = this;
		routingMode = String(uci.get('zakop', 'main', 'routing_mode') || 'custom').trim();
		firstOutbound = ruleForm.firstOutboundTag();

		if (routingMode != 'custom')
			return m.render();

		s = m.section(form.GridSection, 'rule', _('Rules'),
			_('These are literal string operations, not DNS-aware matching.') + ' ' +
			_('For root + subdomains, use Equals: example.com and Ends with: .example.com'));
		s.anonymous = true;
		s.addremove = true;
		s.sortable = true;
		s.renderHeaderRows = function() {
			var sortable = this.sortable;

			try {
				// Keep row drag ordering, but do not let header clicks reorder rules by column values.
				this.sortable = false;
				return form.GridSection.prototype.renderHeaderRows.apply(this, arguments);
			} finally {
				this.sortable = sortable;
			}
		};
		s.cfgsections = function() {
			return sortRuleSectionIDs(form.GridSection.prototype.cfgsections.apply(this, arguments));
		};
		s.handleAdd = function(ev, name) {
			if (firstOutbound == '') {
				zakopUI.notify(E('p', {}, [ _('Create an outbound before adding a proxy rule.') ]), 'warning');
				return Promise.resolve();
			}
			var data = this.map.data;
			var add = data.add;
			var priority = nextRulePriority();

			data.add = function(configName, sectionType, sectionName) {
				var sid = add.apply(this, arguments);

				initializeNewRuleSection(sid, priority, firstOutbound);
				return sid;
			};

			try {
				return form.GridSection.prototype.handleAdd.apply(this, arguments);
			} finally {
				data.add = add;
			}
		};
		s.modaltitle = _('Rule details');
		s.renderSectionAdd = function() {
			var el = form.GridSection.prototype.renderSectionAdd.apply(this, arguments);

			el.appendChild(E('button', {
				'class': 'cbi-button cbi-button-action',
				'style': 'margin-left:.5em',
				'click': function(ev) {
					ev.preventDefault();
					self.showImportRules();
				}
			}, _('Import')));

			el.appendChild(E('button', {
				'class': 'cbi-button cbi-button-action',
				'style': 'margin-left:.5em',
				'click': function(ev) {
					ev.preventDefault();
					self.showExportRules();
				}
			}, _('Export')));

			return el;
		};

		ruleForm.addRuleOptions(s);
		ruleForm.addMatcherOptions(s);

		return m.render();
	}
});

'use strict';
'require fs';
'require form';
'require ui';
'require uci';
'require view';
'require zakop.i18n as zakopI18n';
'require zakop.ui as zakopUI';

var _ = zakopI18n.translate;
var ruleListOptions = [
	'domain_equals',
	'domain_contains',
	'domain_starts_with',
	'domain_ends_with',
	'exclude_domain_equals',
	'exclude_domain_contains',
	'exclude_domain_starts_with',
	'exclude_domain_ends_with',
	'domain_provider',
	'domain_file',
	'ip_cidr',
	'ip_provider',
	'ip_file',
	'file',
	'provider',
	'proto',
	'src_port',
	'dst_port'
];

function ruleSectionIDs() {
	var ids = [];

	uci.sections('zakop', 'rule', function(section, sid) {
		ids.push(sid);
	});

	return ids;
}

function rulePriority(section_id, fallback) {
	var value = String(uci.get('zakop', section_id, 'priority') || '').trim();

	if (/^-?[0-9]+$/.test(value))
		return parseInt(value, 10);

	return fallback;
}

function sortRuleSectionIDs(ids) {
	var order = {};

	for (var i = 0; i < ids.length; i++)
		order[ids[i]] = i;

	ids.sort(function(a, b) {
		var pa = rulePriority(a, 1000 + order[a]);
		var pb = rulePriority(b, 1000 + order[b]);

		if (pa != pb)
			return pa - pb;

		return order[a] - order[b];
	});

	return ids;
}

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

function firstOutboundTag() {
	var first = '';

	uci.sections('zakop', 'outbound', function(section, sid) {
		var tag = String(section.tag || sid || section['.name'] || '').trim();

		if (first == '' && tag != '' && tag != 'direct' && tag != 'blocked' && tag != 'block' && tag != 'proxy_default')
			first = tag;
	});

	return first;
}

function outboundTagExists(wanted) {
	var found = false;

	wanted = String(wanted || '').trim();
	if (wanted == '')
		return false;

	uci.sections('zakop', 'outbound', function(section, sid) {
		var tag = String(section.tag || sid || section['.name'] || '').trim();

		if (tag == wanted && tag != 'direct' && tag != 'blocked' && tag != 'block' && tag != 'proxy_default')
			found = true;
	});
	uci.sections('zakop', 'outbound_pool', function(section, sid) {
		var tag = String(section.tag || sid || section['.name'] || '').trim();

		if (tag == wanted && tag != 'direct' && tag != 'blocked' && tag != 'block' && tag != 'proxy_default')
			found = true;
	});

	return found;
}

function rewriteRuleState() {
	var n = 0;
	var firstOutbound = firstOutboundTag();

	if (String(uci.get('zakop', 'main', 'routing_mode') || 'custom').trim() != 'custom')
		return;

	var ids = orderedRuleSectionIDs();
	for (var i = 0; i < ids.length; i++) {
		var sid = ids[i];
		var action = String(uci.get('zakop', sid, 'action') || 'proxy').trim();
		var outbound = uci.get('zakop', sid, 'outbound');
		var domainInput = String(uci.get('zakop', sid, 'domain_input') || '').trim();
		var ipInput = String(uci.get('zakop', sid, 'ip_input') || '').trim();
		var protoValues = optionValues(sid, 'proto');
		var hasTCP = protoValues.indexOf('tcp') >= 0;
		var hasUDP = protoValues.indexOf('udp') >= 0;

		n++;
		uci.set('zakop', sid, 'priority', String(n * 100));

		if (uci.get('zakop', sid, 'enabled') == null)
			uci.set('zakop', sid, 'enabled', '1');

		uci.set('zakop', sid, 'dns_mode', 'auto');

		if (hasTCP && !hasUDP)
			setListOption(sid, 'proto', [ 'tcp' ]);
		else if (hasUDP && !hasTCP)
			setListOption(sid, 'proto', [ 'udp' ]);
		else
			uci.unset('zakop', sid, 'proto');

		if (action != 'proxy') {
			uci.set('zakop', sid, 'outbound', 'direct');
		} else if (outbound == null || outbound == '' || outbound == 'direct' || outbound == 'blocked' || outbound == 'block' || outbound == 'proxy_default' || !outboundTagExists(outbound)) {
			if (firstOutbound != '')
				uci.set('zakop', sid, 'outbound', firstOutbound);
			else
				uci.unset('zakop', sid, 'outbound');
		}

		if (domainInput == '') {
			if (optionValues(sid, 'domain_provider').length > 0)
				domainInput = 'provider';
			else if (optionValues(sid, 'domain_file').length > 0)
				domainInput = 'file';
		}
		if (domainInput != 'text' && domainInput != 'provider' && domainInput != 'file')
			domainInput = 'fields';
		uci.set('zakop', sid, 'domain_input', domainInput);

		if (domainInput == 'provider') {
			unsetRuleOptions(sid, [
				'domain_equals', 'domain_contains', 'domain_starts_with', 'domain_ends_with',
				'exclude_domain_equals', 'exclude_domain_contains', 'exclude_domain_starts_with', 'exclude_domain_ends_with',
				'domain_file'
			]);
		} else if (domainInput == 'file') {
			unsetRuleOptions(sid, [
				'domain_equals', 'domain_contains', 'domain_starts_with', 'domain_ends_with',
				'exclude_domain_equals', 'exclude_domain_contains', 'exclude_domain_starts_with', 'exclude_domain_ends_with',
				'domain_provider'
			]);
		} else {
			uci.unset('zakop', sid, 'domain_provider');
			uci.unset('zakop', sid, 'domain_file');
		}

		if (ipInput == '') {
			if (optionValues(sid, 'ip_provider').length > 0)
				ipInput = 'provider';
			else if (optionValues(sid, 'ip_file').length > 0 || optionValues(sid, 'file').length > 0)
				ipInput = 'file';
			else
				ipInput = 'list';
		}
		if (ipInput != 'text' && ipInput != 'provider' && ipInput != 'file')
			ipInput = 'list';
		uci.set('zakop', sid, 'ip_input', ipInput);

		if (ipInput == 'provider') {
			unsetRuleOptions(sid, [ 'ip_cidr', 'ip_file', 'file' ]);
		} else if (ipInput == 'file') {
			unsetRuleOptions(sid, [ 'ip_cidr', 'ip_provider', 'file' ]);
		} else {
			uci.unset('zakop', sid, 'ip_provider');
			uci.unset('zakop', sid, 'ip_file');
			uci.unset('zakop', sid, 'file');
		}
	}
}

function addOutboundChoices(option) {
	var first = '';

	uci.sections('zakop', 'outbound', function(section, sid) {
		var tag = String(section.tag || sid || section['.name'] || '').trim();
		var label = String(section.label || section.name || tag).trim();

		if (tag == '' || tag == 'direct' || tag == 'blocked' || tag == 'block' || tag == 'proxy_default')
			return;

		if (first == '')
			first = tag;
		option.value(tag, label || tag);
	});
	uci.sections('zakop', 'outbound_pool', function(section, sid) {
		var tag = String(section.tag || sid || section['.name'] || '').trim();
		var label = String(section.label || section.name || tag).trim();

		if (tag != '' && tag != 'direct' && tag != 'blocked' && tag != 'block' && tag != 'proxy_default')
			option.value(tag, _('Pool: %s').format(label || tag));
	});

	option.default = first;
	return first;
}

function toArray(value) {
	if (Array.isArray(value))
		return value;
	if (value == null || value == '')
		return [];
	return [ value ];
}

function cleanValues(values) {
	var seen = {};
	var out = [];

	values = toArray(values);
	for (var i = 0; i < values.length; i++) {
		var value = String(values[i] || '').trim();

		if (value == '' || seen[value])
			continue;

		seen[value] = true;
		out.push(value);
	}

	return out;
}

function splitTextValues(value) {
	var values = [];
	var lines = String(value || '').replace(/\r/g, '\n').split('\n');

	for (var i = 0; i < lines.length; i++) {
		var line = lines[i];
		var comment = line.indexOf('#');

		if (comment >= 0)
			line = line.slice(0, comment);

		values.push(line);
	}

	return cleanValues(values);
}

function optionValues(section_id, option) {
	return cleanValues(uci.get('zakop', section_id, option));
}

function setListOption(section_id, option, values) {
	values = cleanValues(values);
	if (values.length > 0)
		uci.set('zakop', section_id, option, values);
	else
		uci.unset('zakop', section_id, option);
}

function unsetRuleOptions(section_id, options) {
	for (var i = 0; i < options.length; i++)
		uci.unset('zakop', section_id, options[i]);
}

function addDynamicList(section, option, title, modeOption, modeValue, placeholder) {
	var o = section.option(form.DynamicList, option, title);

	o.rmempty = true;
	o.modalonly = true;
	if (placeholder)
		o.placeholder = placeholder;
	o.depends(modeOption, modeValue);

	return o;
}

function addTextList(section, option, target, title, modeOption, modeValue, placeholder) {
	var o = section.option(form.TextValue, option, title);

	o.rows = 6;
	o.rmempty = true;
	o.modalonly = true;
	o.depends(modeOption, modeValue);
	if (placeholder)
		o.placeholder = placeholder;
	o.cfgvalue = function(section_id) {
		return optionValues(section_id, target).join('\n');
	};
	o.write = function(section_id, formvalue) {
		setListOption(section_id, target, splitTextValues(formvalue));
	};

	return o;
}

function addProviderChoices(option, type) {
	uci.sections('zakop', 'provider', function(section, sid) {
		var name = String(sid || section['.name'] || '').trim();
		var label = String(section.label || section.name || name).trim();
		var providerType = String(section.type || '').trim();

		if (name == '' || providerType != type)
			return;

		option.value(name, label || name);
	});
}

function addProviderList(section, option, title, providerType, modeOption, modeValue) {
	var o = section.option(form.DynamicList, option, title);

	o.rmempty = true;
	o.modalonly = true;
	o.depends(modeOption, modeValue);
	addProviderChoices(o, providerType);

	return o;
}

function packetProtoValue(section_id) {
	var values = optionValues(section_id, 'proto');
	var hasTCP = values.indexOf('tcp') >= 0;
	var hasUDP = values.indexOf('udp') >= 0;

	if (hasTCP && hasUDP)
		return 'any';
	if (hasTCP)
		return 'tcp';
	if (hasUDP)
		return 'udp';
	return 'any';
}

function writePacketProto(section_id, formvalue) {
	switch (formvalue) {
	case 'tcp':
		setListOption(section_id, 'proto', [ 'tcp' ]);
		break;
		case 'udp':
			setListOption(section_id, 'proto', [ 'udp' ]);
			break;
		default:
			uci.unset('zakop', section_id, 'proto');
	}
}

function validatePortMatch(section_id, value) {
	var values = Array.isArray(value) ? value : [ value ];

	for (var i = 0; i < values.length; i++) {
		var port = String(values[i] || '').trim();
		var parts, start, end;

		if (port == '')
			continue;

		if (!/^[0-9]+(-[0-9]+)?$/.test(port))
			return _('Port must be a number or range, for example 443 or 1000-2000');

		parts = port.split('-');
		start = parseInt(parts[0], 10);
		end = parts.length > 1 ? parseInt(parts[1], 10) : start;

		if (start < 1 || start > 65535 || end < 1 || end > 65535)
			return _('Port must be between 1 and 65535');

		if (start > end)
			return _('Port range start must be less than or equal to range end');
	}

	return true;
}

function validInputMode(value, fallback, modes) {
	value = String(value || '').trim();
	for (var i = 0; i < modes.length; i++) {
		if (value == modes[i])
			return value;
	}
	return fallback;
}

function cleanBool(value, fallback) {
	if (value == null || value == '')
		return fallback;
	if (value === false)
		return '0';
	if (value === true)
		return '1';

	value = String(value).trim().toLowerCase();
	if (value == '0' || value == 'false' || value == 'off' || value == 'no')
		return '0';
	if (value == '1' || value == 'true' || value == 'on' || value == 'yes')
		return '1';
	return fallback;
}

function isProviderRuleOption(option) {
	return option == 'domain_provider' || option == 'ip_provider' || option == 'provider';
}

function exportRule(section_id) {
	var rule = {
		name: String(uci.get('zakop', section_id, 'name') || section_id).trim() || section_id,
		enabled: cleanBool(uci.get('zakop', section_id, 'enabled'), '1'),
		action: 'direct',
		outbound: 'direct',
		dns_mode: 'auto'
	};
	var domainInput = validInputMode(uci.get('zakop', section_id, 'domain_input'), '', [ 'fields', 'text', 'provider', 'file' ]);
	var ipInput = validInputMode(uci.get('zakop', section_id, 'ip_input'), '', [ 'list', 'text', 'provider', 'file' ]);

	if (domainInput != '' && domainInput != 'provider')
		rule.domain_input = domainInput;
	if (ipInput != '' && ipInput != 'provider')
		rule.ip_input = ipInput;

	for (var i = 0; i < ruleListOptions.length; i++) {
		var option = ruleListOptions[i];
		var values = optionValues(section_id, option);

		if (isProviderRuleOption(option))
			continue;

		if (values.length > 0)
			rule[option] = values;
	}

	return rule;
}

function exportRulesJSON() {
	var rules = [];

	uci.sections('zakop', 'rule', function(section, sid) {
		rules.push(exportRule(sid));
	});

	return JSON.stringify({
		version: 1,
		rules: rules
	}, null, '\t');
}

function parseImportedRules(text) {
	var data, rules, out;

	try {
		data = JSON.parse(String(text || ''));
	} catch (err) {
		throw new Error(_('Import failed') + ': ' + err.message);
	}

	rules = Array.isArray(data) ? data : data && data.rules;
	if (!Array.isArray(rules))
		throw new Error(_('Import failed') + ': ' + _('rules must be an array'));

	out = [];
	for (var i = 0; i < rules.length; i++) {
		var rule = rules[i];
		var cleanRule;
		var domainInput;
		var ipInput;

		if (rule == null || typeof rule != 'object' || Array.isArray(rule))
			continue;

		domainInput = validInputMode(rule.domain_input, '', [ 'fields', 'text', 'provider', 'file' ]);
		ipInput = validInputMode(rule.ip_input, '', [ 'list', 'text', 'provider', 'file' ]);
		if (domainInput == 'provider')
			domainInput = '';
		if (ipInput == 'provider')
			ipInput = '';

		cleanRule = {
			name: String(rule.name || ('imported_rule_' + (i + 1))).trim() || ('imported_rule_' + (i + 1)),
			enabled: cleanBool(rule.enabled, '1'),
			action: 'direct',
			outbound: 'direct',
			dns_mode: 'auto',
			domain_input: domainInput,
			ip_input: ipInput
		};

		for (var j = 0; j < ruleListOptions.length; j++) {
			var option = ruleListOptions[j];
			var values = cleanValues(rule[option]);

			if (isProviderRuleOption(option))
				continue;

			if (values.length > 0)
				cleanRule[option] = values;
		}

		out.push(cleanRule);
	}

	if (out.length == 0)
		throw new Error(_('Import failed') + ': ' + _('no rules found'));

	return out;
}

function addRuleSection() {
	var before = {};
	var added = null;

	uci.sections('zakop', 'rule', function(section, sid) {
		before[sid] = true;
	});

	added = uci.add('zakop', 'rule');
	if (added)
		return added;

	uci.sections('zakop', 'rule', function(section, sid) {
		if (!before[sid] && added == null)
			added = sid;
	});

	if (added == null)
		throw new Error(_('Import failed'));

	return added;
}

function writeImportedRule(section_id, rule) {
	uci.set('zakop', section_id, 'name', rule.name);
	uci.set('zakop', section_id, 'enabled', rule.enabled);
	uci.set('zakop', section_id, 'action', 'direct');
	uci.set('zakop', section_id, 'outbound', 'direct');
	uci.set('zakop', section_id, 'dns_mode', 'auto');

	if (rule.domain_input != '')
		uci.set('zakop', section_id, 'domain_input', rule.domain_input);
	if (rule.ip_input != '')
		uci.set('zakop', section_id, 'ip_input', rule.ip_input);

	for (var i = 0; i < ruleListOptions.length; i++) {
		var option = ruleListOptions[i];

		if (isProviderRuleOption(option))
			continue;

		if (rule[option])
			setListOption(section_id, option, rule[option]);
	}
}

function replaceRules(rules) {
	var sections = [];

	uci.sections('zakop', 'rule', function(section, sid) {
		sections.push(sid);
	});

	for (var i = 0; i < sections.length; i++)
		uci.remove('zakop', sections[i]);

	for (var j = 0; j < rules.length; j++)
		writeImportedRule(addRuleSection(), rules[j]);
}

function removeNode(node) {
	if (node && node.parentNode)
		node.parentNode.removeChild(node);
}

function downloadTextFile(filename, text) {
	var urlAPI = window.URL || window.webkitURL;
	var blob, url, link;

	if (typeof Blob == 'undefined' || !urlAPI)
		throw new Error(_('File download is not supported by this browser'));

	blob = new Blob([ text ], { type: 'application/json' });
	url = urlAPI.createObjectURL(blob);
	link = E('a', {
		'href': url,
		'download': filename,
		'style': 'display:none'
	});

	document.body.appendChild(link);
	link.click();

	window.setTimeout(function() {
		removeNode(link);
		urlAPI.revokeObjectURL(url);
	}, 0);
}

function pickTextFile(accept) {
	return new Promise(function(resolve, reject) {
		var input;
		var done = false;

		if (typeof FileReader == 'undefined') {
			reject(new Error(_('File import is not supported by this browser')));
			return;
		}

		input = E('input', {
			'type': 'file',
			'accept': accept,
			'style': 'display:none',
			'change': function(ev) {
				var file = ev.target.files && ev.target.files[0];
				var reader;

				if (!file) {
					done = true;
					removeNode(input);
					resolve(null);
					return;
				}

				reader = new FileReader();
				reader.onload = function() {
					done = true;
					removeNode(input);
					resolve(String(reader.result || ''));
				};
				reader.onerror = function() {
					done = true;
					removeNode(input);
					reject(new Error(_('Import failed')));
				};
				reader.readAsText(file);
			}
		});

		document.body.appendChild(input);
		input.click();

		window.setTimeout(function() {
			if (!done)
				removeNode(input);
		}, 60000);
	});
}

return view.extend({
	load: function() {
		return uci.load('zakop').then(function() {
			zakopUI.syncRulesTab();
		});
	},

	showExportRules: function() {
		try {
			downloadTextFile('zakop-rules.json', exportRulesJSON());
		} catch (err) {
			ui.addNotification(null, E('p', {}, [ err.message || err ]), 'danger');
		}
	},

	showImportRules: function() {
		return pickTextFile('.json,application/json')
			.then(L.bind(function(text) {
				text = String(text || '').trim();
				if (text == '')
					return Promise.resolve();

				return this.handleImportRules(text);
			}, this))
			.catch(function(err) {
				ui.addNotification(null, E('p', {}, [ err.message || err ]), 'danger');
			});
	},

	handleImportRules: function(text) {
		var rules = parseImportedRules(text);

		return this.map.save(rewriteRuleState)
			.then(function() {
				replaceRules(rules);
				rewriteRuleState();
				return uci.save('zakop');
			})
			.then(function(ok) {
				if (ok === false)
					throw new Error(_('Save failed'));

				return fs.exec('/sbin/uci', [ 'commit', 'zakop' ]);
			})
			.then(function(res) {
				if (res.code)
					throw new Error(res.stderr || res.stdout || _('Commit failed'));

				return fs.exec('/etc/init.d/zakop', [ 'restart' ]);
			})
			.then(function() {
				window.location.reload();
			});
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
		var m, s, o, routingMode, self, firstOutbound;

		zakopUI.syncRulesTab();

		m = new form.Map('zakop', _('zakop'));
		this.map = m;
		self = this;
		routingMode = String(uci.get('zakop', 'main', 'routing_mode') || 'custom').trim();
		firstOutbound = firstOutboundTag();

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
				ui.addNotification(null, E('p', {}, [ _('Create an outbound before adding a proxy rule.') ]), 'warning');
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
					if (firstOutbound == '') {
						ui.addNotification(null, E('p', {}, [ _('Create an outbound before adding a proxy rule.') ]), 'warning');
						return;
					}
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

		o = s.option(form.Flag, 'enabled', _('Enabled'));
		o.enabled = '1';
		o.disabled = '0';
		o.default = '1';
		o.rmempty = false;
		o.editable = true;

		o = s.option(form.Value, 'name', _('Name'));
		o.rmempty = false;

		o = s.option(form.ListValue, 'action', _('Action'));
		o.value('proxy', 'proxy');
		o.value('direct', 'direct');
		o.value('block', 'block');
		o.default = 'proxy';
		o.rmempty = false;
		o.editable = true;

		o = s.option(form.ListValue, 'outbound', _('Outbound'));
		addOutboundChoices(o);
		o.depends('action', 'proxy');
		o.rmempty = false;
		o.forcewrite = true;
		o.cfgvalue = function(section_id) {
			var value = String(uci.get('zakop', section_id, 'outbound') || '').trim();

			return outboundTagExists(value) ? value : firstOutbound;
		};
		o.validate = function(section_id, value) {
			if (!outboundTagExists(value))
				return _('Create an outbound before adding a proxy rule.');

			return true;
		};
		o.editable = true;

		o = s.option(form.ListValue, 'domain_input', _('Domain input'));
		o.value('fields', _('Fields'));
		o.value('text', _('Textbox'));
		o.value('file', _('File paths'));
		o.value('provider', _('Providers'));
		o.default = 'fields';
		o.rmempty = false;
		o.modalonly = true;
		o.cfgvalue = function(section_id) {
			var value = String(uci.get('zakop', section_id, 'domain_input') || '').trim();

			if (value != '')
				return value;
			if (optionValues(section_id, 'domain_provider').length > 0)
				return 'provider';
			if (optionValues(section_id, 'domain_file').length > 0)
				return 'file';
			return 'fields';
		};

		addDynamicList(s, 'domain_equals', _('Equals'), 'domain_input', 'fields');
		addDynamicList(s, 'domain_contains', _('Contains'), 'domain_input', 'fields');
		addDynamicList(s, 'domain_starts_with', _('Starts with'), 'domain_input', 'fields');
		addDynamicList(s, 'domain_ends_with', _('Ends with'), 'domain_input', 'fields');
		addDynamicList(s, 'exclude_domain_equals', _('Exclude equals'), 'domain_input', 'fields');
		addDynamicList(s, 'exclude_domain_contains', _('Exclude contains'), 'domain_input', 'fields');
		addDynamicList(s, 'exclude_domain_starts_with', _('Exclude starts with'), 'domain_input', 'fields');
		addDynamicList(s, 'exclude_domain_ends_with', _('Exclude ends with'), 'domain_input', 'fields');

		addTextList(s, '_domain_equals_text', 'domain_equals', _('Equals text'), 'domain_input', 'text', 'example.com\nexample.org');
		addTextList(s, '_domain_contains_text', 'domain_contains', _('Contains text'), 'domain_input', 'text', 'youtube');
		addTextList(s, '_domain_starts_with_text', 'domain_starts_with', _('Starts with text'), 'domain_input', 'text', 'www.');
		addTextList(s, '_domain_ends_with_text', 'domain_ends_with', _('Ends with text'), 'domain_input', 'text', '.example.com');
		addTextList(s, '_exclude_domain_equals_text', 'exclude_domain_equals', _('Exclude equals text'), 'domain_input', 'text');
		addTextList(s, '_exclude_domain_contains_text', 'exclude_domain_contains', _('Exclude contains text'), 'domain_input', 'text');
		addTextList(s, '_exclude_domain_starts_with_text', 'exclude_domain_starts_with', _('Exclude starts with text'), 'domain_input', 'text');
		addTextList(s, '_exclude_domain_ends_with_text', 'exclude_domain_ends_with', _('Exclude ends with text'), 'domain_input', 'text');

		addProviderList(s, 'domain_provider', _('Domain providers'), 'domain', 'domain_input', 'provider');
		addDynamicList(s, 'domain_file', _('Domain file paths'), 'domain_input', 'file', '/etc/zakop/domains.txt');

		o = s.option(form.ListValue, 'ip_input', _('IP input'));
		o.value('list', _('List'));
		o.value('text', _('Textbox'));
		o.value('file', _('File paths'));
		o.value('provider', _('Providers'));
		o.default = 'list';
		o.rmempty = false;
		o.modalonly = true;
		o.cfgvalue = function(section_id) {
			var value = String(uci.get('zakop', section_id, 'ip_input') || '').trim();

			if (value != '')
				return value;
			if (optionValues(section_id, 'ip_provider').length > 0)
				return 'provider';
			if (optionValues(section_id, 'ip_file').length > 0 || optionValues(section_id, 'file').length > 0)
				return 'file';
			return 'list';
		};

		addDynamicList(s, 'ip_cidr', _('IP/CIDR list'), 'ip_input', 'list', '1.1.1.1');
		addTextList(s, '_ip_cidr_text', 'ip_cidr', _('IP/CIDR text'), 'ip_input', 'text', '1.1.1.1\n8.8.8.0/24');
		addProviderList(s, 'ip_provider', _('IP providers'), 'ip', 'ip_input', 'provider');
		addDynamicList(s, 'ip_file', _('IP/CIDR file paths'), 'ip_input', 'file', '/etc/zakop/ipv4-cidr.txt');

		o = s.option(form.DummyValue, '_packet_match', _('Advanced packet match'),
			_('Port matching is packet-level. It applies only to provider/CIDR/IP matches, not to DNS/FakeIP domain matching.'));
		o.modalonly = true;
		o.cfgvalue = function() {
			return '';
		};

		o = s.option(form.ListValue, '_packet_proto', _('Protocol'));
		o.value('any', _('Any'));
		o.value('tcp', _('TCP'));
		o.value('udp', _('UDP'));
		o.default = 'any';
		o.rmempty = false;
		o.modalonly = true;
		o.cfgvalue = packetProtoValue;
		o.write = writePacketProto;

		o = s.option(form.DynamicList, 'src_port', _('Source ports'),
			_('Source ports are client-side ports chosen by the LAN device. Usually leave empty. Syntax: 443 or 1000-2000.'));
		o.placeholder = '1000-2000';
		o.rmempty = true;
		o.modalonly = true;
		o.validate = validatePortMatch;

		o = s.option(form.DynamicList, 'dst_port', _('Destination ports'),
			_('Destination ports are service ports on the remote IP, for example 443 for HTTPS or 53 for DNS. Syntax: 443 or 1000-2000.'));
		o.placeholder = '443';
		o.rmempty = true;
		o.modalonly = true;
		o.validate = validatePortMatch;

		return m.render();
	}
});

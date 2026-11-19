'use strict';
'require baseclass';
'require uci';
'require zakop.i18n as zakopI18n';
'require zakop.ruleform as ruleForm';
'require zakop.ui as zakopUI';

var _ = zakopI18n.translate;

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

function referenceCatalog() {
	var outbounds = Object.create(null);
	var providers = Object.create(null);

	[ 'outbound', 'outbound_pool' ].forEach(function(type) {
		uci.sections('zakop', type, function(section, sid) {
			var tag = String(section.tag || sid).trim();
			if (!tag || [ 'direct', 'blocked', 'block', 'proxy_default' ].indexOf(tag) >= 0)
				return;
			outbounds[tag] = {
				type: type == 'outbound_pool' ? 'pool' : String(section.type || '').trim(),
				label: String(section.label || section.name || '').trim(),
				server: String(section.server || section.address || '').trim().toLowerCase(),
				port: Number(section.port) || 0,
				subscription: String(section.subscription || '')
			};
		});
	});
	uci.sections('zakop', 'provider', function(section, sid) {
		providers[sid] = {
			type: String(section.type || 'ip').trim(),
			label: String(section.label || section.name || '').trim(),
			source: String(section.source || 'url').trim(),
			script_path: String(section.script_path || '').trim()
		};
	});

	return { outbounds: outbounds, providers: providers };
}

function exportRule(section_id, catalog, references, scoped) {
	var section = uci.get('zakop', section_id);
	var action = section.action || 'proxy';
	var outbound = String(section.outbound || '').trim();
	if (action == 'proxy' && [ '', 'direct', 'block', 'blocked', 'proxy_default' ].indexOf(outbound) >= 0)
		outbound = ruleForm.firstOutboundTag();
	var rule = {
		name: String(section.name || section_id).trim() || section_id,
		enabled: cleanBool(section.enabled, '1'),
		action: action,
		outbound: action == 'proxy' ? outbound : 'direct',
		dns_mode: section.dns_mode || 'auto'
	};
	if (scoped) {
		rule.client_ip = section.client_ip;
		if (section.override)
			rule.override = section.override;
	} else {
		rule.id = ruleForm.ruleID(section, section_id);
	}

	if (action == 'proxy' && catalog.outbounds[outbound])
		references.outbounds[outbound] = catalog.outbounds[outbound];
	if (rule.override)
		return rule;
	if (section.domain_input)
		rule.domain_input = section.domain_input;
	if (section.ip_input)
		rule.ip_input = section.ip_input;

	ruleForm.ruleListOptions.forEach(function(option) {
		var values = ruleForm.optionValues(section_id, option);
		if (values.length)
			rule[option] = values;
		if (isProviderRuleOption(option))
			values.forEach(function(id) {
				if (catalog.providers[id])
					references.providers[id] = catalog.providers[id];
			});
	});

	return rule;
}

function sectionIDs(type) {
	return uci.sections('zakop', type).map(function(section) { return section['.name']; });
}

function exportRulesJSON(orderedIDs) {
	var catalog = referenceCatalog();
	var references = { outbounds: Object.create(null), providers: Object.create(null) };
	var rules = (orderedIDs || sortRuleSectionIDs(sectionIDs('rule'))).map(function(sid) {
		return exportRule(sid, catalog, references, false);
	});
	var names = new Map(rules.map(function(rule) { return [ rule.id, rule.name ]; }));
	var clients = sortRuleSectionIDs(sectionIDs('client_rule')).map(function(sid) {
		var rule = exportRule(sid, catalog, references, true);
		if (names.has(rule.override))
			rule.name = names.get(rule.override);
		return rule;
	});

	return JSON.stringify({ version: 2, rules: rules, client_rules: clients, references: references }, null, '	');
}

function uniqueReference(catalog, predicate) {
	var matches = Object.keys(catalog).filter(function(id) { return predicate(catalog[id]); });
	return matches.length == 1 ? matches[0] : '';
}

function findOutbound(catalog, tag, hint) {
	if (catalog[tag] && (!hint || catalog[tag].type == hint.type))
		return tag;
	if (!hint || !hint.type)
		return '';

	var endpoint = uniqueReference(catalog, function(node) {
		return hint.server && node.type == hint.type && node.server == hint.server && node.port == hint.port;
	});
	return endpoint || uniqueReference(catalog, function(node) {
		return hint.label && node.type == hint.type && node.label == hint.label &&
			(!hint.subscription || node.subscription == hint.subscription);
	});
}

function findProvider(catalog, id, hint, option) {
	var type = option == 'domain_provider' ? 'domain' : (option == 'ip_provider' ? 'ip' : hint && hint.type);
	if (catalog[id] && (!type || catalog[id].type == type))
		return id;
	if (!hint || !type)
		return '';
	return uniqueReference(catalog, function(provider) {
		return provider.type == type && provider.source == hint.source &&
			((hint.script_path && provider.script_path == hint.script_path) ||
			 (hint.label && provider.label == hint.label));
	});
}

function parseImportedRules(text) {
	var data;
	try {
		data = JSON.parse(String(text || ''));
	} catch (err) {
		throw new Error(_('Import failed') + ': ' + err.message);
	}

	var rules = Array.isArray(data) ? data : data && data.rules;
	var clients = data && data.client_rules;
	if (!Array.isArray(rules) || (clients != null && !Array.isArray(clients)))
		throw new Error(_('Import failed') + ': ' + _('rules must be an array'));

	var catalog = referenceCatalog();
	var references = data.references || {};
	var warnings = [];
	var configuredClients = new Set(uci.sections('zakop', 'client').map(function(client) { return client.ip; }));
	var ids = new Set();

	function parse(rule, index, scoped) {
		if (!rule || typeof rule != 'object' || Array.isArray(rule))
			return null;
		var fallbackName = 'imported_rule_' + (index + 1);
		var clean = {
			name: String(rule.name || fallbackName).trim() || fallbackName,
			enabled: cleanBool(rule.enabled, '1'),
			action: [ 'proxy', 'direct', 'block' ].indexOf(rule.action) >= 0 ? rule.action : 'direct',
			outbound: 'direct',
			dns_mode: 'auto'
		};
		if (scoped) {
			clean.client_ip = String(rule.client_ip || '').trim();
			var octets = clean.client_ip.split('.');
			if (octets.length != 4 || octets.some(function(octet) { return !/^(0|[1-9][0-9]{0,2})$/.test(octet) || Number(octet) > 255; }))
				throw new Error(_('Invalid client IPv4 address in rule "%s".').format(clean.name));
			if (!configuredClients.has(clean.client_ip))
				warnings.push(_('Rule "%s": client %s is unavailable. The rule will apply when the client is configured.').format(clean.name, clean.client_ip));
			clean.override = String(rule.override || '').trim();
		} else {
			var id = String(rule.id || '').trim();
			if (id && !ids.has(id)) {
				clean.rule_id = id;
				ids.add(id);
			}
		}

		if (clean.action == 'proxy') {
			var tag = String(rule.outbound || '').trim();
			var outbound = findOutbound(catalog.outbounds, tag, references.outbounds && references.outbounds[tag]);
			if (outbound) {
				clean.outbound = outbound;
				if (rule.dns_mode == 'real_ip' || rule.dns_mode == 'fakeip')
					clean.dns_mode = rule.dns_mode;
			} else {
				clean.action = 'direct';
				warnings.push(_('Rule "%s": outbound "%s" was not found. Action reset to direct.').format(clean.name, tag || '—'));
			}
		}
		if (clean.override) {
			if (!ids.has(clean.override))
				warnings.push(_('Rule "%s": shared rule "%s" was not found. The override is inactive.').format(clean.name, clean.override));
			return clean;
		}

		ruleForm.ruleListOptions.forEach(function(option) {
			var values = ruleForm.cleanValues(rule[option]);
			if (isProviderRuleOption(option)) {
				values = values.map(function(id) {
					var replacement = findProvider(catalog.providers, id, references.providers && references.providers[id], option);
					if (!replacement)
						warnings.push(_('Rule "%s": provider "%s" was not found and was removed.').format(clean.name, id));
					return replacement;
				}).filter(Boolean);
			}
			if (values.length)
				clean[option] = values;
		});
		clean.domain_input = validInputMode(rule.domain_input, '', [ 'fields', 'text', 'provider', 'file' ]);
		clean.ip_input = validInputMode(rule.ip_input, '', [ 'list', 'text', 'provider', 'file' ]);
		if (clean.domain_input == 'provider' && !clean.domain_provider)
			delete clean.domain_input;
		if (clean.ip_input == 'provider' && !clean.ip_provider)
			delete clean.ip_input;

		var hasIP = clean.ip_cidr || clean.ip_file || clean.file || clean.ip_provider ||
			(clean.provider || []).some(function(id) { return catalog.providers[id].type == 'ip'; });
		if (!hasIP && (clean.proto || clean.src_port || clean.dst_port)) {
			delete clean.proto;
			delete clean.src_port;
			delete clean.dst_port;
			warnings.push(_('Rule "%s": protocol and ports were cleared because no IP matcher remains.').format(clean.name));
		}

		return clean;
	}

	var shared = rules.map(function(rule, index) { return parse(rule, index, false); }).filter(Boolean);
	var scoped = clients && clients.map(function(rule, index) { return parse(rule, index, true); }).filter(Boolean);
	if (!shared.length && !(scoped && scoped.length) && !(data.version == 2 && !rules.length && clients && !clients.length))
		throw new Error(_('Import failed') + ': ' + _('no rules found'));
	return { rules: shared, clientRules: scoped, warnings: warnings };
}

function writeImportedRule(type, rule) {
	var sid = uci.add('zakop', type);
	if (!sid)
		throw new Error(_('Import failed'));
	Object.keys(rule).forEach(function(option) {
		if (rule[option] && (option != 'name' || !rule.override))
			uci.set('zakop', sid, option, rule[option]);
	});
	return sid;
}

function replaceRules(imported) {
	var previousNames = Object.create(null);
	uci.sections('zakop', 'rule', function(section, sid) {
		previousNames[ruleForm.ruleID(section, sid)] = section.name || sid;
		uci.remove('zakop', sid);
	});
	var names = Object.create(null);
	imported.rules.forEach(function(rule, index) {
		rule.priority = String((index + 1) * 100);
		var sid = writeImportedRule('rule', rule);
		names[rule.name] = Object.prototype.hasOwnProperty.call(names, rule.name) ? null : sid;
	});
	if (imported.clientRules) {
		uci.sections('zakop', 'client_rule', function(section, sid) { uci.remove('zakop', sid); });
		imported.clientRules.forEach(function(rule) { writeImportedRule('client_rule', rule); });
		return;
	}
	// Legacy exports only replace shared rules. Keep existing client rules,
	// and reattach overrides by name when the imported name is unambiguous.
	uci.sections('zakop', 'client_rule', function(section, sid) {
		if (!section.override)
			return;
		var base = names[previousNames[section.override]];
		if (!base) {
			imported.warnings.push(_('Rule "%s": shared rule "%s" was not found. The override is inactive.').format(section.name || sid, section.override));
			return;
		}
		var id = uci.get('zakop', base, 'rule_id') || section.override;
		uci.set('zakop', base, 'rule_id', id);
		uci.set('zakop', sid, 'override', id);
	});
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
			if (!done) {
				done = true;
				removeNode(input);
				resolve('');
			}
		}, 60000);
	});
}

function showExportRules(orderedIDs) {
	try {
		downloadTextFile('zakop-rules.json', exportRulesJSON(orderedIDs));
	} catch (err) {
		zakopUI.notify(String(err.message || err), 'danger');
	}
}

function showImportRules(map, rewrite) {
	return pickTextFile('.json,application/json')
		.then(function(text) {
			text = String(text || '').trim();
			if (text)
				return handleImportRules(text, map, rewrite);
		})
		.catch(function(err) {
			zakopUI.notify(String(err.message || err), 'danger');
		});
}

function handleImportRules(text, map, rewrite) {
	var imported = parseImportedRules(text);

	return map.save(rewrite)
		.then(function() {
			replaceRules(imported);
			return uci.save('zakop');
		})
		.then(function(ok) {
			if (ok === false)
				throw new Error(_('Save failed'));

			return zakopUI.commitConfig();
		})
		.then(function() {
			return zakopUI.restart().finally(function() {
				zakopUI.notify(E('div', {}, [
					E('p', {}, _('Rules imported.')),
					imported.warnings.length ? E('ul', {}, imported.warnings.map(function(message) {
						return E('li', {}, message);
					})) : ''
				]), imported.warnings.length ? 'warning' : 'success');
				return map.load().then(function() { return map.reset(); });
			});
		});
}

return baseclass.extend({
	rulePriority: rulePriority,
	sortRuleSectionIDs: sortRuleSectionIDs,
	showExportRules: showExportRules,
	showImportRules: showImportRules
});

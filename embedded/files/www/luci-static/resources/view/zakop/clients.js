'use strict';
'require rpc';
'require form';
'require ui';
'require uci';
'require validation';
'require view';
'require zakop.i18n as zakopI18n';
'require zakop.ui as zakopUI';
'require zakop.ruleform as ruleForm';
'require zakop.rulebackup as ruleBackup';

var _ = zakopI18n.translate;
var callHostHints = rpc.declare({
	object: 'luci-rpc',
	method: 'getHostHints',
	expect: { '': {} }
});

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

function rewriteClientState() {
	var first = '';
	var available = {};

	uci.sections('zakop', 'outbound', function(section, sid) {
		var tag = String(section.tag || sid || section['.name'] || '').trim();

		if (tag == '' || tag == 'direct' || tag == 'blocked' || tag == 'block' || tag == 'proxy_default')
			return;
		if (first == '')
			first = tag;
		available[tag] = true;
	});
	uci.sections('zakop', 'outbound_pool', function(section, sid) {
		var tag = String(section.tag || sid || section['.name'] || '').trim();

		if (tag != '' && tag != 'direct' && tag != 'blocked' && tag != 'block' && tag != 'proxy_default')
			available[tag] = true;
	});

	uci.sections('zakop', 'client', function(section, sid) {
		var policy = String(uci.get('zakop', sid, 'policy') || 'default').trim();
		var outbound = String(uci.get('zakop', sid, 'outbound') || '').trim();

		if (policy != 'proxy')
			uci.unset('zakop', sid, 'outbound');
		else if (!available[outbound] && first != '')
			uci.set('zakop', sid, 'outbound', first);
	});
	uci.sections('zakop', 'client_rule', function(section, sid) {
		ruleForm.rewriteRule(sid, first);
	});
}

function clientChoices() {
	var clients = Object.create(null);
	uci.sections('zakop', 'client', function(section) {
		var ip = String(section.ip || '').trim();
		if (ip)
			clients[ip] = section.name ? section.name + ' (' + ip + ')' : ip;
	});
	uci.sections('zakop', 'client_rule', function(section) {
		if (section.client_ip && !clients[section.client_ip])
			clients[section.client_ip] = _('Client not found: %s').format(section.client_ip);
	});
	return clients;
}

function addClientRuleFields(s, routingMode) {
	var shared = Object.create(null);
	var pendingIDs = Object.create(null);

	s.load = function() {
		shared = Object.create(null);
		uci.sections('zakop', 'rule', function(section, sid) {
			var id = section.rule_id || pendingIDs[sid] || ruleForm.stableRuleChoice(section, sid);
			pendingIDs[sid] = id;
			shared[id] = { sid: sid, name: section.name || sid };
		});
		return form.GridSection.prototype.load.apply(this, arguments);
	};

	var o = s.option(form.DummyValue, '_rule_name', _('Name'));
	o.modalonly = false;
	o.cfgvalue = function(section_id) {
		var override = uci.get('zakop', section_id, 'override');
		if (override)
			return shared[override] ? shared[override].name : _('Shared rule unavailable: %s').format(override);
		return uci.get('zakop', section_id, 'name') || section_id;
	};

	o = s.option(form.ListValue, 'client_ip', _('Client'));
	o.rmempty = false;
	o.load = function(section_id) {
		var clients = clientChoices();
		this.keylist = [];
		this.vallist = [];
		Object.keys(clients).forEach(L.bind(function(ip) { this.value(ip, clients[ip]); }, this));
		return form.ListValue.prototype.load.call(this, section_id);
	};
	o.validate = function(section_id, value) {
		return clientChoices()[value] ? true : _('Add the device to Clients first, then create a rule for it.');
	};
	o = s.option(form.ListValue, 'override', _('Shared rule'),
		_('Pick a shared rule to change how it works for this device. For example, send YouTube through a different proxy on your TV.'));
	o.modalonly = true;
	o.default = '-';
	o.rmempty = false;
	o.load = function(section_id) {
		var option = this;
		var current = uci.get('zakop', section_id, 'override');
		this.keylist = [];
		this.vallist = [];
		this.value('-', _('New standalone rule'));
		if (routingMode == 'custom') {
			Object.keys(shared).forEach(function(id) {
				option.value(id, shared[id].name);
			});
		}
		if (current && (routingMode != 'custom' || !shared[current]))
			this.value(current, _('Shared rule unavailable: %s').format(shared[current] ? shared[current].name : current));
		return form.ListValue.prototype.load.call(this, section_id);
	};
	o.cfgvalue = function(section_id) {
		return uci.get('zakop', section_id, 'override') || '-';
	};
	o.write = function(section_id, value) {
		if (value == '-') {
			uci.unset('zakop', section_id, 'override');
		} else {
			if (shared[value])
				uci.set('zakop', shared[value].sid, 'rule_id', value);
			uci.set('zakop', section_id, 'override', value);
		}
	};

	o = s.option(form.Value, 'name', _('Name'));
	o.modalonly = true;
	o.rmempty = false;
	o.depends('override', '-');
}

function addClientRules(map, routingMode) {
	if (routingMode == 'global')
		return;

	var s = map.section(form.GridSection, 'client_rule', _('Client rules'),
		_('Set the device to Default to use these rules. If several rules match, the one higher in the list wins. If none match, the shared settings apply.'));
	s.anonymous = true;
	s.addremove = true;
	s.sortable = true;
	s.modaltitle = _('Client rule details');
	s.renderHeaderRows = function() {
		this.sortable = false;
		try {
			return form.GridSection.prototype.renderHeaderRows.apply(this, arguments);
		} finally {
			this.sortable = true;
		}
	};

	s.handleAdd = function() {
		if (!uci.sections('zakop', 'client').some(function(client) { return client.ip; })) {
			zakopUI.notify(_('Add the device to Clients first, then create a rule for it.'), 'warning');
			return Promise.resolve();
		}
		return form.GridSection.prototype.handleAdd.apply(this, arguments);
	};

	s.renderSectionAdd = function() {
		var el = form.GridSection.prototype.renderSectionAdd.apply(this, arguments);
		el.appendChild(E('button', {
			'class': 'cbi-button cbi-button-action',
			'style': 'margin-left:.5em',
			'click': function(ev) {
				ev.preventDefault();
				return ruleBackup.showImportRules(map, rewriteClientState);
			}
		}, _('Import')));
		el.appendChild(E('button', {
			'class': 'cbi-button cbi-button-action',
			'style': 'margin-left:.5em',
			'click': function(ev) {
				ev.preventDefault();
				return ruleBackup.showExportRules();
			}
		}, _('Export')));
		return el;
	};

	ruleForm.addRuleOptions(s, function(section) {
		addClientRuleFields(section, routingMode);
	});
	ruleForm.addMatcherOptions(s);
}

function deviceAddresses(hints) {
	var addresses = new Map();
	Object.keys(hints).forEach(function(mac) {
		var host = hints[mac];
		if (!host)
			return;
		var label = host.name ? host.name + ' · ' + mac : mac;
		L.toArray(host.ipaddrs || host.ipv4).forEach(function(value) {
			var octets = typeof value == 'string' && validation.parseIPv4(value);
			if (octets) {
				var ip = octets.join('.');
				if (!addresses.has(ip) || host.name)
					addresses.set(ip, label);
			}
		});
	});
	return addresses;
}

function addDeviceSuggestions(modalSection) {
	var address = modalSection.children.find(function(option) { return option.option == 'ip'; });
	return callHostHints().catch(function() { return {}; }).then(function(hints) {
		var addresses = deviceAddresses(hints);
		if (!addresses.size)
			return;
		address.description = _('Select a device from the list or enter its IPv4 address. Search by name, IP address or MAC address.');
		address.renderWidget = function(section_id) {
			var widget = form.Value.prototype.renderWidget.apply(this, arguments);
			var input = widget.matches('input') ? widget : widget.querySelector('input');
			var listID = this.cbid(section_id) + '-devices';
			input.setAttribute('list', listID);
			return E('div', {}, [
				widget,
				E('datalist', { 'id': listID }, Array.from(addresses, function(entry) {
					return E('option', { 'value': entry[0], 'label': entry[1] });
				}))
			]);
		};
	});
}

return view.extend({
	load: function() {
		return uci.load('zakop').then(function() {
			zakopUI.syncRulesTab();
		});
	},

	handleSave: function() {
		return this.map.save(rewriteClientState).then(function() {
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
		var m, s, o, firstOutbound;

		zakopUI.syncRulesTab();

		m = new form.Map('zakop', _('zakop'));
		this.map = m;

		s = m.section(form.GridSection, 'client', _('Clients'),
			_('Default — use the rules for this device first, then the shared settings.') + '<br>' +
			_('proxy — use the selected proxy for internet traffic. Local traffic goes directly.') + '<br>' +
			_('direct — use the internet directly, bypassing zakop.'));
		s.anonymous = true;
		s.addremove = true;
		s.addModalOptions = addDeviceSuggestions;

		o = s.option(form.Value, 'name', _('Name'));
		o.rmempty = false;

		o = s.option(form.Value, 'ip', _('IPv4 address'));
		o.datatype = 'ip4addr';
		o.rmempty = false;

		o = s.option(form.ListValue, 'policy', _('Policy'));
		o.value('default', _('Default'));
		o.value('proxy', 'proxy');
		o.value('direct', 'direct');
		o.default = 'default';
		o.rmempty = false;
		o.editable = true;

		o = s.option(form.ListValue, 'outbound', _('Outbound'));
		firstOutbound = addOutboundChoices(o);
		o.depends('policy', 'proxy');
		o.rmempty = false;
		o.forcewrite = true;
		o.cfgvalue = function(section_id) {
			var value = String(uci.get('zakop', section_id, 'outbound') || '').trim();

			return outboundTagExists(value) ? value : firstOutbound;
		};
		o.validate = function(section_id, value) {
			if (!outboundTagExists(value))
				return _('Add a proxy on the Outbounds page first.');

			return true;
		};
		o.editable = true;

		addClientRules(m, String(uci.get('zakop', 'main', 'routing_mode') || 'custom').trim());
		return m.render();
	}
});

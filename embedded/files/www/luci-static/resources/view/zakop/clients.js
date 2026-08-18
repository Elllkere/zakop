'use strict';
'require fs';
'require form';
'require ui';
'require uci';
'require view';
'require zakop.i18n as zakopI18n';
'require zakop.ui as zakopUI';

var _ = zakopI18n.translate;

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
			_('Default follows general routing mode. Proxy forces non-reserved traffic through zakop. Direct bypasses zakop completely.'));
		s.anonymous = true;
		s.addremove = true;

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
				return _('Create an outbound before selecting proxy policy.');

			return true;
		};
		o.editable = true;

		return m.render();
	}
});

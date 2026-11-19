'use strict';
'require baseclass';
'require form';
'require uci';
'require zakop.i18n as zakopI18n';

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
	matcherDepends(section, o, modeOption, modeValue);

	return o;
}

function addTextList(section, option, target, title, modeOption, modeValue, placeholder) {
	var o = section.option(form.TextValue, option, title);

	o.rows = 6;
	o.rmempty = true;
	o.modalonly = true;
	matcherDepends(section, o, modeOption, modeValue);
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
	matcherDepends(section, o, modeOption, modeValue);
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

function matcherDepends(section, option, field, value) {
	var dependencies = {};
	if (field)
		dependencies[field] = value;
	if (section.sectiontype == 'client_rule')
		dependencies.override = '-';
	if (Object.keys(dependencies).length > 0)
		option.depends(dependencies);
}

function rewriteRule(sid, firstOutbound) {
	var action = String(uci.get('zakop', sid, 'action') || 'proxy').trim();
	var outbound = uci.get('zakop', sid, 'outbound');
	var domainInput = String(uci.get('zakop', sid, 'domain_input') || '').trim();
	var ipInput = String(uci.get('zakop', sid, 'ip_input') || '').trim();
	var protoValues = optionValues(sid, 'proto');
	var hasTCP = protoValues.indexOf('tcp') >= 0;
	var hasUDP = protoValues.indexOf('udp') >= 0;

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

	if (uci.get('zakop', sid, 'override')) {
		unsetRuleOptions(sid, ruleListOptions.concat([ 'name', 'domain_input', 'ip_input' ]));
		return;
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

function addRuleOptions(s, addFields) {
	var o;
	var firstOutbound = firstOutboundTag();
	o = s.option(form.Flag, 'enabled', _('Enabled'));
	o.enabled = '1';
	o.disabled = '0';
	o.default = '1';
	o.rmempty = false;
	o.editable = true;

	if (addFields) {
		addFields(s);
	} else {
		o = s.option(form.Value, 'name', _('Name'));
		o.rmempty = false;
	}

	o = s.option(form.ListValue, 'action', _('Action'));
	o.value('proxy', 'proxy');
	o.value('direct', 'direct');
	o.value('block', 'block');
	o.default = s.sectiontype == 'client_rule' && firstOutbound == '' ? 'direct' : 'proxy';
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

}

function addMatcherOptions(s) {
	var o;
	o = s.option(form.ListValue, 'domain_input', _('Domain input'));
	matcherDepends(s, o);
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
	matcherDepends(s, o);
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
	matcherDepends(s, o);
	o.modalonly = true;
	o.cfgvalue = function() {
		return '';
	};

	o = s.option(form.ListValue, '_packet_proto', _('Protocol'));
	matcherDepends(s, o);
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
	matcherDepends(s, o);
	o.placeholder = '1000-2000';
	o.rmempty = true;
	o.modalonly = true;
	o.validate = validatePortMatch;

	o = s.option(form.DynamicList, 'dst_port', _('Destination ports'),
		_('Destination ports are service ports on the remote IP, for example 443 for HTTPS or 53 for DNS. Syntax: 443 or 1000-2000.'));
	matcherDepends(s, o);
	o.placeholder = '443';
	o.rmempty = true;
	o.modalonly = true;
	o.validate = validatePortMatch;
}

function ruleID(section, sid) {
	return section.rule_id || sid;
}

function stableRuleChoice(section, sid) {
	if (section.rule_id || section['.anonymous'] === false)
		return ruleID(section, sid);
	// Persist the generated key when an override first uses this choice.
	return 'rule_' + Date.now().toString(36) + '_' + Math.random().toString(36).slice(2);
}

return baseclass.extend({
	addRuleOptions: addRuleOptions,
	addMatcherOptions: addMatcherOptions,
	firstOutboundTag: firstOutboundTag,
	outboundTagExists: outboundTagExists,
	ruleListOptions: ruleListOptions,
	cleanValues: cleanValues,
	optionValues: optionValues,
	setListOption: setListOption,
	rewriteRule: rewriteRule,
	ruleID: ruleID,
	stableRuleChoice: stableRuleChoice
});

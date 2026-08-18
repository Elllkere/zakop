'use strict';
'require fs';
'require form';
'require ui';
'require uci';
'require view';
'require zakop.i18n as zakopI18n';
'require zakop.ui as zakopUI';

var _ = zakopI18n.translate;

function commandResult(path, args) {
	return fs.exec(path, args).catch(function(err) {
		return {
			code: -1,
			stdout: '',
			stderr: String(err)
		};
	});
}

function outputLine(res) {
	return String((res && (res.stdout || res.stderr)) || '').trim().split('\n')[0] || '-';
}

function parseUpdateState(res) {
	var state = {
		current: '',
		latest: '',
		status: 'unavailable'
	};
	var lines = String((res && res.stdout) || '').trim().split('\n');

	if (!res || res.code != 0)
		return state;

	for (var i = 0; i < lines.length; i++) {
		var pos = lines[i].indexOf('=');
		var key = pos > 0 ? lines[i].substring(0, pos) : '';
		var value = pos > 0 ? lines[i].substring(pos + 1).trim() : '';

		if (key == 'current' || key == 'latest' || key == 'status')
			state[key] = value;
	}

	if (state.status != 'available' && state.status != 'current')
		state.status = 'unavailable';

	return state;
}

function updateStatusText(update) {
	switch (update.status) {
	case 'checking':
		return _('Checking for updates...');
	case 'available':
		return _('Update available');
	case 'current':
		return _('Up to date');
	default:
		return _('Version check failed');
	}
}

function processStatus(res) {
	return res && res.code == 0 ? _('Running') : _('Stopped');
}

function serviceStatus(res) {
	return res && res.code == 0 ? _('Running') : _('Stopped');
}

function waitForServiceState(running, attempts) {
	return fs.exec('/etc/init.d/zakop', [ 'status' ])
		.then(function(res) {
			if (!!res && (res.code == 0) == running)
				return res;

			if (attempts <= 1)
				throw new Error(running ? _('Service did not start in time') : _('Service did not stop in time'));

			return new Promise(function(resolve) {
				window.setTimeout(resolve, 250);
			}).then(function() {
				return waitForServiceState(running, attempts - 1);
			});
		});
}

function autostartStatus(res) {
	return res && res.code == 0 ? _('Enabled') : _('Disabled');
}

function defaultDNSPort(protocol) {
	switch (protocol) {
	case 'tls':
		return '853';
	case 'https':
		return '443';
	default:
		return '53';
	}
}

function normalizeDNSProtocol(protocol) {
	switch (String(protocol || '').trim()) {
	case 'tcp':
		return 'tcp';
	case 'tls':
	case 'dot':
		return 'tls';
	case 'https':
	case 'doh':
		return 'https';
	default:
		return 'udp';
	}
}

function normalizeDNSMode(mode) {
	mode = String(mode || '').trim();
	return mode == 'proxy' ? 'proxy' : 'direct';
}

function isReservedOutboundTag(tag) {
	return tag == 'direct' || tag == 'blocked' || tag == 'block' || tag == 'proxy_default';
}

function proxyOutboundExists(tag) {
	var found = false;

	tag = String(tag || '').trim();
	if (tag == '' || isReservedOutboundTag(tag))
		return false;

	uci.sections('zakop', 'outbound', function(section, sid) {
		var existing = String(section.tag || sid || section['.name'] || '').trim();

		if (existing == tag)
			found = true;
	});
	uci.sections('zakop', 'outbound_pool', function(section, sid) {
		var existing = String(section.tag || sid || section['.name'] || '').trim();

		if (existing == tag)
			found = true;
	});

	return found;
}

function firstProxyOutboundTag() {
	var first = '';

	uci.sections('zakop', 'outbound', function(section, sid) {
		var tag = String(section.tag || sid || section['.name'] || '').trim();

		if (first == '' && tag != '' && !isReservedOutboundTag(tag))
			first = tag;
	});

	return first;
}

function addProxyOutboundChoices(option) {
	var first = '';

	uci.sections('zakop', 'outbound', function(section, sid) {
		var tag = String(section.tag || sid || section['.name'] || '').trim();
		var label = String(section.label || section.name || tag).trim();

		if (tag == '' || isReservedOutboundTag(tag))
			return;

		if (first == '')
			first = tag;
		option.value(tag, label || tag);
	});
	uci.sections('zakop', 'outbound_pool', function(section, sid) {
		var tag = String(section.tag || sid || section['.name'] || '').trim();
		var label = String(section.label || section.name || tag).trim();

		if (tag != '' && !isReservedOutboundTag(tag))
			option.value(tag, _('Pool: %s').format(label || tag));
	});

	option.default = first;
	return first;
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

function inputDepends(modeOption, modeValue) {
	var depends = { 'routing_mode': 'simple' };

	depends[modeOption] = modeValue;
	return depends;
}

function addSimpleProviderList(section, option, title, providerType, modeOption, modeValue) {
	var o = section.option(form.DynamicList, option, title);

	o.depends(inputDepends(modeOption, modeValue));
	o.rmempty = true;
	o.retain = true;
	addProviderChoices(o, providerType);

	return o;
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

function unsetMainOptions(options) {
	for (var i = 0; i < options.length; i++)
		uci.unset('zakop', 'main', options[i]);
}

function addSimpleDynamicList(section, option, title, modeOption, modeValue, placeholder) {
	var o = section.option(form.DynamicList, option, title);

	o.depends(inputDepends(modeOption, modeValue));
	o.rmempty = true;
	o.retain = true;
	if (placeholder)
		o.placeholder = placeholder;

	return o;
}

function addSimpleTextList(section, option, target, title, modeOption, modeValue, placeholder) {
	var o = section.option(form.TextValue, option, title);

	o.depends(inputDepends(modeOption, modeValue));
	o.rows = 6;
	o.rmempty = true;
	o.retain = true;
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

function normalizeDNSPreset(preset) {
	preset = String(preset || '').trim();
	if (preset == 'google' || preset == 'custom')
		return preset;
	return 'cloudflare';
}

function splitHostPort(value) {
	value = String(value || '').trim();
	var idx = value.lastIndexOf(':');

	if (idx > 0 && value.indexOf(':') == idx)
		return [ value.slice(0, idx), value.slice(idx + 1) ];

	return [ value, '' ];
}

function presetDNS(preset, protocol) {
	switch (preset) {
	case 'google':
		if (protocol == 'tls' || protocol == 'https') {
			return {
				server: 'dns.google',
				serverName: 'dns.google',
				path: '/dns-query'
			};
		}
		return {
			server: '8.8.8.8',
			serverName: 'dns.google',
			path: '/dns-query'
		};
	default:
		return {
			server: '1.1.1.1',
			serverName: 'cloudflare-dns.com',
			path: '/dns-query'
		};
	}
}

function splitDoHValue(value, fallbackName, fallbackPath) {
	value = String(value || '').trim().replace(/^https:\/\//, '');
	fallbackName = String(fallbackName || '').trim();
	fallbackPath = String(fallbackPath || '').trim() || '/dns-query';

	if (value == '')
		return [ fallbackName, fallbackPath ];

	var slash = value.indexOf('/');
	if (slash < 0)
		return [ value, fallbackPath ];

	var name = value.slice(0, slash).trim();
	var path = value.slice(slash).trim();
	return [ name || fallbackName, path || fallbackPath ];
}

function normalizeDNSState() {
	var preset = normalizeDNSPreset(uci.get('zakop', 'main', 'dns_upstream_preset'));
	var mode = normalizeDNSMode(uci.get('zakop', 'main', 'real_dns_mode'));
	var dnsOutbound = String(uci.get('zakop', 'main', 'real_dns_outbound') || '').trim();
	var protocol = normalizeDNSProtocol(uci.get('zakop', 'main', 'real_dns_transport') || uci.get('zakop', 'main', 'dns_upstream_protocol'));
	var host = String(uci.get('zakop', 'main', 'real_dns_server') || uci.get('zakop', 'main', 'dns_upstream_host') || '').trim();
	var port = '';
	var tlsName = String(uci.get('zakop', 'main', 'real_dns_server_name') || uci.get('zakop', 'main', 'dns_upstream_tls_name') || '').trim();
	var path = String(uci.get('zakop', 'main', 'real_dns_path') || uci.get('zakop', 'main', 'dns_upstream_path') || '').trim();
	var legacy = splitHostPort(uci.get('zakop', 'main', 'real_dns_upstream'));
	var serverParts, presetValues, dohParts;

	if (preset != 'custom') {
		presetValues = presetDNS(preset, protocol);
		host = presetValues.server;
		tlsName = presetValues.serverName;
		path = presetValues.path;
		port = defaultDNSPort(protocol);
	} else if (host == '') {
		host = legacy[0] || '1.1.1.1';
		port = legacy[1] || '';
	}

	serverParts = splitHostPort(host);
	if (serverParts[1] != '') {
		host = serverParts[0];
		port = serverParts[1];
	}
	if (port == '')
		port = defaultDNSPort(protocol);
	if (tlsName == '' && host == '1.1.1.1')
		tlsName = 'cloudflare-dns.com';
	if (tlsName == '' && host == '8.8.8.8')
		tlsName = 'dns.google';
	if (protocol == 'https' && preset == 'custom') {
		dohParts = splitDoHValue(uci.get('zakop', 'main', '_real_dns_doh'), tlsName, path);
		tlsName = dohParts[0];
		path = dohParts[1];
	}
	if (path == '')
		path = '/dns-query';
	if (dnsOutbound == '' || isReservedOutboundTag(dnsOutbound) || !proxyOutboundExists(dnsOutbound))
		uci.unset('zakop', 'main', 'real_dns_outbound');
	else
		uci.set('zakop', 'main', 'real_dns_outbound', dnsOutbound);

	uci.set('zakop', 'main', 'real_dns_mode', mode);
	uci.set('zakop', 'main', 'real_dns_transport', protocol);
	uci.set('zakop', 'main', 'real_dns_server', host + ':' + port);
	uci.set('zakop', 'main', 'real_dns_port', port);
	uci.set('zakop', 'main', 'real_dns_server_name', tlsName);
	uci.set('zakop', 'main', 'real_dns_path', path);
	uci.set('zakop', 'main', 'dns_upstream_preset', preset);
	uci.set('zakop', 'main', 'dns_upstream_protocol', protocol);
	uci.set('zakop', 'main', 'dns_upstream_host', host);
	uci.set('zakop', 'main', 'dns_upstream_port', port);
	uci.set('zakop', 'main', 'dns_upstream_tls_name', tlsName);
	uci.set('zakop', 'main', 'dns_upstream_path', path);
	uci.set('zakop', 'main', 'real_dns_upstream', host + ':' + port);
	uci.unset('zakop', 'main', '_real_dns_doh');
}

function normalizeSimpleRuleState() {
	var routingMode = String(uci.get('zakop', 'main', 'routing_mode') || 'custom').trim();
	var action = String(uci.get('zakop', 'main', 'simple_action') || 'proxy').trim();
	var outbound = String(uci.get('zakop', 'main', 'simple_outbound') || '').trim();
	var domainInput = String(uci.get('zakop', 'main', 'simple_domain_input') || '').trim();
	var ipInput = String(uci.get('zakop', 'main', 'simple_ip_input') || '').trim();

	if (routingMode != 'simple')
		return;

	if (action != 'proxy' && action != 'direct' && action != 'block')
		action = 'proxy';

	if (action != 'proxy') {
		uci.set('zakop', 'main', 'simple_outbound', 'direct');
	} else if (outbound == '' || isReservedOutboundTag(outbound) || !proxyOutboundExists(outbound)) {
		outbound = firstProxyOutboundTag();
		if (outbound == '')
			uci.unset('zakop', 'main', 'simple_outbound');
		else
			uci.set('zakop', 'main', 'simple_outbound', outbound);
	} else {
		uci.set('zakop', 'main', 'simple_outbound', outbound);
	}

	uci.set('zakop', 'main', 'simple_action', action);

	if (domainInput == '') {
		if (optionValues('main', 'simple_domain_provider').length > 0)
			domainInput = 'provider';
		else if (optionValues('main', 'simple_domain_file').length > 0)
			domainInput = 'file';
		else
			domainInput = 'fields';
	}
	if (domainInput != 'text' && domainInput != 'provider' && domainInput != 'file')
		domainInput = 'fields';
	uci.set('zakop', 'main', 'simple_domain_input', domainInput);

	if (domainInput == 'provider') {
		unsetMainOptions([
			'simple_domain_equals', 'simple_domain_contains', 'simple_domain_starts_with', 'simple_domain_ends_with',
			'simple_exclude_domain_equals', 'simple_exclude_domain_contains', 'simple_exclude_domain_starts_with', 'simple_exclude_domain_ends_with',
			'simple_domain_file'
		]);
	} else if (domainInput == 'file') {
		unsetMainOptions([
			'simple_domain_equals', 'simple_domain_contains', 'simple_domain_starts_with', 'simple_domain_ends_with',
			'simple_exclude_domain_equals', 'simple_exclude_domain_contains', 'simple_exclude_domain_starts_with', 'simple_exclude_domain_ends_with',
			'simple_domain_provider'
		]);
	} else {
		uci.unset('zakop', 'main', 'simple_domain_provider');
		uci.unset('zakop', 'main', 'simple_domain_file');
	}

	if (ipInput == '') {
		if (optionValues('main', 'simple_ip_provider').length > 0)
			ipInput = 'provider';
		else if (optionValues('main', 'simple_ip_file').length > 0 || optionValues('main', 'simple_file').length > 0)
			ipInput = 'file';
		else
			ipInput = 'list';
	}
	if (ipInput != 'text' && ipInput != 'provider' && ipInput != 'file')
		ipInput = 'list';
	uci.set('zakop', 'main', 'simple_ip_input', ipInput);

	if (ipInput == 'provider') {
		unsetMainOptions([ 'simple_ip_cidr', 'simple_ip_file', 'simple_file' ]);
	} else if (ipInput == 'file') {
		unsetMainOptions([ 'simple_ip_cidr', 'simple_ip_provider', 'simple_file' ]);
	} else {
		uci.unset('zakop', 'main', 'simple_ip_provider');
		uci.unset('zakop', 'main', 'simple_ip_file');
		uci.unset('zakop', 'main', 'simple_file');
	}
}

function forceGeneralState() {
	uci.set('zakop', 'main', 'fakeip_enabled', '1');
	normalizeDNSState();
	normalizeSimpleRuleState();
}

return view.extend({
	load: function() {
		return uci.load('zakop').then(function() {
			var singboxBin = uci.get('zakop', 'main', 'singbox_bin') || '/usr/libexec/zakop/sing-box';

			zakopUI.syncRulesTab();

			return Promise.all([
				commandResult('/etc/init.d/zakop', [ 'status' ]),
				commandResult('/etc/init.d/zakop', [ 'enabled' ]),
				commandResult('/bin/pidof', [ 'zakopd' ]),
				commandResult('/bin/pidof', [ 'sing-box' ]),
				commandResult('/usr/bin/zakopd', [ 'version' ]),
				commandResult(singboxBin, [ 'version' ])
			]);
		});
	},

	handleSave: function() {
		return this.map.save(forceGeneralState).then(function() {
			return ui.changes.init();
		});
	},

	handleSaveApply: function(ev) {
		return this.handleSave(ev)
			.then(function() {
				return zakopUI.applyAndRestart();
			});
	},

	handleService: function(action) {
		var chain = Promise.resolve();

		if (action == 'start') {
			chain = chain
				.then(function() {
					return fs.exec('/sbin/uci', [ 'set', 'zakop.main.enabled=1' ]);
				})
				.then(function(res) {
					if (res.code)
						throw new Error(res.stderr || res.stdout || _('Update failed'));

					return fs.exec('/sbin/uci', [ 'commit', 'zakop' ]);
				});
		}

		return chain
			.then(function(res) {
				if (res && res.code)
					throw new Error(res.stderr || res.stdout || _('Update failed'));

				return fs.exec('/etc/init.d/zakop', [ action ]);
			})
			.then(function(res) {
				if (res.code)
					throw new Error(res.stderr || res.stdout || _('Update failed'));

				return waitForServiceState(action == 'start', 40);
			})
			.then(function() {
				window.location.reload();
			});
	},

	handleAutostart: function(action) {
		return fs.exec('/etc/init.d/zakop', [ action ])
			.then(function(res) {
				if (res.code)
					throw new Error(res.stderr || res.stdout || _('Update failed'));

				window.location.reload();
			});
	},

	handleUpgrade: function() {
		ui.showModal(_('zakop update'), [
			E('p', { 'class': 'spinning' }, [ _('Downloading and installing the latest release...') ])
		]);

		return fs.exec('/usr/share/zakop/upgrade.sh', [ '--luci' ])
			.then(function(res) {
				if (!res || res.code) {
					ui.showModal(_('zakop update'), [
						E('p', { 'class': 'alert-message danger' }, [
							(res && (res.stderr || res.stdout)) || _('Update failed')
						]),
						E('div', { 'class': 'right' }, [
							E('button', {
								'class': 'btn cbi-button',
								'click': ui.hideModal
							}, [ _('Close') ])
						])
					]);
					return;
				}

				ui.showModal(_('zakop update'), [
					E('p', { 'class': 'spinning' }, [ _('Update installed. Reconnecting to LuCI...') ])
				]);
				window.setTimeout(function() {
					window.location.reload();
				}, 5000);
			})
			.catch(function() {
				ui.showModal(_('zakop update'), [
					E('p', { 'class': 'spinning' }, [ _('Connection interrupted. Reconnecting to verify the installed version...') ])
				]);
				window.setTimeout(function() {
					window.location.reload();
				}, 5000);
			});
	},

	showUpgradeConfirmation: function() {
		ui.showModal(_('zakop update'), [
			E('p', {}, [ _('Install the latest zakop release now? The zakop service will restart.') ]),
			E('div', { 'class': 'right' }, [
				E('button', {
					'class': 'btn cbi-button',
					'click': ui.hideModal
				}, [ _('Cancel') ]), ' ',
				E('button', {
					'class': 'btn cbi-button cbi-button-positive important',
					'click': L.bind(function() {
						return this.handleUpgrade();
					}, this)
				}, [ _('Update') ])
			])
		]);
	},

	refreshUpdateState: function() {
		return commandResult('/usr/share/zakop/check-version.sh', [])
			.then(L.bind(function(res) {
				var update = parseUpdateState(res);
				var latestField = document.getElementById('cbi-zakop-main-_latest_version');
				var statusField = document.getElementById('cbi-zakop-main-_update_status');
				var updateField = document.getElementById('cbi-zakop-main-_zakop_update');
				var latestOutput = latestField ? latestField.querySelector('output') : null;
				var statusOutput = statusField ? statusField.querySelector('output') : null;
				var updateButton = updateField ? updateField.querySelector('button') : null;
				var available = update.status == 'available';

				this.updateState.current = update.current;
				this.updateState.latest = update.latest;
				this.updateState.status = update.status;

				if (latestOutput)
					latestOutput.textContent = update.latest || '-';
				if (statusOutput)
					statusOutput.textContent = updateStatusText(update);
				if (updateButton) {
					updateButton.disabled = !available;
					updateButton.textContent = available ? _('Update') : updateStatusText(update);
					updateButton.classList.toggle('cbi-button-apply', available);
					updateButton.classList.toggle('cbi-button-button', !available);
				}
			}, this));
	},

	render: function(data) {
		var m, s, o, state, serviceRunning, autostartEnabled, updateAvailable;

		zakopUI.syncRulesTab();

		data = data || [];
		this.updateState = this.updateState || {
			current: '',
			latest: '',
			status: 'checking'
		};
		state = {
			service: data[0],
			autostart: data[1],
			zakopd: data[2],
			singbox: data[3],
			zakopdVersion: data[4],
			singboxVersion: data[5],
			update: this.updateState
		};
		serviceRunning = state.service && state.service.code == 0;
		autostartEnabled = state.autostart && state.autostart.code == 0;
		updateAvailable = state.update.status == 'available';

		m = new form.Map('zakop', _('zakop'));
		this.map = m;

		s = m.section(form.NamedSection, 'main', 'main', _('General'));

		o = s.option(form.DummyValue, '_zakop_status', _('zakop status'));
		o.cfgvalue = function() {
			return serviceStatus(state.service);
		};

		o = s.option(form.DummyValue, '_singbox_status', _('sing-box status'));
		o.cfgvalue = function() {
			return processStatus(state.singbox);
		};

		o = s.option(form.DummyValue, '_autostart_status', _('Autostart'));
		o.cfgvalue = function() {
			return autostartStatus(state.autostart);
		};

		o = s.option(form.DummyValue, '_zakopd_version', _('zakopd version'));
		o.cfgvalue = function() {
			return outputLine(state.zakopdVersion);
		};

		o = s.option(form.DummyValue, '_singbox_version', _('sing-box version'));
		o.cfgvalue = function() {
			return outputLine(state.singboxVersion);
		};

		o = s.option(form.DummyValue, '_latest_version', _('Latest zakop release'));
		o.cfgvalue = function() {
			return state.update.latest || '-';
		};

		o = s.option(form.DummyValue, '_update_status', _('Update status'));
		o.cfgvalue = function() {
			return updateStatusText(state.update);
		};

		o = s.option(form.ListValue, 'update_via', _('Update via'), _('Choose how zakop release checks and downloads are performed. Save & Apply before updating.'));
		o.value('direct', 'direct');
		o.value('proxy', 'proxy');
		o.default = 'direct';
		o.rmempty = false;

		o = s.option(form.ListValue, 'update_outbound', _('Update outbound'));
		addProxyOutboundChoices(o);
		o.depends('update_via', 'proxy');
		o.rmempty = false;

		o = s.option(form.Button, '_zakop_update', _('zakop update'));
		o.inputtitle = updateAvailable ? _('Update') : updateStatusText(state.update);
		o.inputstyle = updateAvailable ? 'apply' : 'button';
		o.readonly = !updateAvailable;
		o.onclick = L.bind(function() {
			this.showUpgradeConfirmation();
		}, this);

		o = s.option(form.Button, '_service', _('Service'));
		o.inputtitle = serviceRunning ? _('Stop') : _('Start');
		o.inputstyle = serviceRunning ? 'reset' : 'apply';
		o.onclick = L.bind(function() {
			return this.handleService(serviceRunning ? 'stop' : 'start').catch(function(err) {
				ui.addNotification(null, E('p', {}, [ err.message || err ]), 'danger');
			});
		}, this);

		o = s.option(form.Button, '_autostart', _('Autostart'));
		o.inputtitle = autostartEnabled ? _('Disable') : _('Enable');
		o.inputstyle = autostartEnabled ? 'reset' : 'apply';
		o.onclick = L.bind(function() {
			return this.handleAutostart(autostartEnabled ? 'disable' : 'enable').catch(function(err) {
				ui.addNotification(null, E('p', {}, [ err.message || err ]), 'danger');
			});
		}, this);

		if (zakopI18n.ruAvailable()) {
			o = s.option(form.ListValue, 'language', _('Language'));
			o.value('en', _('English'));
			o.value('ru', _('Russian'));
			o.default = 'en';
			o.rmempty = false;
		}

		o = s.option(form.ListValue, 'dns_upstream_preset', _('DNS'));
		o.value('cloudflare', _('Cloudflare'));
		o.value('google', _('Google'));
		o.value('custom', _('Custom'));
		o.default = 'cloudflare';
		o.rmempty = false;

		o = s.option(form.ListValue, 'real_dns_mode', _('DNS mode'));
		o.value('direct', 'direct');
		o.value('proxy', 'proxy');
		o.default = 'direct';
		o.rmempty = false;

		o = s.option(form.ListValue, 'real_dns_outbound', _('DNS outbound'));
		addProxyOutboundChoices(o);
		o.depends('real_dns_mode', 'proxy');
		o.rmempty = false;

		o = s.option(form.ListValue, 'real_dns_transport', _('DNS transport'));
		o.value('udp', _('UDP'));
		o.value('tcp', _('TCP'));
		o.value('tls', _('DNS over TLS'));
		o.value('https', _('DNS over HTTPS'));
		o.default = 'udp';
		o.rmempty = false;

		o = s.option(form.Value, 'real_dns_server', _('Server'));
		o.depends('dns_upstream_preset', 'custom');
		o.placeholder = '1.1.1.1:53';
		o.rmempty = false;

		o = s.option(form.Value, 'real_dns_server_name', _('DNS server name'));
		o.depends({ 'dns_upstream_preset': 'custom', 'real_dns_transport': 'tls' });
		o.placeholder = 'cloudflare-dns.com';

		o = s.option(form.Value, '_real_dns_doh', _('DoH server/path'));
		o.depends({ 'dns_upstream_preset': 'custom', 'real_dns_transport': 'https' });
		o.placeholder = 'cloudflare-dns.com/dns-query';
		o.cfgvalue = function() {
			var serverName = String(uci.get('zakop', 'main', 'real_dns_server_name') || '').trim();
			var path = String(uci.get('zakop', 'main', 'real_dns_path') || '').trim();

			if (serverName == '' && path == '')
				return '';
			if (path == '')
				path = '/dns-query';
			return serverName + path;
		};
		o.write = function(section_id, formvalue) {
			var parts = splitDoHValue(formvalue, uci.get('zakop', 'main', 'real_dns_server_name'), uci.get('zakop', 'main', 'real_dns_path'));

			uci.set('zakop', section_id, 'real_dns_server_name', parts[0]);
			uci.set('zakop', section_id, 'real_dns_path', parts[1]);
			uci.unset('zakop', section_id, '_real_dns_doh');
		};

		o = s.option(form.ListValue, 'routing_mode', _('Routing mode'));
		o.value('custom', _('Custom'));
		o.value('simple', _('Simple'));
		o.value('global', _('Global'));
		o.default = 'custom';

		o = s.option(form.ListValue, 'simple_action', _('Simple action'));
		o.value('proxy', 'proxy');
		o.value('direct', 'direct');
		o.value('block', 'block');
		o.default = 'proxy';
		o.rmempty = false;
		o.retain = true;
		o.depends('routing_mode', 'simple');

		o = s.option(form.ListValue, 'simple_outbound', _('Simple outbound'));
		addProxyOutboundChoices(o);
		o.rmempty = false;
		o.retain = true;
		o.depends({ 'routing_mode': 'simple', 'simple_action': 'proxy' });

		o = s.option(form.ListValue, 'simple_domain_input', _('Simple domain input'));
		o.value('fields', _('Fields'));
		o.value('text', _('Textbox'));
		o.value('file', _('File paths'));
		o.value('provider', _('Providers'));
		o.default = 'fields';
		o.rmempty = false;
		o.retain = true;
		o.depends('routing_mode', 'simple');
		o.cfgvalue = function(section_id) {
			var value = String(uci.get('zakop', section_id, 'simple_domain_input') || '').trim();

			if (value != '')
				return value;
			if (optionValues(section_id, 'simple_domain_provider').length > 0)
				return 'provider';
			if (optionValues(section_id, 'simple_domain_file').length > 0)
				return 'file';
			return 'fields';
		};

		addSimpleProviderList(s, 'simple_domain_provider', _('Domain providers'), 'domain', 'simple_domain_input', 'provider');

		addSimpleDynamicList(s, 'simple_domain_equals', _('Equals'), 'simple_domain_input', 'fields', 'example.com');
		addSimpleDynamicList(s, 'simple_domain_ends_with', _('Ends with'), 'simple_domain_input', 'fields', '.example.com');
		addSimpleTextList(s, '_simple_domain_equals_text', 'simple_domain_equals', _('Equals text'), 'simple_domain_input', 'text', 'example.com\nexample.org');
		addSimpleTextList(s, '_simple_domain_ends_with_text', 'simple_domain_ends_with', _('Ends with text'), 'simple_domain_input', 'text', '.example.com');
		addSimpleDynamicList(s, 'simple_domain_file', _('Domain file paths'), 'simple_domain_input', 'file', '/etc/zakop/domains.txt');

		o = s.option(form.ListValue, 'simple_ip_input', _('Simple IP input'));
		o.value('list', _('List'));
		o.value('text', _('Textbox'));
		o.value('file', _('File paths'));
		o.value('provider', _('Providers'));
		o.default = 'list';
		o.rmempty = false;
		o.retain = true;
		o.depends('routing_mode', 'simple');
		o.cfgvalue = function(section_id) {
			var value = String(uci.get('zakop', section_id, 'simple_ip_input') || '').trim();

			if (value != '')
				return value;
			if (optionValues(section_id, 'simple_ip_provider').length > 0)
				return 'provider';
			if (optionValues(section_id, 'simple_ip_file').length > 0 || optionValues(section_id, 'simple_file').length > 0)
				return 'file';
			return 'list';
		};

		addSimpleProviderList(s, 'simple_ip_provider', _('IP providers'), 'ip', 'simple_ip_input', 'provider');
		addSimpleDynamicList(s, 'simple_ip_cidr', _('IP/CIDR list'), 'simple_ip_input', 'list', '1.1.1.1');
		addSimpleTextList(s, '_simple_ip_cidr_text', 'simple_ip_cidr', _('IP/CIDR text'), 'simple_ip_input', 'text', '1.1.1.1\n8.8.8.0/24');
		addSimpleDynamicList(s, 'simple_ip_file', _('IP/CIDR file paths'), 'simple_ip_input', 'file', '/etc/zakop/ipv4-cidr.txt');

		o = s.option(form.ListValue, 'default_outbound', _('Default outbound'));
		o.value('direct', 'direct');
		o.default = 'direct';

		return m.render().then(L.bind(function(node) {
			window.setTimeout(L.bind(this.refreshUpdateState, this), 0);
			return node;
		}, this));
	}
});

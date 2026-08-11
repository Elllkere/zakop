'use strict';
'require fs';
'require form';
'require ui';
'require uci';
'require view';
'require neto.i18n as netoI18n';
'require neto.ui as netoUI';

var _ = netoI18n.translate;
var communityDomainProviders = [
	{
		section: 'community_telegram_domains',
		label: 'Telegram domains',
		type: 'domain',
		source: 'url',
		url: 'https://raw.githubusercontent.com/itdoginfo/allow-domains/refs/heads/main/Services/telegram.lst'
	},
	{
		section: 'community_tiktok_domains',
		label: 'TikTok domains',
		type: 'domain',
		source: 'url',
		url: 'https://raw.githubusercontent.com/itdoginfo/allow-domains/refs/heads/main/Services/tiktok.lst'
	},
	{
		section: 'community_twitter_domains',
		label: 'Twitter domains',
		type: 'domain',
		source: 'url',
		url: 'https://raw.githubusercontent.com/itdoginfo/allow-domains/refs/heads/main/Services/twitter.lst'
	},
	{
		section: 'community_youtube_domains',
		label: 'YouTube domains',
		type: 'domain',
		source: 'url',
		url: 'https://raw.githubusercontent.com/itdoginfo/allow-domains/refs/heads/main/Services/youtube.lst'
	},
	{
		section: 'community_meta_domains',
		label: 'Meta domains',
		type: 'domain',
		source: 'url',
		url: 'https://raw.githubusercontent.com/itdoginfo/allow-domains/refs/heads/main/Services/meta.lst'
	},
	{
		section: 'community_discord_domains',
		label: 'Discord domains',
		type: 'domain',
		source: 'url',
		url: 'https://raw.githubusercontent.com/itdoginfo/allow-domains/refs/heads/main/Services/discord.lst'
	},
	{
		section: 'community_anime_domains',
		label: 'Anime domains',
		type: 'domain',
		source: 'url',
		url: 'https://raw.githubusercontent.com/itdoginfo/allow-domains/refs/heads/main/Categories/anime.lst'
	}
];

var builtinIPProviders = [
	{
		section: 'cloudflare_ipv4',
		label: 'Cloudflare IPv4',
		type: 'ip',
		source: 'url',
		url: 'https://www.cloudflare.com/ips-v4/',
		update_minute: '5'
	},
	{
		section: 'telegram_ipv4',
		label: 'Telegram IPv4',
		type: 'ip',
		source: 'url',
		url: 'https://core.telegram.org/resources/cidr.txt',
		update_minute: '10'
	},
	{
		section: 'akamai_ipv4',
		label: 'Akamai IPv4',
		type: 'ip',
		source: 'script',
		script_path: '/usr/share/neto/providers/akamai-ipv4.sh',
		update_minute: '15'
	},
	{
		section: 'aws_ipv4',
		label: 'AWS CDN IPv4',
		type: 'ip',
		source: 'script',
		script_path: '/usr/share/neto/providers/aws-ipv4.sh',
		update_minute: '20'
	},
	{
		section: 'aws_full_ipv4',
		label: 'AWS Full IPv4 (may affect game ping)',
		type: 'ip',
		source: 'script',
		script_path: '/usr/share/neto/providers/aws-full-ipv4.sh',
		update_minute: '25'
	},
	{
		section: 'aws_full_eu_ipv4',
		label: 'AWS Full EU IPv4 (may affect game ping)',
		type: 'ip',
		source: 'script',
		script_path: '/usr/share/neto/providers/aws-full-eu-ipv4.sh',
		update_minute: '30'
	},
	{
		section: 'google_cloud_eu_ipv4',
		label: 'Google Cloud Europe IPv4',
		type: 'ip',
		source: 'script',
		script_path: '/usr/share/neto/providers/google-cloud-eu-ipv4.sh',
		update_minute: '35'
	}
];

function normalizeProviders() {
	var firstOutbound = firstProxyOutboundTag();

	uci.sections('neto', 'provider', function(section, sid) {
		uci.unset('neto', sid, 'enabled');

		if (uci.get('neto', sid, 'label') == null)
			uci.set('neto', sid, 'label', uci.get('neto', sid, 'name') || sid);

		if (uci.get('neto', sid, 'type') == null)
			uci.set('neto', sid, 'type', 'domain');

		if (uci.get('neto', sid, 'source') == null)
			uci.set('neto', sid, 'source', 'url');

		if (uci.get('neto', sid, 'auto_update') == null)
			uci.set('neto', sid, 'auto_update', '0');

		if (uci.get('neto', sid, 'update_schedule') == null)
			uci.set('neto', sid, 'update_schedule', 'time');

		if (uci.get('neto', sid, 'update_hour') == null)
			uci.set('neto', sid, 'update_hour', '0');

		if (uci.get('neto', sid, 'update_minute') == null)
			uci.set('neto', sid, 'update_minute', '5');

		if (uci.get('neto', sid, 'update_interval_minutes') == null)
			uci.set('neto', sid, 'update_interval_minutes', '360');

		if (uci.get('neto', sid, 'update_via') == null)
			uci.set('neto', sid, 'update_via', 'direct');

		if (String(uci.get('neto', sid, 'update_via') || 'direct').trim() == 'proxy') {
			var outbound = String(uci.get('neto', sid, 'update_outbound') || '').trim();

			if (!proxyOutboundTagExists(outbound)) {
				if (firstOutbound != '')
					uci.set('neto', sid, 'update_outbound', firstOutbound);
				else
					uci.unset('neto', sid, 'update_outbound');
			}
		} else {
			uci.unset('neto', sid, 'update_outbound');
		}
	});
}

function cleanValues(value) {
	var values = [];

	if (Array.isArray(value)) {
		values = value;
	} else if (value != null) {
		values = String(value).split(/\s+/);
	}

	var out = [];
	var seen = {};
	for (var i = 0; i < values.length; i++) {
		var item = String(values[i] || '').trim();

		if (item == '' || seen[item])
			continue;

		seen[item] = true;
		out.push(item);
	}

	return out;
}

function optionValues(section_id, option) {
	return cleanValues(uci.get('neto', section_id, option));
}

function activeProviderNames() {
	var names = {};

	uci.sections('neto', 'provider', function(section, sid) {
		names[sid] = true;

		var name = String(uci.get('neto', sid, 'name') || '').trim();
		if (name != '')
			names[name] = true;
	});

	return names;
}

function referencedProviders(section_id) {
	return cleanValues([]
		.concat(optionValues(section_id, 'domain_provider'))
		.concat(optionValues(section_id, 'ip_provider'))
		.concat(optionValues(section_id, 'provider')));
}

function validateProviderReferences() {
	var providers = activeProviderNames();
	var error = null;

	uci.sections('neto', 'rule', function(section, sid) {
		if (error != null || uci.get('neto', sid, 'enabled') == '0')
			return;

		var ruleName = String(uci.get('neto', sid, 'name') || sid).trim();
		var refs = referencedProviders(sid);

		for (var i = 0; i < refs.length; i++) {
			if (!providers[refs[i]]) {
				error = _('Rule "%s" references missing provider "%s". Remove the provider from the rule before deleting it.').format(ruleName, refs[i]);
				return;
			}
		}
	});

	if (error != null)
		throw new Error(error);
}

function firstProxyOutboundTag() {
	var first = '';

	uci.sections('neto', 'outbound', function(section, sid) {
		var tag = String(section.tag || sid || section['.name'] || '').trim();

		if (first == '' && tag != '' && tag != 'direct' && tag != 'blocked' && tag != 'block' && tag != 'proxy_default')
			first = tag;
	});

	return first;
}

function proxyOutboundTagExists(wanted) {
	var found = false;

	wanted = String(wanted || '').trim();
	if (wanted == '')
		return false;

	uci.sections('neto', 'outbound', function(section, sid) {
		var tag = String(section.tag || sid || section['.name'] || '').trim();

		if (tag == wanted && tag != 'direct' && tag != 'blocked' && tag != 'block' && tag != 'proxy_default')
			found = true;
	});
	uci.sections('neto', 'outbound_pool', function(section, sid) {
		var tag = String(section.tag || sid || section['.name'] || '').trim();

		if (tag == wanted && tag != 'direct' && tag != 'blocked' && tag != 'block' && tag != 'proxy_default')
			found = true;
	});

	return found;
}

function addProxyOutboundChoices(option) {
	var first = firstProxyOutboundTag();

	uci.sections('neto', 'outbound', function(section, sid) {
		var tag = String(section.tag || sid || section['.name'] || '').trim();
		var label = String(section.label || section.name || tag).trim();

		if (tag == '' || tag == 'direct' || tag == 'blocked' || tag == 'block' || tag == 'proxy_default')
			return;

		option.value(tag, label || tag);
	});
	uci.sections('neto', 'outbound_pool', function(section, sid) {
		var tag = String(section.tag || sid || section['.name'] || '').trim();
		var label = String(section.label || section.name || tag).trim();

		if (tag != '' && tag != 'direct' && tag != 'blocked' && tag != 'block' && tag != 'proxy_default')
			option.value(tag, _('Pool: %s').format(label || tag));
	});

	option.default = first;
	option.rmempty = false;
	return first;
}

function addUpdateIntervalChoices(option) {
	option.value('15', _('Every 15 minutes'));
	option.value('30', _('Every 30 minutes'));
	option.value('60', _('Every hour'));
	option.value('120', _('Every 2 hours'));
	option.value('180', _('Every 3 hours'));
	option.value('360', _('Every 6 hours'));
	option.value('720', _('Every 12 hours'));
	option.value('1440', _('Every day'));
}

function providerURLExists(url) {
	var exists = false;

	url = String(url || '').trim();
	uci.sections('neto', 'provider', function(section, sid) {
		if (String(uci.get('neto', sid, 'url') || '').trim() == url)
			exists = true;
	});

	return exists;
}

function providerScriptExists(scriptPath) {
	var exists = false;

	scriptPath = String(scriptPath || '').trim();
	uci.sections('neto', 'provider', function(section, sid) {
		if (String(uci.get('neto', sid, 'script_path') || '').trim() == scriptPath)
			exists = true;
	});

	return exists;
}

function uniqueProviderSection(base) {
	var section = base;
	var n = 2;

	while (uci.get('neto', section) != null) {
		section = base + '_' + n;
		n++;
	}

	return section;
}

function addProviderPreset(def) {
	var section;
	var source = String(def.source || 'url').trim();

	if (source == 'script') {
		if (providerScriptExists(def.script_path))
			return false;
	} else if (providerURLExists(def.url)) {
		return false;
	}

	section = uniqueProviderSection(def.section);
	uci.add('neto', 'provider', section);
	uci.set('neto', section, 'label', def.label);
	uci.set('neto', section, 'type', def.type || 'domain');
	uci.set('neto', section, 'source', source);
	if (source == 'script')
		uci.set('neto', section, 'script_path', def.script_path);
	else
		uci.set('neto', section, 'url', def.url);
	uci.set('neto', section, 'auto_update', '0');
	uci.set('neto', section, 'update_schedule', 'time');
	uci.set('neto', section, 'update_hour', '0');
	uci.set('neto', section, 'update_minute', def.update_minute || '5');
	uci.set('neto', section, 'update_interval_minutes', '360');
	uci.set('neto', section, 'update_via', 'direct');
	return true;
}

return view.extend({
	load: function() {
		return uci.load('neto').then(function() {
			netoUI.syncRulesTab();
		});
	},

	handleSave: function() {
		return this.map.save(normalizeProviders)
			.then(function() {
				validateProviderReferences();
				return ui.changes.init();
			})
			.catch(function(err) {
				ui.addNotification(null, E('p', {}, [ err.message || err ]), 'danger');
				if (err != null && typeof err == 'object')
					err.notified = true;
				return Promise.reject(err);
			});
	},

	handleSaveCommitConfig: function() {
		return this.handleSave()
			.then(function() {
				return fs.exec('/sbin/uci', [ 'commit', 'neto' ]);
			})
			.then(function(res) {
				if (res.code)
					throw new Error(res.stderr || res.stdout || _('Commit failed'));

				return uci.load('neto');
			});
	},

	handleSaveApply: function(ev) {
		return this.handleSave(ev)
			.then(function() {
				return netoUI.applyAndRestart();
			});
	},

	handleProviderUpdate: function(section_id) {
		return this.handleSaveCommitConfig()
			.then(function() {
				return fs.exec('/usr/bin/netod', [ 'providers', 'update', section_id ]);
			})
			.then(function(res) {
				if (res.code)
					throw new Error(res.stderr || res.stdout || _('Update failed'));

				return fs.exec('/etc/init.d/neto', [ 'restart' ]);
			})
			.then(function() {
				window.location.reload();
			});
	},

	handleProviderUpdateAll: function() {
		var failures;
		var names;

		this.providerUpdateAllRunning = true;
		this.setProviderUpdateAllButton(true);

		return this.handleSaveCommitConfig()
			.then(L.bind(function() {
				names = [];
				uci.sections('neto', 'provider', function(section, sid) {
					names.push(String(section['.name'] || sid));
				});
				if (names.length == 0)
					throw new Error(_('No providers configured'));

				return this.runProviderUpdates(names);
			}, this))
			.then(function(updateFailures) {
				failures = updateFailures;
				return fs.exec('/etc/init.d/neto', [ 'restart' ]);
			})
			.then(function(res) {
				if (res.code)
					throw new Error(res.stderr || res.stdout || _('Restart failed'));
				if (failures.length == 0)
					window.location.reload();
				return failures;
			});
	},

	runProviderUpdates: function(names) {
		var failures = [];
		var self = this;
		var chain = Promise.resolve();

		names.forEach(function(name, index) {
			chain = chain.then(function() {
				self.setProviderUpdateAllButton(true, index + 1, names.length);
				return fs.exec('/usr/bin/netod', [ 'providers', 'update', name ])
					.then(function(res) {
						if (res.code)
							failures.push({ name: name, error: res.stderr || res.stdout || _('Update failed') });
					}, function(err) {
						failures.push({ name: name, error: err.message || err });
					});
			});
		});

		return chain.then(function() { return failures; });
	},

	setProviderUpdateAllButton: function(running, current, total) {
		var buttons = document.querySelectorAll('[data-neto-providers-update-all]');
		var text = running && total ? _('Updating %d/%d…').format(current, total) : (running ? _('Updating all…') : _('Update all'));

		for (var i = 0; i < buttons.length; i++) {
			buttons[i].disabled = running ? true : null;
			buttons[i].textContent = text;
		}
	},

	showProviderUpdateFailures: function(failures) {
		ui.addNotification(null, E('div', {}, [
			E('p', {}, _('Some providers failed to update. Successful updates were kept.')),
			E('ul', {}, failures.map(function(item) {
				return E('li', {}, [ E('strong', {}, item.name + ': '), String(item.error || _('Update failed')) ]);
			})),
			E('p', {}, _('Reload the page to show successful updates.'))
		]), 'warning');
	},

	handleImportProviderPresets: function() {
		return this.map.save(normalizeProviders)
			.then(function() {
				var added = 0;

				for (var i = 0; i < communityDomainProviders.length; i++) {
					if (addProviderPreset(communityDomainProviders[i]))
						added++;
				}

				for (var j = 0; j < builtinIPProviders.length; j++) {
					if (addProviderPreset(builtinIPProviders[j]))
						added++;
				}

				if (added == 0) {
					ui.addNotification(null, E('p', {}, [ _('Provider presets already exist') ]), 'info');
					return Promise.resolve();
				}

				return uci.save('neto')
					.then(function(ok) {
						if (ok === false)
							throw new Error(_('Save failed'));

						return fs.exec('/sbin/uci', [ 'commit', 'neto' ]);
					})
					.then(function(res) {
						if (res.code)
							throw new Error(res.stderr || res.stdout || _('Commit failed'));

						return uci.load('neto');
					})
					.then(function() {
						return fs.exec('/etc/init.d/neto', [ 'restart' ]);
					})
					.then(function() {
						window.location.reload();
					});
			});
	},

	render: function() {
		var m, s, o, self;

		netoUI.syncRulesTab();

		m = new form.Map('neto', _('neto'));
		this.map = m;
		self = this;

		s = m.section(form.GridSection, 'provider', _('Providers'));
		s.anonymous = false;
		s.addremove = true;
		s.modaltitle = _('Provider details');
		s.sectiontitle = function(section_id) {
			return uci.get('neto', section_id, 'label') || uci.get('neto', section_id, 'name') || section_id;
		};
		s.renderSectionAdd = function() {
			var el = form.GridSection.prototype.renderSectionAdd.apply(this, arguments);

			el.appendChild(E('button', {
				'class': 'cbi-button cbi-button-action',
				'click': function(ev) {
					ev.preventDefault();
					return self.handleImportProviderPresets().catch(function(err) {
						ui.addNotification(null, E('p', {}, [ err.message || err ]), 'danger');
					});
				}
			}, _('Import provider presets')));

			el.appendChild(E('button', {
				'class': 'cbi-button cbi-button-action',
				'style': 'margin-left:.5em',
				'data-neto-providers-update-all': '1',
				'disabled': self.providerUpdateAllRunning ? true : null,
				'click': function(ev) {
					ev.preventDefault();
					return self.handleProviderUpdateAll().then(function(failures) {
						self.providerUpdateAllRunning = false;
						self.setProviderUpdateAllButton(false);
						if (failures.length)
							self.showProviderUpdateFailures(failures);
					}, function(err) {
						self.providerUpdateAllRunning = false;
						self.setProviderUpdateAllButton(false);
						ui.addNotification(null, E('p', {}, [ err.message || err ]), 'danger');
					});
				}
			}, self.providerUpdateAllRunning ? _('Updating all…') : _('Update all')));

			return el;
		};

		o = s.option(form.Value, 'label', _('Name'));
		o.cfgvalue = function(section_id) {
			return uci.get('neto', section_id, 'label') || uci.get('neto', section_id, 'name') || section_id;
		};
		o.write = function(section_id, formvalue) {
			var label = String(formvalue || '').trim();
			uci.set('neto', section_id, 'label', label || section_id);
		};
		o.rmempty = false;
		o.modalonly = true;

		o = s.option(form.ListValue, 'type', _('Type'));
		o.value('domain', _('Domains'));
		o.value('ip', _('IP/CIDR'));
		o.default = 'domain';
		o.rmempty = false;

		o = s.option(form.ListValue, 'source', _('Source'));
		o.value('url', _('URL'));
		o.value('script', _('Script'));
		o.default = 'url';
		o.rmempty = false;

		o = s.option(form.Value, 'url', _('URL'));
		o.datatype = 'url';
		o.depends('source', 'url');
		o.rmempty = true;

		o = s.option(form.DummyValue, '_url_help', _('URL provider notes'));
		o.cfgvalue = function() {
			return _('Use a plain text URL with one domain, IPv4 address, or IPv4 CIDR per line. Use Script for JSON feeds or custom filtering.');
		};
		o.depends('source', 'url');
		o.modalonly = true;

		o = s.option(form.Value, 'script_path', _('Script path'));
		o.placeholder = '/usr/share/neto/providers/custom.sh';
		o.depends('source', 'script');
		o.rmempty = true;
		o.modalonly = true;

		o = s.option(form.DummyValue, '_script_help', _('Script provider notes'));
		o.cfgvalue = function() {
			return _('Use an absolute executable path. The script must print the final list to stdout or write it to NETO_PROVIDER_OUTPUT; one item per line. In proxy mode, neto exports NETO_PROVIDER_PROXY and HTTP_PROXY/HTTPS_PROXY/ALL_PROXY to the script.');
		};
		o.depends('source', 'script');
		o.modalonly = true;

		o = s.option(form.Flag, 'auto_update', _('Auto update'));
		o.enabled = '1';
		o.disabled = '0';
		o.default = '0';
		o.rmempty = false;
		o.editable = true;

		o = s.option(form.ListValue, 'update_schedule', _('Schedule'));
		o.value('time', _('Fixed time'));
		o.value('interval', _('Interval'));
		o.default = 'time';
		o.depends('auto_update', '1');
		o.rmempty = false;
		o.modalonly = true;

		o = s.option(form.ListValue, 'update_hour', _('Update time'));
		for (var hour = 0; hour < 24; hour++)
			o.value(String(hour), _('%d:00').format(hour));
		o.default = '0';
		o.depends({ 'auto_update': '1', 'update_schedule': 'time' });
		o.rmempty = false;
		o.modalonly = true;

		o = s.option(form.ListValue, 'update_minute', _('Update minute'));
		for (var minute = 0; minute < 60; minute++)
			o.value(String(minute), String(minute));
		o.default = '5';
		o.depends({ 'auto_update': '1', 'update_schedule': 'time' });
		o.rmempty = false;
		o.modalonly = true;

		o = s.option(form.ListValue, 'update_interval_minutes', _('Update interval'));
		addUpdateIntervalChoices(o);
		o.default = '360';
		o.depends({ 'auto_update': '1', 'update_schedule': 'interval' });
		o.rmempty = false;
		o.modalonly = true;

		o = s.option(form.ListValue, 'update_via', _('Update via'));
		o.value('direct', 'direct');
		o.value('proxy', 'proxy');
		o.default = 'direct';
		o.rmempty = false;

		o = s.option(form.ListValue, 'update_outbound', _('Update outbound'));
		var firstUpdateOutbound = addProxyOutboundChoices(o);
		o.depends('update_via', 'proxy');
		o.forcewrite = true;
		o.cfgvalue = function(section_id) {
			var value = String(uci.get('neto', section_id, 'update_outbound') || '').trim();

			return proxyOutboundTagExists(value) ? value : firstUpdateOutbound;
		};
		o.validate = function(section_id, value) {
			if (!proxyOutboundTagExists(value))
				return _('Create an outbound before selecting proxy update.');

			return true;
		};
		o.modalonly = true;

		o = s.option(form.DummyValue, 'item_count', _('Items'));
		o.cfgvalue = function(section_id) {
			return uci.get('neto', section_id, 'item_count') || '-';
		};

		o = s.option(form.DummyValue, 'last_update', _('Updated'));
		o.cfgvalue = function(section_id) {
			var value = uci.get('neto', section_id, 'last_update');
			var timestamp = Number(value);

			if (!timestamp)
				return '-';

			return new Date(timestamp * 1000).toLocaleString();
		};

		o = s.option(form.DummyValue, 'local_path', _('Local cache'));
		o.cfgvalue = function(section_id) {
			return uci.get('neto', section_id, 'local_path') || '-';
		};
		o.modalonly = true;

		o = s.option(form.Button, '_update', _('Update'));
		o.inputstyle = 'action';
		o.inputtitle = _('Update');
		o.cfgvalue = function() {
			return true;
		};
		o.modalonly = true;
		o.onclick = L.bind(function(ev, section_id) {
			return this.handleProviderUpdate(section_id).catch(function(err) {
				if (!(err != null && typeof err == 'object' && err.notified))
					ui.addNotification(null, E('p', {}, [ err.message || err ]), 'danger');
			});
		}, this);

		return m.render();
	}
});

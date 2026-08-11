'use strict';
'require fs';
'require form';
'require ui';
'require uci';
'require view';
'require neto.i18n as netoI18n';
'require neto.ui as netoUI';

var importPath = '/tmp/neto-import.txt';
var _ = netoI18n.translate;

function isReservedTag(tag) {
	return tag == 'direct' || tag == 'blocked' || tag == 'block' || tag == 'proxy_default';
}

function allowInsecureConfirm(ev, _section_id, value) {
	if (value == '1' && !confirm(_('Are you sure to allow insecure TLS?')))
		ev.target.firstElementChild.checked = null;
}

function dependsTLS(option) {
	option.depends({ 'type': 'vless', 'tls': '1' });
	option.depends({ 'type': 'trojan', 'tls': '1' });
	option.depends('type', 'hysteria2');
}

function dependsECH(option) {
	option.depends({ 'type': 'vless', 'tls': '1', 'ech': '1' });
	option.depends({ 'type': 'trojan', 'tls': '1', 'ech': '1' });
	option.depends({ 'type': 'hysteria2', 'ech': '1' });
}

function dependsReality(option) {
	option.depends({ 'type': 'vless', 'tls': '1', 'reality': '1' });
}

function dependsTransport(option, transport) {
	option.depends({ 'type': 'vless', 'transport': transport });
	option.depends({ 'type': 'trojan', 'transport': transport });
}

function addProxyOutboundChoices(option) {
	var first = '';

	uci.sections('neto', 'outbound', function(section, sid) {
		var tag = String(section.tag || sid || section['.name'] || '').trim();
		var label = String(section.label || section.name || tag).trim();

		if (tag == '' || isReservedTag(tag))
			return;

		if (first == '')
			first = tag;
		option.value(tag, label || tag);
	});
	uci.sections('neto', 'outbound_pool', function(section, sid) {
		var tag = String(section.tag || sid || section['.name'] || '').trim();
		var label = String(section.label || section.name || tag).trim();

		if (tag != '' && !isReservedTag(tag))
			option.value(tag, _('Pool: %s').format(label || tag));
	});

	option.default = first;
	option.rmempty = false;
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

function outboundTagExists(tag) {
	var found = false;

	tag = String(tag || '').trim();
	uci.sections('neto', 'outbound', function(section, sid) {
		var existing = String(section.tag || sid || section['.name'] || '').trim();

		if (existing == tag)
			found = true;
	});
	uci.sections('neto', 'outbound_pool', function(section, sid) {
		var existing = String(section.tag || sid || section['.name'] || '').trim();

		if (existing == tag)
			found = true;
	});

	return found;
}

function concreteOutboundTagExists(tag) {
	var found = false;

	tag = String(tag || '').trim();
	uci.sections('neto', 'outbound', function(section, sid) {
		var existing = String(section.tag || sid || section['.name'] || '').trim();

		if (existing == tag && !isReservedTag(existing))
			found = true;
	});

	return found;
}

function outboundDisplayLabels() {
	var labels = {};

	uci.sections('neto', 'outbound', function(section, sid) {
		var tag = String(section.tag || sid || section['.name'] || '').trim();
		var label = String(section.label || section.name || tag).trim();

		if (tag != '')
			labels[tag] = label || tag;
	});

	return labels;
}

function poolPriorityText(section_id) {
	var members = L.toArray(uci.get('neto', section_id, 'outbound'));
	var labels = outboundDisplayLabels();
	var full = [];
	var content = [];

	if (!members.length)
		return '-';

	for (var i = 0; i < members.length; i++) {
		var label = labels[members[i]] || members[i];

		full.push(label);
		if (i > 0)
			content.push(E('span', { 'style': 'flex:0 0 auto' }, '→'));
		content.push(E('span', {
			'title': label,
			'style': 'display:block;flex:0 1 auto;min-width:5ch;max-width:100%;overflow:hidden;text-overflow:ellipsis;white-space:nowrap'
		}, label));
	}

	return E('span', {
		'title': full.join(' → '),
		'style': 'display:flex;width:100%;min-width:0;max-width:100%;align-items:center;gap:.35em;overflow:hidden'
	}, content);
}

function outboundTag(section_id) {
	return String(uci.get('neto', section_id, 'tag') || section_id || '').trim();
}

function addNamedSectionValidator(el, section, reservedMessage, checkOutboundTags) {
	var nameEl = el.querySelector('.cbi-section-create-name');

	ui.addValidator(nameEl, 'uciname', true, L.bind(function(value) {
		var button = el.querySelector('.cbi-section-create > .cbi-button-add');
		var config = this.uciconfig || this.map.config;
		var name = String(value || '').trim();

		if (name == '') {
			button.disabled = true;
			return true;
		}

		if (isReservedTag(name)) {
			button.disabled = true;
			return reservedMessage;
		}

		if (uci.get(config, name)) {
			button.disabled = true;
			return _('Expecting: %s').format(_('unique UCI identifier'));
		}

		if (checkOutboundTags && outboundTagExists(name)) {
			button.disabled = true;
			return _('Expecting: %s').format(_('unique outbound tag'));
		}

		button.disabled = null;
		return true;
	}, section), 'blur', 'keyup');
}

function normalizeOutbounds() {
	uci.sections('neto', 'outbound', function(section, sid) {
		var tag = String(uci.get('neto', sid, 'tag') || '').trim();
		var label = String(uci.get('neto', sid, 'label') || uci.get('neto', sid, 'name') || '').trim();

		if (sid == 'proxy_default' || tag == 'proxy_default')
			return;

		if (tag == '')
			uci.set('neto', sid, 'tag', sid);

		if (label == '')
			uci.set('neto', sid, 'label', sid);

		if (uci.get('neto', sid, 'type') == null)
			uci.set('neto', sid, 'type', 'vless');
	});
}

function normalizeSubscriptions() {
	uci.sections('neto', 'subscription', function(section, sid) {
		if (uci.get('neto', sid, 'enabled') == null)
			uci.set('neto', sid, 'enabled', '1');

		if (uci.get('neto', sid, 'label') == null)
			uci.set('neto', sid, 'label', sid);

		if (uci.get('neto', sid, 'auto_update') == null)
			uci.set('neto', sid, 'auto_update', '0');

		if (uci.get('neto', sid, 'update_schedule') == null)
			uci.set('neto', sid, 'update_schedule', 'time');

		if (uci.get('neto', sid, 'update_hour') == null)
			uci.set('neto', sid, 'update_hour', '0');

		if (uci.get('neto', sid, 'update_interval_minutes') == null)
			uci.set('neto', sid, 'update_interval_minutes', '360');

		if (uci.get('neto', sid, 'update_via') == null)
			uci.set('neto', sid, 'update_via', 'direct');
	});
}

function normalizeAll() {
	normalizeOutbounds();
	normalizeSubscriptions();
}

return view.extend({
	load: function() {
		return uci.load('neto').then(function() {
			netoUI.syncRulesTab();
		});
	},

	handleSave: function() {
		return this.map.save(normalizeAll).then(function() {
			return ui.changes.init();
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

				uci.unload('neto');
				return uci.load('neto').then(function() {
					return ui.changes.init();
				});
			});
	},

	handleSaveApply: function(ev) {
		return this.handleSave(ev)
			.then(function() {
				return netoUI.applyAndRestart();
			});
	},

	showImportModal: function() {
		var importButton, cancelButton;
		var textarea = E('textarea', {
			'class': 'cbi-input-textarea',
			'style': 'width:100%',
			'rows': 8,
			'placeholder': 'vless://...\nhysteria2://...\nss://...\ntrojan://...'
		});
		var status = E('span', {
			'style': 'display:none;margin-left:1em'
		}, _('Importing...'));

		cancelButton = E('button', {
			'class': 'cbi-button cbi-button-neutral',
			'click': ui.hideModal
		}, _('Cancel'));
		importButton = E('button', {
			'class': 'cbi-button cbi-button-action',
			'click': L.bind(function(ev) {
				var value = String(textarea.value || '').trim();

				ev.preventDefault();
				if (value == '') {
					textarea.focus();
					return Promise.resolve();
				}

				importButton.disabled = true;
				cancelButton.disabled = true;
				textarea.disabled = true;
				importButton.textContent = _('Importing...');
				status.style.display = '';

				return this.handleManualImport(value).catch(function(err) {
					importButton.disabled = null;
					cancelButton.disabled = null;
					textarea.disabled = null;
					importButton.textContent = _('Import');
					status.style.display = 'none';
					ui.addNotification(null, E('p', {}, [ err.message || err ]), 'danger');
				});
			}, this)
		}, _('Import'));

		ui.showModal(_('Import outbounds'), [
			E('div', { 'class': 'cbi-value' }, [
				E('label', { 'class': 'cbi-value-title' }, _('Links')),
				E('div', { 'class': 'cbi-value-field' }, textarea)
			]),
			E('div', { 'class': 'right' }, [
				cancelButton,
				' ',
				importButton,
				status
			])
		]);
	},

	handleManualImport: function(value) {
		value = String(value || '').trim();

		if (value == '')
			return Promise.resolve();

		return fs.write(importPath, value + '\n', 384)
			.then(function() {
				return fs.exec('/usr/bin/netod', [ 'import-uri', '-file', importPath ]);
			})
			.then(function(res) {
				if (res.code)
					throw new Error(res.stderr || res.stdout || _('Import failed'));

				return fs.exec('/etc/init.d/neto', [ 'restart' ]);
			})
			.then(function() {
				window.location.reload();
			});
	},

	handleSubscriptionUpdate: function(section_id) {
		return this.handleSaveCommitConfig()
			.then(function() {
				return fs.exec('/usr/bin/netod', [ 'subscriptions', 'update', section_id ]);
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

	handleSubscriptionUpdateAll: function() {
		var failures;
		var names;

		this.subscriptionUpdateAllRunning = true;
		this.setSubscriptionUpdateAllButton(true);

		return this.handleSaveCommitConfig()
			.then(L.bind(function() {
				names = [];
				uci.sections('neto', 'subscription', function(section, sid) {
					if (String(section.enabled == null ? '1' : section.enabled) != '0')
						names.push(String(section['.name'] || sid));
				});
				if (names.length == 0)
					throw new Error(_('No enabled subscriptions configured'));

				return this.runSubscriptionUpdates(names);
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

	runSubscriptionUpdates: function(names) {
		var failures = [];
		var self = this;
		var chain = Promise.resolve();

		names.forEach(function(name, index) {
			chain = chain.then(function() {
				self.setSubscriptionUpdateAllButton(true, index + 1, names.length);
				return fs.exec('/usr/bin/netod', [ 'subscriptions', 'update', name ])
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

	setSubscriptionUpdateAllButton: function(running, current, total) {
		var buttons = document.querySelectorAll('[data-neto-subscriptions-update-all]');
		var text = running && total ? _('Updating %d/%d…').format(current, total) : (running ? _('Updating all…') : _('Update all'));

		for (var i = 0; i < buttons.length; i++) {
			buttons[i].disabled = running ? true : null;
			buttons[i].textContent = text;
		}
	},

	showSubscriptionUpdateFailures: function(failures) {
		ui.addNotification(null, E('div', {}, [
			E('p', {}, _('Some subscriptions failed to update. Successful updates were kept.')),
			E('ul', {}, failures.map(function(item) {
				return E('li', {}, [ E('strong', {}, item.name + ': '), String(item.error || _('Update failed')) ]);
			})),
			E('p', {}, _('Reload the page to show successful updates.'))
		]), 'warning');
	},

	handleOutboundLatencyTest: function() {
		var targets;

		this.latencyTesting = true;
		this.setOutboundLatencyButton(true);

		return this.handleSaveCommitConfig()
			.then(L.bind(function() {
				targets = [];
				uci.sections('neto', 'outbound', function(section, sid) {
					var tag = String(section.tag || section['.name'] || sid || '').trim();
					if (tag == '' || isReservedTag(tag))
						return;
					targets.push({
						tag: tag,
						label: String(section.label || section.name || tag),
						server: String(section.server || section.address || '') + (section.port ? ':' + section.port : '')
					});
				});
				if (targets.length == 0)
					throw new Error(_('No custom outbounds configured'));

				// map.save() redraws the GridSection. Start progress only after
				// the new table nodes have replaced the old ones.
				this.setOutboundLatencyStatus(_('Not tested'), '', '');
				return this.runOutboundLatencyTests(targets);
			}, this))
			.then(L.bind(function(report) {
				this.updateOutboundLatencyResults(report);
				ui.addNotification(null, E('p', {}, _('Latency test completed.')), 'info');
			}, this));
	},

	runOutboundLatencyTests: function(targets) {
		var report = { target: '', results: [] };
		var self = this;
		var chain = Promise.resolve();

		targets.forEach(function(target, index) {
			chain = chain.then(function() {
				self.setOutboundLatencyButton(true, index + 1, targets.length);
				self.setOutboundLatencyStatus(_('Testing…'), 'spinning', '', target.tag);

				return fs.exec('/usr/bin/netod', [ 'outbounds', 'latency', target.tag ])
					.then(function(res) {
						var itemReport;

						if (res.code) {
							report.results.push(self.outboundLatencyFailure(target, res.stderr || res.stdout || _('Latency test failed')));
							return;
						}
						try {
							itemReport = JSON.parse(String(res.stdout || ''));
							if (!Array.isArray(itemReport.results) || itemReport.results.length == 0)
								throw new Error(_('Invalid latency test response'));
							report.target = report.target || String(itemReport.target || '');
							itemReport.results[0].tag = target.tag;
							itemReport.results[0].label = itemReport.results[0].label || target.label;
							itemReport.results[0].server = itemReport.results[0].server || target.server;
							report.results.push(itemReport.results[0]);
						} catch (err) {
							report.results.push(self.outboundLatencyFailure(target, _('Invalid latency test response') + ': ' + (err.message || err)));
						}
					}, function(err) {
						report.results.push(self.outboundLatencyFailure(target, err.message || err));
					})
					.then(function() {
						self.updateOutboundLatencyResults(report);
					});
			});
		});

		return chain.then(function() { return report; });
	},

	outboundLatencyFailure: function(target, error) {
		return {
			tag: target.tag,
			label: target.label,
			server: target.server,
			ok: false,
			error: String(error || _('Latency test failed'))
		};
	},

	setOutboundLatencyButton: function(testing, current, total) {
		var buttons = document.querySelectorAll('[data-neto-latency-button]');
		var text = testing && total ? _('Testing %d/%d…').format(current, total) : (testing ? _('Testing…') : _('Test latency'));

		for (var i = 0; i < buttons.length; i++) {
			buttons[i].disabled = testing ? true : null;
			buttons[i].textContent = text;
		}
	},

	setOutboundLatencyStatus: function(text, className, title, tag) {
		var cells = document.querySelectorAll('[data-neto-latency-tag]');

		for (var i = 0; i < cells.length; i++) {
			if (tag && cells[i].getAttribute('data-neto-latency-tag') != tag)
				continue;
			cells[i].textContent = text;
			cells[i].className = className || '';
			cells[i].title = title || '';
		}
	},

	updateOutboundLatencyResults: function(report) {
		var results = Array.isArray(report && report.results) ? report.results : [];
		var byTag = Object.create(null);
		var best = null;
		var cells = document.querySelectorAll('[data-neto-latency-tag]');

		for (var i = 0; i < results.length; i++) {
			var result = results[i] || {};
			var tag = String(result.tag || '');

			if (tag == '')
				continue;
			byTag[tag] = result;
			if (result.ok && Number(result.latency_ms) > 0 && (best == null || Number(result.latency_ms) < best))
				best = Number(result.latency_ms);
		}

		for (var j = 0; j < cells.length; j++) {
			var cell = cells[j];
			var cellResult = byTag[cell.getAttribute('data-neto-latency-tag')];

			if (!cellResult) {
				cell.textContent = _('Not tested');
				cell.className = '';
				cell.title = '';
			} else if (cellResult.ok && Number(cellResult.latency_ms) > 0) {
				cell.textContent = String(cellResult.latency_ms) + ' ms';
				cell.className = Number(cellResult.latency_ms) == best ? 'label success' : 'label';
				cell.title = String(report && report.target || '');
			} else {
				cell.textContent = _('Failed');
				cell.className = 'label danger';
				cell.title = cellResult.error || _('Latency test failed');
			}
		}
	},

	render: function() {
		var m, s, o, self, pool, sub;

		netoUI.syncRulesTab();

		m = new form.Map('neto', _('neto'));
		this.map = m;
		self = this;

		s = m.section(form.GridSection, 'outbound', _('Outbounds'));
		s.anonymous = false;
		s.addremove = true;
		s.modaltitle = _('Outbound details');
		s.sectiontitle = function(section_id) {
			return uci.get('neto', section_id, 'label') || uci.get('neto', section_id, 'name') || section_id;
		};
		s.filter = function(section_id) {
			var tag = String(uci.get('neto', section_id, 'tag') || section_id || '').trim();
			return tag != 'proxy_default';
		};
		s.renderSectionAdd = function() {
			var el = form.GridSection.prototype.renderSectionAdd.apply(this, arguments);
			var latencyButton;
			addNamedSectionValidator(el, this, _('This tag is reserved'), true);
			el.appendChild(E('button', {
				'class': 'cbi-button cbi-button-action',
				'click': function(ev) {
					ev.preventDefault();
					return self.showImportModal();
				}
			}, _('Import')));
			latencyButton = E('button', {
				'class': 'cbi-button cbi-button-action',
				'style': 'margin-left:.5em',
				'data-neto-latency-button': '1',
				'disabled': self.latencyTesting ? true : null,
				'click': function(ev) {
					ev.preventDefault();
					return self.handleOutboundLatencyTest().then(function() {
						self.latencyTesting = false;
						self.setOutboundLatencyButton(false);
					}, function(err) {
						self.latencyTesting = false;
						self.setOutboundLatencyButton(false);
						self.setOutboundLatencyStatus(_('Not tested'), '', err.message || err);
						ui.addNotification(null, E('p', {}, [ err.message || err ]), 'danger');
					});
				}
			}, self.latencyTesting ? _('Testing…') : _('Test latency'));
			el.appendChild(latencyButton);

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
		o.validate = function(section_id, value) {
			var label = String(value || section_id || '').trim();

			if (label == '')
				return _('Name is required');

			return true;
		};
		o.rmempty = false;
		o.modalonly = true;

		o = s.option(form.ListValue, 'type', _('Type'));
		o.value('vless', _('VLESS'));
		o.value('hysteria2', _('Hysteria2'));
		o.value('shadowsocks', _('Shadowsocks'));
		o.value('trojan', _('Trojan'));
		o.default = 'vless';
		o.rmempty = false;

		o = s.option(form.Value, 'server', _('Address'));
		o.cfgvalue = function(section_id) {
			return uci.get('neto', section_id, 'server') || uci.get('neto', section_id, 'address');
		};
		o.write = function(section_id, formvalue) {
			uci.set('neto', section_id, 'server', String(formvalue || '').trim());
		};
		o.datatype = 'host';
		o.rmempty = false;

		o = s.option(form.Value, 'port', _('Port'));
		o.datatype = 'port';
		o.rmempty = false;

		o = s.option(form.DummyValue, '_latency', _('URLTest delay'));
		o.textvalue = function(section_id) {
			return E('span', {
				'data-neto-latency-tag': outboundTag(section_id),
				'style': 'font-size:1.1em;font-weight:600;white-space:nowrap'
			}, _('Not tested'));
		};

		o = s.option(form.Value, 'uuid', _('UUID'));
		o.depends('type', 'vless');
		o.rmempty = true;
		o.modalonly = true;

		o = s.option(form.ListValue, 'flow', _('Flow'));
		o.value('', _('None'));
		o.value('xtls-rprx-vision');
		o.depends('type', 'vless');
		o.rmempty = true;
		o.modalonly = true;

		o = s.option(form.ListValue, 'hysteria_obfs_type', _('Obfuscate type'));
		o.value('', _('Disable'));
		o.value('salamander', _('Salamander'));
		o.depends('type', 'hysteria2');
		o.modalonly = true;

		o = s.option(form.Value, 'hysteria_obfs_password', _('Obfuscate password'));
		o.password = true;
		o.depends({ 'type': 'hysteria2', 'hysteria_obfs_type': /[\s\S]/ });
		o.modalonly = true;

		o = s.option(form.Value, 'hysteria_down_mbps', _('Max download speed'));
		o.datatype = 'uinteger';
		o.depends('type', 'hysteria2');
		o.modalonly = true;

		o = s.option(form.Value, 'hysteria_up_mbps', _('Max upload speed'));
		o.datatype = 'uinteger';
		o.depends('type', 'hysteria2');
		o.modalonly = true;

		o = s.option(form.Flag, 'tls', _('TLS'));
		o.enabled = '1';
		o.disabled = '0';
		o.depends('type', 'vless');
		o.depends('type', 'trojan');
		o.rmempty = true;
		o.modalonly = true;

		o = s.option(form.Value, 'server_name', _('TLS SNI'));
		dependsTLS(o);
		o.rmempty = true;
		o.modalonly = true;

		o = s.option(form.DynamicList, 'alpn', _('TLS ALPN'));
		dependsTLS(o);
		o.placeholder = 'h2';
		o.rmempty = true;
		o.modalonly = true;

		o = s.option(form.Flag, 'insecure', _('Allow insecure'));
		o.enabled = '1';
		o.disabled = '0';
		dependsTLS(o);
		o.onchange = allowInsecureConfirm;
		o.rmempty = true;
		o.modalonly = true;

		o = s.option(form.ListValue, 'tls_min_version', _('Minimum TLS version'));
		o.value('', _('Default'));
		o.value('1.0');
		o.value('1.1');
		o.value('1.2');
		o.value('1.3');
		dependsTLS(o);
		o.modalonly = true;

		o = s.option(form.ListValue, 'tls_max_version', _('Maximum TLS version'));
		o.value('', _('Default'));
		o.value('1.0');
		o.value('1.1');
		o.value('1.2');
		o.value('1.3');
		dependsTLS(o);
		o.modalonly = true;

		o = s.option(form.DynamicList, 'tls_cipher_suites', _('Cipher suites'));
		o.placeholder = 'TLS_AES_128_GCM_SHA256';
		dependsTLS(o);
		o.modalonly = true;

		o = s.option(form.Flag, 'ech', _('Enable ECH'));
		o.enabled = '1';
		o.disabled = '0';
		dependsTLS(o);
		o.rmempty = true;
		o.modalonly = true;

		o = s.option(form.DynamicList, 'ech_config', _('ECH config'));
		dependsECH(o);
		o.rmempty = true;
		o.modalonly = true;

		o = s.option(form.Value, 'ech_config_path', _('ECH config path'));
		dependsECH(o);
		o.rmempty = true;
		o.modalonly = true;

		o = s.option(form.ListValue, 'utls_fingerprint', _('uTLS fingerprint'));
		o.value('', _('Disable'));
		o.value('360');
		o.value('android');
		o.value('chrome');
		o.value('edge');
		o.value('firefox');
		o.value('ios');
		o.value('qq');
		o.value('random');
		o.value('randomized');
		o.value('safari');
		o.depends({ 'type': 'vless', 'tls': '1' });
		o.depends({ 'type': 'trojan', 'tls': '1' });
		o.rmempty = true;
		o.modalonly = true;

		o = s.option(form.Flag, 'reality', _('REALITY'));
		o.enabled = '1';
		o.disabled = '0';
		o.depends({ 'type': 'vless', 'tls': '1' });
		o.rmempty = true;
		o.modalonly = true;

		o = s.option(form.Value, 'reality_public_key', _('REALITY public key'));
		o.password = true;
		dependsReality(o);
		o.rmempty = false;
		o.modalonly = true;

		o = s.option(form.Value, 'reality_short_id', _('REALITY short ID'));
		o.password = true;
		dependsReality(o);
		o.rmempty = true;
		o.modalonly = true;

		o = s.option(form.ListValue, 'transport', _('Transport'));
		o.value('', _('None'));
		o.value('grpc', _('gRPC'));
		o.value('http', _('HTTP'));
		o.value('httpupgrade', _('HTTPUpgrade'));
		o.value('quic', _('QUIC'));
		o.value('ws', _('WebSocket'));
		o.depends('type', 'vless');
		o.depends('type', 'trojan');
		o.rmempty = true;
		o.modalonly = true;

		o = s.option(form.Value, 'grpc_service_name', _('gRPC service name'));
		dependsTransport(o, 'grpc');
		o.modalonly = true;

		o = s.option(form.DynamicList, 'http_host', _('Host'));
		o.datatype = 'hostname';
		dependsTransport(o, 'http');
		o.modalonly = true;

		o = s.option(form.Value, 'httpupgrade_host', _('Host'));
		o.datatype = 'hostname';
		dependsTransport(o, 'httpupgrade');
		o.modalonly = true;

		o = s.option(form.Value, 'http_path', _('Path'));
		dependsTransport(o, 'http');
		dependsTransport(o, 'httpupgrade');
		o.modalonly = true;

		o = s.option(form.ListValue, 'http_method', _('Method'));
		o.value('', _('Default'));
		o.value('GET');
		o.value('PUT');
		dependsTransport(o, 'http');
		o.modalonly = true;

		o = s.option(form.Value, 'ws_host', _('Host'));
		dependsTransport(o, 'ws');
		o.modalonly = true;

		o = s.option(form.Value, 'ws_path', _('Path'));
		dependsTransport(o, 'ws');
		o.modalonly = true;

		o = s.option(form.Value, 'websocket_early_data', _('Early data'));
		o.datatype = 'uinteger';
		o.placeholder = '2048';
		dependsTransport(o, 'ws');
		o.modalonly = true;

		o = s.option(form.Value, 'websocket_early_data_header', _('Early data header name'));
		o.placeholder = 'Sec-WebSocket-Protocol';
		dependsTransport(o, 'ws');
		o.modalonly = true;

		o = s.option(form.ListValue, 'packet_encoding', _('Packet encoding'));
		o.value('', _('none'));
		o.value('packetaddr', _('packet addr'));
		o.value('xudp', _('XUDP'));
		o.depends('type', 'vless');
		o.modalonly = true;

		o = s.option(form.Value, 'password', _('Password'));
		o.password = true;
		o.depends('type', 'hysteria2');
		o.depends('type', 'shadowsocks');
		o.depends('type', 'trojan');
		o.rmempty = true;
		o.modalonly = true;

		o = s.option(form.ListValue, 'method', _('Encrypt method'));
		o.value('2022-blake3-aes-128-gcm');
		o.value('2022-blake3-aes-256-gcm');
		o.value('2022-blake3-chacha20-poly1305');
		o.value('aes-128-gcm');
		o.value('aes-256-gcm');
		o.value('chacha20-ietf-poly1305');
		o.value('xchacha20-ietf-poly1305');
		o.value('aes-128-ctr');
		o.value('aes-192-ctr');
		o.value('aes-256-ctr');
		o.value('aes-128-cfb');
		o.value('aes-192-cfb');
		o.value('aes-256-cfb');
		o.value('chacha20');
		o.value('chacha20-ietf');
		o.value('rc4-md5');
		o.depends('type', 'shadowsocks');
		o.default = '2022-blake3-aes-128-gcm';
		o.rmempty = true;
		o.modalonly = true;

		pool = m.section(form.GridSection, 'outbound_pool', _('Outbound pools'),
			_('A pool checks members in the displayed order, uses the first reachable outbound, and automatically returns to a higher-priority member after it recovers.'));
		pool.anonymous = false;
		pool.addremove = true;
		pool.nodescriptions = true;
		pool.modaltitle = _('Outbound pool details');
		pool.sectiontitle = function(section_id) {
			return uci.get('neto', section_id, 'label') || uci.get('neto', section_id, 'name') || section_id;
		};
		pool.renderSectionAdd = function() {
			var el = form.GridSection.prototype.renderSectionAdd.apply(this, arguments);
			addNamedSectionValidator(el, this, _('This tag is reserved'), true);
			return el;
		};

		o = pool.option(form.Value, 'label', _('Name'));
		o.cfgvalue = function(section_id) {
			return uci.get('neto', section_id, 'label') || uci.get('neto', section_id, 'name') || section_id;
		};
		o.write = function(section_id, formvalue) {
			var label = String(formvalue || '').trim();
			uci.set('neto', section_id, 'label', label || section_id);
		};
		o.validate = function(section_id, value) {
			return String(value || section_id || '').trim() != '' ? true : _('Name is required');
		};
		o.rmempty = false;
		o.modalonly = true;

		o = pool.option(form.DynamicList, 'outbound', _('Priority order'),
			_('The top outbound has the highest priority. The next one is used only when every outbound above it is unavailable.'));
		uci.sections('neto', 'outbound', function(section, sid) {
			var tag = String(section.tag || sid || section['.name'] || '').trim();
			var label = String(section.label || section.name || tag).trim();

			if (tag != '' && !isReservedTag(tag))
				o.value(tag, label || tag);
		});
		o.textvalue = function(section_id) {
			return poolPriorityText(section_id);
		};
		o.width = '55%';
		o.validate = function(_section_id, value) {
			var members = L.toArray(value).map(function(member) {
				return String(member || '').trim();
			}).filter(function(member) {
				return member != '';
			});
			for (var i = 0; i < members.length; i++) {
				if (!concreteOutboundTagExists(members[i]))
					return _('Unknown outbound: %s').format(members[i]);
			}
			return true;
		};
		o.rmempty = false;

		o = pool.option(form.Value, 'check_url', _('Health check URL'));
		o.datatype = 'url';
		o.default = 'https://www.gstatic.com/generate_204';
		o.rmempty = false;
		o.modalonly = true;

		o = pool.option(form.Value, 'check_interval', _('Check interval (seconds)'));
		o.datatype = 'range(5,86400)';
		o.default = '60';
		o.rmempty = false;
		o.modalonly = true;

		sub = m.section(form.GridSection, 'subscription', _('Subscriptions'));
		sub.anonymous = false;
		sub.addremove = true;
		sub.modaltitle = _('Subscription details');
		sub.sectiontitle = function(section_id) {
			return uci.get('neto', section_id, 'label') || uci.get('neto', section_id, 'name') || section_id;
		};
		sub.renderSectionAdd = function() {
			var el = form.GridSection.prototype.renderSectionAdd.apply(this, arguments);
			addNamedSectionValidator(el, this, _('This name is reserved'));
			el.appendChild(E('button', {
				'class': 'cbi-button cbi-button-action',
				'style': 'margin-left:.5em',
				'data-neto-subscriptions-update-all': '1',
				'disabled': self.subscriptionUpdateAllRunning ? true : null,
				'click': function(ev) {
					ev.preventDefault();
					return self.handleSubscriptionUpdateAll().then(function(failures) {
						self.subscriptionUpdateAllRunning = false;
						self.setSubscriptionUpdateAllButton(false);
						if (failures.length)
							self.showSubscriptionUpdateFailures(failures);
					}, function(err) {
						self.subscriptionUpdateAllRunning = false;
						self.setSubscriptionUpdateAllButton(false);
						ui.addNotification(null, E('p', {}, [ err.message || err ]), 'danger');
					});
				}
			}, self.subscriptionUpdateAllRunning ? _('Updating all…') : _('Update all')));
			return el;
		};

		o = sub.option(form.Flag, 'enabled', _('Enabled'));
		o.enabled = '1';
		o.disabled = '0';
		o.default = '1';
		o.rmempty = false;
		o.editable = true;

		o = sub.option(form.Value, 'label', _('Name'));
		o.cfgvalue = function(section_id) {
			return uci.get('neto', section_id, 'label') || uci.get('neto', section_id, 'name') || section_id;
		};
		o.write = function(section_id, formvalue) {
			var label = String(formvalue || '').trim();
			uci.set('neto', section_id, 'label', label || section_id);
		};
		o.rmempty = false;
		o.modalonly = true;

		o = sub.option(form.Value, 'url', _('URL'));
		o.datatype = 'url';
		o.rmempty = false;
		o.editable = true;

		o = sub.option(form.Flag, 'auto_update', _('Auto update'));
		o.enabled = '1';
		o.disabled = '0';
		o.default = '0';
		o.rmempty = false;
		o.editable = true;

		o = sub.option(form.ListValue, 'update_schedule', _('Schedule'));
		o.value('time', _('Fixed time'));
		o.value('interval', _('Interval'));
		o.default = 'time';
		o.depends('auto_update', '1');
		o.rmempty = false;
		o.modalonly = true;

		o = sub.option(form.ListValue, 'update_hour', _('Update time'));
		for (var hour = 0; hour < 24; hour++)
			o.value(String(hour), _('%d:00').format(hour));
		o.default = '0';
		o.depends({ 'auto_update': '1', 'update_schedule': 'time' });
		o.rmempty = false;
		o.modalonly = true;

		o = sub.option(form.ListValue, 'update_interval_minutes', _('Update interval'));
		addUpdateIntervalChoices(o);
		o.default = '360';
		o.depends({ 'auto_update': '1', 'update_schedule': 'interval' });
		o.rmempty = false;
		o.modalonly = true;

		o = sub.option(form.ListValue, 'update_via', _('Update via'));
		o.value('direct', 'direct');
		o.value('proxy', 'proxy');
		o.default = 'direct';
		o.rmempty = false;
		o.editable = true;

		o = sub.option(form.ListValue, 'update_outbound', _('Update outbound'));
		addProxyOutboundChoices(o);
		o.depends('update_via', 'proxy');
		o.modalonly = true;

		o = sub.option(form.DummyValue, 'node_count', _('Nodes'));
		o.cfgvalue = function(section_id) {
			return uci.get('neto', section_id, 'node_count') || '-';
		};

		o = sub.option(form.DummyValue, 'last_update', _('Updated'));
		o.cfgvalue = function(section_id) {
			var value = uci.get('neto', section_id, 'last_update');
			var timestamp = Number(value);

			if (!timestamp)
				return '-';

			return new Date(timestamp * 1000).toLocaleString();
		};

		o = sub.option(form.Button, '_update', _('Update'));
		o.inputstyle = 'action';
		o.inputtitle = _('Update');
		o.cfgvalue = function() {
			return true;
		};
		o.modalonly = true;
		o.onclick = L.bind(function(ev, section_id) {
			return this.handleSubscriptionUpdate(section_id).catch(function(err) {
				ui.addNotification(null, E('p', {}, [ err.message || err ]), 'danger');
			});
		}, this);

		return m.render();
	}
});

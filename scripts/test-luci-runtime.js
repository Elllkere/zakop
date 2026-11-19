'use strict';
// Exercise the LuCI helper without a router or browser.
const assert = require('node:assert/strict');
const fs = require('node:fs');
String.prototype.format = function(...args) {
	let index = 0;
	return this.replace(/%[sd]/g, () => String(args[index++]));
};
const source = fs.readFileSync('embedded/files/www/luci-static/resources/zakop/ui.js', 'utf8');

function fixture(pending) {
	const calls = [], modals = [], links = [ 'general', 'outbounds', 'rules', 'clients', 'providers', 'advanced', 'logs', 'debug' ].map(name => ({
		getAttribute: () => '/cgi-bin/luci/admin/services/zakop/' + name,
		childNodes: [ { nodeType: 1, icon: true }, { nodeType: 3, textContent: 'English' } ],
		style: {}
	}));
	let status = 0, reloaded = false;
	const api = {
		changes: async () => pending,
		apply: async timeout => { calls.push(['apply', timeout]); },
		get: () => 'custom', unload: () => {}, load: async () => {}
	};
	const exec = async (path, args) => {
		calls.push(args);
		if (args[0] === 'start') return { code: 0, stdout: 'job.ab12\n' };
		if (args[0] === 'status-direct') return { code: 0, stdout: (++status === 1 ? 'running\n' : 'exit:0\n') + 'result\n\n__ZAKOP_JOB_STDERR__\n' };
		return { code: 0 };
	};
	const fsApi = { exec, exec_direct: async (path, args) => {
		const result = await exec(path, args);
		if (result.code) throw Error('command failed');
		return result.stdout || '';
	} };
	const ui = { showModal: (...args) => modals.push(args), changes: { init: async () => {} } };
	const window = { setTimeout: fn => fn(), location: { reload: () => { reloaded = true; } } };
	const document = { getElementById: () => true, querySelectorAll: () => links };
	const helper = new Function('baseclass', 'fs', 'ui', 'uci', 'rpc', 'zakopI18n', 'window', 'document', 'E', source)(
		{ extend: obj => obj }, fsApi, ui, api, { declare: () => async () => { calls.push(['commit']); } },
		{ translate: text => 'RU:' + text }, window, document, (...args) => args);
	return { helper, calls, modals, links, api, fsApi, get reloaded() { return reloaded; } };
}

async function subscriptions() {
	const sections = [], calls = [];
	class Map {
		section(type, name) {
			const section = { type, name, options: [], option(type, name) {
				const option = { type, option: name, map: this.map, value() {}, depends() {} };
				this.options.push(option);
				return option;
			} };
			section.map = this;
			sections.push(section);
			return section;
		}
		render() {}
		async load() { calls.push('parent.load'); }
		async reset() { calls.push('parent.reset'); }
	}
	const source = fs.readFileSync('embedded/files/www/luci-static/resources/view/zakop/outbounds.js', 'utf8');
	let completeUpdate;
	const notifications = [];
	const buttons = [ 'new_sub', 'other_sub' ].map(name => ({
		getAttribute: () => name, classList: { toggle() {} }, disabled: false
	}));
	const doc = { querySelectorAll: () => buttons, querySelector: () => null };
	const helper = {
		syncRulesTab() {},
		runJob: async (name, args) => {
			calls.push(name + ':' + args[0]);
			return new Promise(resolve => { completeUpdate = resolve; });
		},
		restart: async () => calls.push('restart'),
		notify: (message, severity) => notifications.push([message, severity])
	};
	const page = new Function('view', 'fs', 'form', 'ui', 'uci', 'zakopI18n', 'zakopUI', 'outboundCards', 'L', 'E', 'document', source)(
		{ extend: obj => obj }, {}, { Map, GridSection: function() {} },
		{ hideModal: () => calls.push('hide') },
		{ get() {}, sections() {}, unload: () => calls.push('unload') }, { translate: text => text }, helper, {},
		{ bind: (fn, self) => fn.bind(self), toArray: value => value || [] }, () => ({}), doc);
	page.render();
	const sub = sections.find(section => section.name === 'subscription');
	assert.equal(sub.options.some(option => option.option === 'enabled'), false);
	assert.notEqual(sub.options.find(option => option.option === 'url').editable, true);
	assert.equal(sub.options.find(option => option.option === 'auto_update').editable, true);
	assert.equal(sub.options.find(option => option.option === 'update_minute').default, '0');
	const update = sub.options.find(option => option.option === '_update');
	page.handleSaveCommitConfig = async () => calls.push('commit');
	const modalMap = { save: async () => calls.push('modal.save') };
	const pending = update.onclick.call({ map: modalMap }, {}, 'new_sub');
	assert.equal(buttons[0].textContent, 'Updating…');
	assert.ok(buttons.every(button => button.disabled));
	await page.handleSubscriptionUpdate('other_sub');
	await new Promise(resolve => setImmediate(resolve));
	assert.deepEqual(calls, ['modal.save', 'parent.load', 'parent.reset', 'hide', 'commit', 'subscriptions:new_sub']);
	completeUpdate({ code: 0 });
	await pending;
	assert.deepEqual(calls.slice(-4), ['restart', 'unload', 'parent.load', 'parent.reset']);
	assert.deepEqual(notifications, [['Subscription "new_sub" updated.', 'success']]);
	assert.ok(buttons.every(button => !button.disabled));
	calls.length = 0;
	await update.onclick.call({ map: { save: async () => { throw Error('invalid URL'); } } }, {}, 'new_sub');
	assert.deepEqual(calls, []);
	assert.deepEqual(notifications.at(-1), ['invalid URL', 'danger']);
	assert.ok(buttons.every(button => !button.disabled));
}

async function clientForms() {
	const root = 'embedded/files/www/luci-static/resources/';
	const source = fs.readFileSync(root + 'view/zakop/clients.js', 'utf8');
	const fields = fs.readFileSync(root + 'zakop/ruleform.js', 'utf8');
	const shared = { '.name': 'rule0', name: 'YouTube', rule_id: 'youtube' };
	const rules = {
		override: { override: 'youtube', name: 'Old override label' },
		custom: { name: 'Laptop only' }
	};
	const sections = {};
	const api = {
		get: (_conf, sid, key) => rules[sid]?.[key],
		sections: (_conf, type, cb) => {
			const rows = type === 'rule' ? [shared] : [];
			if (cb) rows.forEach(row => cb(row, row['.name']));
			return rows;
		}
	};
	const tag = (name, attrs, children) => ({ name, attrs, children });
	function GridSection(sectiontype) {
		this.sectiontype = sectiontype;
		this.children = [];
	}
	GridSection.prototype.option = function(type, option, title, description) {
		const field = { type, option, title, description, value() {},
			depends(...args) { this.dependency = args; } };
		this.children.push(field);
		return field;
	};
	GridSection.prototype.load = async function() {};
	function Value() {}
	const attributes = {};
	Value.prototype.renderWidget = () => ({
		matches: selector => selector === 'input',
		setAttribute: (key, value) => { attributes[key] = value; }
	});
	const form = { GridSection, Value, ListValue: function() {}, Flag: function() {},
		Map: class {
			section(Type, name) { return sections[name] = new Type(name); }
			render() {}
		}
	};
	const translation = { translate: text => text };
	const hints = {
		'02:00:00:00:00:01': { ipv4: ['192.168.8.20'] },
		'02:00:00:00:00:02': { ipaddrs: ['192.168.8.20', '192.168.8.21', 'fd00::2'], name: 'Laptop' },
		'02:00:00:00:00:03': { ipv6: ['fd00::3'] }
	};
	let available = true;
	const rpc = { declare: spec => {
		assert.equal(spec.method, 'getHostHints');
		return async () => {
			if (!available) throw Error('RPC unavailable');
			return hints;
		};
	} };
	const validation = { parseIPv4: ip => /^([0-9]+\.){3}[0-9]+$/.test(ip) ? ip.split('.').map(Number) : null };
	const L = { toArray: value => value == null ? [] : Array.isArray(value) ? value : [value] };
	const ruleForm = new Function('baseclass', 'form', 'uci', 'zakopI18n', fields)({ extend: obj => obj }, form, api, translation);
	const page = new Function('rpc', 'form', 'uci', 'view', 'validation', 'zakopI18n', 'zakopUI', 'ruleForm', 'L', 'E', source)(
		rpc, form, api, { extend: obj => obj }, validation, translation, { syncRulesTab() {} }, ruleForm, L, tag);
	page.render();
	const grid = sections.client_rule;
	await grid.load();
	const columns = grid.children.filter(option => !option.modalonly);
	assert.deepEqual(columns.slice(0, 2).map(option => option.title), ['Enabled', 'Name']);
	assert.equal(columns.some(option => option.option === 'override'), false);
	assert.equal(columns[1].cfgvalue('override'), 'YouTube');
	assert.equal(columns[1].cfgvalue('custom'), 'Laptop only');
	assert.equal(columns[1].modalonly, false);
	assert.deepEqual(grid.children.find(option => option.option === 'name').dependency, ['override', '-']);
	shared.name = 'YouTube renamed';
	await grid.load();
	assert.equal(columns[1].cfgvalue('override'), 'YouTube renamed');

	const ip = { option: 'ip', cbid: () => 'ip-field' };
	await sections.client.addModalOptions({ children: [ip] });
	const suggestions = ip.renderWidget('client').children[1];
	assert.equal(attributes.list, 'ip-field-devices');
	assert.deepEqual(suggestions.children.map(option => option.attrs), [
		{ value: '192.168.8.20', label: 'Laptop · 02:00:00:00:00:02' },
		{ value: '192.168.8.21', label: 'Laptop · 02:00:00:00:00:02' }
	]);
	available = false;
	const manual = { option: 'ip' };
	await sections.client.addModalOptions({ children: [manual] });
	assert.equal(manual.renderWidget, undefined);
}

function ruleBackups() {
	const root = 'embedded/files/www/luci-static/resources/';
	const fields = fs.readFileSync(root + 'zakop/ruleform.js', 'utf8');
	const source = fs.readFileSync(root + 'zakop/rulebackup.js', 'utf8');
	const translation = { translate: text => text };
	let next = 0;
	let sections = [
		{ '.name': 'client', '.type': 'client', ip: '192.168.8.20' },
		{ '.name': 'old_node', '.type': 'outbound', type: 'vless', server: 'node.example', port: '443', uuid: 'secret', label: 'Poland' },
		{ '.name': 'old_provider', '.type': 'provider', type: 'ip', label: 'Networks', url: 'https://example.com?token=secret' },
		{ '.name': 'rule0', '.type': 'rule', rule_id: 'youtube', name: 'YouTube', action: 'proxy', outbound: 'old_node', domain_equals: ['youtube.com'] },
		{ '.name': 'rule1', '.type': 'client_rule', client_ip: '192.168.8.20', override: 'youtube', name: 'Laptop YouTube', action: 'block' },
		{ '.name': 'rule2', '.type': 'client_rule', client_ip: '192.168.8.20', name: 'Laptop IP rule', action: 'proxy', outbound: 'old_node',
			ip_input: 'provider', ip_provider: ['old_provider'], proto: ['tcp'], dst_port: ['443'] }
	];
	const api = {
		sections: (conf, type, cb) => sections.filter(section => section['.type'] === type).map(section => {
			if (cb) cb(section, section['.name']);
			return section;
		}),
		get: (conf, sid, key) => {
			const section = sections.find(section => section['.name'] === sid);
			return key ? section?.[key] : section;
		},
		add: (conf, type) => {
			const sid = 'added' + (++next);
			sections.push({ '.name': sid, '.type': type });
			return sid;
		},
		set: (conf, sid, key, value) => { api.get(conf, sid)[key] = value; },
		remove: (conf, sid) => { sections = sections.filter(section => section['.name'] !== sid); }
	};
	const ruleForm = new Function('baseclass', 'form', 'uci', 'zakopI18n', fields)({ extend: obj => obj }, {}, api, translation);
	const transfer = new Function('uci', 'zakopI18n', 'ruleForm', source.slice(0, source.indexOf('return baseclass.extend')) +
		'return { exportRulesJSON, parseImportedRules, replaceRules };')(api, translation, ruleForm);
	const text = transfer.exportRulesJSON();
	assert.doesNotMatch(text, /secret/);
	const exported = JSON.parse(text);
	assert.equal(exported.rules[0].outbound, 'old_node');
	assert.equal(exported.client_rules[0].override, 'youtube');
	assert.equal(exported.client_rules[0].name, 'YouTube');
	api.get('zakop', 'old_node')['.name'] = 'new_node';
	api.get('zakop', 'old_provider')['.name'] = 'new_provider';

	const remapped = transfer.parseImportedRules(text);
	assert.equal(remapped.rules[0].outbound, 'new_node');
	assert.equal(remapped.clientRules[0].action, 'block');
	assert.deepEqual(remapped.clientRules[1].ip_provider, ['new_provider']);
	assert.deepEqual(remapped.warnings, []);
	transfer.replaceRules(remapped);
	assert.equal(api.sections('zakop', 'rule')[0].rule_id, 'youtube');
	assert.equal(api.sections('zakop', 'client_rule')[0].override, 'youtube');
	assert.equal(api.sections('zakop', 'client_rule')[0].name, undefined);
	assert.notEqual(api.sections('zakop', 'client_rule')[1].outbound, 'old_node');

	api.remove('zakop', 'new_node');
	api.remove('zakop', 'new_provider');
	const missing = transfer.parseImportedRules(text);
	assert.equal(missing.rules[0].action, 'direct');
	assert.equal(missing.rules[0].outbound, 'direct');
	assert.equal(missing.clientRules[1].dst_port, undefined);
	assert.equal(missing.clientRules[1].proto, undefined);
	assert.match(missing.warnings.join('\n'), /old_node.*not found/);
	assert.match(missing.warnings.join('\n'), /old_provider.*not found/);

	transfer.replaceRules(transfer.parseImportedRules(JSON.stringify({ version: 1, rules: [{ name: 'YouTube' }] })));
	assert.equal(api.sections('zakop', 'client_rule').length, 2);
	assert.equal(api.sections('zakop', 'rule')[0].rule_id, 'youtube');
}

(async () => {
	const empty = fixture({});
	await empty.helper.applyAndRestart();
	assert.equal(empty.calls.some(call => call[0] === 'apply'), false);
	assert.equal(empty.reloaded, true);
	const pending = fixture({ zakop: [['set', 'main', 'enabled', '1']] });
	await pending.helper.applyAndRestart();
	assert.deepEqual(pending.calls[0], ['apply', 60]);
	assert.equal(pending.reloaded, true);
	const failed = fixture({ zakop: [['set']] });
	failed.api.apply = async () => { throw Error('permission denied'); };
	await failed.helper.applyAndRestart();
	assert.equal(failed.reloaded, false);
	assert.equal(failed.calls.some(call => call[0] === 'start'), false);
	assert.match(JSON.stringify(failed.modals), /permission denied/);
	const job = fixture({}), snapshots = [];
	assert.deepEqual(await job.helper.runJob('latency', [], text => snapshots.push(text)), { code: 0, stdout: 'result\n', stderr: '' });
	assert.equal(snapshots.length, 2);
	assert.equal(job.calls.at(-1)[0], 'remove');
	job.helper.syncRulesTab();
	assert.equal(job.links[0].childNodes[1].textContent, 'RU:General');
	assert.equal(job.links[0].childNodes[0].icon, true);
	const retry = fixture({});
	const direct = retry.fsApi.exec_direct;
	let failures = 0;
	retry.fsApi.exec_direct = async (path, args) => {
		if (args[0] === 'status-direct' && failures++ < 2) throw Error('XHR request timed out');
		return direct(path, args);
	};
	await retry.helper.runJob('latency');
	assert.equal(retry.calls.filter(call => call[0] === 'start').length, 1);
	assert.equal(retry.calls.at(-1)[0], 'remove');
	const unavailable = fixture({});
	unavailable.fsApi.exec_direct = async (_path, args) => {
		if (args[0] === 'start') return 'job.ab12';
		throw Error('XHR request timed out');
	};
	await assert.rejects(unavailable.helper.runJob('latency'), /Reading background task status failed: XHR request timed out/);
	const failedStart = fixture({});
	failedStart.fsApi.exec_direct = async () => { throw Error('XHR request timed out'); };
	await assert.rejects(failedStart.helper.runJob('latency'), /Starting background task failed: XHR request timed out/);
	const unchanged = fixture({});
	await unchanged.helper.commitConfig();
	assert.equal(unchanged.calls.some(call => call[0] === 'commit'), false);
	const changed = fixture({ zakop: [['set', 'main', 'enabled', '1']] });
	await changed.helper.commitConfig();
	assert.equal(changed.calls.filter(call => call[0] === 'commit').length, 1);
	await subscriptions();
	await clientForms();
	ruleBackups();
	console.log('LuCI runtime checks passed');
})().catch(err => { console.error(err); process.exitCode = 1; });

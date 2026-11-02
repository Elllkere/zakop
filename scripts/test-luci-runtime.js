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
	const page = new Function('view', 'fs', 'form', 'ui', 'uci', 'zakopI18n', 'zakopUI', 'outboundCards', 'L', 'E', source)(
		{ extend: obj => obj }, {}, { Map, GridSection: function() {} },
		{ hideModal: () => calls.push('hide'), addNotification: () => calls.push('error') },
		{ get() {}, sections() {} }, { translate: text => text }, { syncRulesTab() {} }, {},
		{ bind: (fn, self) => fn.bind(self), toArray: value => value || [] }, () => ({}));
	page.render();
	const sub = sections.find(section => section.name === 'subscription');
	assert.equal(sub.options.some(option => option.option === 'enabled'), false);
	assert.notEqual(sub.options.find(option => option.option === 'url').editable, true);
	assert.equal(sub.options.find(option => option.option === 'auto_update').editable, true);
	const update = sub.options.find(option => option.option === '_update');
	page.handleSubscriptionUpdate = async name => calls.push('update:' + name);
	const modalMap = { save: async () => calls.push('modal.save') };
	await update.onclick.call({ map: modalMap }, {}, 'new_sub');
	assert.deepEqual(calls, ['modal.save', 'parent.load', 'parent.reset', 'hide', 'update:new_sub']);
	calls.length = 0;
	await update.onclick.call({ map: { save: async () => { throw Error('invalid URL'); } } }, {}, 'new_sub');
	assert.deepEqual(calls, ['error']);
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
	console.log('LuCI runtime checks passed');
})().catch(err => { console.error(err); process.exitCode = 1; });

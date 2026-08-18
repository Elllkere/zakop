'use strict';
'require baseclass';
'require fs';
'require ui';
'require uci';

function commandSuccess(res, message) {
	if (!res || res.code)
		throw new Error((res && (res.stderr || res.stdout)) || message);

	return res;
}

function showApplyProgress() {
	ui.showModal(_('Save & Apply'), [
		E('p', { 'class': 'spinning' }, [ _('Applying configuration changes…') ])
	]);
}

function showApplySuccess() {
	ui.showModal(_('Save & Apply'), [
		E('p', {}, [ _('Configuration changes applied.') ])
	]);
}

function showApplyError(err) {
	var message = err && err.message ? err.message : String(err || _('Unknown error'));

	ui.showModal(_('Save & Apply'), [
		E('p', {}, [ _('Failed to apply configuration changes.') ]),
		E('p', {}, [
			E('em', { 'style': 'white-space:pre-wrap' }, [ message ])
		]),
		E('div', { 'class': 'right' }, [
			E('button', {
				'class': 'cbi-button',
				'click': ui.hideModal
			}, [ _('Dismiss') ])
		])
	]);
}

function applyAndRestart() {
	showApplyProgress();

	return uci.apply()
		.then(function() {
			/*
			 * LuCI 23.05 and 24.10 resolve uci.apply() immediately after
			 * scheduling confirmation one second later. Keep this page alive
			 * until that confirmation has completed, otherwise reload cancels
			 * the timer and rpcd rolls the staged changes back.
			 */
			return new Promise(function(resolve) {
				window.setTimeout(resolve, 2500);
			});
		})
		.then(function() {
			return fs.exec('/etc/init.d/zakop', [ 'restart' ]);
		})
		.then(function(res) {
			return commandSuccess(res, _('Restart failed'));
		})
		.then(function() {
			return ui.changes.init();
		})
		.then(function() {
			showApplySuccess();
			return new Promise(function(resolve) {
				window.setTimeout(resolve, 1000);
			});
		})
		.then(function() {
			window.location.reload();
		})
		.catch(function(err) {
			showApplyError(err);
		});
}

function rulesTabVisible() {
	return String(uci.get('zakop', 'main', 'routing_mode') || 'custom').trim() == 'custom';
}

function hideElement(el, hidden) {
	if (el)
		el.style.display = hidden ? 'none' : '';
}

function tabContainer(link) {
	var node = link;

	while (node && node.parentNode) {
		if (String(node.nodeName || '').toLowerCase() == 'li')
			return node;

		node = node.parentNode;
	}

	return link;
}

function updateRulesTab() {
	var links, hidden;

	if (typeof document == 'undefined')
		return;

	links = document.querySelectorAll('a[href]');
	hidden = !rulesTabVisible();

	for (var i = 0; i < links.length; i++) {
		var href = String(links[i].getAttribute('href') || '');

		if (href.indexOf('/admin/services/zakop/rules') < 0 && href.indexOf('/zakop/rules') < 0)
			continue;

		hideElement(tabContainer(links[i]), hidden);
	}
}

function syncRulesTab() {
	updateRulesTab();

	if (typeof window != 'undefined') {
		window.setTimeout(updateRulesTab, 0);
		window.setTimeout(updateRulesTab, 250);
	}
}

return baseclass.extend({
	applyAndRestart: applyAndRestart,
	syncRulesTab: syncRulesTab
});

'use strict';
'require baseclass';
'require fs';
'require ui';
'require uci';
'require rpc';
'require zakop.i18n as zakopI18n';

var _ = zakopI18n.translate;

var countryFlagFontName = 'Twemoji Country Flags';
var countryFlagFontURL = '/luci-static/resources/zakop-assets/TwemojiCountryFlags-0.1.8.woff2';
var countryFlagStyleID = 'zakop-country-flag-emoji';

function colorEmojiContext() {
	var canvas = document.createElement('canvas');
	var context;

	canvas.width = 1;
	canvas.height = 1;
	context = canvas.getContext('2d');
	if (!context)
		return null;

	context.textBaseline = 'top';
	context.font = '100px "Apple Color Emoji", "Segoe UI Emoji", "Noto Color Emoji", sans-serif';
	context.scale(0.01, 0.01);
	return context;
}

function emojiPixel(context, value, color) {
	context.clearRect(0, 0, 100, 100);
	context.fillStyle = color;
	context.fillText(value, 0, 0);
	return context.getImageData(0, 0, 1, 1).data.join(',');
}

function supportsColorEmoji(value) {
	var context, light, dark;

	try {
		context = colorEmojiContext();
		if (!context)
			return false;
		light = emojiPixel(context, value, '#fff');
		dark = emojiPixel(context, value, '#000');
		return dark == light && dark.indexOf('0,0,0,') != 0;
	} catch (err) {
		return false;
	}
}

function enableCountryFlagEmoji() {
	var style;

	if (typeof document == 'undefined' || document.getElementById(countryFlagStyleID))
		return;
	if (supportsColorEmoji('🇨🇭'))
		return;

	style = document.createElement('style');
	style.id = countryFlagStyleID;
	style.textContent = '@font-face {' +
		'font-family:"' + countryFlagFontName + '";' +
		'unicode-range:U+1F1E6-1F1FF,U+1F3F4,U+E0062-E0063,U+E0065,U+E0067,' +
		'U+E006C,U+E006E,U+E0073-E0074,U+E0077,U+E007F;' +
		'src:url("' + countryFlagFontURL + '") format("woff2");' +
		'font-display:swap}' +
		'html.zakop-country-flags body,' +
		'html.zakop-country-flags input,' +
		'html.zakop-country-flags button,' +
		'html.zakop-country-flags select,' +
		'html.zakop-country-flags option{' +
		'font-family:"' + countryFlagFontName + '",system-ui,-apple-system,"Segoe UI",sans-serif}';
	document.head.appendChild(style);
	document.documentElement.classList.add('zakop-country-flags');
}

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

// CGI-IO avoids rpcd's file.exec buffering and the shared RPC XHR batch.
// Only the short start/status commands run in CGI; the test stays detached.
function runJob(action, args, progress) {
	var id, failures = 0, deadline = Date.now() + 30 * 60 * 1000;
	function call(params) {
		return fs.exec_direct('/usr/share/zakop/luci-job.sh', params, 'text');
	}
	function contextualError(label, err) {
		return new Error(label.format(err && err.message ? err.message : String(err)));
	}
	function wait(milliseconds) {
		return new Promise(function(resolve) { window.setTimeout(resolve, milliseconds); });
	}
	function poll() {
		return call([ 'status-direct', id ]).then(function(output) {
			failures = 0;
			output = String(output || '');
			var newline = output.indexOf('\n');
			var state = output.substring(0, newline);
			var marker = '\n__ZAKOP_JOB_STDERR__\n';
			var split = output.indexOf(marker, newline);
			if (newline < 0 || split < 0 || (state != 'running' && !/^exit:\d+$/.test(state)))
				throw new Error(_('Invalid background task response'));
			var result = { code: 0, stdout: output.substring(newline + 1, split), stderr: output.substring(split + marker.length) };
			if (progress)
				progress(result.stdout);
			if (state.indexOf('exit:') == 0) {
				result.code = Number(state.substring(5));
				return call([ 'remove', id ]).catch(function() {}).then(function() { return result; });
			}
			if (Date.now() > deadline)
				throw new Error(_('Operation is still running. Check the logs.'));
			return wait(1000).then(poll);
		}, function(err) {
			// A lost status reply does not mean the proxy failed. Resume polling
			// the same job; never launch a second test on a transport timeout.
			var message = String(err && err.message || err);
			if (/timeout|timed out|network|failed to fetch/i.test(message) && ++failures <= 3 && Date.now() < deadline)
				return wait(1500 * failures).then(poll);
			throw contextualError(_('Reading background task status failed: %s'), err);
		});
	}
	return call([ 'start', action ].concat(args || [])).then(function(output) {
		id = String(output || '').trim();
		if (!/^job\.[a-zA-Z0-9]+$/.test(id))
			throw new Error(_('Invalid background task response'));
		return poll();
	}, function(err) {
		throw contextualError(_('Starting background task failed: %s'), err);
	});
}

// Commit the LuCI session's staged data, rather than the CLI's /tmp/.uci.
var callCommit = rpc.declare({
	object: 'uci', method: 'commit', params: [ 'config' ], reject: true
});

function commitConfig() {
	return uci.changes().then(function(changes) {
		// Even a no-op uci/commit emits config.change and can reload zakop.
		if (changes && changes.zakop && changes.zakop.length)
			return callCommit('zakop');
	}).then(function() {
		uci.unload('zakop');
		return uci.load('zakop');
	}).then(function() { return ui.changes.init(); });
}

function restart() {
	return runJob('restart').then(function(res) {
		return commandSuccess(res, _('Restart failed'));
	});
}

function applyAndRestart() {
	showApplyProgress();

	// rpcd can return "No data" when Apply is called with no staged changes.
	// This happens after action buttons have already committed the config.
	return uci.changes().then(function(changes) {
		var pending = Object.keys(changes || {}).some(function(name) {
			return changes[name] && changes[name].length;
		});
		if (!pending)
			return;
		return uci.apply(60).then(function() {
			// Older LuCI versions resolve before their scheduled confirmation.
			return new Promise(function(resolve) { window.setTimeout(resolve, 2500); });
		});
	})
		.then(restart)
		.then(function() { return ui.changes.init(); })
		.then(function() {
			showApplySuccess();
			window.location.reload();
		})
		.catch(showApplyError);
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

function translateTabs() {
	var titles = { general: 'General', outbounds: 'Outbounds', rules: 'Rules',
		clients: 'Clients', providers: 'Providers', advanced: 'Advanced', logs: 'Logs', debug: 'Debug' };
	var links = document.querySelectorAll('a[href]');
	for (var i = 0; i < links.length; i++) {
		var match = String(links[i].getAttribute('href') || '').match(/\/admin\/services\/zakop\/([a-z]+)\/?(?:[?#].*)?$/);
		if (match && titles[match[1]]) {
			// Preserve theme icons and other elements inside navigation links.
			for (var j = 0; j < links[i].childNodes.length; j++) {
				if (links[i].childNodes[j].nodeType == 3 && links[i].childNodes[j].textContent.trim())
					links[i].childNodes[j].textContent = _(titles[match[1]]);
			}
		}
	}
}

function syncRulesTab() {
	enableCountryFlagEmoji();
	translateTabs();
	updateRulesTab();

	if (typeof window != 'undefined') {
		window.setTimeout(updateRulesTab, 0);
		window.setTimeout(function() { translateTabs(); updateRulesTab(); }, 250);
	}
}

return baseclass.extend({
	applyAndRestart: applyAndRestart,
	runJob: runJob,
	commitConfig: commitConfig,
	restart: restart,
	enableCountryFlagEmoji: enableCountryFlagEmoji,
	syncRulesTab: syncRulesTab
});

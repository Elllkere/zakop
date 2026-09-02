'use strict';
'require baseclass';
'require fs';
'require ui';
'require uci';

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
	enableCountryFlagEmoji();
	updateRulesTab();

	if (typeof window != 'undefined') {
		window.setTimeout(updateRulesTab, 0);
		window.setTimeout(updateRulesTab, 250);
	}
}

return baseclass.extend({
	applyAndRestart: applyAndRestart,
	enableCountryFlagEmoji: enableCountryFlagEmoji,
	syncRulesTab: syncRulesTab
});

'use strict';
'require fs';
'require uci';
'require view';
'require zakop.i18n as zakopI18n';
'require zakop.ui as zakopUI';

var _ = zakopI18n.translate;

return view.extend({
	load: function() {
		return uci.load('zakop').then(function() {
			zakopUI.syncRulesTab();

			return fs.exec('/usr/bin/zakopd', [ 'debug' ]).catch(function(err) {
				return {
					code: -1,
					stdout: '',
					stderr: String(err)
				};
			});
		});
	},

	render: function(res) {
		var text = res.stdout || res.stderr || _('No status available');

		zakopUI.syncRulesTab();

		return E([
			E('h2', {}, [ _('Debug') ]),
			E('pre', {
				'style': 'white-space: pre-wrap; overflow-wrap: anywhere'
			}, [ text.trim() ])
		]);
	}
});

'use strict';
'require view';
'require fs';
'require rpc';

var callServiceList = rpc.declare({
	object: 'service',
	method: 'list',
	params: [ 'name' ],
	expect: { '': {} }
});

function getServiceStatus() {
	return L.resolveDefault(callServiceList('netnotify'), {}).then(function (res) {
		var instances = (res.netnotify && res.netnotify.instances) ? res.netnotify.instances : {};
		for (var key in instances) {
			if (instances[key].running) {
				return true;
			}
		}
		return false;
	});
}

return view.extend({
	load: function () {
		return Promise.all([
			getServiceStatus(),
			L.resolveDefault(fs.read('/tmp/netnotify/devices.json'), '')
		]);
	},

	render: function (data) {
		var running = data[0];
		var raw = data[1] || '';
		var status = { running: running, version: '-', devices: [] };

		if (raw) {
			try {
				var parsed = JSON.parse(raw);
				status.version = parsed.version || '-';
				status.devices = parsed.devices || [];
			} catch (e) {
				status.devices = [];
			}
		}

		var header = E('div', { 'class': 'cbi-section' }, [
			E('h3', _('Service status')),
			E('p', {}, [
				_('Running') + ': ',
				E('strong', {
					'style': running ? 'color:green' : 'color:red'
				}, running ? _('yes') : _('no')),
				' · ',
				_('Version') + ': ' + status.version
			])
		]);

		var rows = [
			E('tr', { 'class': 'tr table-titles' }, [
				E('th', { 'class': 'th' }, _('Name')),
				E('th', { 'class': 'th' }, _('MAC')),
				E('th', { 'class': 'th' }, _('IP')),
				E('th', { 'class': 'th' }, _('Iface')),
				E('th', { 'class': 'th' }, _('State'))
			])
		];

		(status.devices || []).forEach(function (d) {
			rows.push(E('tr', { 'class': 'tr' }, [
				E('td', { 'class': 'td' }, d.name || '-'),
				E('td', { 'class': 'td' }, d.mac || '-'),
				E('td', { 'class': 'td' }, d.ip || '-'),
				E('td', { 'class': 'td' }, d.iface || '-'),
				E('td', { 'class': 'td' }, d.state || '-')
			]));
		});

		if ((status.devices || []).length === 0) {
			rows.push(E('tr', { 'class': 'tr' }, [
				E('td', { 'class': 'td', 'colspan': 5 },
					_('No device state yet. Enable the service and wait for neighbour / DHCP events.'))
			]));
		}

		var table = E('div', { 'class': 'cbi-section' }, [
			E('h3', _('Devices')),
			E('table', { 'class': 'table' }, rows)
		]);

		return E('div', {}, [ header, table ]);
	},

	handleSaveApply: null,
	handleSave: null,
	handleReset: null
});

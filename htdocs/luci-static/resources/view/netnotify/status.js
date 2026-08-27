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
		var status = { running: running, version: '-', devices: [], netcheck: {} };

		if (raw) {
			try {
				var parsed = JSON.parse(raw);
				status.version = parsed.version || '-';
				status.devices = parsed.devices || [];
				status.netcheck = parsed.netcheck || {};
			} catch (e) {
				status.devices = [];
			}
		}

		var nc = status.netcheck || {};
		var header = E('div', { 'class': 'cbi-section' }, [
			E('h3', _('服务状态')),
			E('p', {}, [
				_('运行中') + ': ',
				E('strong', {
					'style': running ? 'color:green' : 'color:red'
				}, running ? _('是') : _('否')),
				' · ',
				_('版本') + ': ' + status.version
			])
		]);

		var netcheckBox = E('div', { 'class': 'cbi-section' }, [
			E('h3', _('网络检测')),
			E('p', {}, [
				_('启用') + ': ' + (nc.enabled ? _('是') : _('否')),
				' · ',
				_('连通') + ': ',
				E('strong', {
					'style': nc.ok ? 'color:green' : 'color:orange'
				}, nc.enabled ? (nc.ok ? _('正常') : _('异常/未知')) : '-'),
			]),
			E('p', {}, [
				_('最近动作') + ': ' + (nc.last_action || '-'),
				' · ',
				_('WAN IP') + ': ' + (nc.wan_ip || '-'),
			]),
			E('p', {}, [
				_('负载') + ': ' + (nc.loadavg || '-'),
				' · ',
				_('运行时长') + ': ' + (nc.uptime || '-'),
			]),
			E('p', {}, [
				_('详情') + ': ' + (nc.last_detail || '-')
			])
		]);

		var rows = [
			E('tr', { 'class': 'tr table-titles' }, [
				E('th', { 'class': 'th' }, _('名称')),
				E('th', { 'class': 'th' }, _('MAC')),
				E('th', { 'class': 'th' }, _('IP')),
				E('th', { 'class': 'th' }, _('接口')),
				E('th', { 'class': 'th' }, _('状态'))
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
					_('暂无设备状态。请启用服务并等待邻居 / DHCP 事件。'))
			]));
		}

		var table = E('div', { 'class': 'cbi-section' }, [
			E('h3', _('局域网设备')),
			E('table', { 'class': 'table' }, rows)
		]);

		return E('div', {}, [ header, netcheckBox, table ]);
	},

	handleSaveApply: null,
	handleSave: null,
	handleReset: null
});

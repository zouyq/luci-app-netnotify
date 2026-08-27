'use strict';
'require form';
'require uci';
'require view';
'require fs';
'require ui';

return view.extend({
	load: function () {
		return Promise.all([
			uci.load('netnotify')
		]);
	},

	render: function () {
		var m, s, o;

		m = new form.Map('netnotify', _('NetNotify'),
			_('Event-driven LAN device online/offline notifications (Go daemon).'));

		s = m.section(form.TypedSection, 'netnotify', _('General'));
		s.anonymous = true;
		s.addremove = false;

		o = s.option(form.Flag, 'enable', _('Enable'));
		o.rmempty = false;

		o = s.option(form.Value, 'device_name', _('Device name'));
		o.placeholder = 'OpenWrt';
		o.rmempty = false;

		o = s.option(form.ListValue, 'channel', _('Push channel'));
		o.value('dingtalk', _('DingTalk robot'));
		o.value('wecom_bot', _('WeCom robot webhook'));
		o.value('wecom_app', _('WeCom application'));
		o.value('bark', _('Bark'));
		o.value('webhook', _('Generic JSON webhook'));
		o.rmempty = false;

		/* ---- channel-specific ---- */
		o = s.option(form.Value, 'webhook_url', _('Webhook URL'));
		o.datatype = 'string';
		o.rmempty = true;
		o.placeholder = 'https://oapi.dingtalk.com/robot/send?access_token=...';
		o.depends('channel', 'dingtalk');
		o.depends('channel', 'wecom_bot');
		o.depends('channel', 'webhook');

		o = s.option(form.TextValue, 'webhook_template', _('Custom JSON template (optional)'));
		o.rows = 4;
		o.rmempty = true;
		o.placeholder = '{"title":"{{title}}","content":"{{content}}"}';
		o.depends('channel', 'webhook');

		o = s.option(form.Value, 'qywx_corpid', _('WeCom CorpID'));
		o.rmempty = true;
		o.depends('channel', 'wecom_app');

		o = s.option(form.Value, 'qywx_agentid', _('WeCom AgentID'));
		o.rmempty = true;
		o.depends('channel', 'wecom_app');

		o = s.option(form.Value, 'qywx_corpsecret', _('WeCom CorpSecret'));
		o.password = true;
		o.rmempty = true;
		o.depends('channel', 'wecom_app');

		o = s.option(form.Value, 'qywx_touser', _('WeCom ToUser'));
		o.placeholder = '@all';
		o.rmempty = true;
		o.depends('channel', 'wecom_app');

		o = s.option(form.Value, 'bark_token', _('Bark Device Key / Token'));
		o.rmempty = true;
		o.depends('channel', 'bark');

		o = s.option(form.Flag, 'bark_srv_enable', _('Custom Bark server'));
		o.default = '0';
		o.rmempty = true;
		o.depends('channel', 'bark');

		o = s.option(form.Value, 'bark_srv', _('Bark Server'));
		o.placeholder = 'https://api.day.app';
		o.rmempty = true;
		o.depends('bark_srv_enable', '1');

		o = s.option(form.Value, 'bark_sound', _('Bark Sound'));
		o.placeholder = 'silence.caf';
		o.rmempty = true;
		o.depends('channel', 'bark');

		o = s.option(form.Value, 'bark_icon', _('Bark Icon URL'));
		o.rmempty = true;
		o.depends('channel', 'bark');

		o = s.option(form.ListValue, 'bark_level', _('Bark Level'));
		o.value('active', 'active');
		o.value('timeSensitive', 'timeSensitive');
		o.value('passive', 'passive');
		o.rmempty = true;
		o.depends('channel', 'bark');

		o = s.option(form.Button, '_test_push', _('Test notification'));
		o.inputtitle = _('Send test');
		o.inputstyle = 'apply';
		o.onclick = function () {
			return fs.exec('/usr/bin/netnotifyd', ['test']).then(function (res) {
				var code = res.code;
				var errText = (res.stderr || res.stdout || '').toString().trim();
				if (code !== 0) {
					ui.addNotification(null, E('p', {}, [
						errText || _('Test notification failed')
					]), 'error');
				} else {
					ui.addNotification(null, E('p', {}, [
						_('Test notification sent')
					]), 'info');
				}
			}).catch(function (err) {
				ui.addNotification(null, E('p', {}, [
					(err && err.message) ? err.message : String(err)
				]), 'error');
			});
		};

		o = s.option(form.Value, 'suspect_timeout_sec', _('Suspect timeout (seconds)'));
		o.datatype = 'uinteger';
		o.placeholder = '60';
		o.rmempty = false;

		o = s.option(form.Value, 'probe_max_parallel', _('Probe parallelism (max 2)'));
		o.datatype = 'range(1,2)';
		o.placeholder = '2';
		o.rmempty = false;

		o = s.option(form.Value, 'offline_fail_count', _('Offline after N probe failures'));
		o.datatype = 'uinteger';
		o.placeholder = '3';
		o.rmempty = false;

		o = s.option(form.Value, 'sleeptime', _('DHCP leases poll interval (seconds)'));
		o.datatype = 'uinteger';
		o.placeholder = '30';
		o.rmempty = true;

		o = s.option(form.Flag, 'debug', _('Debug logging'));
		o.rmempty = false;

		o = s.option(form.Flag, 'oui_enable', _('MAC vendor lookup (OUI)'));
		o.default = '1';
		o.rmempty = false;
		o.description = _('When DHCP hostname is missing, use built-in OUI database for vendor name');

		o = s.option(form.Flag, 'notify_list_enable', _('Append online device list to up/down push'));
		o.default = '1';
		o.rmempty = false;

		o = s.option(form.Value, 'notify_list_max', _('Online list max rows'));
		o.datatype = 'range(1,50)';
		o.placeholder = '15';
		o.default = '15';
		o.rmempty = false;
		o.depends('notify_list_enable', '1');

		o = s.option(form.DynamicList, 'aliases', _('MAC aliases (mac=name)'));
		o.placeholder = 'aa:bb:cc:dd:ee:ff=Phone';
		o.rmempty = true;

		o = s.option(form.DynamicList, 'whitelist', _('MAC whitelist (empty = all)'));
		o.rmempty = true;

		o = s.option(form.DynamicList, 'blacklist', _('MAC blacklist'));
		o.rmempty = true;

		o = s.option(form.DynamicList, 'watch_ifaces', _('Watch interfaces (empty = auto br-lan)'));
		o.placeholder = 'br-lan';
		o.rmempty = true;

		/* ---- scheduled report ---- */
		s = m.section(form.TypedSection, 'netnotify', _('Scheduled report'));
		s.anonymous = true;
		s.addremove = false;

		o = s.option(form.ListValue, 'crontab', _('Schedule mode'));
		o.value('', _('Disabled'));
		o.value('1', _('Fixed hours daily'));
		o.value('2', _('Every N hours'));
		o.rmempty = true;

		o = s.option(form.ListValue, 'regular_time', _('Send hour #1'));
		o.depends('crontab', '1');
		for (var t = 0; t <= 23; t++)
			o.value(String(t), _('Daily at %d:00').format(t));
		o.default = '8';

		o = s.option(form.ListValue, 'regular_time_2', _('Send hour #2'));
		o.value('', _('Off'));
		o.depends('crontab', '1');
		for (var t2 = 0; t2 <= 23; t2++)
			o.value(String(t2), _('Daily at %d:00').format(t2));

		o = s.option(form.ListValue, 'regular_time_3', _('Send hour #3'));
		o.value('', _('Off'));
		o.depends('crontab', '1');
		for (var t3 = 0; t3 <= 23; t3++)
			o.value(String(t3), _('Daily at %d:00').format(t3));

		o = s.option(form.ListValue, 'interval_time', _('Interval (hours)'));
		o.depends('crontab', '2');
		for (var h = 1; h <= 23; h++)
			o.value(String(h), _('%d hour(s)').format(h));
		o.default = '6';

		o = s.option(form.Value, 'send_title', _('Report title'));
		o.placeholder = '路由状态';
		o.depends('crontab', '1');
		o.depends('crontab', '2');

		o = s.option(form.Flag, 'cron_status', _('Include system status'));
		o.default = '1';
		o.depends('crontab', '1');
		o.depends('crontab', '2');

		o = s.option(form.Flag, 'cron_devices', _('Include online device list'));
		o.default = '1';
		o.depends('crontab', '1');
		o.depends('crontab', '2');

		o = s.option(form.Button, '_cron_send', _('Send report now'));
		o.inputtitle = _('Send');
		o.inputstyle = 'apply';
		o.depends('crontab', '1');
		o.depends('crontab', '2');
		o.onclick = function () {
			return fs.exec('/usr/bin/netnotifyd', ['cron']).then(function (res) {
				var code = res.code;
				var errText = (res.stderr || res.stdout || '').toString().trim();
				if (code !== 0) {
					ui.addNotification(null, E('p', {}, [
						errText || _('Cron send failed')
					]), 'error');
				} else {
					ui.addNotification(null, E('p', {}, [
						_('Scheduled report sent')
					]), 'info');
				}
			}).catch(function (err) {
				ui.addNotification(null, E('p', {}, [
					(err && err.message) ? err.message : String(err)
				]), 'error');
			});
		};

		return m.render();
	}
});

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

		m = new form.Map('netnotify', _('网络通知'),
			_('局域网设备上下线事件推送（Go 守护进程，无 Lua）。网络检测请到「网络检测」页配置。'));

		s = m.section(form.TypedSection, 'netnotify', _('常规'));
		s.anonymous = true;
		s.addremove = false;

		o = s.option(form.Flag, 'enable', _('启用服务'));
		o.rmempty = false;

		o = s.option(form.Value, 'device_name', _('设备名称'));
		o.placeholder = 'OpenWrt';
		o.rmempty = false;

		o = s.option(form.ListValue, 'channel', _('推送渠道'));
		o.value('dingtalk', _('钉钉机器人'));
		o.value('wecom_bot', _('企业微信群机器人'));
		o.value('wecom_app', _('企业微信应用'));
		o.value('bark', _('Bark'));
		o.value('webhook', _('通用 JSON Webhook'));
		o.rmempty = false;

		o = s.option(form.Value, 'webhook_url', _('Webhook 地址'));
		o.datatype = 'string';
		o.rmempty = true;
		o.placeholder = 'https://oapi.dingtalk.com/robot/send?access_token=...';
		o.depends('channel', 'dingtalk');
		o.depends('channel', 'wecom_bot');
		o.depends('channel', 'webhook');

		o = s.option(form.TextValue, 'webhook_template', _('自定义 JSON 模板（可选）'));
		o.rows = 4;
		o.rmempty = true;
		o.placeholder = '{"title":"{{title}}","content":"{{content}}"}';
		o.depends('channel', 'webhook');

		o = s.option(form.Value, 'qywx_corpid', _('企业微信 CorpID'));
		o.rmempty = true;
		o.depends('channel', 'wecom_app');

		o = s.option(form.Value, 'qywx_agentid', _('企业微信 AgentID'));
		o.rmempty = true;
		o.depends('channel', 'wecom_app');

		o = s.option(form.Value, 'qywx_corpsecret', _('企业微信 CorpSecret'));
		o.password = true;
		o.rmempty = true;
		o.depends('channel', 'wecom_app');

		o = s.option(form.Value, 'qywx_touser', _('企业微信接收用户'));
		o.placeholder = '@all';
		o.rmempty = true;
		o.depends('channel', 'wecom_app');

		o = s.option(form.Value, 'bark_token', _('Bark 设备 Key'));
		o.rmempty = true;
		o.depends('channel', 'bark');

		o = s.option(form.Flag, 'bark_srv_enable', _('自定义 Bark 服务器'));
		o.default = '0';
		o.rmempty = true;
		o.depends('channel', 'bark');

		o = s.option(form.Value, 'bark_srv', _('Bark 服务器'));
		o.placeholder = 'https://api.day.app';
		o.rmempty = true;
		o.depends('bark_srv_enable', '1');

		o = s.option(form.Value, 'bark_sound', _('Bark 提示音'));
		o.placeholder = 'silence.caf';
		o.rmempty = true;
		o.depends('channel', 'bark');

		o = s.option(form.Value, 'bark_icon', _('Bark 图标 URL'));
		o.rmempty = true;
		o.depends('channel', 'bark');

		o = s.option(form.ListValue, 'bark_level', _('Bark 级别'));
		o.value('active', 'active');
		o.value('timeSensitive', 'timeSensitive');
		o.value('passive', 'passive');
		o.rmempty = true;
		o.depends('channel', 'bark');

		o = s.option(form.Button, '_test_push', _('测试推送'));
		o.inputtitle = _('发送测试');
		o.inputstyle = 'apply';
		o.onclick = function () {
			return fs.exec('/usr/bin/netnotifyd', ['test']).then(function (res) {
				var code = res.code;
				var errText = (res.stderr || res.stdout || '').toString().trim();
				if (code !== 0) {
					ui.addNotification(null, E('p', {}, [
						errText || _('测试推送失败')
					]), 'error');
				} else {
					ui.addNotification(null, E('p', {}, [
						_('测试推送已发送')
					]), 'info');
				}
			}).catch(function (err) {
				ui.addNotification(null, E('p', {}, [
					(err && err.message) ? err.message : String(err)
				]), 'error');
			});
		};

		o = s.option(form.Value, 'suspect_timeout_sec', _('疑似离线超时（秒）'));
		o.datatype = 'uinteger';
		o.placeholder = '60';
		o.rmempty = false;

		o = s.option(form.Value, 'probe_max_parallel', _('ARP 探测并发（最大 2）'));
		o.datatype = 'range(1,2)';
		o.placeholder = '2';
		o.rmempty = false;

		o = s.option(form.Value, 'offline_fail_count', _('连续失败 N 次判定离线'));
		o.datatype = 'uinteger';
		o.placeholder = '3';
		o.rmempty = false;

		o = s.option(form.Value, 'sleeptime', _('DHCP 租约轮询间隔（秒）'));
		o.datatype = 'uinteger';
		o.placeholder = '30';
		o.rmempty = true;

		o = s.option(form.Flag, 'debug', _('调试日志'));
		o.rmempty = false;

		o = s.option(form.Flag, 'oui_enable', _('MAC 厂商库（OUI）'));
		o.default = '1';
		o.rmempty = false;
		o.description = _('无 DHCP 主机名时，用内置 OUI 库显示厂商名');

		o = s.option(form.Flag, 'notify_list_enable', _('上下线消息附带在线列表'));
		o.default = '1';
		o.rmempty = false;

		o = s.option(form.Value, 'notify_list_max', _('在线列表最多显示行数'));
		o.datatype = 'range(1,50)';
		o.placeholder = '15';
		o.default = '15';
		o.rmempty = false;
		o.depends('notify_list_enable', '1');

		o = s.option(form.DynamicList, 'aliases', _('MAC 别名（mac=名称）'));
		o.placeholder = 'aa:bb:cc:dd:ee:ff=手机';
		o.rmempty = true;

		o = s.option(form.DynamicList, 'whitelist', _('MAC 白名单（空=全部）'));
		o.rmempty = true;

		o = s.option(form.DynamicList, 'blacklist', _('MAC 黑名单'));
		o.rmempty = true;

		o = s.option(form.DynamicList, 'watch_ifaces', _('监视网卡（空=自动 br-lan）'));
		o.placeholder = 'br-lan';
		o.rmempty = true;

		s = m.section(form.TypedSection, 'netnotify', _('定时汇报'));
		s.anonymous = true;
		s.addremove = false;

		o = s.option(form.ListValue, 'crontab', _('定时模式'));
		o.value('', _('关闭'));
		o.value('1', _('每天固定整点'));
		o.value('2', _('每隔 N 小时'));
		o.rmempty = true;

		o = s.option(form.ListValue, 'regular_time', _('发送时刻 #1'));
		o.depends('crontab', '1');
		for (var t = 0; t <= 23; t++)
			o.value(String(t), _('每天 %d:00').format(t));
		o.default = '8';

		o = s.option(form.ListValue, 'regular_time_2', _('发送时刻 #2'));
		o.value('', _('关闭'));
		o.depends('crontab', '1');
		for (var t2 = 0; t2 <= 23; t2++)
			o.value(String(t2), _('每天 %d:00').format(t2));

		o = s.option(form.ListValue, 'regular_time_3', _('发送时刻 #3'));
		o.value('', _('关闭'));
		o.depends('crontab', '1');
		for (var t3 = 0; t3 <= 23; t3++)
			o.value(String(t3), _('每天 %d:00').format(t3));

		o = s.option(form.ListValue, 'interval_time', _('间隔（小时）'));
		o.depends('crontab', '2');
		for (var h = 1; h <= 23; h++)
			o.value(String(h), _('%d 小时').format(h));
		o.default = '6';

		o = s.option(form.Value, 'send_title', _('汇报标题'));
		o.placeholder = '路由状态';
		o.depends('crontab', '1');
		o.depends('crontab', '2');

		o = s.option(form.Flag, 'cron_status', _('包含系统状态'));
		o.default = '1';
		o.depends('crontab', '1');
		o.depends('crontab', '2');

		o = s.option(form.Flag, 'cron_devices', _('包含在线设备列表'));
		o.default = '1';
		o.depends('crontab', '1');
		o.depends('crontab', '2');

		o = s.option(form.Button, '_cron_send', _('立即发送汇报'));
		o.inputtitle = _('发送');
		o.inputstyle = 'apply';
		o.depends('crontab', '1');
		o.depends('crontab', '2');
		o.onclick = function () {
			return fs.exec('/usr/bin/netnotifyd', ['cron']).then(function (res) {
				var code = res.code;
				var errText = (res.stderr || res.stdout || '').toString().trim();
				if (code !== 0) {
					ui.addNotification(null, E('p', {}, [
						errText || _('定时汇报发送失败')
					]), 'error');
				} else {
					ui.addNotification(null, E('p', {}, [
						_('定时汇报已发送')
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
